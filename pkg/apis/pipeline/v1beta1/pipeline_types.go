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

package v1beta1

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/tektoncd/pipeline/pkg/apis/config"

	"github.com/tektoncd/pipeline/pkg/reconciler/pipeline/dag"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation"
	"knative.dev/pkg/apis"
)

const (
	// PipelineTasksAggregateStatus is a param representing aggregate status of all dag pipelineTasks
	PipelineTasksAggregateStatus = "tasks.status"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient:noStatus

// Pipeline describes a list of Tasks to execute. It expresses how outputs
// of tasks feed into inputs of subsequent tasks.
// +k8s:openapi-gen=true
type Pipeline struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec holds the desired state of the Pipeline from the client
	// +optional
	Spec PipelineSpec `json:"spec"`
}

func (p *Pipeline) PipelineMetadata() metav1.ObjectMeta {
	return p.ObjectMeta
}

func (p *Pipeline) PipelineSpec() PipelineSpec {
	return p.Spec
}

func (p *Pipeline) Copy() PipelineObject {
	return p.DeepCopy()
}

// PipelineSpec defines the desired state of Pipeline.
type PipelineSpec struct {
	// Description is a user-facing description of the pipeline that may be
	// used to populate a UI.
	// +optional
	Description string `json:"description,omitempty"`
	// Resources declares the names and types of the resources given to the
	// Pipeline's tasks as inputs and outputs.
	Resources []PipelineDeclaredResource `json:"resources,omitempty"`
	// Tasks declares the graph of Tasks that execute when this Pipeline is run.
	Tasks []PipelineTask `json:"tasks,omitempty"`
	// Params declares a list of input parameters that must be supplied when
	// this Pipeline is run.
	Params []ParamSpec `json:"params,omitempty"`
	// Workspaces declares a set of named workspaces that are expected to be
	// provided by a PipelineRun.
	// +optional
	Workspaces []PipelineWorkspaceDeclaration `json:"workspaces,omitempty"`
	// Results are values that this pipeline can output once run
	// +optional
	Results []PipelineResult `json:"results,omitempty"`
	// Finally declares the list of Tasks that execute just before leaving the Pipeline
	// i.e. either after all Tasks are finished executing successfully
	// or after a failure which would result in ending the Pipeline
	Finally []PipelineTask `json:"finally,omitempty"`
}

// PipelineResult used to describe the results of a pipeline
type PipelineResult struct {
	// Name the given name
	Name string `json:"name"`

	// Description is a human-readable description of the result
	// +optional
	Description string `json:"description"`

	// Value the expression used to retrieve the value
	Value string `json:"value"`
}

type PipelineTaskMetadata struct {
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
}

type EmbeddedTask struct {
	// +optional
	runtime.TypeMeta `json:",inline,omitempty"`

	// Spec is a specification of a custom task
	// +optional
	Spec runtime.RawExtension `json:"spec,omitempty"`

	// +optional
	Metadata PipelineTaskMetadata `json:"metadata,omitempty"`

	// TaskSpec is a specification of a task
	// +optional
	TaskSpec `json:",inline,omitempty"`
}

// PipelineTask defines a task in a Pipeline, passing inputs from both
// Params and from the output of previous tasks.
type PipelineTask struct {
	// Name is the name of this task within the context of a Pipeline. Name is
	// used as a coordinate with the `from` and `runAfter` fields to establish
	// the execution order of tasks relative to one another.
	Name string `json:"name,omitempty"`

	// TaskRef is a reference to a task definition.
	// +optional
	TaskRef *TaskRef `json:"taskRef,omitempty"`

	// TaskSpec is a specification of a task
	// +optional
	TaskSpec *EmbeddedTask `json:"taskSpec,omitempty"`

	// Conditions is a list of conditions that need to be true for the task to run
	// Conditions are deprecated, use WhenExpressions instead
	// +optional
	Conditions []PipelineTaskCondition `json:"conditions,omitempty"`

	// WhenExpressions is a list of when expressions that need to be true for the task to run
	// +optional
	WhenExpressions WhenExpressions `json:"when,omitempty"`

	// Retries represents how many times this task should be retried in case of task failure: ConditionSucceeded set to False
	// +optional
	Retries int `json:"retries,omitempty"`

	// RunAfter is the list of PipelineTask names that should be executed before
	// this Task executes. (Used to force a specific ordering in graph execution.)
	// +optional
	RunAfter []string `json:"runAfter,omitempty"`

	// Resources declares the resources given to this task as inputs and
	// outputs.
	// +optional
	Resources *PipelineTaskResources `json:"resources,omitempty"`
	// Parameters declares parameters passed to this task.
	// +optional
	Params []Param `json:"params,omitempty"`

	// Workspaces maps workspaces from the pipeline spec to the workspaces
	// declared in the Task.
	// +optional
	Workspaces []WorkspacePipelineTaskBinding `json:"workspaces,omitempty"`

	// Time after which the TaskRun times out. Defaults to 1 hour.
	// Specified TaskRun timeout should be less than 24h.
	// Refer Go's ParseDuration documentation for expected format: https://golang.org/pkg/time/#ParseDuration
	// +optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`
}

