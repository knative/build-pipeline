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
	"github.com/tektoncd/pipeline/pkg/reconciler/pipeline/dag"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
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
	Metadata PipelineTaskMetadata `json:"metadata,omitempty"`

	// TaskSpec is a specification of a task
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

func (pt *PipelineTask) TaskSpecMetadata() PipelineTaskMetadata {
	return pt.TaskSpec.Metadata
}

func (pt *PipelineTask) TaskRefMetadata() TaskRefMetadata {
	return pt.TaskRef.Metadata
}

func (pt PipelineTask) HashKey() string {
	return pt.Name
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
		for _, param := range cond.Params {
			expressions, ok := GetVarSubstitutionExpressionsForParam(param)
			if ok {
				resultRefs := NewResultRefs(expressions)
				for _, resultRef := range resultRefs {
					resourceDeps = append(resourceDeps, resultRef.PipelineTask)
				}
			}
		}
	}

	// Add any dependents from task results
	for _, param := range pt.Params {
		expressions, ok := GetVarSubstitutionExpressionsForParam(param)
		if ok {
			resultRefs := NewResultRefs(expressions)
			for _, resultRef := range resultRefs {
				resourceDeps = append(resourceDeps, resultRef.PipelineTask)
			}
		}
	}
	// Add any dependents from when expressions
	for _, whenExpression := range pt.WhenExpressions {
		expressions, ok := whenExpression.GetVarSubstitutionExpressions()
		if ok {
			resultRefs := NewResultRefs(expressions)
			for _, resultRef := range resultRefs {
				resourceDeps = append(resourceDeps, resultRef.PipelineTask)
			}
		}
	}
	return resourceDeps
}

func (pt PipelineTask) orderingDeps() []string {
	orderingDeps := []string{}
	resourceDeps := pt.resourceDeps()
	for _, runAfter := range pt.RunAfter {
		if !contains(runAfter, resourceDeps) {
			orderingDeps = append(orderingDeps, runAfter)
		}
	}
	return orderingDeps
}

func contains(s string, arr []string) bool {
	for _, elem := range arr {
		if elem == s {
			return true
		}
	}
	return false
}

type PipelineTaskList []PipelineTask

func (l PipelineTaskList) Deps() map[string][]string {
	deps := map[string][]string{}
	for _, pt := range l {
		deps[pt.HashKey()] = pt.Deps()
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
