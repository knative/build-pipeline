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
	"encoding/json"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
)

// This package exists to avoid an import cycle between v1alpha1 and v1beta1.
// It contains common definitions needed by v1alpha1.Run and v1beta1.PipelineRun.

// +k8s:deepcopy-gen=true

// RunStatus defines the observed state of Run
type RunStatus struct {
	duckv1.Status `json:",inline"`

	// RunStatusFields inlines the status fields.
	RunStatusFields `json:",inline"`
}

// +k8s:deepcopy-gen=true

// RunStatusFields holds the fields of Run's status.  This is defined
// separately and inlined so that other types can readily consume these fields
// via duck typing.
type RunStatusFields struct {
	// StartTime is the time the build is actually started.
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// CompletionTime is the time the build completed.
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`

	// Results reports any output result values to be consumed by later
	// tasks in a pipeline.
	// +optional
	Results []RunResult `json:"results,omitempty"`

	// RetriesStatus contains the history of RunStatus, in case of a retry.
	// +optional
	RetriesStatus []RunStatus `json:"retriesStatus,omitempty"`

	// ExtraFields holds arbitrary fields provided by the custom task
	// controller.
	ExtraFields runtime.RawExtension `json:"extraFields,omitempty"`
}

// RunResult used to describe the results of a task
type RunResult struct {
	// Name the given name
	Name string `json:"name"`
	// Value the given value of the result
	Value string `json:"value"`
}

var runCondSet = apis.NewBatchConditionSet()

// GetCondition returns the Condition matching the given type.
func (r *RunStatus) GetCondition(t apis.ConditionType) *apis.Condition {
	return runCondSet.Manage(r).GetCondition(t)
}

// InitializeConditions will set all conditions in runCondSet to unknown for the PipelineRun
// and set the started time to the current time
func (r *RunStatus) InitializeConditions() {
	started := false
	if r.StartTime.IsZero() {
		r.StartTime = &metav1.Time{Time: time.Now()}
		started = true
	}
	conditionManager := runCondSet.Manage(r)
	conditionManager.InitializeConditions()
	// Ensure the started reason is set for the "Succeeded" condition
	if started {
		initialCondition := conditionManager.GetCondition(apis.ConditionSucceeded)
		initialCondition.Reason = "Started"
		conditionManager.SetCondition(*initialCondition)
	}
}

// SetCondition sets the condition, unsetting previous conditions with the same
// type as necessary.
func (r *RunStatus) SetCondition(newCond *apis.Condition) {
	if newCond != nil {
		runCondSet.Manage(r).SetCondition(*newCond)
	}
}

// MarkRunSucceeded changes the Succeeded condition to True with the provided reason and message.
func (r *RunStatus) MarkRunSucceeded(reason, messageFormat string, messageA ...interface{}) {
	runCondSet.Manage(r).MarkTrueWithReason(apis.ConditionSucceeded, reason, messageFormat, messageA...)
	succeeded := r.GetCondition(apis.ConditionSucceeded)
	r.CompletionTime = &succeeded.LastTransitionTime.Inner
}

// MarkRunFailed changes the Succeeded condition to False with the provided reason and message.
func (r *RunStatus) MarkRunFailed(reason, messageFormat string, messageA ...interface{}) {
	runCondSet.Manage(r).MarkFalse(apis.ConditionSucceeded, reason, messageFormat, messageA...)
	succeeded := r.GetCondition(apis.ConditionSucceeded)
	r.CompletionTime = &succeeded.LastTransitionTime.Inner
}

// MarkRunRunning changes the Succeeded condition to Unknown with the provided reason and message.
func (r *RunStatus) MarkRunRunning(reason, messageFormat string, messageA ...interface{}) {
	runCondSet.Manage(r).MarkUnknown(apis.ConditionSucceeded, reason, messageFormat, messageA...)
}

// DecodeExtraFields deserializes the extra fields in the Run status.
func (r *RunStatus) DecodeExtraFields(into interface{}) error {
	if len(r.ExtraFields.Raw) == 0 {
		return nil
	}
	return json.Unmarshal(r.ExtraFields.Raw, into)
}

// EncodeExtraFields serializes the extra fields in the Run status.
func (r *RunStatus) EncodeExtraFields(from interface{}) error {
	data, err := json.Marshal(from)
	if err != nil {
		return err
	}
	r.ExtraFields = runtime.RawExtension{
		Raw: data,
	}
	return nil
}