// validateRefOrSpec validates at least one of taskRef or taskSpec is specified
func (pt PipelineTask) validateRefOrSpec() (errs *apis.FieldError) {
	// can't have both taskRef and taskSpec at the same time
	if pt.TaskRef != nil && pt.TaskSpec != nil {
		errs = errs.Also(apis.ErrMultipleOneOf("taskRef", "taskSpec"))
	}
	// Check that one of TaskRef and TaskSpec is present
	if pt.TaskRef == nil && pt.TaskSpec == nil {
		errs = errs.Also(apis.ErrMissingOneOf("taskRef", "taskSpec"))
	}
	return errs
}

// validateCustomTask validates custom task specifications - checking kind and fail if not yet supported features specified
func (pt PipelineTask) validateCustomTask() (errs *apis.FieldError) {
	if pt.TaskRef != nil && pt.TaskRef.Kind == "" {
		errs = errs.Also(apis.ErrInvalidValue("custom task ref must specify kind", "taskRef.kind"))
	}
	if pt.TaskSpec != nil && pt.TaskSpec.Kind == "" {
		errs = errs.Also(apis.ErrInvalidValue("custom task spec must specify kind", "taskSpec.kind"))
	}
	if pt.TaskRef != nil && pt.TaskRef.APIVersion == "" {
		errs = errs.Also(apis.ErrInvalidValue("custom task ref must specify apiVersion", "taskRef.apiVersion"))
	}
	if pt.TaskSpec != nil && pt.TaskSpec.APIVersion == "" {
		errs = errs.Also(apis.ErrInvalidValue("custom task spec must specify apiVersion", "taskSpec.apiVersion"))
	}

	// Conditions are deprecated so the effort to support them with custom tasks is not justified.
	// When expressions should be used instead.
	if len(pt.Conditions) > 0 {
		errs = errs.Also(apis.ErrInvalidValue("custom tasks do not support conditions - use when expressions instead", "conditions"))
	}
	// TODO(#3133): Support these features if possible.
	if pt.Resources != nil {
		errs = errs.Also(apis.ErrInvalidValue("custom tasks do not support PipelineResources", "resources"))
	}
	if pt.Timeout != nil {
		errs = errs.Also(apis.ErrInvalidValue("custom tasks do not support timeout", "timeout"))
	}
	return errs
}

// validateBundle validates bundle specifications - checking name and bundle
func (pt PipelineTask) validateBundle() (errs *apis.FieldError) {
	// bundle requires a TaskRef to be specified
	if (pt.TaskRef != nil && pt.TaskRef.Bundle != "") && pt.TaskRef.Name == "" {
		errs = errs.Also(apis.ErrMissingField("taskRef.name"))
	}
	// If a bundle url is specified, ensure it is parsable
	if pt.TaskRef != nil && pt.TaskRef.Bundle != "" {
		if _, err := name.ParseReference(pt.TaskRef.Bundle); err != nil {
			errs = errs.Also(apis.ErrInvalidValue(fmt.Sprintf("invalid bundle reference (%s)", err.Error()), "taskRef.bundle"))
		}
	}
	return errs
}

