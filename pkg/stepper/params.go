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

package stepper

import (
	"context"
	"fmt"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"strings"
)

// UseLocation defines the location where we are using one or more steps where we may need to modify
// the parameters, results and workspaces
type UseLocation struct {
	PipelineRunSpec *v1beta1.PipelineRunSpec
	PipelineSpec    *v1beta1.PipelineSpec
	TaskName        string
	TaskRunSpec     *v1beta1.TaskRunSpec
	TaskSpec        *v1beta1.TaskSpec
}

// UseParametersAndResults adds the parameters from the used Task to the PipelineSpec if specified and the PipelineTask
func UseParametersAndResults(ctx context.Context, loc *UseLocation, uses *v1beta1.TaskSpec) error {
	parameterSpecs := uses.Params
	parameters := ToParams(parameterSpecs)
	results := uses.Results

	ps := loc.PipelineSpec
	if ps != nil {
		ps.Params = useParameterSpecs(ctx, ps.Params, parameterSpecs)
		ps.Results = usePipelineResults(ps.Results, results)
		ps.Workspaces = usePipelineWorkspaces(ps.Workspaces, uses.Workspaces)
	}
	trs := loc.TaskRunSpec
	if trs != nil {
		trs.Params = useParameters(trs.Params, parameters)
	}
	ts := loc.TaskSpec
	if ts != nil {
		ts.Params = useParameterSpecs(ctx, ts.Params, parameterSpecs)
		ts.Results = useResults(ts.Results, results)
		ts.Workspaces = useWorkspaces(ts.Workspaces, uses.Workspaces)

		// lets create a step template if its not already defined
		if len(parameters) > 0 {
			stepTemplate := ts.StepTemplate
			created := false
			if stepTemplate == nil {
				stepTemplate = &corev1.Container{}
				created = true
			}
			stepTemplate.Env = useParameterEnvVars(stepTemplate.Env, parameters)
			if len(stepTemplate.Env) > 0 && created {
				ts.StepTemplate = stepTemplate
			}
		}
	}
	return nil
}

// ToParams converts the param specs to params
func ToParams(params []v1beta1.ParamSpec) []v1beta1.Param {
	var answer []v1beta1.Param
	for _, p := range params {
		answer = append(answer, v1beta1.Param{
			Name: p.Name,
			Value: v1beta1.ArrayOrString{
				Type:      v1beta1.ParamTypeString,
				StringVal: fmt.Sprintf("$(params.%s)", p.Name),
			},
		})
	}
	return answer
}

func useParameterSpecs(ctx context.Context, params []v1beta1.ParamSpec, uses []v1beta1.ParamSpec) []v1beta1.ParamSpec {
	for _, u := range uses {
		found := false
		for i := range params {
			param := &params[i]
			if param.Name == u.Name {
				found = true
				if param.Description == "" {
					param.Description = u.Description
				}
				param.SetDefaults(ctx)
				break
			}
		}
		if !found {
			u.SetDefaults(ctx)
			params = append(params, u)
		}
	}
	return params
}

func useParameters(params []v1beta1.Param, uses []v1beta1.Param) []v1beta1.Param {
	for _, u := range uses {
		found := false
		for i := range params {
			p := &params[i]
			if p.Name == u.Name {
				found = true
				if p.Value.Type == u.Value.Type {
					switch p.Value.Type {
					case v1beta1.ParamTypeString:
						if p.Value.StringVal == "" {
							p.Value.StringVal = u.Value.StringVal
						}
					case v1beta1.ParamTypeArray:
						if len(p.Value.ArrayVal) == 0 {
							p.Value.ArrayVal = u.Value.ArrayVal
						}
					}
				}
				break
			}
		}
		if !found {
			params = append(params, u)
		}
	}
	return params
}

func useParameterEnvVars(env []corev1.EnvVar, uses []v1beta1.Param) []corev1.EnvVar {
	for _, u := range uses {
		name := u.Name
		upperName := strings.ToUpper(name)
		if upperName != name {
			// ignore parameters which are not already suitable environment names being upper case
			// with optional _ characters
			continue
		}
		found := false
		for i := range env {
			p := &env[i]
			if p.Name == name {
				found = true
				if p.Value == "" {
					p.Value = u.Value.StringVal
				}
				break
			}
		}
		if !found {
			env = append(env, corev1.EnvVar{
				Name:  name,
				Value: u.Value.StringVal,
			})
		}
	}
	return env
}

func usePipelineResults(results []v1beta1.PipelineResult, uses []v1beta1.TaskResult) []v1beta1.PipelineResult {
	for _, u := range uses {
		found := false
		for i := range results {
			param := &results[i]
			if param.Name == u.Name {
				found = true
				if param.Description == "" {
					param.Description = u.Description
				}
				break
			}
		}
		if !found {
			results = append(results, v1beta1.PipelineResult{
				Name:        u.Name,
				Description: u.Description,
			})
		}
	}
	return results
}

func useResults(results []v1beta1.TaskResult, uses []v1beta1.TaskResult) []v1beta1.TaskResult {
	for _, u := range uses {
		found := false
		for i := range results {
			param := &results[i]
			if param.Name == u.Name {
				found = true
				if param.Description == "" {
					param.Description = u.Description
				}
				break
			}
		}
		if !found {
			results = append(results, u)
		}
	}
	return results
}

func usePipelineWorkspaces(ws []v1beta1.PipelineWorkspaceDeclaration, uses []v1beta1.WorkspaceDeclaration) []v1beta1.PipelineWorkspaceDeclaration {
	for _, u := range uses {
		found := false
		for i := range ws {
			param := &ws[i]
			if param.Name == u.Name {
				found = true
				if param.Description == "" {
					param.Description = u.Description
				}
				break
			}
		}
		if !found {
			ws = append(ws, v1beta1.PipelineWorkspaceDeclaration{
				Name:        u.Name,
				Description: u.Description,
				Optional:    u.Optional,
			})
		}
	}
	return ws
}

func useWorkspaces(ws []v1beta1.WorkspaceDeclaration, uses []v1beta1.WorkspaceDeclaration) []v1beta1.WorkspaceDeclaration {
	for _, u := range uses {
		found := false
		for i := range ws {
			param := &ws[i]
			if param.Name == u.Name {
				found = true
				if param.Description == "" {
					param.Description = u.Description
				}
				break
			}
		}
		if !found {
			ws = append(ws, u)
		}
	}
	return ws
}
