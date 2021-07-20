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
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/tektoncd/pipeline/pkg/apis/config"
	"github.com/tektoncd/pipeline/pkg/apis/validate"
	"github.com/tektoncd/pipeline/pkg/substitution"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation"
	"knative.dev/pkg/apis"
)

var _ apis.Validatable = (*Task)(nil)

func (t *Task) Validate(ctx context.Context) *apis.FieldError {
	errs := validate.ObjectMetadata(t.GetObjectMeta()).ViaField("metadata")
	if apis.IsInDelete(ctx) {
		return nil
	}
	return errs.Also(t.Spec.Validate(apis.WithinSpec(ctx)).ViaField("spec"))
}

func (ts *TaskSpec) Validate(ctx context.Context) (errs *apis.FieldError) {
	if len(ts.Steps) == 0 {
		errs = errs.Also(apis.ErrMissingField("steps"))
	}
	errs = errs.Also(ValidateVolumes(ts.Volumes).ViaField("volumes"))
	errs = errs.Also(validateDeclaredWorkspaces(ts.Workspaces, ts.Steps, ts.StepTemplate).ViaField("workspaces"))
	errs = errs.Also(validateWorkspaceUsages(ctx, ts))
	mergedSteps, err := MergeStepsWithStepTemplate(ts.StepTemplate, ts.Steps)
	if err != nil {
		errs = errs.Also(&apis.FieldError{
			Message: fmt.Sprintf("error merging step template and steps: %s", err),
			Paths:   []string{"stepTemplate"},
			Details: err.Error(),
		})
	}

	errs = errs.Also(validateSteps(ctx, mergedSteps).ViaField("steps"))
	errs = errs.Also(ts.Resources.Validate(ctx).ViaField("resources"))
	errs = errs.Also(ValidateParameterTypes(ts.Params).ViaField("params"))
	errs = errs.Also(ValidateParameterVariables(ts.Steps, ts.Params))
	errs = errs.Also(ValidateResourcesVariables(ts.Steps, ts.Resources))
	errs = errs.Also(validateTaskContextVariables(ts.Steps))
	errs = errs.Also(validateResults(ctx, ts.Results).ViaField("results"))
	return errs
}

func validateResults(ctx context.Context, results []TaskResult) (errs *apis.FieldError) {
	for index, result := range results {
		errs = errs.Also(result.Validate(ctx).ViaIndex(index))
	}
	return errs
}

func (tr TaskResult) Validate(_ context.Context) *apis.FieldError {
	if !resultNameFormatRegex.MatchString(tr.Name) {
		return apis.ErrInvalidKeyName(tr.Name, "name", fmt.Sprintf("Name must consist of alphanumeric characters, '-', '_', and must start and end with an alphanumeric character (e.g. 'MyName',  or 'my-name',  or 'my_name', regex used for validation is '%s')", ResultNameFormat))
	}
	return nil
}

// a mount path which conflicts with any other declared workspaces, with the explicitly
// declared volume mounts, or with the stepTemplate. The names must also be unique.
func validateDeclaredWorkspaces(workspaces []WorkspaceDeclaration, steps []Step, stepTemplate *corev1.Container) (errs *apis.FieldError) {
	mountPaths := sets.NewString()
	for _, step := range steps {
		for _, vm := range step.VolumeMounts {
			mountPaths.Insert(filepath.Clean(vm.MountPath))
		}
	}
	if stepTemplate != nil {
		for _, vm := range stepTemplate.VolumeMounts {
			mountPaths.Insert(filepath.Clean(vm.MountPath))
		}
	}

	wsNames := sets.NewString()
	for idx, w := range workspaces {
		// Workspace names must be unique
		if wsNames.Has(w.Name) {
			errs = errs.Also(apis.ErrGeneric(fmt.Sprintf("workspace name %q must be unique", w.Name), "name").ViaIndex(idx))
		} else {
			wsNames.Insert(w.Name)
		}
		// Workspaces must not try to use mount paths that are already used
		mountPath := filepath.Clean(w.GetMountPath())
		if _, ok := mountPaths[mountPath]; ok {
			errs = errs.Also(apis.ErrGeneric(fmt.Sprintf("workspace mount path %q must be unique", mountPath), "mountpath").ViaIndex(idx))
		}
		mountPaths[mountPath] = struct{}{}
	}
	return errs
}

