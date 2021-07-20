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

package pipelinerun

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strconv"
	"time"

	"github.com/hashicorp/go-multierror"
	"github.com/tektoncd/pipeline/pkg/apis/config"
	apisconfig "github.com/tektoncd/pipeline/pkg/apis/config"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1alpha1"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"github.com/tektoncd/pipeline/pkg/artifacts"
	clientset "github.com/tektoncd/pipeline/pkg/client/clientset/versioned"
	pipelinerunreconciler "github.com/tektoncd/pipeline/pkg/client/injection/reconciler/pipeline/v1beta1/pipelinerun"
	listersv1alpha1 "github.com/tektoncd/pipeline/pkg/client/listers/pipeline/v1alpha1"
	listers "github.com/tektoncd/pipeline/pkg/client/listers/pipeline/v1beta1"
	resourcelisters "github.com/tektoncd/pipeline/pkg/client/resource/listers/resource/v1alpha1"
	"github.com/tektoncd/pipeline/pkg/contexts"
	tknreconciler "github.com/tektoncd/pipeline/pkg/reconciler"
	"github.com/tektoncd/pipeline/pkg/reconciler/events"
	"github.com/tektoncd/pipeline/pkg/reconciler/events/cloudevent"
	"github.com/tektoncd/pipeline/pkg/reconciler/pipeline/dag"
	"github.com/tektoncd/pipeline/pkg/reconciler/pipelinerun/resources"
	"github.com/tektoncd/pipeline/pkg/reconciler/taskrun"
	tresources "github.com/tektoncd/pipeline/pkg/reconciler/taskrun/resources"
	"github.com/tektoncd/pipeline/pkg/reconciler/volumeclaim"
	"github.com/tektoncd/pipeline/pkg/workspace"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"knative.dev/pkg/apis"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/kmeta"
	"knative.dev/pkg/logging"
	pkgreconciler "knative.dev/pkg/reconciler"
	"knative.dev/pkg/tracker"
)

const (
	// ReasonCouldntGetPipeline indicates that the reason for the failure status is that the
	// associated Pipeline couldn't be retrieved
	ReasonCouldntGetPipeline = "CouldntGetPipeline"
	// ReasonInvalidBindings indicates that the reason for the failure status is that the
	// PipelineResources bound in the PipelineRun didn't match those declared in the Pipeline
	ReasonInvalidBindings = "InvalidPipelineResourceBindings"
	// ReasonInvalidWorkspaceBinding indicates that a Pipeline expects a workspace but a
	// PipelineRun has provided an invalid binding.
	ReasonInvalidWorkspaceBinding = "InvalidWorkspaceBindings"
	// ReasonInvalidServiceAccountMapping indicates that PipelineRun.Spec.ServiceAccountNames defined with a wrong taskName
	ReasonInvalidServiceAccountMapping = "InvalidServiceAccountMappings"
	// ReasonParameterTypeMismatch indicates that the reason for the failure status is that
	// parameter(s) declared in the PipelineRun do not have the some declared type as the
	// parameters(s) declared in the Pipeline that they are supposed to override.
	ReasonParameterTypeMismatch = "ParameterTypeMismatch"
	// ReasonCouldntGetTask indicates that the reason for the failure status is that the
	// associated Pipeline's Tasks couldn't all be retrieved
	ReasonCouldntGetTask = "CouldntGetTask"
	// ReasonCouldntGetResource indicates that the reason for the failure status is that the
	// associated PipelineRun's bound PipelineResources couldn't all be retrieved
	ReasonCouldntGetResource = "CouldntGetResource"
	// ReasonCouldntGetCondition indicates that the reason for the failure status is that the
	// associated Pipeline's Conditions couldn't all be retrieved
	ReasonCouldntGetCondition = "CouldntGetCondition"
	// ReasonParameterMissing indicates that the reason for the failure status is that the
	// associated PipelineRun didn't provide all the required parameters
	ReasonParameterMissing = "ParameterMissing"
	// ReasonFailedValidation indicates that the reason for failure status is
	// that pipelinerun failed runtime validation
	ReasonFailedValidation = "PipelineValidationFailed"
	// ReasonInvalidGraph indicates that the reason for the failure status is that the
	// associated Pipeline is an invalid graph (a.k.a wrong order, cycle, …)
	ReasonInvalidGraph = "PipelineInvalidGraph"
	// ReasonCancelled indicates that a PipelineRun was cancelled.
	ReasonCancelled = "Cancelled"
	// Deprecated: "PipelineRunCancelled" indicates that a PipelineRun was cancelled.
	ReasonCancelledDeprecated = "PipelineRunCancelled"
	// ReasonPending indicates that a PipelineRun is pending.
	ReasonPending = "PipelineRunPending"
	// ReasonCouldntCancel indicates that a PipelineRun was cancelled but attempting to update
	// all of the running TaskRuns as cancelled failed.
	ReasonCouldntCancel = "PipelineRunCouldntCancel"
	// ReasonInvalidTaskResultReference indicates a task result was declared
	// but was not initialized by that task
	ReasonInvalidTaskResultReference = "InvalidTaskResultReference"
	// ReasonRequiredWorkspaceMarkedOptional indicates an optional workspace
	// has been passed to a Task that is expecting a non-optional workspace
	ReasonRequiredWorkspaceMarkedOptional = "RequiredWorkspaceMarkedOptional"
)

// Reconciler implements controller.Reconciler for Configuration resources.
type Reconciler struct {
	KubeClientSet     kubernetes.Interface
	PipelineClientSet clientset.Interface
	Images            pipeline.Images

	// listers index properties about resources
	pipelineRunLister listers.PipelineRunLister
	pipelineLister    listers.PipelineLister
	taskRunLister     listers.TaskRunLister
	runLister         listersv1alpha1.RunLister
	taskLister        listers.TaskLister
	clusterTaskLister listers.ClusterTaskLister
	resourceLister    resourcelisters.PipelineResourceLister
	conditionLister   listersv1alpha1.ConditionLister
	cloudEventClient  cloudevent.CEClient
	tracker           tracker.Interface
	metrics           *Recorder
	pvcHandler        volumeclaim.PvcHandler

	snooze func(kmeta.Accessor, time.Duration)
}

var (
	// Check that our Reconciler implements pipelinerunreconciler.Interface
	_ pipelinerunreconciler.Interface = (*Reconciler)(nil)
)