// validateTask validates a pipeline task or a final task for taskRef and taskSpec
func (pt PipelineTask) validateTask(ctx context.Context) (errs *apis.FieldError) {
	// Validate TaskSpec if it's present
	if pt.TaskSpec != nil {
		errs = errs.Also(pt.TaskSpec.Validate(ctx).ViaField("taskSpec"))
	}
	if pt.TaskRef != nil {
		if pt.TaskRef.Name != "" {
			// TaskRef name must be a valid k8s name
			if errSlice := validation.IsQualifiedName(pt.TaskRef.Name); len(errSlice) != 0 {
				errs = errs.Also(apis.ErrInvalidValue(strings.Join(errSlice, ","), "name"))
			}
		} else {
			errs = errs.Also(apis.ErrInvalidValue("taskRef must specify name", "taskRef.name"))
		}
		// fail if bundle is present when EnableTektonOCIBundles feature flag is off (as it won't be allowed nor used)
		if pt.TaskRef.Bundle != "" {
			errs = errs.Also(apis.ErrDisallowedFields("taskref.bundle"))
		}
	}
	return errs
}

func (pt *PipelineTask) TaskSpecMetadata() PipelineTaskMetadata {
	return pt.TaskSpec.Metadata
}

func (pt PipelineTask) HashKey() string {
	return pt.Name
}

func (pt PipelineTask) ValidateName() *apis.FieldError {
	if err := validation.IsDNS1123Label(pt.Name); len(err) > 0 {
		return &apis.FieldError{
			Message: fmt.Sprintf("invalid value %q", pt.Name),
			Paths:   []string{"name"},
			Details: "Pipeline Task name must be a valid DNS Label." +
				"For more info refer to https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names",
		}
	}
	return nil
}

// Validate classifies whether a task is a custom task, bundle, or a regular task(dag/final)
// calls the validation routine based on the type of the task
func (pt PipelineTask) Validate(ctx context.Context) (errs *apis.FieldError) {
	errs = errs.Also(pt.validateRefOrSpec())
	cfg := config.FromContextOrDefaults(ctx)
	// If EnableCustomTasks feature flag is on, validate custom task specifications
	// pipeline task having taskRef with APIVersion is classified as custom task
	switch {
	case cfg.FeatureFlags.EnableCustomTasks && pt.TaskRef != nil && pt.TaskRef.APIVersion != "":
		errs = errs.Also(pt.validateCustomTask())
	case cfg.FeatureFlags.EnableCustomTasks && pt.TaskSpec != nil && pt.TaskSpec.APIVersion != "":
		errs = errs.Also(pt.validateCustomTask())
		// If EnableTektonOCIBundles feature flag is on, validate bundle specifications
	case cfg.FeatureFlags.EnableTektonOCIBundles && pt.TaskRef != nil && pt.TaskRef.Bundle != "":
		errs = errs.Also(pt.validateBundle())
	default:
		errs = errs.Also(pt.validateTask(ctx))
	}
	return
}

func (pt PipelineTask) Deps() []string {
	deps := []string{}

	deps = append(deps, pt.resourceDeps()...)
	deps = append(deps, pt.orderingDeps()...)

	uniqueDeps := sets.NewString()
	for _, w := range deps {
		if uniqueDeps.Has(w) {
			continue
		}
		uniqueDeps.Insert(w)
	}

	return uniqueDeps.List()
}

func (pt PipelineTask) resourceDeps() []string {
	resourceDeps := []string{}
	if pt.Resources != nil {
		for _, rd := range pt.Resources.Inputs {
			resourceDeps = append(resourceDeps, rd.From...)
		}
	}

	// Add any dependents from conditional resources.
	for _, cond := range pt.Conditions {
		for _, rd := range cond.Resources {
			resourceDeps = append(resourceDeps, rd.From...)
		}
	}

	// Add any dependents from result references.
	for _, ref := range PipelineTaskResultRefs(&pt) {
		resourceDeps = append(resourceDeps, ref.PipelineTask)
	}

	return resourceDeps
}

func (pt PipelineTask) orderingDeps() []string {
	orderingDeps := []string{}
	for _, runAfter := range pt.RunAfter {
		orderingDeps = append(orderingDeps, runAfter)
	}
	return orderingDeps
}

type PipelineTaskList []PipelineTask

// Deps returns a map with key as name of a pipelineTask and value as a list of its dependencies
func (l PipelineTaskList) Deps() map[string][]string {
	deps := map[string][]string{}
	for _, pt := range l {
		// get the list of deps for this pipelineTask
		d := pt.Deps()
		// add the pipelineTask into the map if it has any deps
		if len(d) > 0 {
			deps[pt.HashKey()] = d
		}
	}
	return deps
}

