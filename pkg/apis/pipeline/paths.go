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

package pipeline

const (
	// WorkspaceDir is the root directory used for PipelineResources and (by default) Workspaces
	WorkspaceDir = "/workspace"
	// DefaultResultPath is the path for a task result to create the result file
	DefaultResultPath = "/tekton/results"
	// HomeDir is the HOME directory of PipelineResources
	HomeDir = "/tekton/home"
	// CredsDir is the directory where credentials are placed to meet the legacy credentials
	// helpers image (aka "creds-init") contract
	CredsDir = "/tekton/creds"
	// StepsPath is the directory used for a step to store any metadata related to the step
	StepsPath = "/tekton/steps"
)