// ReconcileKind compares the actual state with the desired, and attempts to
// converge the two. It then updates the Status block of the Pipeline Run
// resource with the current status of the resource.
func (c *Reconciler) ReconcileKind(ctx context.Context, pr *v1beta1.PipelineRun) pkgreconciler.Event {
	logger := logging.FromContext(ctx)
	ctx = cloudevent.ToContext(ctx, c.cloudEventClient)

	// Read the initial condition
	before := pr.Status.GetCondition(apis.ConditionSucceeded)

	if !pr.HasStarted() && !pr.IsPending() {
		pr.Status.InitializeConditions()
		// In case node time was not synchronized, when controller has been scheduled to other nodes.
		if pr.Status.StartTime.Sub(pr.CreationTimestamp.Time) < 0 {
			logger.Warnf("PipelineRun %s createTimestamp %s is after the pipelineRun started %s", pr.GetNamespacedName().String(), pr.CreationTimestamp, pr.Status.StartTime)
			pr.Status.StartTime = &pr.CreationTimestamp
		}

		// Emit events. During the first reconcile the status of the PipelineRun may change twice
		// from not Started to Started and then to Running, so we need to sent the event here
		// and at the end of 'Reconcile' again.
		// We also want to send the "Started" event as soon as possible for anyone who may be waiting
		// on the event to perform user facing initialisations, such has reset a CI check status
		afterCondition := pr.Status.GetCondition(apis.ConditionSucceeded)
		events.Emit(ctx, nil, afterCondition, pr)

		// We already sent an event for start, so update `before` with the current status
		before = pr.Status.GetCondition(apis.ConditionSucceeded)
	}

	if pr.IsDone() {
		// We may be reading a version of the object that was stored at an older version
		// and may not have had all of the assumed default specified.
		pr.SetDefaults(contexts.WithUpgradeViaDefaulting(ctx))

		if err := artifacts.CleanupArtifactStorage(ctx, pr, c.KubeClientSet); err != nil {
			logger.Errorf("Failed to delete PVC for PipelineRun %s: %v", pr.Name, err)
			return c.finishReconcileUpdateEmitEvents(ctx, pr, before, err)
		}
		if err := c.cleanupAffinityAssistants(ctx, pr); err != nil {
			logger.Errorf("Failed to delete StatefulSet for PipelineRun %s: %v", pr.Name, err)
			return c.finishReconcileUpdateEmitEvents(ctx, pr, before, err)
		}
		if err := c.updateTaskRunsStatusDirectly(pr); err != nil {
			logger.Errorf("Failed to update TaskRun status for PipelineRun %s: %v", pr.Name, err)
			return c.finishReconcileUpdateEmitEvents(ctx, pr, before, err)
		}
		if err := c.updateRunsStatusDirectly(pr); err != nil {
			logger.Errorf("Failed to update Run status for PipelineRun %s: %v", pr.Name, err)
			return c.finishReconcileUpdateEmitEvents(ctx, pr, before, err)
		}
		go func(metrics *Recorder) {
			err := metrics.DurationAndCount(pr)
			if err != nil {
				logger.Warnf("Failed to log the metrics : %v", err)
			}
		}(c.metrics)
		return c.finishReconcileUpdateEmitEvents(ctx, pr, before, nil)
	}

	// If the pipelinerun is cancelled, cancel tasks and update status
	if pr.IsCancelled() {
		err := cancelPipelineRun(ctx, logger, pr, c.PipelineClientSet)
		return c.finishReconcileUpdateEmitEvents(ctx, pr, before, err)
	}

	if err := c.tracker.TrackReference(tracker.Reference{
		APIVersion: v1beta1.SchemeGroupVersion.String(),
		Kind:       "TaskRun",
		Namespace:  pr.GetNamespace(),
		Name:       pr.GetName(),
	}, pr); err != nil {
		logger.Errorf("Failed to create tracker for TaskRuns for PipelineRun %s: %v", pr.Name, err)
		return c.finishReconcileUpdateEmitEvents(ctx, pr, before, err)
	}

	// Make sure that the PipelineRun status is in sync with the actual TaskRuns
	err := c.updatePipelineRunStatusFromInformer(ctx, pr)
	if err != nil {
		// This should not fail. Return the error so we can re-try later.
		logger.Errorf("Error while syncing the pipelinerun status: %v", err.Error())
		return c.finishReconcileUpdateEmitEvents(ctx, pr, before, err)
	}
	defer func() {
		if pr.Status.StartTime == nil {
			return
		}
		// Compute the time since the task started.
		elapsed := time.Since(pr.Status.StartTime.Time)
		// Snooze this resource until the timeout has elapsed.
		c.snooze(pr, pr.GetTimeout(ctx)-elapsed)
	}()

	// Reconcile this copy of the pipelinerun and then write back any status or label
	// updates regardless of whether the reconciliation errored out.
	if err = c.reconcile(ctx, pr); err != nil {
		logger.Errorf("Reconcile error: %v", err.Error())
	}

	return c.finishReconcileUpdateEmitEvents(ctx, pr, before, err)
}

func (c *Reconciler) finishReconcileUpdateEmitEvents(ctx context.Context, pr *v1beta1.PipelineRun, beforeCondition *apis.Condition, previousError error) error {
	logger := logging.FromContext(ctx)

	afterCondition := pr.Status.GetCondition(apis.ConditionSucceeded)
	events.Emit(ctx, beforeCondition, afterCondition, pr)
	_, err := c.updateLabelsAndAnnotations(ctx, pr)
	if err != nil {
		logger.Warn("Failed to update PipelineRun labels/annotations", zap.Error(err))
		events.EmitError(controller.GetEventRecorder(ctx), err, pr)
	}

	merr := multierror.Append(previousError, err).ErrorOrNil()
	if controller.IsPermanentError(previousError) {
		return controller.NewPermanentError(merr)
	}
	return merr
}

// resolvePipelineState will attempt to resolve each referenced task in the pipeline's spec and all of the resources
// specified by those tasks.
func (c *Reconciler) resolvePipelineState(
	ctx context.Context,
	tasks []v1beta1.PipelineTask,
	pipelineMeta *metav1.ObjectMeta,
	pr *v1beta1.PipelineRun,
	providedResources map[string]*v1alpha1.PipelineResource) (resources.PipelineRunState, error) {
	pst := resources.PipelineRunState{}
	// Resolve each task individually because they each could have a different reference context (remote or local).
	for _, task := range tasks {
		fn, err := tresources.GetTaskFunc(ctx, c.KubeClientSet, c.PipelineClientSet, task.TaskRef, pr.Namespace, pr.Spec.ServiceAccountName)
		if err != nil {
			// This Run has failed, so we need to mark it as failed and stop reconciling it
			pr.Status.MarkFailed(ReasonCouldntGetTask, "Pipeline %s/%s can't be Run; task %s could not be fetched: %s",
				pipelineMeta.Namespace, pipelineMeta.Name, task.Name, err)
			return nil, controller.NewPermanentError(err)
		}

		resolvedTask, err := resources.ResolvePipelineRunTask(ctx,
			*pr,
			fn,
			func(name string) (*v1beta1.TaskRun, error) {
				return c.taskRunLister.TaskRuns(pr.Namespace).Get(name)
			},
			func(name string) (*v1alpha1.Run, error) {
				return c.runLister.Runs(pr.Namespace).Get(name)
			},
			func(name string) (*v1alpha1.Condition, error) {
				return c.conditionLister.Conditions(pr.Namespace).Get(name)
			},
			task, providedResources,
		)
		if err != nil {
			switch err := err.(type) {
			case *resources.TaskNotFoundError:
				pr.Status.MarkFailed(ReasonCouldntGetTask,
					"Pipeline %s/%s can't be Run; it contains Tasks that don't exist: %s",
					pipelineMeta.Namespace, pipelineMeta.Name, err)
			case *resources.ConditionNotFoundError:
				pr.Status.MarkFailed(ReasonCouldntGetCondition,
					"PipelineRun %s/%s can't be Run; it contains Conditions that don't exist:  %s",
					pipelineMeta.Namespace, pr.Name, err)
			default:
				pr.Status.MarkFailed(ReasonFailedValidation,
					"PipelineRun %s/%s can't be Run; couldn't resolve all references: %s",
					pipelineMeta.Namespace, pr.Name, err)
			}
			return nil, controller.NewPermanentError(err)
		}
		pst = append(pst, resolvedTask)
	}
	return pst, nil
}

