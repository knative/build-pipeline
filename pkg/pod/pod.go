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

package pod

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/tektoncd/pipeline/pkg/apis/config"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/pod"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"github.com/tektoncd/pipeline/pkg/names"
	"github.com/tektoncd/pipeline/pkg/version"
	"github.com/tektoncd/pipeline/pkg/workspace"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
)

const (
	// TaskRunLabelKey is the name of the label added to the Pod to identify the TaskRun
	TaskRunLabelKey = pipeline.GroupName + pipeline.TaskRunLabelKey

	// TektonHermeticEnvVar is the env var we set in containers to indicate they should be run hermetically
	TektonHermeticEnvVar = "TEKTON_HERMETIC"

	// ExecutionModeAnnotation is an experimental optional annotation to set the execution mode on a TaskRun
	ExecutionModeAnnotation = "experimental.tekton.dev/execution-mode"

	// ExecutionModeHermetic indicates hermetic execution mode
	ExecutionModeHermetic = "hermetic"
)

// These are effectively const, but Go doesn't have such an annotation.
var (
	ReleaseAnnotation = "pipeline.tekton.dev/release"

	groupVersionKind = schema.GroupVersionKind{
		Group:   v1beta1.SchemeGroupVersion.Group,
		Version: v1beta1.SchemeGroupVersion.Version,
		Kind:    "TaskRun",
	}
	// These are injected into all of the source/step containers.
	implicitVolumeMounts = []corev1.VolumeMount{{
		Name:      "tekton-internal-workspace",
		MountPath: pipeline.WorkspaceDir,
	}, {
		Name:      "tekton-internal-home",
		MountPath: pipeline.HomeDir,
	}, {
		Name:      "tekton-internal-results",
		MountPath: pipeline.DefaultResultPath,
	}, {
		Name:      "tekton-internal-steps",
		MountPath: pipeline.StepsPath,
	}}
	implicitVolumes = []corev1.Volume{{
		Name:         "tekton-internal-workspace",
		VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
	}, {
		Name:         "tekton-internal-home",
		VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
	}, {
		Name:         "tekton-internal-results",
		VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
	}, {
		Name:         "tekton-internal-steps",
		VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
	}}
)

// Builder exposes options to configure Pod construction from TaskSpecs/Runs.
type Builder struct {
	Images          pipeline.Images
	KubeClient      kubernetes.Interface
	EntrypointCache EntrypointCache
	OverrideHomeEnv bool
}