// validateWorkspaceUsages checks that all WorkspaceUsage objects in Steps
// refer to workspaces that are defined in the Task.
//
// This is an alpha feature and will fail validation if it's used by a step
// or sidecar when the enable-api-fields feature gate is anything but "alpha".
func validateWorkspaceUsages(ctx context.Context, ts *TaskSpec) (errs *apis.FieldError) {
	workspaces := ts.Workspaces
	steps := ts.Steps
	sidecars := ts.Sidecars

	wsNames := sets.NewString()
	for _, w := range workspaces {
		wsNames.Insert(w.Name)
	}

	for stepIdx, step := range steps {
		if len(step.Workspaces) != 0 {
			errs = errs.Also(ValidateEnabledAPIFields(ctx, "step workspaces", config.AlphaAPIFields).ViaIndex(stepIdx).ViaField("steps"))
		}
		for workspaceIdx, w := range step.Workspaces {
			if !wsNames.Has(w.Name) {
				errs = errs.Also(apis.ErrGeneric(fmt.Sprintf("undefined workspace %q", w.Name), "name").ViaIndex(workspaceIdx).ViaField("workspaces").ViaIndex(stepIdx).ViaField("steps"))
			}
		}
	}

	for sidecarIdx, sidecar := range sidecars {
		if len(sidecar.Workspaces) != 0 {
			errs = errs.Also(ValidateEnabledAPIFields(ctx, "sidecar workspaces", config.AlphaAPIFields).ViaIndex(sidecarIdx).ViaField("sidecars"))
		}
		for workspaceIdx, w := range sidecar.Workspaces {
			if !wsNames.Has(w.Name) {
				errs = errs.Also(apis.ErrGeneric(fmt.Sprintf("undefined workspace %q", w.Name), "name").ViaIndex(workspaceIdx).ViaField("workspaces").ViaIndex(sidecarIdx).ViaField("sidecars"))
			}
		}
	}

	return errs
}

func ValidateVolumes(volumes []corev1.Volume) (errs *apis.FieldError) {
	// Task must not have duplicate volume names.
	vols := sets.NewString()
	for idx, v := range volumes {
		if vols.Has(v.Name) {
			errs = errs.Also(apis.ErrGeneric(fmt.Sprintf("multiple volumes with same name %q", v.Name), "name").ViaIndex(idx))
		} else {
			vols.Insert(v.Name)
		}
	}
	return errs
}

func validateSteps(ctx context.Context, steps []Step) (errs *apis.FieldError) {
	// Task must not have duplicate step names.
	names := sets.NewString()
	for idx, s := range steps {
		errs = errs.Also(validateStep(ctx, s, names).ViaIndex(idx))
	}
	return errs
}

