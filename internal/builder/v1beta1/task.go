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

package builder

import (
	"time"

	"github.com/tektoncd/pipeline/pkg/apis/config"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/pod"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	resource "github.com/tektoncd/pipeline/pkg/apis/resource/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/apis"
)

// TaskOp is an operation which modify a Task struct.
type TaskOp func(*v1beta1.Task)

// ClusterTaskOp is an operation which modify a ClusterTask struct.
type ClusterTaskOp func(*v1beta1.ClusterTask)

// TaskSpecOp is an operation which modify a TaskSpec struct.
type TaskSpecOp func(*v1beta1.TaskSpec)

// TaskResourcesOp is an operation which modify a TaskResources struct.
type TaskResourcesOp func(*v1beta1.TaskResources)

// TaskRunOp is an operation which modify a TaskRun struct.
type TaskRunOp func(*v1beta1.TaskRun)

// TaskRunSpecOp is an operation which modify a TaskRunSpec struct.
type TaskRunSpecOp func(*v1beta1.TaskRunSpec)

// TaskRunResourcesOp is an operation which modify a TaskRunResources struct.
type TaskRunResourcesOp func(*v1beta1.TaskRunResources)

// TaskResourceOp is an operation which modify a TaskResource struct.
type TaskResourceOp func(*v1beta1.TaskResource)

// TaskResourceBindingOp is an operation which modify a TaskResourceBinding struct.
type TaskResourceBindingOp func(*v1beta1.TaskResourceBinding)

// TaskRunStatusOp is an operation which modify a TaskRunStatus struct.
type TaskRunStatusOp func(*v1beta1.TaskRunStatus)

// TaskRefOp is an operation which modify a TaskRef struct.
type TaskRefOp func(*v1beta1.TaskRef)

// TaskResultOp is an operation which modifies there
type TaskResultOp func(result *v1beta1.TaskResult)

// StepStateOp is an operation which modifies a StepState struct.
type StepStateOp func(*v1beta1.StepState)

// SidecarStateOp is an operation which modifies a SidecarState struct.
type SidecarStateOp func(*v1beta1.SidecarState)

// VolumeOp is an operation which modify a Volume struct.
type VolumeOp func(*corev1.Volume)

var (
	trueB = true
)

// Task creates a Task with default values.
// Any number of Task modifier can be passed to transform it.
func Task(name string, ops ...TaskOp) *v1beta1.Task {
	t := &v1beta1.Task{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}

	for _, op := range ops {
		op(t)
	}

	return t
}

// TaskType sets the TypeMeta on the Task which is useful for making it serializable/deserializable.
func TaskType(t *v1beta1.Task) {
	t.TypeMeta = metav1.TypeMeta{
		APIVersion: "tekton.dev/v1beta1",
		Kind:       "Task",
	}
}

// ClusterTask creates a ClusterTask with default values.
// Any number of ClusterTask modifier can be passed to transform it.
func ClusterTask(name string, ops ...ClusterTaskOp) *v1beta1.ClusterTask {
	t := &v1beta1.ClusterTask{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}

	for _, op := range ops {
		op(t)
	}

	return t
}

// TaskNamespace sets the namespace to the Task.
func TaskNamespace(namespace string) TaskOp {
	return func(t *v1beta1.Task) {
		t.ObjectMeta.Namespace = namespace
	}
}

// ClusterTaskType sets the TypeMeta on the ClusterTask which is useful for making it serializable/deserializable.
func ClusterTaskType(t *v1beta1.ClusterTask) {
	t.TypeMeta = metav1.TypeMeta{
		APIVersion: "tekton.dev/v1beta1",
		Kind:       "ClusterTask",
	}
}

// ClusterTaskSpec sets the specified spec of the cluster task.
// Any number of TaskSpec modifier can be passed to create it.
func ClusterTaskSpec(ops ...TaskSpecOp) ClusterTaskOp {
	return func(t *v1beta1.ClusterTask) {
		spec := &t.Spec
		for _, op := range ops {
			op(spec)
		}
		t.Spec = *spec
	}
}

// TaskSpec sets the specified spec of the task.
// Any number of TaskSpec modifier can be passed to create/modify it.
func TaskSpec(ops ...TaskSpecOp) TaskOp {
	return func(t *v1beta1.Task) {
		spec := &t.Spec
		for _, op := range ops {
			op(spec)
		}
		t.Spec = *spec
	}
}

