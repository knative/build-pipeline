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

package v1beta1

import (
	"context"
	"fmt"
	"time"

	"github.com/tektoncd/pipeline/pkg/apis/config"
	apisconfig "github.com/tektoncd/pipeline/pkg/apis/config"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"knative.dev/pkg/apis"
	duckv1beta1 "knative.dev/pkg/apis/duck/v1beta1"
)

var (
	taskRunGroupVersionKind = schema.GroupVersionKind{
		Group:   SchemeGroupVersion.Group,
		Version: SchemeGroupVersion.Version,
		Kind:    pipeline.TaskRunControllerName,
	}
)

// TaskRunSpec defines the desired state of TaskRun
type TaskRunSpec struct {
	// +optional
	Params []Param `json:"params,omitempty"`
	// +optional
	Resources *TaskRunResources `json:"resources,omitempty"`
	// +optional
	ServiceAccountName string `json:"serviceAccountName"`
	// no more than one of the TaskRef and TaskSpec may be specified.
	// +optional
	TaskRef *TaskRef `json:"taskRef,omitempty"`
	// +optional
	TaskSpec *TaskSpec `json:"taskSpec,omitempty"`
	// Used for cancelling a taskrun (and maybe more later on)
	// +optional
	Status TaskRunSpecStatus `json:"status,omitempty"`
	// Time after which the build times out. Defaults to 1 hour.
	// Specified build timeout should be less than 24h.
	// Refer Go's ParseDuration documentation for expected format: https://golang.org/pkg/time/#ParseDuration
	// +optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`
	// PodTemplate holds pod specific configuration
	PodTemplate *PodTemplate `json:"podTemplate,omitempty"`
	// Workspaces is a list of WorkspaceBindings from volumes to workspaces.
	// +optional
	Workspaces []WorkspaceBinding `json:"workspaces,omitempty"`
}

// TaskRunSpecStatus defines the taskrun spec status the user can provide
type TaskRunSpecStatus string

const (
	// TaskRunSpecStatusCancelled indicates that the user wants to cancel the task,
	// if not already cancelled or terminated
	TaskRunSpecStatusCancelled = "TaskRunCancelled"
)

// TaskRunInputs holds the input values that this task was invoked with.
type TaskRunInputs struct {
	// +optional
	Resources []TaskResourceBinding `json:"resources,omitempty"`
	// +optional
	Params []Param `json:"params,omitempty"`
}

// TaskRunOutputs holds the output values that this task was invoked with.
type TaskRunOutputs struct {
	// +optional
	Resources []TaskResourceBinding `json:"resources,omitempty"`
}

var taskRunCondSet = apis.NewBatchConditionSet()

// TaskRunStatus defines the observed state of TaskRun
type TaskRunStatus struct {
	duckv1beta1.Status `json:",inline"`

	// TaskRunStatusFields inlines the status fields.
	TaskRunStatusFields `json:",inline"`
}

// TaskRunReason is an enum used to store all TaskRun reason for
// the Succeeded condition that are controlled by the TaskRun itself. Failure
// reasons that emerge from underlying resources are not included here
type TaskRunReason string

const (
	// TaskRunReasonStarted is the reason set when the TaskRun has just started
	TaskRunReasonStarted TaskRunReason = "Started"
	// TaskRunReasonRunning is the reason set when the TaskRun is running
	TaskRunReasonRunning TaskRunReason = "Running"
	// TaskRunReasonSuccessful is the reason set when the TaskRun completed successfully
	TaskRunReasonSuccessful TaskRunReason = "Succeeded"
	// TaskRunReasonFailed is the reason set when the TaskRun completed with a failure
	TaskRunReasonFailed TaskRunReason = "Failed"
	// TaskRunReasonCancelled is the reason set when the Taskrun is cancelled by the user
	TaskRunReasonCancelled TaskRunReason = "TaskRunCancelled"
	// TaskRunReasonTimedOut is the reason set when the Taskrun has timed out
	TaskRunReasonTimedOut TaskRunReason = "TaskRunTimeout"
)

