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

package resources

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/tektoncd/pipeline/pkg/apis/config"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"github.com/tektoncd/pipeline/pkg/substitution"
)

// ApplyParameters applies the params from a TaskRun.Input.Parameters to a TaskSpec
func ApplyParameters(spec *v1beta1.TaskSpec, tr *v1beta1.TaskRun, defaults ...v1beta1.ParamSpec) *v1beta1.TaskSpec {
	// This assumes that the TaskRun inputs have been validated against what the Task requests.

	// stringReplacements is used for standard single-string stringReplacements, while arrayReplacements contains arrays
	// that need to be further processed.
	stringReplacements := map[string]string{}
	arrayReplacements := map[string][]string{}

	// Set all the default stringReplacements
	for _, p := range defaults {
		if p.Default != nil {
			if p.Default.Type == v1beta1.ParamTypeString {
				stringReplacements[fmt.Sprintf("params.%s", p.Name)] = p.Default.StringVal
				// FIXME(vdemeester) Remove that with deprecating v1beta1
				stringReplacements[fmt.Sprintf("inputs.params.%s", p.Name)] = p.Default.StringVal
			} else {
				arrayReplacements[fmt.Sprintf("params.%s", p.Name)] = p.Default.ArrayVal
				// FIXME(vdemeester) Remove that with deprecating v1beta1
				arrayReplacements[fmt.Sprintf("inputs.params.%s", p.Name)] = p.Default.ArrayVal
			}
		}
	}
	// Set and overwrite params with the ones from the TaskRun
	for _, p := range tr.Spec.Params {
		if p.Value.Type == v1beta1.ParamTypeString {
			stringReplacements[fmt.Sprintf("params.%s", p.Name)] = p.Value.StringVal
			// FIXME(vdemeester) Remove that with deprecating v1beta1
			stringReplacements[fmt.Sprintf("inputs.params.%s", p.Name)] = p.Value.StringVal
		} else {
			arrayReplacements[fmt.Sprintf("params.%s", p.Name)] = p.Value.ArrayVal
			// FIXME(vdemeester) Remove that with deprecating v1beta1
			arrayReplacements[fmt.Sprintf("inputs.params.%s", p.Name)] = p.Value.ArrayVal
		}
	}
	return ApplyReplacements(spec, stringReplacements, arrayReplacements)
}

// ApplyResources applies the substitution from values in resources which are referenced in spec as subitems
// of the replacementStr.
func ApplyResources(spec *v1beta1.TaskSpec, resolvedResources map[string]v1beta1.PipelineResourceInterface, replacementStr string) *v1beta1.TaskSpec {
	replacements := map[string]string{}
	for name, r := range resolvedResources {
		for k, v := range r.Replacements() {
			replacements[fmt.Sprintf("resources.%s.%s.%s", replacementStr, name, k)] = v
			// FIXME(vdemeester) Remove that with deprecating v1beta1
			replacements[fmt.Sprintf("%s.resources.%s.%s", replacementStr, name, k)] = v
		}
	}

	// We always add replacements for 'path'
	if spec.Resources != nil && spec.Resources.Inputs != nil {
		for _, r := range spec.Resources.Inputs {
			replacements[fmt.Sprintf("resources.inputs.%s.path", r.Name)] = v1beta1.InputResourcePath(r.ResourceDeclaration)
			// FIXME(vdemeester) Remove that with deprecating v1beta1
			replacements[fmt.Sprintf("inputs.resources.%s.path", r.Name)] = v1beta1.InputResourcePath(r.ResourceDeclaration)
		}
	}
	if spec.Resources != nil && spec.Resources.Outputs != nil {
		for _, r := range spec.Resources.Outputs {
			replacements[fmt.Sprintf("resources.outputs.%s.path", r.Name)] = v1beta1.OutputResourcePath(r.ResourceDeclaration)
			// FIXME(vdemeester) Remove that with deprecating v1beta1
			replacements[fmt.Sprintf("outputs.resources.%s.path", r.Name)] = v1beta1.OutputResourcePath(r.ResourceDeclaration)
		}
	}

	return ApplyReplacements(spec, replacements, map[string][]string{})
}

