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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strings"
)

const (
	// TaskRunResultType default task run result value
	TaskRunResultType ResultType = "TaskRunResult"
	// PipelineResourceResultType default pipeline result value
	PipelineResourceResultType ResultType = "PipelineResourceResult"
	// InternalTektonResultType default internal tekton result value
	InternalTektonResultType ResultType = "InternalTektonResult"
	// UnknownResultType default unknown result type value
	UnknownResultType ResultType = ""
)

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Task represents a collection of sequential steps that are run as part of a
// Pipeline using a set of inputs and producing a set of outputs. Tasks execute
// when TaskRuns are created that provide the input parameters and resources and
// output resources the Task requires.
//
// +k8s:openapi-gen=true
type Task struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata"`

	// Spec holds the desired state of the Task from the client
	// +optional
	Spec TaskSpec `json:"spec"`
}

func (t *Task) TaskSpec() TaskSpec {
	return t.Spec
}

func (t *Task) TaskMetadata() metav1.ObjectMeta {
	return t.ObjectMeta
}

func (t *Task) Copy() TaskObject {
	return t.DeepCopy()
}

// TaskSpec defines the desired state of Task.
type TaskSpec struct {
	// Resources is a list input and output resource to run the task
	// Resources are represented in TaskRuns as bindings to instances of
	// PipelineResources.
	// +optional
	Resources *TaskResources `json:"resources,omitempty"`

	// Params is a list of input parameters required to run the task. Params
	// must be supplied as inputs in TaskRuns unless they declare a default
	// value.
	// +optional
	Params []ParamSpec `json:"params,omitempty"`

	// Description is a user-facing description of the task that may be
	// used to populate a UI.
	// +optional
	Description string `json:"description,omitempty"`

	// Steps are the steps of the build; each step is run sequentially with the
	// source mounted into /workspace.
	Steps []Step `json:"steps,omitempty"`

	// Volumes is a collection of volumes that are available to mount into the
	// steps of the build.
	Volumes []corev1.Volume `json:"volumes,omitempty"`

	// StepTemplate can be used as the basis for all step containers within the
	// Task, so that the steps inherit settings on the base container.
	StepTemplate *corev1.Container `json:"stepTemplate,omitempty"`

	// Sidecars are run alongside the Task's step containers. They begin before
	// the steps start and end after the steps complete.
	Sidecars []Sidecar `json:"sidecars,omitempty"`

	// Workspaces are the volumes that this Task requires.
	Workspaces []WorkspaceDeclaration `json:"workspaces,omitempty"`

	// Results are values that this Task can output
	Results []TaskResult `json:"results,omitempty"`
}

// TaskResult used to describe the results of a task
type TaskResult struct {
	// Name the given name
	Name string `json:"name"`

	// Description is a human-readable description of the result
	// +optional
	Description string `json:"description"`
}

// Step embeds the Container type, which allows it to include fields not
// provided by Container.
type Step struct {
	corev1.Container `json:",inline"`

	// Script is the contents of an executable file to execute.
	//
	// If Script is not empty, the Step cannot have an Command and the Args will be passed to the Script.
	Script string `json:"script,omitempty"`
	// Timeout is the time after which the step times out. Defaults to never.
	// Refer to Go's ParseDuration documentation for expected format: https://golang.org/pkg/time/#ParseDuration
	Timeout *metav1.Duration `json:"timeout,omitempty"`

	// Uses allows one or more Steps to be used from a source such as git or OCI. The steps
	// can have their properties overridden.
	// see: https://github.com/tektoncd/pipeline/blob/master/docs/step-composition.md
	Uses *Uses `json:"uses,omitempty"`
}

// Sidecar has nearly the same data structure as Step, consisting of a Container and an optional Script, but does not have the ability to timeout.
type Sidecar struct {
	corev1.Container `json:",inline"`

	// Script is the contents of an executable file to execute.
	//
	// If Script is not empty, the Step cannot have an Command or Args.
	Script string `json:"script,omitempty"`
}

