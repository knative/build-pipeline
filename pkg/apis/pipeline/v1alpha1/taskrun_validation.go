/*
Copyright 2018 The Knative Authors.

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
	"strings"

	"github.com/knative/pkg/apis"
	"k8s.io/apimachinery/pkg/api/equality"
)

func (t *TaskRun) Validate() *apis.FieldError {
	if err := validateObjectMetadata(t.GetObjectMeta()).ViaField("metadata"); err != nil {
		return err
	}
	return t.Spec.Validate()
}

func (ts *TaskRunSpec) Validate() *apis.FieldError {
	if equality.Semantic.DeepEqual(ts, &TaskRunSpec{}) {
		return apis.ErrMissingField("spec")
	}

	// Check for TaskRef
	if ts.TaskRef == "" {
		return apis.ErrMissingField("spec.taskref")
	}

	// Check for Trigger
	if err := ts.Trigger.TriggerRef.Validate("spec.trigger.triggerref"); err != nil {
		return err
	}

	// check for input resources
	if err := ts.Inputs.Validate("spec.Inputs"); err != nil {
		return err
	}

	// check for outputs
	if err := ts.Outputs.Validate("spec.Outputs"); err != nil {
		return err
	}

	// check for results
	return ts.Results.Validate("spec.results")
}

func (r *Results) Validate(path string) *apis.FieldError {
	if err := r.Logs.Validate(fmt.Sprintf("%s.logs", path)); err != nil {
		return err
	}
	if err := r.Runs.Validate(fmt.Sprintf("%s.runs", path)); err != nil {
		return err
	}
	if r.Tests != nil {
		return r.Tests.Validate(fmt.Sprintf("%s.tests", path))
	}
	return nil
}

func (i TaskRunInputs) Validate(path string) *apis.FieldError {
	if err := checkForPipelineResourceDuplicates(i.Resources, fmt.Sprintf("%s.Resources.Name", path)); err != nil {
		return err
	}
	return validateParameters(i.Params)
}

func (o Outputs) Validate(path string) *apis.FieldError {
	for _, source := range o.Resources {
		if err := validateResourceType(source, fmt.Sprintf("%s.Resources.%s.Type", path, source.Name)); err != nil {
			return err
		}
		if err := checkForDuplicates(o.Resources, fmt.Sprintf("%s.Resources.%s.Name", path, source.Name)); err != nil {
			return err
		}
	}
	return nil
}

func (r ResultTarget) Validate(path string) *apis.FieldError {
	// if result target is not set then do not error
	var emptyTarget = ResultTarget{}
	if r == emptyTarget {
		return nil
	}

	// If set then verify all variables pass the validation
	if r.Name == "" {
		return apis.ErrMissingField(fmt.Sprintf("%s.name", path))
	}

	if r.Type != ResultTargetTypeGCS {
		return apis.ErrInvalidValue(string(r.Type), fmt.Sprintf("%s.Type", path))
	}

	if r.URL == "" {
		return apis.ErrMissingField(fmt.Sprintf("%s.URL", path))
	}
	return nil
}

func checkForPipelineResourceDuplicates(resources []PipelineResourceVersion, path string) *apis.FieldError {
	encountered := map[string]struct{}{}
	for _, r := range resources {
		// Check the unique combination of resource+version. Covers the use case of inputs with same resource name
		// and different versions
		key := fmt.Sprintf("%s%s", r.ResourceRef.Name, r.Version)
		if _, ok := encountered[strings.ToLower(key)]; ok {
			return apis.ErrMultipleOneOf(path)
		}
		encountered[key] = struct{}{}
	}
	return nil
}

func (r TaskTriggerRef) Validate(path string) *apis.FieldError {
	if r.Type == "" {
		return nil
	}

	taskType := strings.ToLower(string(r.Type))
	for _, allowed := range []TaskTriggerType{TaskTriggerTypePipelineRun, TaskTriggerTypeManual} {
		allowedType := strings.ToLower(string(allowed))

		if taskType == allowedType {
			if allowedType == strings.ToLower(string(TaskTriggerTypePipelineRun)) && r.Name == "" {
				fmt.Println("HERE")
				return apis.ErrMissingField(fmt.Sprintf("%s.name", path))
			}
			return nil
		}
	}
	return apis.ErrInvalidValue(string(r.Type), fmt.Sprintf("%s.type", path))
}

func validateParameters(params []Param) *apis.FieldError {
	// Template must not duplicate parameter names.
	seen := map[string]struct{}{}
	for _, p := range params {
		if _, ok := seen[strings.ToLower(p.Name)]; ok {
			return apis.ErrMultipleOneOf("spec.inputs.params")
		}
		seen[p.Name] = struct{}{}
	}
	return nil
}