func (t TaskRunReason) String() string {
	return string(t)
}

// GetStartedReason returns the reason set to the "Succeeded" condition when
// InitializeConditions is invoked
func (trs *TaskRunStatus) GetStartedReason() string {
	return TaskRunReasonStarted.String()
}

// GetRunningReason returns the reason set to the "Succeeded" condition when
// the RunsToCompletion starts running. This is used indicate that the resource
// could be validated is starting to perform its job.
func (trs *TaskRunStatus) GetRunningReason() string {
	return TaskRunReasonRunning.String()
}

// MarkResourceNotConvertible adds a Warning-severity condition to the resource noting
// that it cannot be converted to a higher version.
func (trs *TaskRunStatus) MarkResourceNotConvertible(err *CannotConvertError) {
	taskRunCondSet.Manage(trs).SetCondition(apis.Condition{
		Type:     ConditionTypeConvertible,
		Status:   corev1.ConditionFalse,
		Severity: apis.ConditionSeverityWarning,
		Reason:   err.Field,
		Message:  err.Message,
	})
}

// MarkResourceOngoing sets the ConditionSucceeded condition to ConditionUnknown
// with the reason and message.
func (trs *TaskRunStatus) MarkResourceOngoing(reason TaskRunReason, message string) {
	taskRunCondSet.Manage(trs).SetCondition(apis.Condition{
		Type:    apis.ConditionSucceeded,
		Status:  corev1.ConditionUnknown,
		Reason:  reason.String(),
		Message: message,
	})
}

// MarkResourceFailed sets the ConditionSucceeded condition to ConditionFalse
// based on an error that occurred and a reason
func (trs *TaskRunStatus) MarkResourceFailed(reason TaskRunReason, err error) {
	taskRunCondSet.Manage(trs).SetCondition(apis.Condition{
		Type:    apis.ConditionSucceeded,
		Status:  corev1.ConditionFalse,
		Reason:  reason.String(),
		Message: err.Error(),
	})
	succeeded := trs.GetCondition(apis.ConditionSucceeded)
	trs.CompletionTime = &succeeded.LastTransitionTime.Inner
}