func validateStep(ctx context.Context, s Step, names sets.String) (errs *apis.FieldError) {
	if s.Image == "" {
		errs = errs.Also(apis.ErrMissingField("Image"))
	}

	if s.Script != "" {
		if len(s.Command) > 0 {
			errs = errs.Also(&apis.FieldError{
				Message: fmt.Sprintf("script cannot be used with command"),
				Paths:   []string{"script"},
			})
		}
	}

	if s.Name != "" {
		if names.Has(s.Name) {
			errs = errs.Also(apis.ErrInvalidValue(s.Name, "name"))
		}
		if e := validation.IsDNS1123Label(s.Name); len(e) > 0 {
			errs = errs.Also(&apis.FieldError{
				Message: fmt.Sprintf("invalid value %q", s.Name),
				Paths:   []string{"name"},
				Details: "Task step name must be a valid DNS Label, For more info refer to https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names",
			})
		}
		names.Insert(s.Name)
	}

	if s.Timeout != nil {
		if s.Timeout.Duration < time.Duration(0) {
			return apis.ErrInvalidValue(s.Timeout.Duration, "negative timeout")
		}
	}

	for j, vm := range s.VolumeMounts {
		if strings.HasPrefix(vm.MountPath, "/tekton/") &&
			!strings.HasPrefix(vm.MountPath, "/tekton/home") {
			errs = errs.Also(apis.ErrGeneric(fmt.Sprintf("volumeMount cannot be mounted under /tekton/ (volumeMount %q mounted at %q)", vm.Name, vm.MountPath), "mountPath").ViaFieldIndex("volumeMounts", j))
		}
		if strings.HasPrefix(vm.Name, "tekton-internal-") {
			errs = errs.Also(apis.ErrGeneric(fmt.Sprintf(`volumeMount name %q cannot start with "tekton-internal-"`, vm.Name), "name").ViaFieldIndex("volumeMounts", j))
		}
	}

	if s.OnError != "" {
		errs = errs.Also(ValidateEnabledAPIFields(ctx, "step onError", config.AlphaAPIFields).ViaField("steps"))
		if s.OnError != "continue" && s.OnError != "fail" {
			errs = errs.Also(&apis.FieldError{
				Message: fmt.Sprintf("invalid value: %v", s.OnError),
				Paths:   []string{"onError"},
				Details: "Task step onError must be either continue or fail",
			})
		}
	}
	return errs
}

func ValidateParameterTypes(params []ParamSpec) (errs *apis.FieldError) {
	for _, p := range params {
		errs = errs.Also(p.ValidateType())
	}
	return errs
}

func (p ParamSpec) ValidateType() *apis.FieldError {
	// Ensure param has a valid type.
	validType := false
	for _, allowedType := range AllParamTypes {
		if p.Type == allowedType {
			validType = true
		}
	}
	if !validType {
		return apis.ErrInvalidValue(p.Type, fmt.Sprintf("%s.type", p.Name))
	}

	// If a default value is provided, ensure its type matches param's declared type.
	if (p.Default != nil) && (p.Default.Type != p.Type) {
		return &apis.FieldError{
			Message: fmt.Sprintf(
				"\"%v\" type does not match default value's type: \"%v\"", p.Type, p.Default.Type),
			Paths: []string{
				fmt.Sprintf("%s.type", p.Name),
				fmt.Sprintf("%s.default.type", p.Name),
			},
		}
	}
	return nil
}

func ValidateParameterVariables(steps []Step, params []ParamSpec) *apis.FieldError {
	parameterNames := sets.NewString()
	arrayParameterNames := sets.NewString()

	for _, p := range params {
		parameterNames.Insert(p.Name)
		if p.Type == ParamTypeArray {
			arrayParameterNames.Insert(p.Name)
		}
	}

	errs := validateVariables(steps, "params", parameterNames)
	return errs.Also(validateArrayUsage(steps, "params", arrayParameterNames))
}

func validateTaskContextVariables(steps []Step) *apis.FieldError {
	taskRunContextNames := sets.NewString().Insert(
		"name",
		"namespace",
		"uid",
	)
	taskContextNames := sets.NewString().Insert(
		"name",
		"retry-count",
	)
	errs := validateVariables(steps, "context\\.taskRun", taskRunContextNames)
	return errs.Also(validateVariables(steps, "context\\.task", taskContextNames))
}

func ValidateResourcesVariables(steps []Step, resources *TaskResources) *apis.FieldError {
	if resources == nil {
		return nil
	}
	resourceNames := sets.NewString()
	if resources.Inputs != nil {
		for _, r := range resources.Inputs {
			resourceNames.Insert(r.Name)
		}
	}
	if resources.Outputs != nil {
		for _, r := range resources.Outputs {
			resourceNames.Insert(r.Name)
		}
	}
	return validateVariables(steps, "resources.(?:inputs|outputs)", resourceNames)
}

func validateArrayUsage(steps []Step, prefix string, vars sets.String) (errs *apis.FieldError) {
	for idx, step := range steps {
		errs = errs.Also(validateStepArrayUsage(step, prefix, vars)).ViaFieldIndex("steps", idx)
	}
	return errs
}

