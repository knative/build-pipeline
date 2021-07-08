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

package builder

import (
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	corev1 "k8s.io/api/core/v1"
)

// StepOp is an operation which modifies a Container struct.
type StepOp func(*v1beta1.Step)

// StepName sets the name of the step.
func StepName(name string) StepOp {
	return func(step *v1beta1.Step) {
		step.Name = name
	}
}

// StepCommand sets the command to the Container (step in this case).
func StepCommand(args ...string) StepOp {
	return func(step *v1beta1.Step) {
		step.Command = args
	}
}

// StepSecurityContext sets the SecurityContext to the Step.
func StepSecurityContext(context *corev1.SecurityContext) StepOp {
	return func(step *v1beta1.Step) {
		step.SecurityContext = context
	}
}

// StepArgs sets the command arguments to the Container (step in this case).
func StepArgs(args ...string) StepOp {
	return func(step *v1beta1.Step) {
		step.Args = args
	}
}

// StepEnvVar add an environment variable, with specified name and value, to the Container (step).
func StepEnvVar(name, value string) StepOp {
	return func(step *v1beta1.Step) {
		step.Env = append(step.Env, corev1.EnvVar{
			Name:  name,
			Value: value,
		})
	}
}

// StepWorkingDir sets the WorkingDir on the Container.
func StepWorkingDir(workingDir string) StepOp {
	return func(step *v1beta1.Step) {
		step.WorkingDir = workingDir
	}
}

// StepVolumeMount add a VolumeMount to the Container (step).
func StepVolumeMount(name, mountPath string, ops ...VolumeMountOp) StepOp {
	return func(step *v1beta1.Step) {
		mount := &corev1.VolumeMount{
			Name:      name,
			MountPath: mountPath,
		}
		for _, op := range ops {
			op(mount)
		}
		step.VolumeMounts = append(step.VolumeMounts, *mount)
	}
}

// StepScript sets the script to the Step.
func StepScript(script string) StepOp {
	return func(step *v1beta1.Step) {
		step.Script = script
	}
}

// StepOnError sets the onError of a step
func StepOnError(e string) StepOp {
	return func(step *v1beta1.Step) {
		step.OnError = e
	}
}
