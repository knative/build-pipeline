/*
Copyright 2019 The Tekton Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package taskrun

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/ghodss/yaml"
	"github.com/hashicorp/go-multierror"
	"github.com/tektoncd/pipeline/pkg/apis/config"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/pod"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"github.com/tektoncd/pipeline/pkg/apis/resource"
	resourcev1alpha1 "github.com/tektoncd/pipeline/pkg/apis/resource/v1alpha1"
	clientset "github.com/tektoncd/pipeline/pkg/client/clientset/versioned"
	taskrunreconciler "github.com/tektoncd/pipeline/pkg/client/injection/reconciler/pipeline/v1beta1/taskrun"
	listers "github.com/tektoncd/pipeline/pkg/client/listers/pipeline/v1beta1"
	resourcelisters "github.com/tektoncd/pipeline/pkg/client/resource/listers/resource/v1alpha1"
	"github.com/tektoncd/pipeline/pkg/contexts"
	podconvert "github.com/tektoncd/pipeline/pkg/pod"
	tknreconciler "github.com/tektoncd/pipeline/pkg/reconciler"
	"github.com/tektoncd/pipeline/pkg/reconciler/events"
	"github.com/tektoncd/pipeline/pkg/reconciler/events/cloudevent"
	"github.com/tektoncd/pipeline/pkg/reconciler/taskrun/resources"
	"github.com/tektoncd/pipeline/pkg/reconciler/volumeclaim"
	"github.com/tektoncd/pipeline/pkg/workspace"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"knative.dev/pkg/apis"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/kmeta"
	"knative.dev/pkg/logging"
	pkgreconciler "knative.dev/pkg/reconciler"
	"knative.dev/pkg/tracker"
)

// Reconciler implements controller.Reconciler for Configuration resources.
type Reconciler struct {
	KubeClientSet     kubernetes.Interface
	PipelineClientSet clientset.Interface
	Images            pipeline.Images

	// listers index properties about resources
	taskRunLister     listers.TaskRunLister
	taskLister        listers.TaskLister
	clusterTaskLister listers.ClusterTaskLister
	resourceLister    resourcelisters.PipelineResourceLister
	cloudEventClient  cloudevent.CEClient
	tracker           tracker.Interface
	entrypointCache   podconvert.EntrypointCache
	metrics           *Recorder
	pvcHandler        volumeclaim.PvcHandler

	snooze func(kmeta.Accessor, time.Duration)
}

// Check that our Reconciler implements taskrunreconciler.Interface
var _ taskrunreconciler.Interface = (*Reconciler)(nil)

// ReconcileKind compares the actual state with the desired, and attempts to
// converge the two. It then updates the Status block of the Task Run
// resource with the current status of the resource.
func (c *Reconciler) ReconcileKind(ctx context.Context, tr *v1beta1.TaskRun) pkgreconciler.Event {
	logger := logging.FromContext(ctx)
	ctx = cloudevent.ToContext(ctx, c.cloudEventClient)

	// Read the initial condition
	before := tr.Status.GetCondition(apis.ConditionSucceeded)

	// If the TaskRun is just starting, this will also set the starttime,
	// from which the timeout will immediately begin counting down.
	if !tr.HasStarted() {
		tr.Status.InitializeConditions()
		// In case node time was not synchronized, when controller has been scheduled to other nodes.
		if tr.Status.StartTime.Sub(tr.CreationTimestamp.Time) < 0 {
			logger.Warnf("TaskRun %s createTimestamp %s is after the taskRun started %s", tr.GetNamespacedName().String(), tr.CreationTimestamp, tr.Status.StartTime)
			tr.Status.StartTime = &tr.CreationTimestamp
		}
		// Emit events. During the first reconcile the status of the TaskRun may change twice
		// from not Started to Started and then to Running, so we need to sent the event here
		// and at the end of 'Reconcile' again.
		// We also want to send the "Started" event as soon as possible for anyone who may be waiting
		// on the event to perform user facing initialisations, such has reset a CI check status
		afterCondition := tr.Status.GetCondition(apis.ConditionSucceeded)
		events.Emit(ctx, nil, afterCondition, tr)
	}

	// If the TaskRun is complete, run some post run fixtures when applicable
	if tr.IsDone() {
		logger.Infof("taskrun done : %s \n", tr.Name)

		// We may be reading a version of the object that was stored at an older version
		// and may not have had all of the assumed default specified.
		tr.SetDefaults(contexts.WithUpgradeViaDefaulting(ctx))

		// Try to send cloud events first
		cloudEventErr := cloudevent.SendCloudEvents(tr, c.cloudEventClient, logger)
		// Regardless of `err`, we must write back any status update that may have
		// been generated by `sendCloudEvents`
		if cloudEventErr != nil {
			// Let's keep timeouts and sidecars running as long as we're trying to
			// send cloud events. So we stop here an return errors encountered this far.
			return cloudEventErr
		}

		pod, err := c.stopSidecars(ctx, tr)
		if err != nil {
			return err
		}

		go func(metrics *Recorder) {
			err := metrics.DurationAndCount(tr)
			if err != nil {
				logger.Warnf("Failed to log the metrics : %v", err)
			}
			if pod != nil {
				err = metrics.RecordPodLatency(pod, tr)
				if err != nil {
					logger.Warnf("Failed to log the metrics : %v", err)
				}
			}
			err = metrics.CloudEvents(tr)
			if err != nil {
				logger.Warnf("Failed to log the metrics : %v", err)
			}
		}(c.metrics)
		return c.finishReconcileUpdateEmitEvents(ctx, tr, before, nil)
	}

	// If the TaskRun is cancelled, kill resources and update status
	if tr.IsCancelled() {
		message := fmt.Sprintf("TaskRun %q was cancelled", tr.Name)
		err := c.failTaskRun(ctx, tr, v1beta1.TaskRunReasonCancelled, message)
		return c.finishReconcileUpdateEmitEvents(ctx, tr, before, err)
	}

	// Check if the TaskRun has timed out; if it is, this will set its status
	// accordingly.
	if tr.HasTimedOut(ctx) {
		message := fmt.Sprintf("TaskRun %q failed to finish within %q", tr.Name, tr.GetTimeout(ctx))
		err := c.failTaskRun(ctx, tr, v1beta1.TaskRunReasonTimedOut, message)
		return c.finishReconcileUpdateEmitEvents(ctx, tr, before, err)
	}
	defer func() {
		if tr.Status.StartTime == nil {
			return
		}
		// Compute the time since the task started.
		elapsed := time.Since(tr.Status.StartTime.Time)
		// Snooze this resource until the timeout has elapsed.
		c.snooze(tr, tr.GetTimeout(ctx)-elapsed)
	}()

	// prepare fetches all required resources, validates them together with the
	// taskrun, runs API convertions. Errors that come out of prepare are
	// permanent one, so in case of error we update, emit events and return
	_, rtr, err := c.prepare(ctx, tr)
	if err != nil {
		logger.Errorf("TaskRun prepare error: %v", err.Error())
		// We only return an error if update failed, otherwise we don't want to
		// reconcile an invalid TaskRun anymore
		return c.finishReconcileUpdateEmitEvents(ctx, tr, nil, err)
	}

	// Store the condition before reconcile
	before = tr.Status.GetCondition(apis.ConditionSucceeded)

	// Reconcile this copy of the task run and then write back any status
	// updates regardless of whether the reconciliation errored out.
	if err = c.reconcile(ctx, tr, rtr); err != nil {
		logger.Errorf("Reconcile: %v", err.Error())
	}

	// Emit events (only when ConditionSucceeded was changed)
	return c.finishReconcileUpdateEmitEvents(ctx, tr, before, err)
}
func (c *Reconciler) stopSidecars(ctx context.Context, tr *v1beta1.TaskRun) (*corev1.Pod, error) {
	logger := logging.FromContext(ctx)
	// do not continue without knowing the associated pod
	if tr.Status.PodName == "" {
		return nil, nil
	}

	// do not continue if the TaskRun was canceled or timed out as this caused the pod to be deleted in failTaskRun
	condition := tr.Status.GetCondition(apis.ConditionSucceeded)
	if condition != nil {
		reason := v1beta1.TaskRunReason(condition.Reason)
		if reason == v1beta1.TaskRunReasonCancelled || reason == v1beta1.TaskRunReasonTimedOut {
			return nil, nil
		}
	}

	pod, err := podconvert.StopSidecars(ctx, c.Images.NopImage, c.KubeClientSet, tr.Namespace, tr.Status.PodName)
	if err == nil {
		// Check if any SidecarStatuses are still shown as Running after stopping
		// Sidecars. If any Running, update SidecarStatuses based on Pod ContainerStatuses.
		if podconvert.IsSidecarStatusRunning(tr) {
			err = updateStoppedSidecarStatus(ctx, pod, tr, c)
		}
	}

	if k8serrors.IsNotFound(err) {
		// At this stage the TaskRun has been completed if the pod is not found, it won't come back,
		// it has probably evicted. We can return the error, but we consider it a permanent one.
		return nil, controller.NewPermanentError(err)
	} else if err != nil {
		logger.Errorf("Error stopping sidecars for TaskRun %q: %v", tr.Name, err)
		tr.Status.MarkResourceFailed(podconvert.ReasonFailedResolution, err)
	}
	return pod, nil
}

func (c *Reconciler) finishReconcileUpdateEmitEvents(ctx context.Context, tr *v1beta1.TaskRun, beforeCondition *apis.Condition, previousError error) error {
	logger := logging.FromContext(ctx)

	afterCondition := tr.Status.GetCondition(apis.ConditionSucceeded)

	// Send k8s events and cloud events (when configured)
	events.Emit(ctx, beforeCondition, afterCondition, tr)

	_, err := c.updateLabelsAndAnnotations(ctx, tr)
	if err != nil {
		logger.Warn("Failed to update PipelineRun labels/annotations", zap.Error(err))
		events.EmitError(controller.GetEventRecorder(ctx), err, tr)
	}

	merr := multierror.Append(previousError, err).ErrorOrNil()
	if controller.IsPermanentError(previousError) {
		return controller.NewPermanentError(merr)
	}
	return merr
}

// `prepare` fetches resources the taskrun depends on, runs validation and conversion
// It may report errors back to Reconcile, it updates the taskrun status in case of
// error but it does not sync updates back to etcd. It does not emit events.
// All errors returned by `prepare` are always handled by `Reconcile`, so they don't cause
// the key to be re-queued directly. Once we start using `PermanentErrors` code in
// `prepare` will be able to control which error is handled by `Reconcile` and which is not
// See https://github.com/tektoncd/pipeline/issues/2474 for details.
// `prepare` returns spec and resources. In future we might store
// them in the TaskRun.Status so we don't need to re-run `prepare` at every
// reconcile (see https://github.com/tektoncd/pipeline/issues/2473).
func (c *Reconciler) prepare(ctx context.Context, tr *v1beta1.TaskRun) (*v1beta1.TaskSpec, *resources.ResolvedTaskResources, error) {
	logger := logging.FromContext(ctx)
	// We may be reading a version of the object that was stored at an older version
	// and may not have had all of the assumed default specified.
	tr.SetDefaults(contexts.WithUpgradeViaDefaulting(ctx))

	getTaskfunc, err := resources.GetTaskFuncFromTaskRun(ctx, c.KubeClientSet, c.PipelineClientSet, tr)
	if err != nil {
		logger.Errorf("Failed to fetch task reference %s: %v", tr.Spec.TaskRef.Name, err)
		tr.Status.SetCondition(&apis.Condition{
			Type:    apis.ConditionSucceeded,
			Status:  corev1.ConditionFalse,
			Reason:  podconvert.ReasonFailedResolution,
			Message: err.Error(),
		})
		return nil, nil, err
	}

	taskMeta, taskSpec, err := resources.GetTaskData(ctx, tr, getTaskfunc)
	if err != nil {
		logger.Errorf("Failed to determine Task spec to use for taskrun %s: %v", tr.Name, err)
		tr.Status.MarkResourceFailed(podconvert.ReasonFailedResolution, err)
		return nil, nil, controller.NewPermanentError(err)
	}

	// Store the fetched TaskSpec on the TaskRun for auditing
	if err := storeTaskSpec(ctx, tr, taskSpec); err != nil {
		logger.Errorf("Failed to store TaskSpec on TaskRun.Statusfor taskrun %s: %v", tr.Name, err)
	}

	// Propagate labels from Task to TaskRun.
	if tr.ObjectMeta.Labels == nil {
		tr.ObjectMeta.Labels = make(map[string]string, len(taskMeta.Labels)+1)
	}
	for key, value := range taskMeta.Labels {
		tr.ObjectMeta.Labels[key] = value
	}
	if tr.Spec.TaskRef != nil {
		if tr.Spec.TaskRef.Kind == "ClusterTask" {
			tr.ObjectMeta.Labels[pipeline.GroupName+pipeline.ClusterTaskLabelKey] = taskMeta.Name
		} else {
			tr.ObjectMeta.Labels[pipeline.GroupName+pipeline.TaskLabelKey] = taskMeta.Name
		}
	}

	// Propagate annotations from Task to TaskRun.
	if tr.ObjectMeta.Annotations == nil {
		tr.ObjectMeta.Annotations = make(map[string]string, len(taskMeta.Annotations))
	}
	for key, value := range taskMeta.Annotations {
		tr.ObjectMeta.Annotations[key] = value
	}

	inputs := []v1beta1.TaskResourceBinding{}
	outputs := []v1beta1.TaskResourceBinding{}
	if tr.Spec.Resources != nil {
		inputs = tr.Spec.Resources.Inputs
		outputs = tr.Spec.Resources.Outputs
	}
	rtr, err := resources.ResolveTaskResources(taskSpec, taskMeta.Name, resources.GetTaskKind(tr), inputs, outputs, c.resourceLister.PipelineResources(tr.Namespace).Get)
	if err != nil {
		if k8serrors.IsNotFound(err) && tknreconciler.IsYoungResource(tr) {
			// For newly created resources, don't fail immediately.
			// Instead return an (non-permanent) error, which will prompt the
			// controller to requeue the key with backoff.
			logger.Warnf("References for taskrun %s not found: %v", tr.Name, err)
			tr.Status.MarkResourceOngoing(podconvert.ReasonFailedResolution,
				fmt.Sprintf("Unable to resolve dependencies for %q: %v", tr.Name, err))
			return nil, nil, err
		}
		logger.Errorf("Failed to resolve references for taskrun %s: %v", tr.Name, err)
		tr.Status.MarkResourceFailed(podconvert.ReasonFailedResolution, err)
		return nil, nil, controller.NewPermanentError(err)
	}

	if err := ValidateResolvedTaskResources(tr.Spec.Params, rtr); err != nil {
		logger.Errorf("TaskRun %q resources are invalid: %v", tr.Name, err)
		tr.Status.MarkResourceFailed(podconvert.ReasonFailedValidation, err)
		return nil, nil, controller.NewPermanentError(err)
	}

	if err := c.updateTaskRunWithDefaultWorkspaces(ctx, tr, taskSpec); err != nil {
		logger.Errorf("Failed to update taskrun %s with default workspace: %v", tr.Name, err)
		tr.Status.MarkResourceFailed(podconvert.ReasonFailedResolution, err)
		return nil, nil, controller.NewPermanentError(err)
	}

	if err := workspace.ValidateBindings(taskSpec.Workspaces, tr.Spec.Workspaces); err != nil {
		logger.Errorf("TaskRun %q workspaces are invalid: %v", tr.Name, err)
		tr.Status.MarkResourceFailed(podconvert.ReasonFailedValidation, err)
		return nil, nil, controller.NewPermanentError(err)
	}

	if _, usesAssistant := tr.Annotations[workspace.AnnotationAffinityAssistantName]; usesAssistant {
		if err := workspace.ValidateOnlyOnePVCIsUsed(tr.Spec.Workspaces); err != nil {
			logger.Errorf("TaskRun %q workspaces incompatible with Affinity Assistant: %v", tr.Name, err)
			tr.Status.MarkResourceFailed(podconvert.ReasonFailedValidation, err)
			return nil, nil, controller.NewPermanentError(err)
		}
	}

	// Initialize the cloud events if at least a CloudEventResource is defined
	// and they have not been initialized yet.
	// FIXME(afrittoli) This resource specific logic will have to be replaced
	// once we have a custom PipelineResource framework in place.
	logger.Infof("Cloud Events: %s", tr.Status.CloudEvents)
	cloudevent.InitializeCloudEvents(tr, rtr.Outputs)

	return taskSpec, rtr, nil
}

// `reconcile` creates the Pod associated to the TaskRun, and it pulls back status
// updates from the Pod to the TaskRun.
// It reports errors back to Reconcile, it updates the taskrun status in case of
// error but it does not sync updates back to etcd. It does not emit events.
// `reconcile` consumes spec and resources returned by `prepare`
func (c *Reconciler) reconcile(ctx context.Context, tr *v1beta1.TaskRun, rtr *resources.ResolvedTaskResources) error {
	logger := logging.FromContext(ctx)
	recorder := controller.GetEventRecorder(ctx)
	// Get the TaskRun's Pod if it should have one. Otherwise, create the Pod.
	var pod *corev1.Pod
	var err error

	if tr.Status.PodName != "" {
		pod, err = c.KubeClientSet.CoreV1().Pods(tr.Namespace).Get(ctx, tr.Status.PodName, metav1.GetOptions{})
		if k8serrors.IsNotFound(err) {
			// Keep going, this will result in the Pod being created below.
		} else if err != nil {
			// This is considered a transient error, so we return error, do not update
			// the task run condition, and return an error which will cause this key to
			// be requeued for reconcile.
			logger.Errorf("Error getting pod %q: %v", tr.Status.PodName, err)
			return err
		}
	} else {
		// List pods that have a label with this TaskRun name.  Do not include other labels from the
		// TaskRun in this selector.  The user could change them during the lifetime of the TaskRun so the
		// current labels may not be set on a previously created Pod.
		labelSelector := fmt.Sprintf("%s=%s", podconvert.TaskRunLabelKey, tr.Name)
		pos, err := c.KubeClientSet.CoreV1().Pods(tr.Namespace).List(ctx, metav1.ListOptions{
			LabelSelector: labelSelector,
		})
		if err != nil {
			logger.Errorf("Error listing pods: %v", err)
			return err
		}
		for index := range pos.Items {
			po := pos.Items[index]
			if metav1.IsControlledBy(&po, tr) && !podconvert.DidTaskRunFail(&po) {
				pod = &po
			}
		}
	}

	if pod == nil {
		if tr.HasVolumeClaimTemplate() {
			if err := c.pvcHandler.CreatePersistentVolumeClaimsForWorkspaces(ctx, tr.Spec.Workspaces, tr.GetOwnerReference(), tr.Namespace); err != nil {
				logger.Errorf("Failed to create PVC for TaskRun %s: %v", tr.Name, err)
				tr.Status.MarkResourceFailed(volumeclaim.ReasonCouldntCreateWorkspacePVC,
					fmt.Errorf("Failed to create PVC for TaskRun %s workspaces correctly: %s",
						fmt.Sprintf("%s/%s", tr.Namespace, tr.Name), err))
				return controller.NewPermanentError(err)
			}

			taskRunWorkspaces := applyVolumeClaimTemplates(tr.Spec.Workspaces, tr.GetOwnerReference())
			// This is used by createPod below. Changes to the Spec are not updated.
			tr.Spec.Workspaces = taskRunWorkspaces
		}

		pod, err = c.createPod(ctx, tr, rtr)
		if err != nil {
			newErr := c.handlePodCreationError(ctx, tr, err)
			logger.Errorf("Failed to create task run pod for taskrun %q: %v", tr.Name, newErr)
			return newErr
		}
	}

	if err := c.tracker.TrackReference(tracker.Reference{
		APIVersion: "v1",
		Kind:       "Pod",
		Namespace:  tr.Namespace,
		Name:       tr.Name,
	}, tr); err != nil {
		logger.Errorf("Failed to create tracker for build pod %q for taskrun %q: %v", tr.Name, tr.Name, err)
		return err
	}

	if podconvert.IsPodExceedingNodeResources(pod) {
		recorder.Eventf(tr, corev1.EventTypeWarning, podconvert.ReasonExceededNodeResources, "Insufficient resources to schedule pod %q", pod.Name)
	}

	if podconvert.SidecarsReady(pod.Status) {
		if err := podconvert.UpdateReady(ctx, c.KubeClientSet, *pod); err != nil {
			return err
		}
	}

	// Convert the Pod's status to the equivalent TaskRun Status.
	tr.Status, err = podconvert.MakeTaskRunStatus(logger, *tr, pod)
	if err != nil {
		return err
	}

	logger.Infof("Successfully reconciled taskrun %s/%s with status: %#v", tr.Name, tr.Namespace, tr.Status.GetCondition(apis.ConditionSucceeded))
	return nil
}

func (c *Reconciler) updateTaskRunWithDefaultWorkspaces(ctx context.Context, tr *v1beta1.TaskRun, taskSpec *v1beta1.TaskSpec) error {
	configMap := config.FromContextOrDefaults(ctx)
	defaults := configMap.Defaults
	if defaults.DefaultTaskRunWorkspaceBinding != "" {
		var defaultWS v1beta1.WorkspaceBinding
		if err := yaml.Unmarshal([]byte(defaults.DefaultTaskRunWorkspaceBinding), &defaultWS); err != nil {
			return fmt.Errorf("failed to unmarshal %v", defaults.DefaultTaskRunWorkspaceBinding)
		}
		workspaceBindings := map[string]v1beta1.WorkspaceBinding{}
		for _, tsWorkspace := range taskSpec.Workspaces {
			if !tsWorkspace.Optional {
				workspaceBindings[tsWorkspace.Name] = v1beta1.WorkspaceBinding{
					Name:                  tsWorkspace.Name,
					SubPath:               defaultWS.SubPath,
					VolumeClaimTemplate:   defaultWS.VolumeClaimTemplate,
					PersistentVolumeClaim: defaultWS.PersistentVolumeClaim,
					EmptyDir:              defaultWS.EmptyDir,
					ConfigMap:             defaultWS.ConfigMap,
					Secret:                defaultWS.Secret,
				}
			}
		}

		for _, trWorkspace := range tr.Spec.Workspaces {
			workspaceBindings[trWorkspace.Name] = trWorkspace
		}

		tr.Spec.Workspaces = []v1beta1.WorkspaceBinding{}
		for _, wsBinding := range workspaceBindings {
			tr.Spec.Workspaces = append(tr.Spec.Workspaces, wsBinding)
		}
	}
	return nil
}

func (c *Reconciler) updateLabelsAndAnnotations(ctx context.Context, tr *v1beta1.TaskRun) (*v1beta1.TaskRun, error) {
	newTr, err := c.taskRunLister.TaskRuns(tr.Namespace).Get(tr.Name)
	if err != nil {
		return nil, fmt.Errorf("error getting TaskRun %s when updating labels/annotations: %w", tr.Name, err)
	}
	if !reflect.DeepEqual(tr.ObjectMeta.Labels, newTr.ObjectMeta.Labels) || !reflect.DeepEqual(tr.ObjectMeta.Annotations, newTr.ObjectMeta.Annotations) {
		// Note that this uses Update vs. Patch because the former is significantly easier to test.
		// If we want to switch this to Patch, then we will need to teach the utilities in test/controller.go
		// to deal with Patch (setting resourceVersion, and optimistic concurrency checks).
		newTr = newTr.DeepCopy()
		newTr.Labels = tr.Labels
		newTr.Annotations = tr.Annotations
		return c.PipelineClientSet.TektonV1beta1().TaskRuns(tr.Namespace).Update(ctx, newTr, metav1.UpdateOptions{})
	}
	return newTr, nil
}

func (c *Reconciler) handlePodCreationError(ctx context.Context, tr *v1beta1.TaskRun, err error) error {
	switch {
	case isExceededResourceQuotaError(err):
		// If we are struggling to create the pod, then it hasn't started.
		tr.Status.StartTime = nil
		tr.Status.SetCondition(&apis.Condition{
			Type:    apis.ConditionSucceeded,
			Status:  corev1.ConditionUnknown,
			Reason:  podconvert.ReasonExceededResourceQuota,
			Message: fmt.Sprint("TaskRun Pod exceeded available resources: ", err),
		})
	case isTaskRunValidationFailed(err):
		tr.Status.MarkResourceFailed(podconvert.ReasonFailedValidation, err)
	default:
		// The pod creation failed with unknown reason. The most likely
		// reason is that something is wrong with the spec of the Task, that we could
		// not check with validation before - i.e. pod template fields
		msg := fmt.Sprintf("failed to create task run pod %q: %v. Maybe ", tr.Name, err)
		if tr.Spec.TaskRef != nil {
			msg += fmt.Sprintf("missing or invalid Task %s/%s", tr.Namespace, tr.Spec.TaskRef.Name)
		} else {
			msg += "invalid TaskSpec"
		}
		err = controller.NewPermanentError(errors.New(msg))
		tr.Status.MarkResourceFailed(podconvert.ReasonCouldntGetTask, err)
	}
	return err
}

// failTaskRun stops a TaskRun with the provided Reason
// If a pod is associated to the TaskRun, it stops it
// failTaskRun function may return an error in case the pod could not be deleted
// failTaskRun may update the local TaskRun status, but it won't push the updates to etcd
func (c *Reconciler) failTaskRun(ctx context.Context, tr *v1beta1.TaskRun, reason v1beta1.TaskRunReason, message string) error {
	logger := logging.FromContext(ctx)

	logger.Warnf("stopping task run %q because of %q", tr.Name, reason)
	tr.Status.MarkResourceFailed(reason, errors.New(message))

	completionTime := metav1.Time{Time: time.Now()}
	// update tr completed time
	tr.Status.CompletionTime = &completionTime

	if tr.Status.PodName == "" {
		logger.Warnf("task run %q has no pod running yet", tr.Name)
		return nil
	}

	// tr.Status.PodName will be empty if the pod was never successfully created. This condition
	// can be reached, for example, by the pod never being schedulable due to limits imposed by
	// a namespace's ResourceQuota.
	err := c.KubeClientSet.CoreV1().Pods(tr.Namespace).Delete(ctx, tr.Status.PodName, metav1.DeleteOptions{})
	if err != nil && !k8serrors.IsNotFound(err) {
		logger.Infof("Failed to terminate pod: %v", err)
		return err
	}

	// Update step states for TaskRun on TaskRun object since pod has been deleted for cancel or timeout
	for i, step := range tr.Status.Steps {
		// If running, include StartedAt for when step began running
		if step.Running != nil {
			step.Terminated = &corev1.ContainerStateTerminated{
				ExitCode:   1,
				StartedAt:  step.Running.StartedAt,
				FinishedAt: completionTime,
				Reason:     reason.String(),
			}
			step.Running = nil
			tr.Status.Steps[i] = step
		}

		if step.Waiting != nil {
			step.Terminated = &corev1.ContainerStateTerminated{
				ExitCode:   1,
				FinishedAt: completionTime,
				Reason:     reason.String(),
			}
			step.Waiting = nil
			tr.Status.Steps[i] = step
		}
	}

	return nil
}

// createPod creates a Pod based on the Task's configuration, with pvcName as a volumeMount
// TODO(dibyom): Refactor resource setup/substitution logic to its own function in the resources package
func (c *Reconciler) createPod(ctx context.Context, tr *v1beta1.TaskRun, rtr *resources.ResolvedTaskResources) (*corev1.Pod, error) {
	logger := logging.FromContext(ctx)
	ts := rtr.TaskSpec.DeepCopy()
	inputResources, err := resourceImplBinding(rtr.Inputs, c.Images)
	if err != nil {
		logger.Errorf("Failed to initialize input resources: %v", err)
		return nil, err
	}
	outputResources, err := resourceImplBinding(rtr.Outputs, c.Images)
	if err != nil {
		logger.Errorf("Failed to initialize output resources: %v", err)
		return nil, err
	}

	// Get actual resource
	err = resources.AddOutputImageDigestExporter(c.Images.ImageDigestExporterImage, tr, ts, c.resourceLister.PipelineResources(tr.Namespace).Get)
	if err != nil {
		logger.Errorf("Failed to create a pod for taskrun: %s due to output image resource error %v", tr.Name, err)
		return nil, err
	}

	ts, err = resources.AddInputResource(ctx, c.KubeClientSet, c.Images, rtr.TaskName, ts, tr, inputResources)
	if err != nil {
		logger.Errorf("Failed to create a pod for taskrun: %s due to input resource error %v", tr.Name, err)
		return nil, err
	}

	ts, err = resources.AddOutputResources(ctx, c.KubeClientSet, c.Images, rtr.TaskName, ts, tr, outputResources)
	if err != nil {
		logger.Errorf("Failed to create a pod for taskrun: %s due to output resource error %v", tr.Name, err)
		return nil, err
	}

	var defaults []v1beta1.ParamSpec
	if len(ts.Params) > 0 {
		defaults = append(defaults, ts.Params...)
	}
	// Apply parameter substitution from the taskrun.
	ts = resources.ApplyParameters(ts, tr, defaults...)

	// Apply context substitution from the taskrun
	ts = resources.ApplyContexts(ts, rtr, tr)

	// Apply bound resource substitution from the taskrun.
	ts = resources.ApplyResources(ts, inputResources, "inputs")
	ts = resources.ApplyResources(ts, outputResources, "outputs")

	// Get the randomized volume names assigned to workspace bindings
	workspaceVolumes := workspace.CreateVolumes(tr.Spec.Workspaces)

	// Apply workspace resource substitution
	ts = resources.ApplyWorkspaces(ctx, ts, ts.Workspaces, tr.Spec.Workspaces, workspaceVolumes)

	// Apply task result substitution
	ts = resources.ApplyTaskResults(ts)

	if validateErr := ts.Validate(ctx); validateErr != nil {
		logger.Errorf("Failed to create a pod for taskrun: %s due to task validation error %v", tr.Name, validateErr)
		return nil, validateErr
	}

	ts, err = workspace.Apply(ctx, *ts, tr.Spec.Workspaces, workspaceVolumes)
	if err != nil {
		logger.Errorf("Failed to create a pod for taskrun: %s due to workspace error %v", tr.Name, err)
		return nil, err
	}

	// Check if the HOME env var of every Step should be set to /tekton/home.
	shouldOverrideHomeEnv := podconvert.ShouldOverrideHomeEnv(ctx)

	// Apply path substitutions for the legacy credentials helper (aka "creds-init")
	ts = resources.ApplyCredentialsPath(ts, pipeline.CredsDir)

	podbuilder := podconvert.Builder{
		Images:          c.Images,
		KubeClient:      c.KubeClientSet,
		EntrypointCache: c.entrypointCache,
		OverrideHomeEnv: shouldOverrideHomeEnv,
	}
	pod, err := podbuilder.Build(ctx, tr, *ts)
	if err != nil {
		return nil, fmt.Errorf("translating TaskSpec to Pod: %w", err)
	}

	pod, err = c.KubeClientSet.CoreV1().Pods(tr.Namespace).Create(ctx, pod, metav1.CreateOptions{})
	if err == nil && willOverwritePodSetAffinity(tr) {
		if recorder := controller.GetEventRecorder(ctx); recorder != nil {
			recorder.Eventf(tr, corev1.EventTypeWarning, "PodAffinityOverwrite", "Pod template affinity is overwritten by affinity assistant for pod %q", pod.Name)
		}
	}
	return pod, err
}

type DeletePod func(podName string, options *metav1.DeleteOptions) error

func isExceededResourceQuotaError(err error) bool {
	return err != nil && k8serrors.IsForbidden(err) && strings.Contains(err.Error(), "exceeded quota")
}

func isTaskRunValidationFailed(err error) bool {
	return err != nil && strings.Contains(err.Error(), "TaskRun validation failed")
}

// resourceImplBinding maps pipeline resource names to the actual resource type implementations
func resourceImplBinding(resources map[string]*resourcev1alpha1.PipelineResource, images pipeline.Images) (map[string]v1beta1.PipelineResourceInterface, error) {
	p := make(map[string]v1beta1.PipelineResourceInterface)
	for rName, r := range resources {
		i, err := resource.FromType(rName, r, images)
		if err != nil {
			return nil, fmt.Errorf("failed to create resource %s : %v with error: %w", rName, r, err)
		}
		p[rName] = i
	}
	return p, nil
}

// updateStoppedSidecarStatus updates SidecarStatus for sidecars that were
// terminated by nop image
func updateStoppedSidecarStatus(ctx context.Context, pod *corev1.Pod, tr *v1beta1.TaskRun, c *Reconciler) error {
	tr.Status.Sidecars = []v1beta1.SidecarState{}
	for _, s := range pod.Status.ContainerStatuses {
		if !podconvert.IsContainerStep(s.Name) {
			var sidecarState corev1.ContainerState
			if s.LastTerminationState.Terminated != nil {
				// Sidecar has successfully by terminated by nop image
				lastTerminatedState := s.LastTerminationState.Terminated
				sidecarState = corev1.ContainerState{
					Terminated: &corev1.ContainerStateTerminated{
						ExitCode:    lastTerminatedState.ExitCode,
						Reason:      "Completed",
						Message:     "Sidecar container successfully stopped by nop image",
						StartedAt:   lastTerminatedState.StartedAt,
						FinishedAt:  lastTerminatedState.FinishedAt,
						ContainerID: lastTerminatedState.ContainerID,
					},
				}
			} else {
				// Sidecar has not been terminated
				sidecarState = s.State
			}

			tr.Status.Sidecars = append(tr.Status.Sidecars, v1beta1.SidecarState{
				ContainerState: *sidecarState.DeepCopy(),
				Name:           podconvert.TrimSidecarPrefix(s.Name),
				ContainerName:  s.Name,
				ImageID:        s.ImageID,
			})
		}
	}
	return nil
}

// applyVolumeClaimTemplates and return WorkspaceBindings were templates is translated to PersistentVolumeClaims
func applyVolumeClaimTemplates(workspaceBindings []v1beta1.WorkspaceBinding, owner metav1.OwnerReference) []v1beta1.WorkspaceBinding {
	taskRunWorkspaceBindings := make([]v1beta1.WorkspaceBinding, 0, len(workspaceBindings))
	for _, wb := range workspaceBindings {
		if wb.VolumeClaimTemplate == nil {
			taskRunWorkspaceBindings = append(taskRunWorkspaceBindings, wb)
			continue
		}

		// apply template
		b := v1beta1.WorkspaceBinding{
			Name:    wb.Name,
			SubPath: wb.SubPath,
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: volumeclaim.GetPersistentVolumeClaimName(wb.VolumeClaimTemplate, wb, owner),
			},
		}
		taskRunWorkspaceBindings = append(taskRunWorkspaceBindings, b)
	}
	return taskRunWorkspaceBindings
}

func storeTaskSpec(ctx context.Context, tr *v1beta1.TaskRun, ts *v1beta1.TaskSpec) error {
	// Only store the TaskSpec once, if it has never been set before.
	if tr.Status.TaskSpec == nil {
		tr.Status.TaskSpec = ts
	}
	return nil
}

// willOverwritePodSetAffinity returns a bool indicating whether the
// affinity for pods will be overwritten with affinity assistant.
func willOverwritePodSetAffinity(taskRun *v1beta1.TaskRun) bool {
	var podTemplate pod.Template
	if taskRun.Spec.PodTemplate != nil {
		podTemplate = *taskRun.Spec.PodTemplate
	}
	return taskRun.Annotations[workspace.AnnotationAffinityAssistantName] != "" && podTemplate.Affinity != nil
}