func (c *Reconciler) reconcile(ctx context.Context, pr *v1beta1.PipelineRun) error {
	logger := logging.FromContext(ctx)
	cfg := config.FromContextOrDefaults(ctx)
	// We may be reading a version of the object that was stored at an older version
	// and may not have had all of the assumed default specified.
	pr.SetDefaults(contexts.WithUpgradeViaDefaulting(ctx))

	// When pipeline run is pending, return to avoid creating the task
	if pr.IsPending() {
		pr.Status.SetCondition(&apis.Condition{
			Type:    apis.ConditionSucceeded,
			Status:  corev1.ConditionUnknown,
			Reason:  ReasonPending,
			Message: fmt.Sprintf("PipelineRun %q is pending", pr.Name),
		})
		return nil
	}

	var (
		err          error
		pipelineMeta metav1.ObjectMeta
		pipelineSpec v1beta1.PipelineSpec
	)

	if pr.Status.PipelineSpec == nil {
		err = fmt.Errorf("pipeline spec expected to be cached in status of pipelinerun %s/%s but it was not found", pr.Namespace, pr.Name)
	} else {
		var name string
		if pr.Spec.PipelineRef != nil {
			name = pr.Spec.PipelineRef.Name
		} else {
			name = pr.ObjectMeta.Name
		}
		pipelineMeta = metav1.ObjectMeta{
			Name:      name,
			Namespace: pr.Namespace,
		}
		pipelineSpec = *pr.Status.PipelineSpec
	}

	if err != nil {
		logger.Errorf("Failed to determine Pipeline spec to use for pipelinerun %s: %v", pr.Name, err)
		pr.Status.MarkFailed(ReasonCouldntGetPipeline,
			"Error retrieving pipeline for pipelinerun %s/%s: %s",
			pr.Namespace, pr.Name, err)
		return controller.NewPermanentError(err)
	}

	d, err := dag.Build(v1beta1.PipelineTaskList(pipelineSpec.Tasks), v1beta1.PipelineTaskList(pipelineSpec.Tasks).Deps())
	if err != nil {
		// This Run has failed, so we need to mark it as failed and stop reconciling it
		pr.Status.MarkFailed(ReasonInvalidGraph,
			"PipelineRun %s/%s's Pipeline DAG is invalid: %s",
			pr.Namespace, pr.Name, err)
		return controller.NewPermanentError(err)
	}

	// build DAG with a list of final tasks, this DAG is used later to identify
	// if a task in PipelineRunState is final task or not
	// the finally section is optional and might not exist
	// dfinally holds an empty Graph in the absence of finally clause
	dfinally, err := dag.Build(v1beta1.PipelineTaskList(pipelineSpec.Finally), map[string][]string{})
	if err != nil {
		// This Run has failed, so we need to mark it as failed and stop reconciling it
		pr.Status.MarkFailed(ReasonInvalidGraph,
			"PipelineRun %s's Pipeline DAG is invalid for finally clause: %s",
			pr.Namespace, pr.Name, err)
		return controller.NewPermanentError(err)
	}

	if err := pipelineSpec.Validate(ctx); err != nil {
		// This Run has failed, so we need to mark it as failed and stop reconciling it
		pr.Status.MarkFailed(ReasonFailedValidation,
			"Pipeline %s/%s can't be Run; it has an invalid spec: %s",
			pipelineMeta.Namespace, pipelineMeta.Name, err)
		return controller.NewPermanentError(err)
	}

	if err := resources.ValidateResourceBindings(&pipelineSpec, pr); err != nil {
		// This Run has failed, so we need to mark it as failed and stop reconciling it
		pr.Status.MarkFailed(ReasonInvalidBindings,
			"PipelineRun %s/%s doesn't bind Pipeline %s/%s's PipelineResources correctly: %s",
			pr.Namespace, pr.Name, pr.Namespace, pipelineMeta.Name, err)
		return controller.NewPermanentError(err)
	}
	providedResources, err := resources.GetResourcesFromBindings(pr, c.resourceLister.PipelineResources(pr.Namespace).Get)
	if err != nil {
		if errors.IsNotFound(err) && tknreconciler.IsYoungResource(pr) {
			// For newly created resources, don't fail immediately.
			// Instead return an (non-permanent) error, which will prompt the
			// controller to requeue the key with backoff.
			logger.Warnf("References for pipelinerun %s not found: %v", pr.Name, err)
			pr.Status.MarkRunning(ReasonCouldntGetResource,
				"Unable to resolve dependencies for %q: %v", pr.Name, err)
			return err
		}
		// This Run has failed, so we need to mark it as failed and stop reconciling it
		pr.Status.MarkFailed(ReasonCouldntGetResource,
			"PipelineRun %s/%s can't be Run; it tries to bind Resources that don't exist: %s",
			pipelineMeta.Namespace, pr.Name, err)
		return controller.NewPermanentError(err)
	}
	// Ensure that the PipelineRun provides all the parameters required by the Pipeline
	if err := resources.ValidateRequiredParametersProvided(&pipelineSpec.Params, &pr.Spec.Params); err != nil {
		// This Run has failed, so we need to mark it as failed and stop reconciling it
		pr.Status.MarkFailed(ReasonParameterMissing,
			"PipelineRun %s parameters is missing some parameters required by Pipeline %s's parameters: %s",
			pr.Namespace, pr.Name, err)
		return controller.NewPermanentError(err)
	}

	// Ensure that the parameters from the PipelineRun are overriding Pipeline parameters with the same type.
	// Weird substitution issues can occur if this is not validated (ApplyParameters() does not verify type).
	err = resources.ValidateParamTypesMatching(&pipelineSpec, pr)
	if err != nil {
		// This Run has failed, so we need to mark it as failed and stop reconciling it
		pr.Status.MarkFailed(ReasonParameterTypeMismatch,
			"PipelineRun %s/%s parameters have mismatching types with Pipeline %s/%s's parameters: %s",
			pr.Namespace, pr.Name, pr.Namespace, pipelineMeta.Name, err)
		return controller.NewPermanentError(err)
	}

	// Ensure that the workspaces expected by the Pipeline are provided by the PipelineRun.
	if err := resources.ValidateWorkspaceBindings(&pipelineSpec, pr); err != nil {
		pr.Status.MarkFailed(ReasonInvalidWorkspaceBinding,
			"PipelineRun %s/%s doesn't bind Pipeline %s/%s's Workspaces correctly: %s",
			pr.Namespace, pr.Name, pr.Namespace, pipelineMeta.Name, err)
		return controller.NewPermanentError(err)
	}

	// Ensure that the ServiceAccountNames defined are correct.
	// This is "deprecated".
	if err := resources.ValidateServiceaccountMapping(&pipelineSpec, pr); err != nil {
		pr.Status.MarkFailed(ReasonInvalidServiceAccountMapping,
			"PipelineRun %s/%s doesn't define ServiceAccountNames correctly: %s",
			pr.Namespace, pr.Name, err)
		return controller.NewPermanentError(err)
	}

	// Ensure that the TaskRunSpecs defined are correct.
	if err := resources.ValidateTaskRunSpecs(&pipelineSpec, pr); err != nil {
		pr.Status.MarkFailed(ReasonInvalidServiceAccountMapping,
			"PipelineRun %s/%s doesn't define taskRunSpecs correctly: %s",
			pr.Namespace, pr.Name, err)
		return controller.NewPermanentError(err)
	}

	// Apply parameter substitution from the PipelineRun
	pipelineSpec = *resources.ApplyParameters(&pipelineSpec, pr)
	pipelineSpec = *resources.ApplyContexts(&pipelineSpec, pipelineMeta.Name, pr)
	pipelineSpec = *resources.ApplyWorkspaces(&pipelineSpec, pr)

	// pipelineState holds a list of pipeline tasks after resolving conditions and pipeline resources
	// pipelineState also holds a taskRun for each pipeline task after the taskRun is created
	// pipelineState is instantiated and updated on every reconcile cycle
	// Resolve the set of tasks (and possibly task runs).
	tasks := pipelineSpec.Tasks
	if len(pipelineSpec.Finally) > 0 {
		tasks = append(tasks, pipelineSpec.Finally...)
	}
	pipelineRunState, err := c.resolvePipelineState(ctx, tasks, &pipelineMeta, pr, providedResources)
	if err != nil {
		return err
	}

	// Build PipelineRunFacts with a list of resolved pipeline tasks,
	// dag tasks graph and final tasks graph
	pipelineRunFacts := &resources.PipelineRunFacts{
		State:           pipelineRunState,
		SpecStatus:      pr.Spec.Status,
		TasksGraph:      d,
		FinalTasksGraph: dfinally,
	}

	for _, rprt := range pipelineRunFacts.State {
		if !rprt.IsCustomTask() {
			err := taskrun.ValidateResolvedTaskResources(rprt.PipelineTask.Params, rprt.ResolvedTaskResources)
			if err != nil {
				logger.Errorf("Failed to validate pipelinerun %q with error %v", pr.Name, err)
				pr.Status.MarkFailed(ReasonFailedValidation, err.Error())
				return controller.NewPermanentError(err)
			}
		}
	}

	// check if pipeline run is not gracefully cancelled and there are active task runs, which require cancelling
	if cfg.FeatureFlags.EnableAPIFields == apisconfig.AlphaAPIFields &&
		pr.IsGracefullyCancelled() && pipelineRunFacts.IsRunning() {
		// If the pipelinerun is cancelled, cancel tasks, but run finally
		err := gracefullyCancelPipelineRun(ctx, logger, pr, c.PipelineClientSet)
		if err != nil {
			// failed to cancel tasks, maybe retry would help (don't return permanent error)
			return err
		}
	}

	if pipelineRunFacts.State.IsBeforeFirstTaskRun() {
		if err := resources.ValidatePipelineTaskResults(pipelineRunFacts.State); err != nil {
			logger.Errorf("Failed to resolve task result reference for %q with error %v", pr.Name, err)
			pr.Status.MarkFailed(ReasonInvalidTaskResultReference, err.Error())
			return controller.NewPermanentError(err)
		}

		if err := resources.ValidatePipelineResults(&pipelineSpec, pipelineRunFacts.State); err != nil {
			logger.Errorf("Failed to resolve task result reference for %q with error %v", pr.Name, err)
			pr.Status.MarkFailed(ReasonInvalidTaskResultReference, err.Error())
			return controller.NewPermanentError(err)
		}

		if err := resources.ValidateOptionalWorkspaces(pipelineSpec.Workspaces, pipelineRunFacts.State); err != nil {
			logger.Errorf("Optional workspace not supported by task: %v", err)
			pr.Status.MarkFailed(ReasonRequiredWorkspaceMarkedOptional, err.Error())
			return controller.NewPermanentError(err)
		}

		if pr.HasVolumeClaimTemplate() {
			// create workspace PVC from template
			if err = c.pvcHandler.CreatePersistentVolumeClaimsForWorkspaces(ctx, pr.Spec.Workspaces, pr.GetOwnerReference(), pr.Namespace); err != nil {
				logger.Errorf("Failed to create PVC for PipelineRun %s: %v", pr.Name, err)
				pr.Status.MarkFailed(volumeclaim.ReasonCouldntCreateWorkspacePVC,
					"Failed to create PVC for PipelineRun %s/%s Workspaces correctly: %s",
					pr.Namespace, pr.Name, err)
				return controller.NewPermanentError(err)
			}
		}

		if !c.isAffinityAssistantDisabled(ctx) {
			// create Affinity Assistant (StatefulSet) so that taskRun pods that share workspace PVC achieve Node Affinity
			if err = c.createAffinityAssistants(ctx, pr.Spec.Workspaces, pr, pr.Namespace); err != nil {
				logger.Errorf("Failed to create affinity assistant StatefulSet for PipelineRun %s: %v", pr.Name, err)
				pr.Status.MarkFailed(ReasonCouldntCreateAffinityAssistantStatefulSet,
					"Failed to create StatefulSet for PipelineRun %s/%s correctly: %s",
					pr.Namespace, pr.Name, err)
				return controller.NewPermanentError(err)
			}
		}
	}

	as, err := artifacts.InitializeArtifactStorage(ctx, c.Images, pr, &pipelineSpec, c.KubeClientSet)
	if err != nil {
		logger.Infof("PipelineRun failed to initialize artifact storage %s", pr.Name)
		return controller.NewPermanentError(err)
	}

	if err := c.runNextSchedulableTask(ctx, pr, pipelineRunFacts, as); err != nil {
		return err
	}

	// Reset the skipped status to trigger recalculation
	pipelineRunFacts.ResetSkippedCache()

	after := pipelineRunFacts.GetPipelineConditionStatus(pr, logger)
	switch after.Status {
	case corev1.ConditionTrue:
		pr.Status.MarkSucceeded(after.Reason, after.Message)
	case corev1.ConditionFalse:
		pr.Status.MarkFailed(after.Reason, after.Message)
	case corev1.ConditionUnknown:
		pr.Status.MarkRunning(after.Reason, after.Message)
	}
	// Read the condition the way it was set by the Mark* helpers
	after = pr.Status.GetCondition(apis.ConditionSucceeded)
	pr.Status.StartTime = pipelineRunFacts.State.AdjustStartTime(pr.Status.StartTime)
	pr.Status.TaskRuns = pipelineRunFacts.State.GetTaskRunsStatus(pr)
	pr.Status.Runs = pipelineRunFacts.State.GetRunsStatus(pr)
	pr.Status.SkippedTasks = pipelineRunFacts.GetSkippedTasks()

	if after.Status == corev1.ConditionTrue {
		pr.Status.PipelineResults = resources.ApplyTaskResultsToPipelineResults(pipelineSpec.Results, pr.Status.TaskRuns, pr.Status.Runs)
	}

	logger.Infof("PipelineRun %s status is being set to %s", pr.Name, after)
	return nil
}