// TaskDescription sets the description of the task
func TaskDescription(desc string) TaskSpecOp {
	return func(spec *v1beta1.TaskSpec) {
		spec.Description = desc
	}
}

// Step adds a step with the specified name and image to the TaskSpec.
// Any number of Container modifier can be passed to transform it.
func Step(image string, ops ...StepOp) TaskSpecOp {
	return func(spec *v1beta1.TaskSpec) {
		if spec.Steps == nil {
			spec.Steps = []v1beta1.Step{}
		}
		step := v1beta1.Step{Container: corev1.Container{
			Image: image,
		}}
		for _, op := range ops {
			op(&step)
		}
		spec.Steps = append(spec.Steps, step)
	}
}

// Sidecar adds a sidecar container with the specified name and image to the TaskSpec.
// Any number of Container modifier can be passed to transform it.
func Sidecar(name, image string, ops ...ContainerOp) TaskSpecOp {
	return func(spec *v1beta1.TaskSpec) {
		c := corev1.Container{
			Name:  name,
			Image: image,
		}
		for _, op := range ops {
			op(&c)
		}
		spec.Sidecars = append(spec.Sidecars, v1beta1.Sidecar{Container: c})
	}
}

// TaskWorkspace adds a workspace declaration.
func TaskWorkspace(name, desc, mountPath string, readOnly bool) TaskSpecOp {
	return func(spec *v1beta1.TaskSpec) {
		spec.Workspaces = append(spec.Workspaces, v1beta1.WorkspaceDeclaration{
			Name:        name,
			Description: desc,
			MountPath:   mountPath,
			ReadOnly:    readOnly,
		})
	}
}

// TaskStepTemplate adds a base container for all steps in the task.
func TaskStepTemplate(ops ...ContainerOp) TaskSpecOp {
	return func(spec *v1beta1.TaskSpec) {
		base := &corev1.Container{}
		for _, op := range ops {
			op(base)
		}
		spec.StepTemplate = base
	}
}

// TaskVolume adds a volume with specified name to the TaskSpec.
// Any number of Volume modifier can be passed to transform it.
func TaskVolume(name string, ops ...VolumeOp) TaskSpecOp {
	return func(spec *v1beta1.TaskSpec) {
		v := &corev1.Volume{Name: name}
		for _, op := range ops {
			op(v)
		}
		spec.Volumes = append(spec.Volumes, *v)
	}
}

// VolumeSource sets the VolumeSource to the Volume.
func VolumeSource(s corev1.VolumeSource) VolumeOp {
	return func(v *corev1.Volume) {
		v.VolumeSource = s
	}
}

// TaskParam sets the Params to the TaskSpec
func TaskParam(name string, pt v1beta1.ParamType, ops ...ParamSpecOp) TaskSpecOp {
	return func(spec *v1beta1.TaskSpec) {
		ps := &v1beta1.ParamSpec{Name: name, Type: pt}
		for _, op := range ops {
			op(ps)
		}
		spec.Params = append(spec.Params, *ps)
	}
}

// TaskResources sets the Resources to the TaskSpec
func TaskResources(ops ...TaskResourcesOp) TaskSpecOp {
	return func(spec *v1beta1.TaskSpec) {
		r := &v1beta1.TaskResources{}
		for _, op := range ops {
			op(r)
		}
		spec.Resources = r
	}
}

// TaskResults sets the Results to the TaskSpec
func TaskResults(name, desc string) TaskSpecOp {
	return func(spec *v1beta1.TaskSpec) {
		r := &v1beta1.TaskResult{
			Name:        name,
			Description: desc,
		}
		spec.Results = append(spec.Results, *r)
	}
}

// TaskResourcesInput adds a TaskResource as Inputs to the TaskResources
func TaskResourcesInput(name string, resourceType resource.PipelineResourceType, ops ...TaskResourceOp) TaskResourcesOp {
	return func(r *v1beta1.TaskResources) {
		i := &v1beta1.TaskResource{
			ResourceDeclaration: v1beta1.ResourceDeclaration{
				Name: name,
				Type: resourceType,
			},
		}
		for _, op := range ops {
			op(i)
		}
		r.Inputs = append(r.Inputs, *i)
	}
}