// Build creates a Pod using the configuration options set on b and the TaskRun
// and TaskSpec provided in its arguments. An error is returned if there are
// any problems during the conversion.
func (b *Builder) Build(ctx context.Context, taskRun *v1beta1.TaskRun, taskSpec v1beta1.TaskSpec) (*corev1.Pod, error) {
	var (
		scriptsInit                                       *corev1.Container
		entrypointInit                                    corev1.Container
		initContainers, stepContainers, sidecarContainers []corev1.Container
		volumes                                           []corev1.Volume
		volumeMounts                                      []corev1.VolumeMount
	)
	implicitEnvVars := []corev1.EnvVar{}
	alphaAPIEnabled := config.FromContextOrDefaults(ctx).FeatureFlags.EnableAPIFields == config.AlphaAPIFields

	// Add our implicit volumes first, so they can be overridden by the user if they prefer.
	volumes = append(volumes, implicitVolumes...)
	volumeMounts = append(volumeMounts, implicitVolumeMounts...)

	if b.OverrideHomeEnv {
		implicitEnvVars = append(implicitEnvVars, corev1.EnvVar{
			Name:  "HOME",
			Value: pipeline.HomeDir,
		})
	}

	// Create Volumes and VolumeMounts for any credentials found in annotated
	// Secrets, along with any arguments needed by Step entrypoints to process
	// those secrets.
	credEntrypointArgs, credVolumes, credVolumeMounts, err := credsInit(ctx, taskRun.Spec.ServiceAccountName, taskRun.Namespace, b.KubeClient)
	if err != nil {
		return nil, err
	}
	volumes = append(volumes, credVolumes...)
	volumeMounts = append(volumeMounts, credVolumeMounts...)

	// Merge step template with steps.
	// TODO(#1605): Move MergeSteps to pkg/pod
	steps, err := v1beta1.MergeStepsWithStepTemplate(taskSpec.StepTemplate, taskSpec.Steps)
	if err != nil {
		return nil, err
	}

	// Convert any steps with Script to command+args.
	// If any are found, append an init container to initialize scripts.
	if alphaAPIEnabled {
		scriptsInit, stepContainers, sidecarContainers = convertScripts(b.Images.ShellImage, steps, taskSpec.Sidecars, taskRun.Spec.Debug)
	} else {
		scriptsInit, stepContainers, sidecarContainers = convertScripts(b.Images.ShellImage, steps, taskSpec.Sidecars, nil)
	}
	if scriptsInit != nil {
		initContainers = append(initContainers, *scriptsInit)
		volumes = append(volumes, scriptsVolume)
	}

	if alphaAPIEnabled && taskRun.Spec.Debug != nil {
		volumes = append(volumes, debugScriptsVolume, debugInfoVolume)
	}

	// Initialize any workingDirs under /workspace.
	if workingDirInit := workingDirInit(b.Images.ShellImage, stepContainers); workingDirInit != nil {
		initContainers = append(initContainers, *workingDirInit)
	}

	// Resolve entrypoint for any steps that don't specify command.
	stepContainers, err = resolveEntrypoints(ctx, b.EntrypointCache, taskRun.Namespace, taskRun.Spec.ServiceAccountName, stepContainers)
	if err != nil {
		return nil, err
	}

	// Rewrite steps with entrypoint binary. Append the entrypoint init
	// container to place the entrypoint binary. Also add timeout flags
	// to entrypoint binary.
	if alphaAPIEnabled {
		entrypointInit, stepContainers, err = orderContainers(b.Images.EntrypointImage, credEntrypointArgs, stepContainers, &taskSpec, taskRun.Spec.Debug)
	} else {
		entrypointInit, stepContainers, err = orderContainers(b.Images.EntrypointImage, credEntrypointArgs, stepContainers, &taskSpec, nil)
	}
	if err != nil {
		return nil, err
	}
	// place the entrypoint first in case other init containers rely on its
	// features (e.g. decode-script).
	initContainers = append([]corev1.Container{entrypointInit}, initContainers...)
	volumes = append(volumes, toolsVolume, downwardVolume)

	limitRangeMin, err := getLimitRangeMinimum(ctx, taskRun.Namespace, b.KubeClient)
	if err != nil {
		return nil, err
	}

	// Zero out non-max resource requests.
	stepContainers = resolveResourceRequests(stepContainers, limitRangeMin)

	// Add implicit env vars.
	// They're prepended to the list, so that if the user specified any
	// themselves their value takes precedence.
	if len(implicitEnvVars) > 0 {
		for i, s := range stepContainers {
			env := append(implicitEnvVars, s.Env...)
			stepContainers[i].Env = env
		}
	}

	// Add env var if hermetic execution was requested & if the alpha API is enabled
	if taskRun.Annotations[ExecutionModeAnnotation] == ExecutionModeHermetic && alphaAPIEnabled {
		for i, s := range stepContainers {
			// Add it at the end so it overrides
			env := append(s.Env, corev1.EnvVar{Name: TektonHermeticEnvVar, Value: "1"})
			stepContainers[i].Env = env
		}
	}

	// Add implicit volume mounts to each step, unless the step specifies
	// its own volume mount at that path.
	for i, s := range stepContainers {
		// Mount /tekton/creds with a fresh volume for each Step. It needs to
		// be world-writeable and empty so creds can be initialized in there. Cant
		// guarantee what UID container runs with. If legacy credential helper (creds-init)
		// is disabled via feature flag then these can be nil since we don't want to mount
		// the automatic credential volume.
		v, vm := getCredsInitVolume(ctx, i)
		if v != nil && vm != nil {
			volumes = append(volumes, *v)
			s.VolumeMounts = append(s.VolumeMounts, *vm)
		}

		requestedVolumeMounts := map[string]bool{}
		for _, vm := range s.VolumeMounts {
			requestedVolumeMounts[filepath.Clean(vm.MountPath)] = true
		}
		var toAdd []corev1.VolumeMount
		for _, imp := range volumeMounts {
			if !requestedVolumeMounts[filepath.Clean(imp.MountPath)] {
				toAdd = append(toAdd, imp)
			}
		}
		vms := append(s.VolumeMounts, toAdd...)
		stepContainers[i].VolumeMounts = vms
	}

	// This loop:
	// - defaults workingDir to /workspace
	// - sets container name to add "step-" prefix or "step-unnamed-#" if not specified.
	// TODO(#1605): Remove this loop and make each transformation in
	// isolation.
	shouldOverrideWorkingDir := shouldOverrideWorkingDir(ctx)
	for i, s := range stepContainers {
		if s.WorkingDir == "" && shouldOverrideWorkingDir {
			stepContainers[i].WorkingDir = pipeline.WorkspaceDir
		}
		stepContainers[i].Name = names.SimpleNameGenerator.RestrictLength(StepName(s.Name, i))
	}

	// By default, use an empty pod template and take the one defined in the task run spec if any
	podTemplate := pod.Template{}

	if taskRun.Spec.PodTemplate != nil {
		podTemplate = *taskRun.Spec.PodTemplate
	}

	// Add podTemplate Volumes to the explicitly declared use volumes
	volumes = append(volumes, taskSpec.Volumes...)
	volumes = append(volumes, podTemplate.Volumes...)

	if err := v1beta1.ValidateVolumes(volumes); err != nil {
		return nil, err
	}

	// Using node affinity on taskRuns sharing PVC workspace, with an Affinity Assistant
	// is mutually exclusive with other affinity on taskRun pods. If other
	// affinity is wanted, that should be added on the Affinity Assistant pod unless
	// assistant is disabled. When Affinity Assistant is disabled, an affinityAssistantName is not set.
	var affinity *corev1.Affinity
	if affinityAssistantName := taskRun.Annotations[workspace.AnnotationAffinityAssistantName]; affinityAssistantName != "" {
		affinity = nodeAffinityUsingAffinityAssistant(affinityAssistantName)
	} else {
		affinity = podTemplate.Affinity
	}

	mergedPodContainers := stepContainers

	// Merge sidecar containers with step containers.
	for _, sc := range sidecarContainers {
		sc.Name = names.SimpleNameGenerator.RestrictLength(fmt.Sprintf("%v%v", sidecarPrefix, sc.Name))
		mergedPodContainers = append(mergedPodContainers, sc)
	}

	var dnsPolicy corev1.DNSPolicy
	if podTemplate.DNSPolicy != nil {
		dnsPolicy = *podTemplate.DNSPolicy
	}

	var priorityClassName string
	if podTemplate.PriorityClassName != nil {
		priorityClassName = *podTemplate.PriorityClassName
	}

	podAnnotations := taskRun.Annotations
	podAnnotations[ReleaseAnnotation] = version.PipelineVersion

	if shouldAddReadyAnnotationOnPodCreate(ctx, taskSpec.Sidecars) {
		podAnnotations[readyAnnotation] = readyAnnotationValue
	}

	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			// We execute the build's pod in the same namespace as where the build was
			// created so that it can access colocated resources.
			Namespace: taskRun.Namespace,
			// Generate a unique name based on the build's name.
			// Add a unique suffix to avoid confusion when a build
			// is deleted and re-created with the same name.
			// We don't use RestrictLengthWithRandomSuffix here because k8s fakes don't support it.
			Name: names.SimpleNameGenerator.RestrictLengthWithRandomSuffix(fmt.Sprintf("%s-pod", taskRun.Name)),
			// If our parent TaskRun is deleted, then we should be as well.
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(taskRun, groupVersionKind),
			},
			Annotations: podAnnotations,
			Labels:      makeLabels(taskRun),
		},
		Spec: corev1.PodSpec{
			RestartPolicy:                corev1.RestartPolicyNever,
			InitContainers:               initContainers,
			Containers:                   mergedPodContainers,
			ServiceAccountName:           taskRun.Spec.ServiceAccountName,
			Volumes:                      volumes,
			NodeSelector:                 podTemplate.NodeSelector,
			Tolerations:                  podTemplate.Tolerations,
			Affinity:                     affinity,
			SecurityContext:              podTemplate.SecurityContext,
			RuntimeClassName:             podTemplate.RuntimeClassName,
			AutomountServiceAccountToken: podTemplate.AutomountServiceAccountToken,
			SchedulerName:                podTemplate.SchedulerName,
			HostNetwork:                  podTemplate.HostNetwork,
			DNSPolicy:                    dnsPolicy,
			DNSConfig:                    podTemplate.DNSConfig,
			EnableServiceLinks:           podTemplate.EnableServiceLinks,
			PriorityClassName:            priorityClassName,
			ImagePullSecrets:             podTemplate.ImagePullSecrets,
			HostAliases:                  podTemplate.HostAliases,
		},
	}, nil
}