// runNextSchedulableTask gets the next schedulable Tasks from the dag based on the current
// pipeline run state, and starts them
// after all DAG tasks are done, it's responsible for scheduling final tasks and start executing them
func (c *Reconciler) runNextSchedulableTask(ctx context.Context, pr *v1beta1.PipelineRun, pipelineRunFacts *resources.PipelineRunFacts, as artifacts.ArtifactStorageInterface) error {

	logger := logging.FromContext(ctx)
	recorder := controller.GetEventRecorder(ctx)

	// nextRprts holds a list of pipeline tasks which should be executed next
	nextRprts, err := pipelineRunFacts.DAGExecutionQueue()
	if err != nil {
		logger.Errorf("Error getting potential next tasks for valid pipelinerun %s: %v", pr.Name, err)
		return controller.NewPermanentError(err)
	}

	resolvedResultRefs, err := resources.ResolveResultRefs(pipelineRunFacts.State, nextRprts)
	if err != nil {
		logger.Infof("Failed to resolve task result reference for %q with error %v", pr.Name, err)
		pr.Status.MarkFailed(ReasonInvalidTaskResultReference, err.Error())
		return controller.NewPermanentError(err)
	}

	resources.ApplyTaskResults(nextRprts, resolvedResultRefs)
	// After we apply Task Results, we may be able to evaluate more
	// when expressions, so reset the skipped cache
	pipelineRunFacts.ResetSkippedCache()

	// GetFinalTasks only returns tasks when a DAG is complete
	fnextRprts := pipelineRunFacts.GetFinalTasks()
	if len(fnextRprts) != 0 {
		// apply the runtime context just before creating taskRuns for final tasks in queue
		resources.ApplyPipelineTaskStateContext(fnextRprts, pipelineRunFacts.GetPipelineTaskStatus())

		// Before creating TaskRun for scheduled final task, check if it's consuming a task result
		// Resolve and apply task result wherever applicable, report warning in case resolution fails
		for _, rprt := range fnextRprts {
			resolvedResultRefs, err := resources.ResolveResultRef(pipelineRunFacts.State, rprt)
			if err != nil {
				logger.Infof("Final task %q is not executed as it could not resolve task params for %q: %v", rprt.PipelineTask.Name, pr.Name, err)
				continue
			}
			resources.ApplyTaskResults(resources.PipelineRunState{rprt}, resolvedResultRefs)
			nextRprts = append(nextRprts, rprt)
		}
	}

	for _, rprt := range nextRprts {
		if rprt == nil || rprt.Skip(pipelineRunFacts) || rprt.IsFinallySkipped(pipelineRunFacts) {
			continue
		}
		if rprt.ResolvedConditionChecks == nil || rprt.ResolvedConditionChecks.IsSuccess() {
			if rprt.IsCustomTask() {
				rprt.Run, err = c.createRun(ctx, rprt, pr)
				if err != nil {
					recorder.Eventf(pr, corev1.EventTypeWarning, "RunCreationFailed", "Failed to create Run %q: %v", rprt.RunName, err)
					return fmt.Errorf("error creating Run called %s for PipelineTask %s from PipelineRun %s: %w", rprt.RunName, rprt.PipelineTask.Name, pr.Name, err)
				}
			} else {
				if rprt.IsFinalTask(pipelineRunFacts) {
					rprt.TaskRun, err = c.createTaskRun(ctx, rprt, pr, as.StorageBasePath(pr), getFinallyTaskRunTimeout)
				} else {
					rprt.TaskRun, err = c.createTaskRun(ctx, rprt, pr, as.StorageBasePath(pr), getTaskRunTimeout)
				}
				if err != nil {
					recorder.Eventf(pr, corev1.EventTypeWarning, "TaskRunCreationFailed", "Failed to create TaskRun %q: %v", rprt.TaskRunName, err)
					return fmt.Errorf("error creating TaskRun called %s for PipelineTask %s from PipelineRun %s: %w", rprt.TaskRunName, rprt.PipelineTask.Name, pr.Name, err)
				}
			}
		} else if !rprt.ResolvedConditionChecks.HasStarted() {
			for _, rcc := range rprt.ResolvedConditionChecks {
				rcc.ConditionCheck, err = c.makeConditionCheckContainer(ctx, rprt, rcc, pr)
				if err != nil {
					recorder.Eventf(pr, corev1.EventTypeWarning, "ConditionCheckCreationFailed", "Failed to create TaskRun %q: %v", rcc.ConditionCheckName, err)
					return fmt.Errorf("error creating ConditionCheck container called %s for PipelineTask %s from PipelineRun %s: %w", rcc.ConditionCheckName, rprt.PipelineTask.Name, pr.Name, err)
				}
			}
		}
	}
	return nil
}