// TaskResourcesOutput adds a TaskResource as Outputs to the TaskResources
func TaskResourcesOutput(name string, resourceType resource.PipelineResourceType, ops ...TaskResourceOp) TaskResourcesOp {
	return func(r *v1beta1.TaskResources) {
		o := &v1beta1.TaskResource{
			ResourceDeclaration: v1beta1.ResourceDeclaration{
				Name: name,
				Type: resourceType,
			},
		}
		for _, op := range ops {
			op(o)
		}
		r.Outputs = append(r.Outputs, *o)
	}
}

// ResourceOptional marks a TaskResource as optional.
func ResourceOptional(optional bool) TaskResourceOp {
	return func(r *v1beta1.TaskResource) {
		r.Optional = optional
	}
}

// ResourceTargetPath sets the target path to a TaskResource.
func ResourceTargetPath(path string) TaskResourceOp {
	return func(r *v1beta1.TaskResource) {
		r.TargetPath = path
	}
}

// TaskRun creates a TaskRun with default values.
// Any number of TaskRun modifier can be passed to transform it.
func TaskRun(name string, ops ...TaskRunOp) *v1beta1.TaskRun {
	tr := &v1beta1.TaskRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Annotations: map[string]string{},
		},
	}

	for _, op := range ops {
		op(tr)
	}

	return tr
}

// TaskRunNamespace sets the namespace for the TaskRun.
func TaskRunNamespace(namespace string) TaskRunOp {
	return func(t *v1beta1.TaskRun) {
		t.ObjectMeta.Namespace = namespace
	}
}

// TaskRunStatus sets the TaskRunStatus to tshe TaskRun
func TaskRunStatus(ops ...TaskRunStatusOp) TaskRunOp {
	return func(tr *v1beta1.TaskRun) {
		status := &tr.Status
		for _, op := range ops {
			op(status)
		}
		tr.Status = *status
	}
}

// PodName sets the Pod name to the TaskRunStatus.
func PodName(name string) TaskRunStatusOp {
	return func(s *v1beta1.TaskRunStatus) {
		s.PodName = name
	}
}

// StatusCondition adds a StatusCondition to the TaskRunStatus.
func StatusCondition(condition apis.Condition) TaskRunStatusOp {
	return func(s *v1beta1.TaskRunStatus) {
		s.Conditions = append(s.Conditions, condition)
	}
}

// TaskRunResult adds a result with the specified name and value to the TaskRunStatus.
func TaskRunResult(name, value string) TaskRunStatusOp {
	return func(s *v1beta1.TaskRunStatus) {
		s.TaskRunResults = append(s.TaskRunResults, v1beta1.TaskRunResult{
			Name:  name,
			Value: value,
		})
	}
}

// Retry adds a RetriesStatus (TaskRunStatus) to the TaskRunStatus.
func Retry(retry v1beta1.TaskRunStatus) TaskRunStatusOp {
	return func(s *v1beta1.TaskRunStatus) {
		s.RetriesStatus = append(s.RetriesStatus, retry)
	}
}

// StepState adds a StepState to the TaskRunStatus.
func StepState(ops ...StepStateOp) TaskRunStatusOp {
	return func(s *v1beta1.TaskRunStatus) {
		state := &v1beta1.StepState{}
		for _, op := range ops {
			op(state)
		}
		s.Steps = append(s.Steps, *state)
	}
}

// SidecarState adds a SidecarState to the TaskRunStatus.
func SidecarState(ops ...SidecarStateOp) TaskRunStatusOp {
	return func(s *v1beta1.TaskRunStatus) {
		state := &v1beta1.SidecarState{}
		for _, op := range ops {
			op(state)
		}
		s.Sidecars = append(s.Sidecars, *state)
	}
}

// TaskRunStartTime sets the start time to the TaskRunStatus.
func TaskRunStartTime(startTime time.Time) TaskRunStatusOp {
	return func(s *v1beta1.TaskRunStatus) {
		s.StartTime = &metav1.Time{Time: startTime}
	}
}

// TaskRunCompletionTime sets the completion time to the TaskRunStatus.
func TaskRunCompletionTime(completionTime time.Time) TaskRunStatusOp {
	return func(s *v1beta1.TaskRunStatus) {
		s.CompletionTime = &metav1.Time{Time: completionTime}
	}
}

// TaskRunRunAtTime sets the runAt time to the TaskRunStatus.
func TaskRunRunAtTime(runAt time.Time) TaskRunStatusOp {
	return func(s *v1beta1.TaskRunStatus) {
		s.RunAt = &metav1.Time{Time: runAt}
	}
}

