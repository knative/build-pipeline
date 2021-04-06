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
	"github.com/pkg/errors"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	v1 "k8s.io/api/core/v1"
)

// OverrideStep overrides the step with the given overrides
func OverrideStep(step *v1beta1.Step, override *v1beta1.Step, overrideAllSteps bool) error {
	if override.Timeout != nil {
		step.Timeout = override.Timeout
	}
	if override.WorkingDir != "" {
		step.WorkingDir = override.WorkingDir
	}
	if string(override.ImagePullPolicy) != "" {
		step.ImagePullPolicy = override.ImagePullPolicy
	}
	step.Env = OverrideEnv(step.Env, override.Env)
	step.EnvFrom = OverrideEnvFrom(step.EnvFrom, override.EnvFrom)
	step.VolumeMounts = OverrideVolumeMounts(step.VolumeMounts, override.VolumeMounts)

	if overrideAllSteps {
		// we are overriding in the included steps from a Task so lets not interfer with individual images or commands
		if len(override.Command) > 0 || len(override.Args) > 0 || override.Script != "" || override.Image != "" {
			return errors.Errorf("reusing multiple steps from Task so cannot override step details for step %s. Please inline the uses task steps to override the step command", step.Name)
		}
		return nil
	}
	if override.Name != "" {
		step.Name = override.Name
	}
	if override.Image != "" {
		step.Image = override.Image
	}
	if override.Script != "" {
		step.Script = override.Script
		step.Command = nil
		step.Args = nil
	} else if len(override.Command) > 0 {
		step.Script = override.Script
		step.Command = override.Command
		step.Args = override.Args
	} else if len(override.Args) > 0 {
		step.Args = override.Args
	}
	return nil
}

// OverrideEnv override either replaces or adds the given env vars
func OverrideEnv(from []v1.EnvVar, overrides []v1.EnvVar) []v1.EnvVar {
	for _, override := range overrides {
		found := false
		for i := range from {
			f := &from[i]
			if f.Name == override.Name {
				found = true
				*f = override
				break
			}
		}
		if !found {
			from = append(from, override)
		}
	}
	return from
}

// OverrideEnvFrom override either replaces or adds the given env froms
func OverrideEnvFrom(from []v1.EnvFromSource, overrides []v1.EnvFromSource) []v1.EnvFromSource {
	for _, override := range overrides {
		found := false
		for i := range from {
			f := &from[i]
			if f.ConfigMapRef != nil && override.ConfigMapRef != nil && f.ConfigMapRef.Name == override.ConfigMapRef.Name {
				found = true
				*f = override
				break
			}
			if f.SecretRef != nil && override.SecretRef != nil && f.SecretRef.Name == override.SecretRef.Name {
				found = true
				*f = override
				break
			}
		}
		if !found {
			from = append(from, override)
		}
	}
	return from
}

// OverrideVolumeMounts override either replaces or adds the given volume mounts
func OverrideVolumeMounts(from []v1.VolumeMount, overrides []v1.VolumeMount) []v1.VolumeMount {
	for _, override := range overrides {
		found := false
		for i := range from {
			f := &from[i]
			if f.Name == override.Name {
				found = true
				*f = override
				break
			}
		}
		if !found {
			from = append(from, override)
		}
	}
	return from
}