func (l PipelineTaskList) Items() []dag.Task {
	tasks := []dag.Task{}
	for _, t := range l {
		tasks = append(tasks, dag.Task(t))
	}
	return tasks
}

// Names returns a set of pipeline task names from the given list of pipeline tasks
func (l PipelineTaskList) Names() sets.String {
	names := sets.String{}
	for _, pt := range l {
		names.Insert(pt.Name)
	}
	return names
}

// Validate a list of pipeline tasks including custom task and bundles
func (l PipelineTaskList) Validate(ctx context.Context, taskNames sets.String, path string) (errs *apis.FieldError) {
	for i, t := range l {
		// validate pipeline task name
		errs = errs.Also(t.ValidateName().ViaFieldIndex(path, i))
		// names cannot be duplicated - checking that pipelineTask names are unique
		if _, ok := taskNames[t.Name]; ok {
			errs = errs.Also(apis.ErrMultipleOneOf("name").ViaFieldIndex(path, i))
		}
		taskNames.Insert(t.Name)
		// validate custom task, bundle, dag, or final task
		errs = errs.Also(t.Validate(ctx).ViaFieldIndex(path, i))
	}
	return errs
}

// PipelineTaskParam is used to provide arbitrary string parameters to a Task.
type PipelineTaskParam struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// PipelineTaskCondition allows a PipelineTask to declare a Condition to be evaluated before
// the Task is run.
type PipelineTaskCondition struct {
	// ConditionRef is the name of the Condition to use for the conditionCheck
	ConditionRef string `json:"conditionRef"`

	// Params declare parameters passed to this Condition
	// +optional
	Params []Param `json:"params,omitempty"`

	// Resources declare the resources provided to this Condition as input
	Resources []PipelineTaskInputResource `json:"resources,omitempty"`
}

// PipelineDeclaredResource is used by a Pipeline to declare the types of the
// PipelineResources that it will required to run and names which can be used to
// refer to these PipelineResources in PipelineTaskResourceBindings.
type PipelineDeclaredResource struct {
	// Name is the name that will be used by the Pipeline to refer to this resource.
	// It does not directly correspond to the name of any PipelineResources Task
	// inputs or outputs, and it does not correspond to the actual names of the
	// PipelineResources that will be bound in the PipelineRun.
	Name string `json:"name"`
	// Type is the type of the PipelineResource.
	Type PipelineResourceType `json:"type"`
	// Optional declares the resource as optional.
	// optional: true - the resource is considered optional
	// optional: false - the resource is considered required (default/equivalent of not specifying it)
	Optional bool `json:"optional,omitempty"`
}

// PipelineTaskResources allows a Pipeline to declare how its DeclaredPipelineResources
// should be provided to a Task as its inputs and outputs.
type PipelineTaskResources struct {
	// Inputs holds the mapping from the PipelineResources declared in
	// DeclaredPipelineResources to the input PipelineResources required by the Task.
	Inputs []PipelineTaskInputResource `json:"inputs,omitempty"`
	// Outputs holds the mapping from the PipelineResources declared in
	// DeclaredPipelineResources to the input PipelineResources required by the Task.
	Outputs []PipelineTaskOutputResource `json:"outputs,omitempty"`
}

// PipelineTaskInputResource maps the name of a declared PipelineResource input
// dependency in a Task to the resource in the Pipeline's DeclaredPipelineResources
// that should be used. This input may come from a previous task.
type PipelineTaskInputResource struct {
	// Name is the name of the PipelineResource as declared by the Task.
	Name string `json:"name"`
	// Resource is the name of the DeclaredPipelineResource to use.
	Resource string `json:"resource"`
	// From is the list of PipelineTask names that the resource has to come from.
	// (Implies an ordering in the execution graph.)
	// +optional
	From []string `json:"from,omitempty"`
}

// PipelineTaskOutputResource maps the name of a declared PipelineResource output
// dependency in a Task to the resource in the Pipeline's DeclaredPipelineResources
// that should be used.
type PipelineTaskOutputResource struct {
	// Name is the name of the PipelineResource as declared by the Task.
	Name string `json:"name"`
	// Resource is the name of the DeclaredPipelineResource to use.
	Resource string `json:"resource"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PipelineList contains a list of Pipeline
type PipelineList struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Pipeline `json:"items"`
}
