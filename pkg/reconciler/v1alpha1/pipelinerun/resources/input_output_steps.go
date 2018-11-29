/*
Copyright 2018 The Knative Authors

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
package resources

import (
	"path/filepath"

	"github.com/knative/build-pipeline/pkg/apis/pipeline/v1alpha1"
)

var (
	pvcDir = "/pvc"
)

// GetOutputSteps function reads output task resources and constructs post task step
// postSteps contains array of resource names and named path(pvcdir + name of task + name of resource) under which the output resource will be dumped in PVC
func GetOutputSteps(taskResources []v1alpha1.TaskResourceBinding, taskName string) []v1alpha1.TaskRunResource {
	var taskOutputResources []v1alpha1.TaskRunResource

	for _, outputRes := range taskResources {
		taskOutputResources = append(taskOutputResources, v1alpha1.TaskRunResource{
			ResourceRef: outputRes.ResourceRef,
			Name:        outputRes.Name,
			Paths:       []string{filepath.Join(pvcDir, taskName, outputRes.Name)},
		})
	}
	return taskOutputResources
}

// GetInputSteps function reads input bindings and constructs pre build step
// with information to create build step to setup altered inputs.
func GetInputSteps(taskResources []v1alpha1.TaskResourceBinding, pt *v1alpha1.PipelineTask) []v1alpha1.TaskRunResource {
	var taskInputResources []v1alpha1.TaskRunResource

	for _, inputResource := range taskResources {
		taskInputResource := v1alpha1.TaskRunResource{
			ResourceRef: inputResource.ResourceRef,
			Name:        inputResource.Name,
		}

		var stepSourceNames []string
		for _, resourceDep := range pt.ResourceDependencies {
			if resourceDep.Name == inputResource.Name {
				for _, constr := range resourceDep.ProvidedBy {
					stepSourceNames = append(stepSourceNames, filepath.Join(pvcDir, constr, inputResource.Name))
				}
			}
		}
		if len(stepSourceNames) > 0 {
			taskInputResource.Paths = append(taskInputResource.Paths, stepSourceNames...)
		}
		taskInputResources = append(taskInputResources, taskInputResource)
	}
	return taskInputResources
}

// WrapSteps input and resources for taskrun along with presteps , poststeps
func WrapSteps(tr *v1alpha1.TaskRunSpec, pipelineResources []v1alpha1.PipelineTaskResource, pt *v1alpha1.PipelineTask) {
	if pt == nil {
		return
	}
	for _, prTask := range pipelineResources {
		if prTask.Name == pt.Name {
			// Add presteps to setup updated input
			tr.Inputs.Resources = append(tr.Inputs.Resources, GetInputSteps(prTask.Inputs, pt)...)
			// Add poststeps to setup outputs
			tr.Outputs.Resources = append(tr.Outputs.Resources, GetOutputSteps(prTask.Outputs, prTask.Name)...)
		}
	}
}