// Uses allows one or more steps to be inherited from a Task in git, via OCI bundles or kubernetes resources.
//
// It is similar to TaskRef but has an additional optional `step` property and uses `task` rather than `name`
// to avoid confusion over what the name is for
type Uses struct {
	// Name of the task if we are referencing a resource which is a Pipeline or PipelineRun which contains multiple
	// Tasks
	// More info: http://kubernetes.io/docs/user-guide/identifiers#names
	// +optional
	Task string `json:"name,omitempty"`
	// TaskKind indicates the kind of the task, namespaced or cluster scoped if referencing a kubernetes resource by name
	Kind TaskKind `json:"kind,omitempty"`
	// API version of the referent if using a kubernetes resource
	// +optional
	APIVersion string `json:"apiVersion,omitempty"`
	// Bundle url reference to a Tekton Bundle.
	// +optional
	Bundle string `json:"bundle,omitempty"`
	// Git the git URI to reference a Task, TaskRun, Pipeline or PipelineRun inside a git repository.
	// For github this is of the form: `repositoryOwner/repositoryName/path@branchTagOrSHA`
	// For other git servers its of the form: `gitCloneURL/path[@branchTagOrSHA]` where the `gitCloneURL` ends in `.git`
	// If the resource is a Pipeline or PipelineRun with multiple Tasks then the `task` in this reference must match the Task you refer to
	// +optional
	Git string `json:"git,omitempty"`
	// Step the name of the step to be included. If not specified all of the steps of the given
	// Task will be included.
	// +optional
	Step string `json:"step,omitempty"`
}

// String returns a useful string representation of the uses clause
func (u *Uses) String() string {
	if u == nil {
		return "nil"
	}
	text := u.Git
	if text == "" {
		text = u.Task
		k := string(u.Kind)
		if k != "" {
			text = k + "/" + text
		}
		if u.Bundle != "" {
			text += "/" + u.Bundle
		}
	}
	if u.Step == "" {
		return text
	}
	return text + ":" + u.Step
}

// Key returns a unique string for the kind and server we can use for caching Remotes
func (u *Uses) Key() string {
	text := ""
	if u.Git != "" {
		text = "git:" + u.Git
	} else if u.Task != "" || u.Bundle != "" {
		text = "ref:"
		k := string(u.Kind)
		if u.APIVersion != "" {
			text += u.APIVersion + "/"
		}
		if k != "" {
			text += k + "/"
		}
		if u.Task != "" {
			text += u.Task
		}
		if u.Bundle != "" {
			if !strings.HasSuffix(text, "/") {
				text += "/"
			}
			text += u.Bundle
		}
	}
	if u.Step == "" {
		return text
	}
	return text + ":" + u.Step
}

// ToTaskRef converts this struct to a TaskRef so that we can find the Task
func (u *Uses) ToTaskRef() TaskRef {
	return TaskRef{
		Name:       u.Task,
		Kind:       u.Kind,
		APIVersion: u.APIVersion,
		Bundle:     u.Bundle,
		Git:        u.Git,
	}
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// TaskList contains a list of Task
type TaskList struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Task `json:"items"`
}

// TaskRef can be used to refer to a specific instance of a task.
// Copied from CrossVersionObjectReference: https://github.com/kubernetes/kubernetes/blob/169df7434155cbbc22f1532cba8e0a9588e29ad8/pkg/apis/autoscaling/types.go#L64
type TaskRef struct {
	// Name of the referent; More info: http://kubernetes.io/docs/user-guide/identifiers#names
	Name string `json:"name,omitempty"`
	// TaskKind indicates the kind of the task, namespaced or cluster scoped.
	Kind TaskKind `json:"kind,omitempty"`
	// API version of the referent
	// +optional
	APIVersion string `json:"apiVersion,omitempty"`
	// Bundle url reference to a Tekton Bundle.
	// +optional
	Bundle string `json:"bundle,omitempty"`
	// Git the git URI to reference a Task, TaskRun, Pipeline or PipelineRun inside a git repository.
	// For github this is of the form: `repositoryOwner/repositoryName/path@branchTagOrSHA`
	// For other git servers its of the form: `gitCloneURL/path[@branchTagOrSHA]` where the `gitCloneURL` ends in `.git`
	// If the resource is a Pipeline or PipelineRun with multiple Tasks then the `name` in this reference must match the Task you refer to
	// +optional
	Git string `json:"git,omitempty"`
}

// Check that Pipeline may be validated and defaulted.
// TaskKind defines the type of Task used by the pipeline.
type TaskKind string

const (
	// NamespacedTaskKind indicates that the task type has a namespaced scope.
	NamespacedTaskKind TaskKind = "Task"
	// ClusterTaskKind indicates that task type has a cluster scope.
	ClusterTaskKind TaskKind = "ClusterTask"
)