func (c *Reconciler) updateTaskRunsStatusDirectly(pr *v1beta1.PipelineRun) error {
	for taskRunName := range pr.Status.TaskRuns {
		// TODO(dibyom): Add conditionCheck statuses here
		prtrs := pr.Status.TaskRuns[taskRunName]
		tr, err := c.taskRunLister.TaskRuns(pr.Namespace).Get(taskRunName)
		if err != nil {
			// If the TaskRun isn't found, it just means it won't be run
			if !errors.IsNotFound(err) {
				return fmt.Errorf("error retrieving TaskRun %s: %w", taskRunName, err)
			}
		} else {
			prtrs.Status = &tr.Status
		}
	}
	return nil
}

func (c *Reconciler) updateRunsStatusDirectly(pr *v1beta1.PipelineRun) error {
	for runName := range pr.Status.Runs {
		prRunStatus := pr.Status.Runs[runName]
		run, err := c.runLister.Runs(pr.Namespace).Get(runName)
		if err != nil {
			if !errors.IsNotFound(err) {
				return fmt.Errorf("error retrieving Run %s: %w", runName, err)
			}
		} else {
			prRunStatus.Status = &run.Status
		}
	}
	return nil
}

type getTimeoutFunc func(ctx context.Context, pr *v1beta1.PipelineRun, rprt *resources.ResolvedPipelineRunTask) *metav1.Duration

func (c *Reconciler) createTaskRun(ctx context.Context, rprt *resources.ResolvedPipelineRunTask, pr *v1beta1.PipelineRun, storageBasePath string, getTimeoutFunc getTimeoutFunc) (*v1beta1.TaskRun, error) {
	logger := logging.FromContext(ctx)

	tr, _ := c.taskRunLister.TaskRuns(pr.Namespace).Get(rprt.TaskRunName)
	if tr != nil {
		// is a retry
		addRetryHistory(tr)
		clearStatus(tr)
		tr.Status.SetCondition(&apis.Condition{
			Type:   apis.ConditionSucceeded,
			Status: corev1.ConditionUnknown,
		})
		return c.PipelineClientSet.TektonV1beta1().TaskRuns(pr.Namespace).UpdateStatus(ctx, tr, metav1.UpdateOptions{})
	}

	rprt.PipelineTask = resources.ApplyPipelineTaskContexts(rprt.PipelineTask)
	taskRunSpec := pr.GetTaskRunSpec(rprt.PipelineTask.Name)
	tr = &v1beta1.TaskRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:            rprt.TaskRunName,
			Namespace:       pr.Namespace,
			OwnerReferences: []metav1.OwnerReference{pr.GetOwnerReference()},
			Labels:          combineTaskRunAndTaskSpecLabels(pr, rprt.PipelineTask),
			Annotations:     combineTaskRunAndTaskSpecAnnotations(pr, rprt.PipelineTask),
		},
		Spec: v1beta1.TaskRunSpec{
			Params:             rprt.PipelineTask.Params,
			ServiceAccountName: taskRunSpec.TaskServiceAccountName,
			Timeout:            getTimeoutFunc(ctx, pr, rprt),
			PodTemplate:        taskRunSpec.TaskPodTemplate,
		}}

	if rprt.ResolvedTaskResources.TaskName != "" {
		// We pass the entire, original task ref because it may contain additional references like a Bundle url.
		tr.Spec.TaskRef = rprt.PipelineTask.TaskRef
	} else if rprt.ResolvedTaskResources.TaskSpec != nil {
		tr.Spec.TaskSpec = rprt.ResolvedTaskResources.TaskSpec
	}

	var pipelinePVCWorkspaceName string
	var err error
	tr.Spec.Workspaces, pipelinePVCWorkspaceName, err = getTaskrunWorkspaces(pr, rprt)
	if err != nil {
		return nil, err
	}

	if !c.isAffinityAssistantDisabled(ctx) && pipelinePVCWorkspaceName != "" {
		tr.Annotations[workspace.AnnotationAffinityAssistantName] = getAffinityAssistantName(pipelinePVCWorkspaceName, pr.Name)
	}

	resources.WrapSteps(&tr.Spec, rprt.PipelineTask, rprt.ResolvedTaskResources.Inputs, rprt.ResolvedTaskResources.Outputs, storageBasePath)
	logger.Infof("Creating a new TaskRun object %s for pipeline task %s", rprt.TaskRunName, rprt.PipelineTask.Name)
	return c.PipelineClientSet.TektonV1beta1().TaskRuns(pr.Namespace).Create(ctx, tr, metav1.CreateOptions{})
}

