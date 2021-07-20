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
	// PipelineRunControllerName holds the name of the PipelineRun controller
	// nolint: golint
	PipelineRunControllerName = "PipelineRun"

	// TaskRunControllerName holds the name of the TaskRun controller
	TaskRunControllerName = "TaskRun"

	// RuncControllerName holds the name of the Custom Task controller
	RunControllerName = "Run"

	// TaskRunResolverControllerName holds the name of the TaskRun Resolver controller
	TaskRunResolverControllerName = "TaskRunResolver"

	// PipelineRunResolverControllerName holds the name of the PipelineRun Resolver controller
	PipelineRunResolverControllerName = "PipelineRunResolver"
)