// TaskRunCloudEvent adds an event to the TaskRunStatus.
func TaskRunCloudEvent(target, error string, retryCount int32, condition v1beta1.CloudEventCondition) TaskRunStatusOp {
	return func(s *v1beta1.TaskRunStatus) {
		if len(s.CloudEvents) == 0 {
			s.CloudEvents = make([]v1beta1.CloudEventDelivery, 0)
		}
		cloudEvent := v1beta1.CloudEventDelivery{
			Target: target,
			Status: v1beta1.CloudEventDeliveryState{
				Condition:  condition,
				RetryCount: retryCount,
				Error:      error,
			},
		}
		s.CloudEvents = append(s.CloudEvents, cloudEvent)
	}
}

// TaskRunTimeout sets the timeout duration to the TaskRunSpec.
func TaskRunTimeout(d time.Duration) TaskRunSpecOp {
	return func(spec *v1beta1.TaskRunSpec) {
		spec.Timeout = &metav1.Duration{Duration: d}
	}
}

// TaskRunNilTimeout sets the timeout duration to nil on the TaskRunSpec.
func TaskRunNilTimeout(spec *v1beta1.TaskRunSpec) {
	spec.Timeout = nil
}

// TaskRunNodeSelector sets the NodeSelector to the TaskRunSpec.
func TaskRunNodeSelector(values map[string]string) TaskRunSpecOp {
	return func(spec *v1beta1.TaskRunSpec) {
		if spec.PodTemplate == nil {
			spec.PodTemplate = &pod.Template{}
		}
		spec.PodTemplate.NodeSelector = values
	}
}

// StateTerminated sets Terminated to the StepState.
func StateTerminated(exitcode int) StepStateOp {
	return func(s *v1beta1.StepState) {
		s.ContainerState = corev1.ContainerState{
			Terminated: &corev1.ContainerStateTerminated{ExitCode: int32(exitcode)},
		}
	}
}

// SetStepStateTerminated sets Terminated state of a step.
func SetStepStateTerminated(terminated corev1.ContainerStateTerminated) StepStateOp {
	return func(s *v1beta1.StepState) {
		s.ContainerState = corev1.ContainerState{
			Terminated: &terminated,
		}
	}
}

// SetStepStateRunning sets Running state of a step.
func SetStepStateRunning(running corev1.ContainerStateRunning) StepStateOp {
	return func(s *v1beta1.StepState) {
		s.ContainerState = corev1.ContainerState{
			Running: &running,
		}
	}
}

// SetStepStateWaiting sets Waiting state of a step.
func SetStepStateWaiting(waiting corev1.ContainerStateWaiting) StepStateOp {
	return func(s *v1beta1.StepState) {
		s.ContainerState = corev1.ContainerState{
			Waiting: &waiting,
		}
	}
}

// TaskRunOwnerReference sets the OwnerReference, with specified kind and name, to the TaskRun.
func TaskRunOwnerReference(kind, name string, ops ...OwnerReferenceOp) TaskRunOp {
	return func(tr *v1beta1.TaskRun) {
		o := &metav1.OwnerReference{
			Kind: kind,
			Name: name,
		}
		for _, op := range ops {
			op(o)
		}
		tr.ObjectMeta.OwnerReferences = append(tr.ObjectMeta.OwnerReferences, *o)
	}
}

// TaskRunLabels add the specified labels to the TaskRun.
func TaskRunLabels(labels map[string]string) TaskRunOp {
	return func(tr *v1beta1.TaskRun) {
		if tr.ObjectMeta.Labels == nil {
			tr.ObjectMeta.Labels = map[string]string{}
		}
		for key, value := range labels {
			tr.ObjectMeta.Labels[key] = value
		}
	}
}

// TaskRunLabel adds a label with the specified key and value to the TaskRun.
func TaskRunLabel(key, value string) TaskRunOp {
	return func(tr *v1beta1.TaskRun) {
		if tr.ObjectMeta.Labels == nil {
			tr.ObjectMeta.Labels = map[string]string{}
		}
		tr.ObjectMeta.Labels[key] = value
	}
}