func (c *Reconciler) createRun(ctx context.Context, rprt *resources.ResolvedPipelineRunTask, pr *v1beta1.PipelineRun) (*v1alpha1.Run, error) {
	logger := logging.FromContext(ctx)
	taskRunSpec := pr.GetTaskRunSpec(rprt.PipelineTask.Name)
	r := &v1alpha1.Run{
		ObjectMeta: metav1.ObjectMeta{
			Name:            rprt.RunName,
			Namespace:       pr.Namespace,
			OwnerReferences: []metav1.OwnerReference{pr.GetOwnerReference()},
			Labels:          getTaskrunLabels(pr, rprt.PipelineTask.Name, true),
			Annotations:     getTaskrunAnnotations(pr),
		},
		Spec: v1alpha1.RunSpec{
			Ref:                rprt.PipelineTask.TaskRef,
			Params:             rprt.PipelineTask.Params,
			ServiceAccountName: taskRunSpec.TaskServiceAccountName,
			PodTemplate:        taskRunSpec.TaskPodTemplate,
		},
	}

	if rprt.PipelineTask.TaskSpec != nil {
		j, err := json.Marshal(rprt.PipelineTask.TaskSpec.Spec)
		if err != nil {
			return nil, err
		}
		r.Spec.Spec = &v1alpha1.EmbeddedRunSpec{
			TypeMeta: runtime.TypeMeta{
				APIVersion: rprt.PipelineTask.TaskSpec.APIVersion,
				Kind:       rprt.PipelineTask.TaskSpec.Kind,
			},
			Metadata: rprt.PipelineTask.TaskSpec.Metadata,
			Spec: runtime.RawExtension{
				Raw: j,
			},
		}
	}
	var pipelinePVCWorkspaceName string
	var err error
	r.Spec.Workspaces, pipelinePVCWorkspaceName, err = getTaskrunWorkspaces(pr, rprt)
	if err != nil {
		return nil, err
	}

	// Set the affinity assistant annotation in case the custom task creates TaskRuns or Pods
	// that can take advantage of it.
	if !c.isAffinityAssistantDisabled(ctx) && pipelinePVCWorkspaceName != "" {
		r.Annotations[workspace.AnnotationAffinityAssistantName] = getAffinityAssistantName(pipelinePVCWorkspaceName, pr.Name)
	}

	logger.Infof("Creating a new Run object %s", rprt.RunName)
	return c.PipelineClientSet.TektonV1alpha1().Runs(pr.Namespace).Create(ctx, r, metav1.CreateOptions{})
}

func getTaskrunWorkspaces(pr *v1beta1.PipelineRun, rprt *resources.ResolvedPipelineRunTask) ([]v1beta1.WorkspaceBinding, string, error) {
	var workspaces []v1beta1.WorkspaceBinding
	var pipelinePVCWorkspaceName string
	pipelineRunWorkspaces := make(map[string]v1beta1.WorkspaceBinding)
	for _, binding := range pr.Spec.Workspaces {
		pipelineRunWorkspaces[binding.Name] = binding
	}
	for _, ws := range rprt.PipelineTask.Workspaces {
		taskWorkspaceName, pipelineTaskSubPath, pipelineWorkspaceName := ws.Name, ws.SubPath, ws.Workspace
		if b, hasBinding := pipelineRunWorkspaces[pipelineWorkspaceName]; hasBinding {
			if b.PersistentVolumeClaim != nil || b.VolumeClaimTemplate != nil {
				pipelinePVCWorkspaceName = pipelineWorkspaceName
			}
			workspaces = append(workspaces, taskWorkspaceByWorkspaceVolumeSource(b, taskWorkspaceName, pipelineTaskSubPath, pr.GetOwnerReference()))
		} else {
			workspaceIsOptional := false
			if rprt.ResolvedTaskResources != nil && rprt.ResolvedTaskResources.TaskSpec != nil {
				for _, taskWorkspaceDeclaration := range rprt.ResolvedTaskResources.TaskSpec.Workspaces {
					if taskWorkspaceDeclaration.Name == taskWorkspaceName && taskWorkspaceDeclaration.Optional {
						workspaceIsOptional = true
						break
					}
				}
			}
			if !workspaceIsOptional {
				return nil, "", fmt.Errorf("expected workspace %q to be provided by pipelinerun for pipeline task %q", pipelineWorkspaceName, rprt.PipelineTask.Name)
			}
		}
	}
	return workspaces, pipelinePVCWorkspaceName, nil
}

// taskWorkspaceByWorkspaceVolumeSource is returning the WorkspaceBinding with the TaskRun specified name.
// If the volume source is a volumeClaimTemplate, the template is applied and passed to TaskRun as a persistentVolumeClaim
func taskWorkspaceByWorkspaceVolumeSource(wb v1beta1.WorkspaceBinding, taskWorkspaceName string, pipelineTaskSubPath string, owner metav1.OwnerReference) v1beta1.WorkspaceBinding {
	if wb.VolumeClaimTemplate == nil {
		binding := *wb.DeepCopy()
		binding.Name = taskWorkspaceName
		binding.SubPath = combinedSubPath(wb.SubPath, pipelineTaskSubPath)
		return binding
	}

	// apply template
	binding := v1beta1.WorkspaceBinding{
		SubPath: combinedSubPath(wb.SubPath, pipelineTaskSubPath),
		PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
			ClaimName: volumeclaim.GetPersistentVolumeClaimName(wb.VolumeClaimTemplate, wb, owner),
		},
	}
	binding.Name = taskWorkspaceName
	return binding
}

// combinedSubPath returns the combined value of the optional subPath from workspaceBinding and the optional
// subPath from pipelineTask. If both is set, they are joined with a slash.
func combinedSubPath(workspaceSubPath string, pipelineTaskSubPath string) string {
	if workspaceSubPath == "" {
		return pipelineTaskSubPath
	} else if pipelineTaskSubPath == "" {
		return workspaceSubPath
	}
	return filepath.Join(workspaceSubPath, pipelineTaskSubPath)
}

func addRetryHistory(tr *v1beta1.TaskRun) {
	newStatus := *tr.Status.DeepCopy()
	newStatus.RetriesStatus = nil
	tr.Status.RetriesStatus = append(tr.Status.RetriesStatus, newStatus)
}

func clearStatus(tr *v1beta1.TaskRun) {
	tr.Status.StartTime = nil
	tr.Status.CompletionTime = nil
	tr.Status.PodName = ""
}

func getTaskrunAnnotations(pr *v1beta1.PipelineRun) map[string]string {
	// Propagate annotations from PipelineRun to TaskRun.
	annotations := make(map[string]string, len(pr.ObjectMeta.Annotations)+1)
	for key, val := range pr.ObjectMeta.Annotations {
		annotations[key] = val
	}
	return annotations
}

func getTaskrunLabels(pr *v1beta1.PipelineRun, pipelineTaskName string, includePipelineLabels bool) map[string]string {
	// Propagate labels from PipelineRun to TaskRun.
	labels := make(map[string]string, len(pr.ObjectMeta.Labels)+1)
	if includePipelineLabels {
		for key, val := range pr.ObjectMeta.Labels {
			labels[key] = val
		}
	}
	labels[pipeline.GroupName+pipeline.PipelineRunLabelKey] = pr.Name
	if pipelineTaskName != "" {
		labels[pipeline.GroupName+pipeline.PipelineTaskLabelKey] = pipelineTaskName
	}
	return labels
}

func combineTaskRunAndTaskSpecLabels(pr *v1beta1.PipelineRun, pipelineTask *v1beta1.PipelineTask) map[string]string {
	var tsLabels map[string]string
	trLabels := getTaskrunLabels(pr, pipelineTask.Name, true)

	if pipelineTask.TaskSpec != nil {
		tsLabels = pipelineTask.TaskSpecMetadata().Labels
	}

	labels := make(map[string]string, len(trLabels)+len(tsLabels))

	// labels from TaskRun takes higher precedence over the ones specified in Pipeline through TaskSpec
	// initialize labels with TaskRun labels
	labels = trLabels
	for key, value := range tsLabels {
		// add labels from TaskSpec if the label does not exist
		if _, ok := labels[key]; !ok {
			labels[key] = value
		}
	}
	return labels
}

func combineTaskRunAndTaskSpecAnnotations(pr *v1beta1.PipelineRun, pipelineTask *v1beta1.PipelineTask) map[string]string {
	var tsAnnotations map[string]string
	trAnnotations := getTaskrunAnnotations(pr)

	if pipelineTask.TaskSpec != nil {
		tsAnnotations = pipelineTask.TaskSpecMetadata().Annotations
	}

	annotations := make(map[string]string, len(trAnnotations)+len(tsAnnotations))

	// annotations from TaskRun takes higher precedence over the ones specified in Pipeline through TaskSpec
	// initialize annotations with TaskRun annotations
	annotations = trAnnotations
	for key, value := range tsAnnotations {
		// add annotations from TaskSpec if the annotation does not exist
		if _, ok := annotations[key]; !ok {
			annotations[key] = value
		}
	}
	return annotations
}