// ApplyContexts applies the substitution from $(context.(taskRun|task).*) with the specified values.
// Uses "" as a default if a value is not available.
func ApplyContexts(spec *v1beta1.TaskSpec, rtr *ResolvedTaskResources, tr *v1beta1.TaskRun) *v1beta1.TaskSpec {
	replacements := map[string]string{
		"context.taskRun.name":      tr.Name,
		"context.task.name":         rtr.TaskName,
		"context.taskRun.namespace": tr.Namespace,
		"context.taskRun.uid":       string(tr.ObjectMeta.UID),
		"context.task.retry-count":  strconv.Itoa(len(tr.Status.RetriesStatus)),
	}
	return ApplyReplacements(spec, replacements, map[string][]string{})
}

// ApplyWorkspaces applies the substitution from paths that the workspaces in declarations mounted to, the
// volumes that bindings are realized with in the task spec and the PersistentVolumeClaim names for the
// workspaces.
func ApplyWorkspaces(ctx context.Context, spec *v1beta1.TaskSpec, declarations []v1beta1.WorkspaceDeclaration, bindings []v1beta1.WorkspaceBinding, vols map[string]corev1.Volume) *v1beta1.TaskSpec {
	stringReplacements := map[string]string{}

	bindNames := sets.NewString()
	for _, binding := range bindings {
		bindNames.Insert(binding.Name)
	}

	for _, declaration := range declarations {
		prefix := fmt.Sprintf("workspaces.%s.", declaration.Name)
		if declaration.Optional && !bindNames.Has(declaration.Name) {
			stringReplacements[prefix+"bound"] = "false"
			stringReplacements[prefix+"path"] = ""
		} else {
			stringReplacements[prefix+"bound"] = "true"
			alphaAPIEnabled := config.FromContextOrDefaults(ctx).FeatureFlags.EnableAPIFields == config.AlphaAPIFields
			if alphaAPIEnabled {
				spec = applyWorkspaceMountPath(prefix+"path", spec, declaration)
			} else {
				stringReplacements[prefix+"path"] = declaration.GetMountPath()
			}
		}
	}

	for name, vol := range vols {
		stringReplacements[fmt.Sprintf("workspaces.%s.volume", name)] = vol.Name
	}
	for _, binding := range bindings {
		if binding.PersistentVolumeClaim != nil {
			stringReplacements[fmt.Sprintf("workspaces.%s.claim", binding.Name)] = binding.PersistentVolumeClaim.ClaimName
		} else {
			stringReplacements[fmt.Sprintf("workspaces.%s.claim", binding.Name)] = ""
		}
	}
	return ApplyReplacements(spec, stringReplacements, map[string][]string{})
}

// applyWorkspaceMountPath accepts a workspace path variable of the form $(workspaces.foo.path) and replaces
// it in the fields of the TaskSpec. A new updated TaskSpec is returned. Steps or Sidecars in the TaskSpec
// that override the mountPath will receive that mountPath in place of the variable's value. Other Steps and
// Sidecars will see either the workspace's declared mountPath or the default of /workspaces/<name>.
func applyWorkspaceMountPath(variable string, spec *v1beta1.TaskSpec, declaration v1beta1.WorkspaceDeclaration) *v1beta1.TaskSpec {
	stringReplacements := map[string]string{variable: ""}
	emptyArrayReplacements := map[string][]string{}
	defaultMountPath := declaration.GetMountPath()
	// Replace instances of the workspace path variable that are overridden per-Step
	for i := range spec.Steps {
		step := &spec.Steps[i]
		for _, usage := range step.Workspaces {
			if usage.Name == declaration.Name && usage.MountPath != "" {
				stringReplacements[variable] = usage.MountPath
				v1beta1.ApplyStepReplacements(step, stringReplacements, emptyArrayReplacements)
			}
		}
	}
	// Replace instances of the workspace path variable that are overridden per-Sidecar
	for i := range spec.Sidecars {
		sidecar := &spec.Sidecars[i]
		for _, usage := range sidecar.Workspaces {
			if usage.Name == declaration.Name && usage.MountPath != "" {
				stringReplacements[variable] = usage.MountPath
				v1beta1.ApplySidecarReplacements(sidecar, stringReplacements, emptyArrayReplacements)
			}
		}
	}
	// Replace any remaining instances of the workspace path variable, which should fall
	// back to the mount path specified in the declaration.
	stringReplacements[variable] = defaultMountPath
	return ApplyReplacements(spec, stringReplacements, emptyArrayReplacements)
}