// makeLabels constructs the labels we will propagate from TaskRuns to Pods.
func makeLabels(s *v1beta1.TaskRun) map[string]string {
	labels := make(map[string]string, len(s.ObjectMeta.Labels)+1)
	// NB: Set this *before* passing through TaskRun labels. If the TaskRun
	// has a managed-by label, it should override this default.

	// Copy through the TaskRun's labels to the underlying Pod's.
	for k, v := range s.ObjectMeta.Labels {
		labels[k] = v
	}

	// NB: Set this *after* passing through TaskRun Labels. If the TaskRun
	// specifies this label, it should be overridden by this value.
	labels[TaskRunLabelKey] = s.Name
	return labels
}

// nodeAffinityUsingAffinityAssistant achieves Node Affinity for taskRun pods
// sharing PVC workspace by setting PodAffinity so that taskRuns is
// scheduled to the Node were the Affinity Assistant pod is scheduled.
func nodeAffinityUsingAffinityAssistant(affinityAssistantName string) *corev1.Affinity {
	return &corev1.Affinity{
		PodAffinity: &corev1.PodAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{{
				LabelSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						workspace.LabelInstance:  affinityAssistantName,
						workspace.LabelComponent: workspace.ComponentNameAffinityAssistant,
					},
				},
				TopologyKey: "kubernetes.io/hostname",
			}},
		},
	}
}