func getFinallyTaskRunTimeout(ctx context.Context, pr *v1beta1.PipelineRun, rprt *resources.ResolvedPipelineRunTask) *metav1.Duration {
	var taskRunTimeout = &metav1.Duration{Duration: apisconfig.NoTimeoutDuration}

	var timeout, tasksTimeout time.Duration
	defaultTimeout := time.Duration(config.FromContextOrDefaults(ctx).Defaults.DefaultTimeoutMinutes)

	switch {
	case pr.Spec.Timeout != nil:
		timeout = pr.Spec.Timeout.Duration
	case pr.Spec.Timeouts != nil:
		// Take into account the elapsed time in order to check if we still have enough time to run
		// If task timeout is defined, add it to finally timeout
		// Else consider pipeline timeout as finally timeout
		switch {
		case pr.Spec.Timeouts.Finally != nil:
			if pr.Spec.Timeouts.Tasks != nil {
				tasksTimeout = pr.Spec.Timeouts.Tasks.Duration
				timeout = tasksTimeout + pr.Spec.Timeouts.Finally.Duration
			} else if pr.Spec.Timeouts.Pipeline != nil {
				tasksTimeout = pr.Spec.Timeouts.Pipeline.Duration - pr.Spec.Timeouts.Finally.Duration
				timeout = pr.Spec.Timeouts.Pipeline.Duration
			}
		case pr.Spec.Timeouts.Pipeline != nil:
			timeout = pr.Spec.Timeouts.Pipeline.Duration
			if pr.Spec.Timeouts.Tasks != nil {
				tasksTimeout = pr.Spec.Timeouts.Tasks.Duration
			}
		default:
			timeout = defaultTimeout * time.Minute
			if pr.Spec.Timeouts.Tasks != nil {
				tasksTimeout = pr.Spec.Timeouts.Tasks.Duration
			}
		}
	default:
		timeout = defaultTimeout * time.Minute
	}

	// If the value of the timeout is 0 for any resource, there is no timeout.
	// It is impossible for pr.Spec.Timeout to be nil, since SetDefault always assigns it with a value.
	taskRunTimeout = taskRunTimeoutHelper(timeout, pr, taskRunTimeout, rprt)

	// Now that we know if we still have time to run the final task, subtract tasksTimeout if needed
	if taskRunTimeout.Duration > time.Second {
		taskRunTimeout.Duration -= tasksTimeout
	}

	return taskRunTimeout
}

func getTaskRunTimeout(ctx context.Context, pr *v1beta1.PipelineRun, rprt *resources.ResolvedPipelineRunTask) *metav1.Duration {
	var taskRunTimeout = &metav1.Duration{Duration: apisconfig.NoTimeoutDuration}

	var timeout time.Duration

	switch {
	case pr.Spec.Timeout != nil:
		timeout = pr.Spec.Timeout.Duration
	case pr.Spec.Timeouts != nil:
		if pr.Spec.Timeouts.Tasks != nil {
			timeout = pr.Spec.Timeouts.Tasks.Duration
			break
		}

		if pr.Spec.Timeouts.Pipeline != nil {
			timeout = pr.Spec.Timeouts.Pipeline.Duration
		}

		if pr.Spec.Timeouts.Finally != nil {
			timeout -= pr.Spec.Timeouts.Finally.Duration
		}
	default:
		defaultTimeout := time.Duration(config.FromContextOrDefaults(ctx).Defaults.DefaultTimeoutMinutes)
		timeout = defaultTimeout * time.Minute
	}

	// If the value of the timeout is 0 for any resource, there is no timeout.
	// It is impossible for pr.Spec.Timeout to be nil, since SetDefault always assigns it with a value.
	taskRunTimeout = taskRunTimeoutHelper(timeout, pr, taskRunTimeout, rprt)

	return taskRunTimeout
}

func taskRunTimeoutHelper(timeout time.Duration, pr *v1beta1.PipelineRun, taskRunTimeout *metav1.Duration, rprt *resources.ResolvedPipelineRunTask) *metav1.Duration {
	if timeout != apisconfig.NoTimeoutDuration {
		pTimeoutTime := pr.Status.StartTime.Add(timeout)
		if time.Now().After(pTimeoutTime) {

			taskRunTimeout = &metav1.Duration{Duration: time.Until(pTimeoutTime)}
			if taskRunTimeout.Duration < 0 {
				taskRunTimeout = &metav1.Duration{Duration: 1 * time.Second}
			}
		} else {

			if rprt.PipelineTask.Timeout != nil {
				taskRunTimeout = &metav1.Duration{Duration: rprt.PipelineTask.Timeout.Duration}
			} else {
				taskRunTimeout = &metav1.Duration{Duration: timeout}
			}
		}
	}

	if timeout == apisconfig.NoTimeoutDuration && rprt.PipelineTask.Timeout != nil {
		taskRunTimeout = &metav1.Duration{Duration: rprt.PipelineTask.Timeout.Duration}
	}
	return taskRunTimeout
}

func (c *Reconciler) updateLabelsAndAnnotations(ctx context.Context, pr *v1beta1.PipelineRun) (*v1beta1.PipelineRun, error) {
	newPr, err := c.pipelineRunLister.PipelineRuns(pr.Namespace).Get(pr.Name)
	if err != nil {
		return nil, fmt.Errorf("error getting PipelineRun %s when updating labels/annotations: %w", pr.Name, err)
	}
	if !reflect.DeepEqual(pr.ObjectMeta.Labels, newPr.ObjectMeta.Labels) || !reflect.DeepEqual(pr.ObjectMeta.Annotations, newPr.ObjectMeta.Annotations) {
		// Note that this uses Update vs. Patch because the former is significantly easier to test.
		// If we want to switch this to Patch, then we will need to teach the utilities in test/controller.go
		// to deal with Patch (setting resourceVersion, and optimistic concurrency checks).
		newPr = newPr.DeepCopy()
		newPr.Labels = pr.Labels
		newPr.Annotations = pr.Annotations
		return c.PipelineClientSet.TektonV1beta1().PipelineRuns(pr.Namespace).Update(ctx, newPr, metav1.UpdateOptions{})
	}
	return newPr, nil
}

func (c *Reconciler) makeConditionCheckContainer(ctx context.Context, rprt *resources.ResolvedPipelineRunTask, rcc *resources.ResolvedConditionCheck, pr *v1beta1.PipelineRun) (*v1beta1.ConditionCheck, error) {
	labels := getTaskrunLabels(pr, rprt.PipelineTask.Name, true)
	labels[pipeline.GroupName+pipeline.ConditionCheckKey] = rcc.ConditionCheckName
	labels[pipeline.GroupName+pipeline.ConditionNameKey] = rcc.Condition.Name

	for key, value := range rcc.Condition.ObjectMeta.Labels {
		labels[key] = value
	}

	// Propagate annotations from PipelineRun to TaskRun.
	annotations := getTaskrunAnnotations(pr)

	for key, value := range rcc.Condition.ObjectMeta.Annotations {
		annotations[key] = value
	}

	taskSpec, err := rcc.ConditionToTaskSpec()
	if err != nil {
		return nil, fmt.Errorf("failed to get TaskSpec from Condition: %w", err)
	}

	taskRunSpec := pr.GetTaskRunSpec(rprt.PipelineTask.Name)
	tr := &v1beta1.TaskRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:            rcc.ConditionCheckName,
			Namespace:       pr.Namespace,
			OwnerReferences: []metav1.OwnerReference{pr.GetOwnerReference()},
			Labels:          labels,
			Annotations:     annotations,
		},
		Spec: v1beta1.TaskRunSpec{
			TaskSpec:           taskSpec,
			ServiceAccountName: taskRunSpec.TaskServiceAccountName,
			Params:             rcc.PipelineTaskCondition.Params,
			Resources: &v1beta1.TaskRunResources{
				Inputs: rcc.ToTaskResourceBindings(),
			},
			Timeout:     getTaskRunTimeout(ctx, pr, rprt),
			PodTemplate: taskRunSpec.TaskPodTemplate,
		}}

	cctr, err := c.PipelineClientSet.TektonV1beta1().TaskRuns(pr.Namespace).Create(ctx, tr, metav1.CreateOptions{})
	cc := v1beta1.ConditionCheck(*cctr)
	return &cc, err
}