// ApplyTaskResults applies the substitution from values in results which are referenced in spec as subitems
// of the replacementStr.
func ApplyTaskResults(spec *v1beta1.TaskSpec) *v1beta1.TaskSpec {
	stringReplacements := map[string]string{}

	for _, result := range spec.Results {
		stringReplacements[fmt.Sprintf("results.%s.path", result.Name)] = filepath.Join(pipeline.DefaultResultPath, result.Name)
	}
	return ApplyReplacements(spec, stringReplacements, map[string][]string{})
}

// ApplyCredentialsPath applies a substitution of the key $(credentials.path) with the path that credentials
// from annotated secrets are written to.
func ApplyCredentialsPath(spec *v1beta1.TaskSpec, path string) *v1beta1.TaskSpec {
	stringReplacements := map[string]string{
		"credentials.path": path,
	}
	return ApplyReplacements(spec, stringReplacements, map[string][]string{})
}

// ApplyReplacements replaces placeholders for declared parameters with the specified replacements.
func ApplyReplacements(spec *v1beta1.TaskSpec, stringReplacements map[string]string, arrayReplacements map[string][]string) *v1beta1.TaskSpec {
	spec = spec.DeepCopy()

	// Apply variable expansion to steps fields.
	steps := spec.Steps
	for i := range steps {
		v1beta1.ApplyStepReplacements(&steps[i], stringReplacements, arrayReplacements)
	}

	// Apply variable expansion to stepTemplate fields.
	if spec.StepTemplate != nil {
		v1beta1.ApplyStepReplacements(&v1beta1.Step{Container: *spec.StepTemplate}, stringReplacements, arrayReplacements)
	}

	// Apply variable expansion to the build's volumes
	for i, v := range spec.Volumes {
		spec.Volumes[i].Name = substitution.ApplyReplacements(v.Name, stringReplacements)
		if v.VolumeSource.ConfigMap != nil {
			spec.Volumes[i].ConfigMap.Name = substitution.ApplyReplacements(v.ConfigMap.Name, stringReplacements)
			for index, item := range v.ConfigMap.Items {
				spec.Volumes[i].ConfigMap.Items[index].Key = substitution.ApplyReplacements(item.Key, stringReplacements)
				spec.Volumes[i].ConfigMap.Items[index].Path = substitution.ApplyReplacements(item.Path, stringReplacements)
			}
		}
		if v.VolumeSource.Secret != nil {
			spec.Volumes[i].Secret.SecretName = substitution.ApplyReplacements(v.Secret.SecretName, stringReplacements)
			for index, item := range v.Secret.Items {
				spec.Volumes[i].Secret.Items[index].Key = substitution.ApplyReplacements(item.Key, stringReplacements)
				spec.Volumes[i].Secret.Items[index].Path = substitution.ApplyReplacements(item.Path, stringReplacements)
			}
		}
		if v.PersistentVolumeClaim != nil {
			spec.Volumes[i].PersistentVolumeClaim.ClaimName = substitution.ApplyReplacements(v.PersistentVolumeClaim.ClaimName, stringReplacements)
		}
		if v.Projected != nil {
			for _, s := range spec.Volumes[i].Projected.Sources {
				if s.ConfigMap != nil {
					s.ConfigMap.Name = substitution.ApplyReplacements(s.ConfigMap.Name, stringReplacements)
				}
				if s.Secret != nil {
					s.Secret.Name = substitution.ApplyReplacements(s.Secret.Name, stringReplacements)
				}
				if s.ServiceAccountToken != nil {
					s.ServiceAccountToken.Audience = substitution.ApplyReplacements(s.ServiceAccountToken.Audience, stringReplacements)
				}
			}
		}
		if v.CSI != nil {
			if v.CSI.NodePublishSecretRef != nil {
				spec.Volumes[i].CSI.NodePublishSecretRef.Name = substitution.ApplyReplacements(v.CSI.NodePublishSecretRef.Name, stringReplacements)
			}
			if v.CSI.VolumeAttributes != nil {
				for key, value := range v.CSI.VolumeAttributes {
					spec.Volumes[i].CSI.VolumeAttributes[key] = substitution.ApplyReplacements(value, stringReplacements)
				}
			}
		}
	}

	for i, v := range spec.Workspaces {
		spec.Workspaces[i].MountPath = substitution.ApplyReplacements(v.MountPath, stringReplacements)
	}

	// Apply variable substitution to the sidecar definitions
	sidecars := spec.Sidecars
	for i := range sidecars {
		v1beta1.ApplySidecarReplacements(&sidecars[i], stringReplacements, arrayReplacements)
	}

	return spec
}