// TaskRunAnnotations adds the specified annotations to the TaskRun.
func TaskRunAnnotations(annotations map[string]string) TaskRunOp {
	return func(tr *v1beta1.TaskRun) {
		if tr.ObjectMeta.Annotations == nil {
			tr.ObjectMeta.Annotations = map[string]string{}
		}
		for key, value := range annotations {
			tr.ObjectMeta.Annotations[key] = value
		}
	}
}

// TaskRunAnnotation adds an annotation with the specified key and value to the TaskRun.
func TaskRunAnnotation(key, value string) TaskRunOp {
	return func(tr *v1beta1.TaskRun) {
		if tr.ObjectMeta.Annotations == nil {
			tr.ObjectMeta.Annotations = map[string]string{}
		}
		tr.ObjectMeta.Annotations[key] = value
	}
}

// TaskRunSelfLink adds a SelfLink
func TaskRunSelfLink(selflink string) TaskRunOp {
	return func(tr *v1beta1.TaskRun) {
		tr.ObjectMeta.SelfLink = selflink
	}
}

// TaskRunSpec sets the specified spec of the TaskRun.
// Any number of TaskRunSpec modifier can be passed to transform it.
func TaskRunSpec(ops ...TaskRunSpecOp) TaskRunOp {
	return func(tr *v1beta1.TaskRun) {
		spec := &tr.Spec
		spec.Resources = &v1beta1.TaskRunResources{}
		// Set a default timeout
		spec.Timeout = &metav1.Duration{Duration: config.DefaultTimeoutMinutes * time.Minute}
		for _, op := range ops {
			op(spec)
		}
		tr.Spec = *spec
	}
}

// TaskRunCancelled sets the status to cancel to the TaskRunSpec.
func TaskRunCancelled(spec *v1beta1.TaskRunSpec) {
	spec.Status = v1beta1.TaskRunSpecStatusCancelled
}

// TaskRunTaskRef sets the specified Task reference to the TaskRunSpec.
// Any number of TaskRef modifier can be passed to transform it.
func TaskRunTaskRef(name string, ops ...TaskRefOp) TaskRunSpecOp {
	return func(spec *v1beta1.TaskRunSpec) {
		ref := &v1beta1.TaskRef{Name: name}
		for _, op := range ops {
			op(ref)
		}
		spec.TaskRef = ref
	}
}

// TaskRunSpecStatus sets the Status in the Spec, used for operations
// such as cancelling executing TaskRuns.
func TaskRunSpecStatus(status v1beta1.TaskRunSpecStatus) TaskRunSpecOp {
	return func(spec *v1beta1.TaskRunSpec) {
		spec.Status = status
	}
}

// TaskRefKind set the specified kind to the TaskRef.
func TaskRefKind(kind v1beta1.TaskKind) TaskRefOp {
	return func(ref *v1beta1.TaskRef) {
		ref.Kind = kind
	}
}

// TaskRefAPIVersion sets the specified api version to the TaskRef.
func TaskRefAPIVersion(version string) TaskRefOp {
	return func(ref *v1beta1.TaskRef) {
		ref.APIVersion = version
	}
}

// TaskRefBundle sets the specified ref to the TaskRef's bundle.
func TaskRefBundle(url string) TaskRefOp {
	return func(ref *v1beta1.TaskRef) {
		ref.Bundle = url
	}
}

// TaskRunTaskSpec sets the specified TaskRunSpec reference to the TaskRunSpec.
// Any number of TaskRunSpec modifier can be passed to transform it.
func TaskRunTaskSpec(ops ...TaskSpecOp) TaskRunSpecOp {
	return func(spec *v1beta1.TaskRunSpec) {
		taskSpec := &v1beta1.TaskSpec{}
		for _, op := range ops {
			op(taskSpec)
		}
		spec.TaskSpec = taskSpec
	}
}

// TaskRunServiceAccountName sets the serviceAccount to the TaskRunSpec.
func TaskRunServiceAccountName(sa string) TaskRunSpecOp {
	return func(trs *v1beta1.TaskRunSpec) {
		trs.ServiceAccountName = sa
	}
}

// TaskRunParam sets the Params to the TaskSpec
func TaskRunParam(name, value string, additionalValues ...string) TaskRunSpecOp {
	return func(spec *v1beta1.TaskRunSpec) {
		spec.Params = append(spec.Params, v1beta1.Param{
			Name:  name,
			Value: *v1beta1.NewArrayOrString(value, additionalValues...),
		})
	}
}