// TaskRunStatusFields holds the fields of TaskRun's status.  This is defined
// separately and inlined so that other types can readily consume these fields
// via duck typing.
type TaskRunStatusFields struct {
	// PodName is the name of the pod responsible for executing this task's steps.
	PodName string `json:"podName"`

	// StartTime is the time the build is actually started.
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// CompletionTime is the time the build completed.
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`

	// RunAt is the time the taskrun status turns to running
	// +optional
	RunAt *metav1.Time `json:"runAt,omitempty"`

	// Steps describes the state of each build step container.
	// +optional
	Steps []StepState `json:"steps,omitempty"`

	// CloudEvents describe the state of each cloud event requested via a
	// CloudEventResource.
	// +optional
	CloudEvents []CloudEventDelivery `json:"cloudEvents,omitempty"`

	// RetriesStatus contains the history of TaskRunStatus in case of a retry in order to keep record of failures.
	// All TaskRunStatus stored in RetriesStatus will have no date within the RetriesStatus as is redundant.
	// +optional
	RetriesStatus []TaskRunStatus `json:"retriesStatus,omitempty"`

	// Results from Resources built during the taskRun. currently includes
	// the digest of build container images
	// +optional
	ResourcesResult []PipelineResourceResult `json:"resourcesResult,omitempty"`

	// TaskRunResults are the list of results written out by the task's containers
	// +optional
	TaskRunResults []TaskRunResult `json:"taskResults,omitempty"`

	// The list has one entry per sidecar in the manifest. Each entry is
	// represents the imageid of the corresponding sidecar.
	Sidecars []SidecarState `json:"sidecars,omitempty"`

	// TaskSpec contains the Spec from the dereferenced Task definition used to instantiate this TaskRun.
	TaskSpec *TaskSpec `json:"taskSpec,omitempty"`
}

// TaskRunResult used to describe the results of a task
type TaskRunResult struct {
	// Name the given name
	Name string `json:"name"`

	// Value the given value of the result
	Value string `json:"value"`
}

// GetOwnerReference gets the task run as owner reference for any related objects
func (tr *TaskRun) GetOwnerReference() metav1.OwnerReference {
	return *metav1.NewControllerRef(tr, taskRunGroupVersionKind)
}

// GetStatusCondition returns the task run status as a ConditionAccessor
func (tr *TaskRun) GetStatusCondition() apis.ConditionAccessor {
	return &tr.Status
}

// GetCondition returns the Condition matching the given type.
func (trs *TaskRunStatus) GetCondition(t apis.ConditionType) *apis.Condition {
	return taskRunCondSet.Manage(trs).GetCondition(t)
}

// InitializeConditions will set all conditions in taskRunCondSet to unknown for the TaskRun
// and set the started time to the current time
func (trs *TaskRunStatus) InitializeConditions() {
	started := false
	if trs.StartTime.IsZero() {
		trs.StartTime = &metav1.Time{Time: time.Now()}
		started = true
	}
	conditionManager := taskRunCondSet.Manage(trs)
	conditionManager.InitializeConditions()
	// Ensure the started reason is set for the "Succeeded" condition
	if started {
		initialCondition := conditionManager.GetCondition(apis.ConditionSucceeded)
		initialCondition.Reason = TaskRunReasonStarted.String()
		conditionManager.SetCondition(*initialCondition)
	}
}

// SetCondition sets the condition, unsetting previous conditions with the same
// type as necessary.
func (trs *TaskRunStatus) SetCondition(newCond *apis.Condition) {
	if newCond != nil {
		taskRunCondSet.Manage(trs).SetCondition(*newCond)
	}
}

// StepState reports the results of running a step in a Task.
type StepState struct {
	corev1.ContainerState `json:",inline"`
	Name                  string `json:"name,omitempty"`
	ContainerName         string `json:"container,omitempty"`
	ImageID               string `json:"imageID,omitempty"`
}

// SidecarState reports the results of running a sidecar in a Task.
type SidecarState struct {
	corev1.ContainerState `json:",inline"`
	Name                  string `json:"name,omitempty"`
	ContainerName         string `json:"container,omitempty"`
	ImageID               string `json:"imageID,omitempty"`
}

// CloudEventDelivery is the target of a cloud event along with the state of
// delivery.
type CloudEventDelivery struct {
	// Target points to an addressable
	Target string                  `json:"target,omitempty"`
	Status CloudEventDeliveryState `json:"status,omitempty"`
}

// CloudEventCondition is a string that represents the condition of the event.
type CloudEventCondition string

const (
	// CloudEventConditionUnknown means that the condition for the event to be
	// triggered was not met yet, or we don't know the state yet.
	CloudEventConditionUnknown CloudEventCondition = "Unknown"
	// CloudEventConditionSent means that the event was sent successfully
	CloudEventConditionSent CloudEventCondition = "Sent"
	// CloudEventConditionFailed means that there was one or more attempts to
	// send the event, and none was successful so far.
	CloudEventConditionFailed CloudEventCondition = "Failed"
)

// CloudEventDeliveryState reports the state of a cloud event to be sent.
type CloudEventDeliveryState struct {
	// Current status
	Condition CloudEventCondition `json:"condition,omitempty"`
	// SentAt is the time at which the last attempt to send the event was made
	// +optional
	SentAt *metav1.Time `json:"sentAt,omitempty"`
	// Error is the text of error (if any)
	Error string `json:"message"`
	// RetryCount is the number of attempts of sending the cloud event
	RetryCount int32 `json:"retryCount"`
}

// +genclient
// +genreconciler:krshapedlogic=false
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// TaskRun represents a single execution of a Task. TaskRuns are how the steps
// specified in a Task are executed; they specify the parameters and resources
// used to run the steps in a Task.
//
// +k8s:openapi-gen=true
type TaskRun struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +optional
	Spec TaskRunSpec `json:"spec,omitempty"`
	// +optional
	Status TaskRunStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// TaskRunList contains a list of TaskRun
type TaskRunList struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TaskRun `json:"items"`
}

// GetPipelineRunPVCName for taskrun gets pipelinerun
func (tr *TaskRun) GetPipelineRunPVCName() string {
	if tr == nil {
		return ""
	}
	for _, ref := range tr.GetOwnerReferences() {
		if ref.Kind == pipeline.PipelineRunControllerName {
			return fmt.Sprintf("%s-pvc", ref.Name)
		}
	}
	return ""
}

// HasPipelineRunOwnerReference returns true of TaskRun has
// owner reference of type PipelineRun
func (tr *TaskRun) HasPipelineRunOwnerReference() bool {
	for _, ref := range tr.GetOwnerReferences() {
		if ref.Kind == pipeline.PipelineRunControllerName {
			return true
		}
	}
	return false
}

// IsDone returns true if the TaskRun's status indicates that it is done.
func (tr *TaskRun) IsDone() bool {
	return !tr.Status.GetCondition(apis.ConditionSucceeded).IsUnknown()
}

// HasStarted function check whether taskrun has valid start time set in its status
func (tr *TaskRun) HasStarted() bool {
	return tr.Status.StartTime != nil && !tr.Status.StartTime.IsZero()
}

// IsSuccessful returns true if the TaskRun's status indicates that it is done.
func (tr *TaskRun) IsSuccessful() bool {
	return tr.Status.GetCondition(apis.ConditionSucceeded).IsTrue()
}

// IsCancelled returns true if the TaskRun's spec status is set to Cancelled state
func (tr *TaskRun) IsCancelled() bool {
	return tr.Spec.Status == TaskRunSpecStatusCancelled
}

// HasTimedOut returns true if the TaskRun runtime is beyond the allowed timeout
func (tr *TaskRun) HasTimedOut(ctx context.Context) bool {
	if tr.Status.StartTime.IsZero() {
		return false
	}
	timeout := tr.GetTimeout(ctx)
	// If timeout is set to 0 or defaulted to 0, there is no timeout.
	if timeout == apisconfig.NoTimeoutDuration {
		return false
	}
	runtime := time.Since(tr.Status.StartTime.Time)
	return runtime > timeout
}

func (tr *TaskRun) GetTimeout(ctx context.Context) time.Duration {
	// Use the platform default is no timeout is set
	if tr.Spec.Timeout == nil {
		defaultTimeout := time.Duration(config.FromContextOrDefaults(ctx).Defaults.DefaultTimeoutMinutes)
		return defaultTimeout * time.Minute
	}
	return tr.Spec.Timeout.Duration
}

// GetNamespacedName returns a k8s namespaced name that identifies this TaskRun
func (tr *TaskRun) GetNamespacedName() types.NamespacedName {
	return types.NamespacedName{Namespace: tr.Namespace, Name: tr.Name}
}

// IsPartOfPipeline return true if TaskRun is a part of a Pipeline.
// It also return the name of Pipeline and PipelineRun
func (tr *TaskRun) IsPartOfPipeline() (bool, string, string) {
	if tr == nil || len(tr.Labels) == 0 {
		return false, "", ""
	}

	if pl, ok := tr.Labels[pipeline.GroupName+pipeline.PipelineLabelKey]; ok {
		return true, pl, tr.Labels[pipeline.GroupName+pipeline.PipelineRunLabelKey]
	}

	return false, "", ""
}

// HasVolumeClaimTemplate returns true if TaskRun contains volumeClaimTemplates that is
// used for creating PersistentVolumeClaims with an OwnerReference for each run
func (tr *TaskRun) HasVolumeClaimTemplate() bool {
	for _, ws := range tr.Spec.Workspaces {
		if ws.VolumeClaimTemplate != nil {
			return true
		}
	}
	return false
}
