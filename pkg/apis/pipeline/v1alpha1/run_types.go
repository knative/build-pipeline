/*
Copyright 2020 The Tekton Authors

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

package v1alpha1

import (
	"fmt"
	"time"

	apisconfig "github.com/tektoncd/pipeline/pkg/apis/config"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	runv1alpha1 "github.com/tektoncd/pipeline/pkg/apis/run/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
)

var (
	runGroupVersionKind = schema.GroupVersionKind{
		Group:   SchemeGroupVersion.Group,
		Version: SchemeGroupVersion.Version,
		Kind:    pipeline.RunControllerName,
	}
)

// EmbeddedRunSpec allows custom task definitions to be embedded
type EmbeddedRunSpec struct {
	runtime.TypeMeta `json:",inline"`

	// +optional
	Metadata v1beta1.PipelineTaskMetadata `json:"metadata,omitempty"`

	// Spec is a specification of a custom task
	// +optional
	Spec runtime.RawExtension `json:"spec,omitempty"`
}

// RunSpec defines the desired state of Run
type RunSpec struct {
	// +optional
	Ref *TaskRef `json:"ref,omitempty"`

	// Spec is a specification of a custom task
	// +optional
	Spec *EmbeddedRunSpec `json:"spec,omitempty"`

	// +optional
	Params []v1beta1.Param `json:"params,omitempty"`

	// Used for cancelling a run (and maybe more later on)
	// +optional
	Status RunSpecStatus `json:"status,omitempty"`

	// +optional
	ServiceAccountName string `json:"serviceAccountName"`

	// PodTemplate holds pod specific configuration
	// +optional
	PodTemplate *PodTemplate `json:"podTemplate,omitempty"`

	// Time after which the custom-task times out.
	// Refer Go's ParseDuration documentation for expected format: https://golang.org/pkg/time/#ParseDuration
	// +optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`

	// Workspaces is a list of WorkspaceBindings from volumes to workspaces.
	// +optional
	Workspaces []v1beta1.WorkspaceBinding `json:"workspaces,omitempty"`

	// TODO(https://github.com/tektoncd/community/pull/128)
	// - timeout
	// - inline task spec
}

// RunSpecStatus defines the taskrun spec status the user can provide
type RunSpecStatus string

const (
	// RunSpecStatusCancelled indicates that the user wants to cancel the run,
	// if not already cancelled or terminated
	RunSpecStatusCancelled RunSpecStatus = "RunCancelled"
)

// TODO(jasonhall): Move this to a Params type so other code can use it?
func (rs RunSpec) GetParam(name string) *v1beta1.Param {
	for _, p := range rs.Params {
		if p.Name == name {
			return &p
		}
	}
	return nil
}

const (
	// RunReasonCancelled must be used in the Condition Reason to indicate that a Run was cancelled.
	RunReasonCancelled = "RunCancelled"
	// RunReasonWorkspaceNotSupported can be used in the Condition Reason to indicate that the
	// Run contains a workspace which is not supported by this custom task.
	RunReasonWorkspaceNotSupported = "RunWorkspaceNotSupported"
	// RunReasonPodTemplateNotSupported can be used in the Condition Reason to indicate that the
	// Run contains a pod template which is not supported by this custom task.
	RunReasonPodTemplateNotSupported = "RunPodTemplateNotSupported"
)

// RunStatus defines the observed state of Run.
type RunStatus = runv1alpha1.RunStatus

var runCondSet = apis.NewBatchConditionSet()

// GetConditionSet retrieves the condition set for this resource. Implements
// the KRShaped interface.
func (r *Run) GetConditionSet() apis.ConditionSet { return runCondSet }

// GetStatus retrieves the status of the Parallel. Implements the KRShaped
// interface.
func (r *Run) GetStatus() *duckv1.Status { return &r.Status.Status }

// RunStatusFields holds the fields of Run's status.  This is defined
// separately and inlined so that other types can readily consume these fields
// via duck typing.
type RunStatusFields = runv1alpha1.RunStatusFields

// RunResult used to describe the results of a task
type RunResult = runv1alpha1.RunResult

// +genclient
// +genreconciler
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Run represents a single execution of a Custom Task.
//
// +k8s:openapi-gen=true
type Run struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +optional
	Spec RunSpec `json:"spec,omitempty"`
	// +optional
	Status RunStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// RunList contains a list of Run
type RunList struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Run `json:"items"`
}

// GetOwnerReference gets the task run as owner reference for any related objects
func (r *Run) GetOwnerReference() metav1.OwnerReference {
	return *metav1.NewControllerRef(r, runGroupVersionKind)
}

// HasPipelineRunOwnerReference returns true of Run has
// owner reference of type PipelineRun
func (r *Run) HasPipelineRunOwnerReference() bool {
	for _, ref := range r.GetOwnerReferences() {
		if ref.Kind == pipeline.PipelineRunControllerName {
			return true
		}
	}
	return false
}

// IsCancelled returns true if the Run's spec status is set to Cancelled state
func (r *Run) IsCancelled() bool {
	return r.Spec.Status == RunSpecStatusCancelled
}

// IsDone returns true if the Run's status indicates that it is done.
func (r *Run) IsDone() bool {
	return !r.Status.GetCondition(apis.ConditionSucceeded).IsUnknown()
}

// HasStarted function check whether taskrun has valid start time set in its status
func (r *Run) HasStarted() bool {
	return r.Status.StartTime != nil && !r.Status.StartTime.IsZero()
}

// IsSuccessful returns true if the Run's status indicates that it is done.
func (r *Run) IsSuccessful() bool {
	return r.Status.GetCondition(apis.ConditionSucceeded).IsTrue()
}

// GetRunKey return the run's key for timeout handler map
func (r *Run) GetRunKey() string {
	// The address of the pointer is a threadsafe unique identifier for the run
	return fmt.Sprintf("%s/%p", "Run", r)
}

// HasTimedOut returns true if the Run's running time is beyond the allowed timeout
func (r *Run) HasTimedOut() bool {
	if r.Status.StartTime == nil || r.Status.StartTime.IsZero() {
		return false
	}
	timeout := r.GetTimeout()
	// If timeout is set to 0 or defaulted to 0, there is no timeout.
	if timeout == apisconfig.NoTimeoutDuration {
		return false
	}
	runtime := time.Since(r.Status.StartTime.Time)
	return runtime > timeout
}

func (r *Run) GetTimeout() time.Duration {
	// Use the platform default if no timeout is set
	if r.Spec.Timeout == nil {
		return apisconfig.DefaultTimeoutMinutes * time.Minute
	}
	return r.Spec.Timeout.Duration
}