// TaskRunResources sets the TaskRunResources to the TaskRunSpec
func TaskRunResources(ops ...TaskRunResourcesOp) TaskRunSpecOp {
	return func(spec *v1beta1.TaskRunSpec) {
		r := &v1beta1.TaskRunResources{}
		for _, op := range ops {
			op(r)
		}
		spec.Resources = r
	}
}

// TaskRunResourcesInput adds a TaskRunResource as Inputs to the TaskRunResources
func TaskRunResourcesInput(name string, ops ...TaskResourceBindingOp) TaskRunResourcesOp {
	return func(r *v1beta1.TaskRunResources) {
		binding := &v1beta1.TaskResourceBinding{
			PipelineResourceBinding: v1beta1.PipelineResourceBinding{
				Name: name,
			},
		}
		for _, op := range ops {
			op(binding)
		}
		r.Inputs = append(r.Inputs, *binding)
	}
}

// TaskRunResourcesOutput adds a TaskRunResource as Outputs to the TaskRunResources
func TaskRunResourcesOutput(name string, ops ...TaskResourceBindingOp) TaskRunResourcesOp {
	return func(r *v1beta1.TaskRunResources) {
		binding := &v1beta1.TaskResourceBinding{
			PipelineResourceBinding: v1beta1.PipelineResourceBinding{
				Name: name,
			},
		}
		for _, op := range ops {
			op(binding)
		}
		r.Outputs = append(r.Outputs, *binding)
	}
}

// TaskResourceBindingRef set the PipelineResourceRef name to the TaskResourceBinding.
func TaskResourceBindingRef(name string) TaskResourceBindingOp {
	return func(b *v1beta1.TaskResourceBinding) {
		b.ResourceRef = &v1beta1.PipelineResourceRef{
			Name: name,
		}
	}
}

// TaskResourceBindingResourceSpec set the PipelineResourceResourceSpec to the TaskResourceBinding.
func TaskResourceBindingResourceSpec(spec *resource.PipelineResourceSpec) TaskResourceBindingOp {
	return func(b *v1beta1.TaskResourceBinding) {
		b.ResourceSpec = spec
	}
}

// TaskResourceBindingRefAPIVersion set the PipelineResourceRef APIVersion to the TaskResourceBinding.
func TaskResourceBindingRefAPIVersion(version string) TaskResourceBindingOp {
	return func(b *v1beta1.TaskResourceBinding) {
		b.ResourceRef.APIVersion = version
	}
}

// TaskResourceBindingPaths add any number of path to the TaskResourceBinding.
func TaskResourceBindingPaths(paths ...string) TaskResourceBindingOp {
	return func(b *v1beta1.TaskResourceBinding) {
		b.Paths = paths
	}
}

// TaskRunPodTemplate add a custom PodTemplate to the TaskRun
func TaskRunPodTemplate(podTemplate *pod.Template) TaskRunSpecOp {
	return func(spec *v1beta1.TaskRunSpec) {
		spec.PodTemplate = podTemplate
	}
}

// TaskRunWorkspaceEmptyDir adds a workspace binding to an empty dir volume source.
func TaskRunWorkspaceEmptyDir(name, subPath string) TaskRunSpecOp {
	return func(spec *v1beta1.TaskRunSpec) {
		spec.Workspaces = append(spec.Workspaces, v1beta1.WorkspaceBinding{
			Name:     name,
			SubPath:  subPath,
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		})
	}
}

// TaskRunWorkspacePVC adds a workspace binding to a PVC volume source.
func TaskRunWorkspacePVC(name, subPath, claimName string) TaskRunSpecOp {
	return func(spec *v1beta1.TaskRunSpec) {
		spec.Workspaces = append(spec.Workspaces, v1beta1.WorkspaceBinding{
			Name:    name,
			SubPath: subPath,
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: claimName,
			},
		})
	}
}

// TaskRunWorkspaceVolumeClaimTemplate adds a workspace binding with a VolumeClaimTemplate volume source.
func TaskRunWorkspaceVolumeClaimTemplate(name, subPath string, volumeClaimTemplate *corev1.PersistentVolumeClaim) TaskRunSpecOp {
	return func(spec *v1beta1.TaskRunSpec) {
		spec.Workspaces = append(spec.Workspaces, v1beta1.WorkspaceBinding{
			Name:                name,
			SubPath:             subPath,
			VolumeClaimTemplate: volumeClaimTemplate,
		})
	}
}