func (c *Reconciler) updatePipelineRunStatusFromInformer(ctx context.Context, pr *v1beta1.PipelineRun) error {
	logger := logging.FromContext(ctx)

	// Get the pipelineRun label that is set on each TaskRun.  Do not include the propagated labels from the
	// Pipeline and PipelineRun.  The user could change them during the lifetime of the PipelineRun so the
	// current labels may not be set on the previously created TaskRuns.
	pipelineRunLabels := getTaskrunLabels(pr, "", false)
	taskRuns, err := c.taskRunLister.TaskRuns(pr.Namespace).List(labels.SelectorFromSet(pipelineRunLabels))
	if err != nil {
		logger.Errorf("could not list TaskRuns %#v", err)
		return err
	}
	updatePipelineRunStatusFromTaskRuns(logger, pr, taskRuns)

	runs, err := c.runLister.Runs(pr.Namespace).List(labels.SelectorFromSet(pipelineRunLabels))
	if err != nil {
		logger.Errorf("could not list Runs %#v", err)
		return err
	}
	updatePipelineRunStatusFromRuns(logger, pr, runs)

	return nil
}

func updatePipelineRunStatusFromTaskRuns(logger *zap.SugaredLogger, pr *v1beta1.PipelineRun, trs []*v1beta1.TaskRun) {
	// If no TaskRun was found, nothing to be done. We never remove taskruns from the status
	if trs == nil || len(trs) == 0 {
		return
	}
	// Store a list of Condition TaskRuns for each PipelineTask (by name)
	conditionTaskRuns := make(map[string][]*v1beta1.TaskRun)
	// Map PipelineTask names to TaskRun names that were already in the status
	taskRunByPipelineTask := make(map[string]string)
	if pr.Status.TaskRuns != nil {
		for taskRunName, pipelineRunTaskRunStatus := range pr.Status.TaskRuns {
			taskRunByPipelineTask[pipelineRunTaskRunStatus.PipelineTaskName] = taskRunName
		}
	} else {
		pr.Status.TaskRuns = make(map[string]*v1beta1.PipelineRunTaskRunStatus)
	}
	// Loop over all the TaskRuns associated to Tasks
	for _, taskrun := range trs {
		// Only process TaskRuns that are owned by this PipelineRun.
		// This skips TaskRuns that are indirectly created by the PipelineRun (e.g. by custom tasks).
		if len(taskrun.OwnerReferences) < 1 || taskrun.OwnerReferences[0].UID != pr.ObjectMeta.UID {
			logger.Debugf("Found a TaskRun %s that is not owned by this PipelineRun", taskrun.Name)
			continue
		}
		lbls := taskrun.GetLabels()
		pipelineTaskName := lbls[pipeline.GroupName+pipeline.PipelineTaskLabelKey]
		if _, ok := lbls[pipeline.GroupName+pipeline.ConditionCheckKey]; ok {
			// Save condition for looping over them after this
			if _, ok := conditionTaskRuns[pipelineTaskName]; !ok {
				// If it's the first condition taskrun, initialise the slice
				conditionTaskRuns[pipelineTaskName] = []*v1beta1.TaskRun{}
			}
			conditionTaskRuns[pipelineTaskName] = append(conditionTaskRuns[pipelineTaskName], taskrun)
			continue
		}
		if _, ok := pr.Status.TaskRuns[taskrun.Name]; !ok {
			// This taskrun was missing from the status.
			// Add it without conditions, which are handled in the next loop
			logger.Infof("Found a TaskRun %s that was missing from the PipelineRun status", taskrun.Name)
			pr.Status.TaskRuns[taskrun.Name] = &v1beta1.PipelineRunTaskRunStatus{
				PipelineTaskName: pipelineTaskName,
				Status:           &taskrun.Status,
				ConditionChecks:  nil,
			}
			// Since this was recovered now, add it to the map, or it might be overwritten
			taskRunByPipelineTask[pipelineTaskName] = taskrun.Name
		}
	}
	// Then loop by pipelinetask name over all the TaskRuns associated to Conditions
	for pipelineTaskName, actualConditionTaskRuns := range conditionTaskRuns {
		taskRunName, ok := taskRunByPipelineTask[pipelineTaskName]
		if !ok {
			// The pipelineTask associated to the conditions was not found in the pipelinerun
			// status. This means that the conditions were orphaned, and never added to the
			// status. In this case we need to generate a new TaskRun name, that will be used
			// to run the TaskRun if the conditions are passed.
			taskRunName = resources.GetTaskRunName(pr.Status.TaskRuns, pipelineTaskName, pr.Name)
			pr.Status.TaskRuns[taskRunName] = &v1beta1.PipelineRunTaskRunStatus{
				PipelineTaskName: pipelineTaskName,
				Status:           nil,
				ConditionChecks:  nil,
			}
		}
		// Build the map of condition checks for the taskrun
		// If there were no other condition, initialise the map
		conditionChecks := pr.Status.TaskRuns[taskRunName].ConditionChecks
		if conditionChecks == nil {
			conditionChecks = make(map[string]*v1beta1.PipelineRunConditionCheckStatus)
		}
		for i, foundTaskRun := range actualConditionTaskRuns {
			lbls := foundTaskRun.GetLabels()
			if _, ok := conditionChecks[foundTaskRun.Name]; !ok {
				// The condition check was not found, so we need to add it
				// We only add the condition name, the status can now be gathered by the
				// normal reconcile process
				if conditionName, ok := lbls[pipeline.GroupName+pipeline.ConditionNameKey]; ok {
					conditionChecks[foundTaskRun.Name] = &v1beta1.PipelineRunConditionCheckStatus{
						ConditionName: fmt.Sprintf("%s-%s", conditionName, strconv.Itoa(i)),
					}
				} else {
					// The condition name label is missing, so we cannot recover this
					logger.Warnf("found an orphaned condition taskrun %#v with missing %s label",
						foundTaskRun, pipeline.ConditionNameKey)
				}
			}
		}
		pr.Status.TaskRuns[taskRunName].ConditionChecks = conditionChecks
	}
}

func updatePipelineRunStatusFromRuns(logger *zap.SugaredLogger, pr *v1beta1.PipelineRun, runs []*v1alpha1.Run) {
	// If no Run was found, nothing to be done. We never remove runs from the status
	if runs == nil || len(runs) == 0 {
		return
	}
	if pr.Status.Runs == nil {
		pr.Status.Runs = make(map[string]*v1beta1.PipelineRunRunStatus)
	}
	// Loop over all the Runs associated to Tasks
	for _, run := range runs {
		// Only process Runs that are owned by this PipelineRun.
		// This skips Runs that are indirectly created by the PipelineRun (e.g. by custom tasks).
		if len(run.OwnerReferences) < 1 && run.OwnerReferences[0].UID != pr.ObjectMeta.UID {
			logger.Debugf("Found a Run %s that is not owned by this PipelineRun", run.Name)
			continue
		}
		lbls := run.GetLabels()
		pipelineTaskName := lbls[pipeline.GroupName+pipeline.PipelineTaskLabelKey]
		if _, ok := pr.Status.Runs[run.Name]; !ok {
			// This run was missing from the status.
			pr.Status.Runs[run.Name] = &v1beta1.PipelineRunRunStatus{
				PipelineTaskName: pipelineTaskName,
				Status:           &run.Status,
			}
		}
	}
}