func validateStepArrayUsage(step Step, prefix string, vars sets.String) *apis.FieldError {
	errs := validateTaskNoArrayReferenced(step.Name, prefix, vars).ViaField("name")
	errs = errs.Also(validateTaskNoArrayReferenced(step.Image, prefix, vars).ViaField("image"))
	errs = errs.Also(validateTaskNoArrayReferenced(step.WorkingDir, prefix, vars).ViaField("workingDir"))
	errs = errs.Also(validateTaskNoArrayReferenced(step.Script, prefix, vars).ViaField("script"))
	for i, cmd := range step.Command {
		errs = errs.Also(validateTaskArraysIsolated(cmd, prefix, vars).ViaFieldIndex("command", i))
	}
	for i, arg := range step.Args {
		errs = errs.Also(validateTaskArraysIsolated(arg, prefix, vars).ViaFieldIndex("args", i))

	}
	for _, env := range step.Env {
		errs = errs.Also(validateTaskNoArrayReferenced(env.Value, prefix, vars).ViaFieldKey("env", env.Name))
	}
	for i, v := range step.VolumeMounts {
		errs = errs.Also(validateTaskNoArrayReferenced(v.Name, prefix, vars).ViaField("name").ViaFieldIndex("volumeMount", i))
		errs = errs.Also(validateTaskNoArrayReferenced(v.MountPath, prefix, vars).ViaField("mountPath").ViaFieldIndex("volumeMount", i))
		errs = errs.Also(validateTaskNoArrayReferenced(v.SubPath, prefix, vars).ViaField("subPath").ViaFieldIndex("volumeMount", i))
	}
	return errs
}

func validateVariables(steps []Step, prefix string, vars sets.String) (errs *apis.FieldError) {
	for idx, step := range steps {
		errs = errs.Also(validateStepVariables(step, prefix, vars).ViaFieldIndex("steps", idx))
	}
	return errs
}

func validateStepVariables(step Step, prefix string, vars sets.String) *apis.FieldError {
	errs := validateTaskVariable(step.Name, prefix, vars).ViaField("name")
	errs = errs.Also(validateTaskVariable(step.Image, prefix, vars).ViaField("image"))
	errs = errs.Also(validateTaskVariable(step.WorkingDir, prefix, vars).ViaField("workingDir"))
	errs = errs.Also(validateTaskVariable(step.Script, prefix, vars).ViaField("script"))
	for i, cmd := range step.Command {
		errs = errs.Also(validateTaskVariable(cmd, prefix, vars).ViaFieldIndex("command", i))
	}
	for i, arg := range step.Args {
		errs = errs.Also(validateTaskVariable(arg, prefix, vars).ViaFieldIndex("args", i))
	}
	for _, env := range step.Env {
		errs = errs.Also(validateTaskVariable(env.Value, prefix, vars).ViaFieldKey("env", env.Name))
	}
	for i, v := range step.VolumeMounts {
		errs = errs.Also(validateTaskVariable(v.Name, prefix, vars).ViaField("name").ViaFieldIndex("volumeMount", i))
		errs = errs.Also(validateTaskVariable(v.MountPath, prefix, vars).ViaField("MountPath").ViaFieldIndex("volumeMount", i))
		errs = errs.Also(validateTaskVariable(v.SubPath, prefix, vars).ViaField("SubPath").ViaFieldIndex("volumeMount", i))
	}
	return errs
}

func validateTaskVariable(value, prefix string, vars sets.String) *apis.FieldError {
	return substitution.ValidateVariableP(value, prefix, vars)
}

func validateTaskNoArrayReferenced(value, prefix string, arrayNames sets.String) *apis.FieldError {
	return substitution.ValidateVariableProhibitedP(value, prefix, arrayNames)
}

func validateTaskArraysIsolated(value, prefix string, arrayNames sets.String) *apis.FieldError {
	return substitution.ValidateVariableIsolatedP(value, prefix, arrayNames)
}