// getLimitRangeMinimum gets all LimitRanges in a namespace and
// searches for if a container minimum is specified. Due to
// https://github.com/kubernetes/kubernetes/issues/79496, the
// max LimitRange minimum must be found in the event of conflicting
// container minimums specified.
func getLimitRangeMinimum(ctx context.Context, namespace string, kubeclient kubernetes.Interface) (corev1.ResourceList, error) {
	limitRanges, err := kubeclient.CoreV1().LimitRanges(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	min := allZeroQty()
	for _, lr := range limitRanges.Items {
		lrItems := lr.Spec.Limits
		for _, lrItem := range lrItems {
			if lrItem.Type == corev1.LimitTypeContainer {
				if lrItem.Min != nil {
					for k, v := range lrItem.Min {
						if v.Cmp(min[k]) > 0 {
							min[k] = v
						}
					}
				}
			}
		}
	}

	return min, nil
}

// ShouldOverrideHomeEnv returns a bool indicating whether a Pod should have its
// $HOME environment variable overwritten with /tekton/home or if it should be
// left unmodified. The default behaviour is to overwrite the $HOME variable
// but this is planned to change in an upcoming release.
//
// For further reference see https://github.com/tektoncd/pipeline/issues/2013
func ShouldOverrideHomeEnv(ctx context.Context) bool {
	cfg := config.FromContextOrDefaults(ctx)
	return !cfg.FeatureFlags.DisableHomeEnvOverwrite
}

// shouldOverrideWorkingDir returns a bool indicating whether a Pod should have its
// working directory overwritten with /workspace or if it should be
// left unmodified. The default behaviour is to overwrite the working directory with '/workspace'
// if not specified by the user,  but this is planned to change in an upcoming release.
//
// For further reference see https://github.com/tektoncd/pipeline/issues/1836
func shouldOverrideWorkingDir(ctx context.Context) bool {
	cfg := config.FromContextOrDefaults(ctx)
	return !cfg.FeatureFlags.DisableWorkingDirOverwrite
}

// shouldAddReadyAnnotationonPodCreate returns a bool indicating whether the
// controller should add the `Ready` annotation when creating the Pod. We cannot
// add the annotation if Tekton is running in a cluster with injected sidecars
// or if the Task specifies any sidecars.
func shouldAddReadyAnnotationOnPodCreate(ctx context.Context, sidecars []v1beta1.Sidecar) bool {
	// If the TaskRun has sidecars, we cannot set the READY annotation early
	if len(sidecars) > 0 {
		return false
	}
	// If the TaskRun has no sidecars, check if we are running in a cluster where sidecars can be injected by other
	// controllers.
	cfg := config.FromContextOrDefaults(ctx)
	return !cfg.FeatureFlags.RunningInEnvWithInjectedSidecars
}
