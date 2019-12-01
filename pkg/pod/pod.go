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
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"path/filepath"

	"github.com/tektoncd/pipeline/pkg/apis/pipeline"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1alpha1"
	"github.com/tektoncd/pipeline/pkg/names"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
)

const (
	workspaceDir = "/workspace"
	homeDir      = "/tekton/home"
	oldHomeDir   = "/builder/home"

	taskRunLabelKey     = pipeline.GroupName + pipeline.TaskRunLabelKey
	ManagedByLabelKey   = "app.kubernetes.io/managed-by"
	ManagedByLabelValue = "tekton-pipelines"
)

// These are effectively const, but Go doesn't have such an annotation.
var (
	groupVersionKind = schema.GroupVersionKind{
		Group:   v1alpha1.SchemeGroupVersion.Group,
		Version: v1alpha1.SchemeGroupVersion.Version,
		Kind:    "TaskRun",
	}
	// These are injected into all of the source/step containers.
	implicitEnvVars = []corev1.EnvVar{{
		Name:  "HOME",
		Value: homeDir,
	}}
	implicitVolumeMounts = []corev1.VolumeMount{{
		Name:      "workspace",
		MountPath: workspaceDir,
	}, {
		Name:      "tekton-home",
		MountPath: homeDir,
	}, {
		// Mount the home Volume to both /tekton/home and (old,
		// deprecated) /builder/home.
		// TODO(#1633): After v0.10, we can remove this old path.
		Name:      "tekton-home",
		MountPath: oldHomeDir,
	}}
	implicitVolumes = []corev1.Volume{{
		Name:         "workspace",
		VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
	}, {
		Name:         "tekton-home",
		VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
	}}

	zeroQty = resource.MustParse("0")

	// Random byte reader used for pod name generation.
	// var for testing.
	randReader = rand.Reader
)

// MakePod converts TaskRun and TaskSpec objects to a Pod which implements the taskrun specified
// by the supplied CRD.
func MakePod(images pipeline.Images, taskRun *v1alpha1.TaskRun, taskSpec v1alpha1.TaskSpec, kubeclient kubernetes.Interface, entrypointCache EntrypointCache) (*corev1.Pod, error) {
	var initContainers []corev1.Container
	var volumes []corev1.Volume

	// Add our implicit volumes first, so they can be overridden by the user if they prefer.
	volumes = append(volumes, implicitVolumes...)

	// Inititalize any credentials found in annotated Secrets.
	if credsInitContainer, secretsVolumes, err := credsInit(images.CredsImage, taskRun.Spec.ServiceAccountName, taskRun.Namespace, kubeclient, implicitVolumeMounts, implicitEnvVars); err != nil {
		return nil, err
	} else if credsInitContainer != nil {
		initContainers = append(initContainers, *credsInitContainer)
		volumes = append(volumes, secretsVolumes...)
	}

	// Merge step template with steps.
	// TODO(#1605): Move MergeSteps to pkg/pod
	steps, err := v1alpha1.MergeStepsWithStepTemplate(taskSpec.StepTemplate, taskSpec.Steps)
	if err != nil {
		return nil, err
	}

	// Convert any steps with Script to command+args.
	// If any are found, append an init container to initialize scripts.
	scriptsInit, stepContainers := convertScripts(images.ShellImage, steps)
	if scriptsInit != nil {
		initContainers = append(initContainers, *scriptsInit)
		volumes = append(volumes, scriptsVolume)
	}

	// Initialize any workingDirs under /workspace.
	if workingDirInit := workingDirInit(images.ShellImage, stepContainers, implicitVolumeMounts); workingDirInit != nil {
		initContainers = append(initContainers, *workingDirInit)
	}

	// Resolve entrypoint for any steps that don't specify command.
	stepContainers, err = resolveEntrypoints(entrypointCache, taskRun.Namespace, taskRun.Spec.ServiceAccountName, stepContainers)
	if err != nil {
		return nil, err
	}

	// Rewrite steps with entrypoint binary. Append the entrypoint init
	// container to place the entrypoint binary.
	entrypointInit, stepContainers, err := orderContainers(images.EntrypointImage, stepContainers)
	if err != nil {
		return nil, err
	}
	initContainers = append(initContainers, entrypointInit)
	volumes = append(volumes, toolsVolume, downwardVolume)

	// Zero out non-max resource requests.
	// TODO(#1605): Split this out so it's more easily testable.
	maxIndicesByResource := findMaxResourceRequest(taskSpec.Steps, corev1.ResourceCPU, corev1.ResourceMemory, corev1.ResourceEphemeralStorage)
	for i := range stepContainers {
		zeroNonMaxResourceRequests(&stepContainers[i], i, maxIndicesByResource)
	}

	// Add implicit env vars.
	// Append to an empty list to ensure we don't alter implicitEnvVars.
	// Precedence: step.Env > taskRun.Spec.Env > implicitEnvVars
	for i, s := range stepContainers {
		env := append([]corev1.EnvVar{}, implicitEnvVars...)
		env = append(env, taskRun.Spec.Env...)
		env = append(env, s.Env...)
		stepContainers[i].Env = env
	}

	// Add implicit volume mounts to each step, unless the step specifies
	// its own volume mount at that path.
	for i, s := range stepContainers {
		requestedVolumeMounts := map[string]bool{}
		for _, vm := range s.VolumeMounts {
			requestedVolumeMounts[filepath.Clean(vm.MountPath)] = true
		}
		var toAdd []corev1.VolumeMount
		for _, imp := range implicitVolumeMounts {
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
	for i, s := range stepContainers {
		if s.WorkingDir == "" {
			stepContainers[i].WorkingDir = workspaceDir
		}
		if s.Name == "" {
			stepContainers[i].Name = names.SimpleNameGenerator.RestrictLength(fmt.Sprintf("%sunnamed-%d", stepPrefix, i))
		} else {
			stepContainers[i].Name = names.SimpleNameGenerator.RestrictLength(fmt.Sprintf("%s%s", stepPrefix, s.Name))
		}
	}

	// Add podTemplate Volumes to the explicitly declared use volumes
	volumes = append(volumes, taskSpec.Volumes...)
	volumes = append(volumes, taskRun.Spec.PodTemplate.Volumes...)

	if err := v1alpha1.ValidateVolumes(volumes); err != nil {
		return nil, err
	}

	// Generate a short random hex string.
	b, err := ioutil.ReadAll(io.LimitReader(randReader, 3))
	if err != nil {
		return nil, err
	}
	gibberish := hex.EncodeToString(b)

	// Merge sidecar containers with step containers.
	mergedPodContainers := stepContainers
	for _, sc := range taskSpec.Sidecars {
		sc.Name = names.SimpleNameGenerator.RestrictLength(fmt.Sprintf("%v%v", sidecarPrefix, sc.Name))
		mergedPodContainers = append(mergedPodContainers, sc)
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
			Name: fmt.Sprintf("%s-pod-%s", taskRun.Name, gibberish),
			// If our parent TaskRun is deleted, then we should be as well.
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(taskRun, groupVersionKind),
			},
			Annotations: taskRun.Annotations,
			Labels:      makeLabels(taskRun),
		},
		Spec: corev1.PodSpec{
			RestartPolicy:      corev1.RestartPolicyNever,
			InitContainers:     initContainers,
			Containers:         mergedPodContainers,
			ServiceAccountName: taskRun.Spec.ServiceAccountName,
			Volumes:            volumes,
			NodeSelector:       taskRun.Spec.PodTemplate.NodeSelector,
			Tolerations:        taskRun.Spec.PodTemplate.Tolerations,
			Affinity:           taskRun.Spec.PodTemplate.Affinity,
			SecurityContext:    taskRun.Spec.PodTemplate.SecurityContext,
			RuntimeClassName:   taskRun.Spec.PodTemplate.RuntimeClassName,
		},
	}, nil
}

// makeLabels constructs the labels we will propagate from TaskRuns to Pods.
func makeLabels(s *v1alpha1.TaskRun) map[string]string {
	labels := make(map[string]string, len(s.ObjectMeta.Labels)+1)
	// NB: Set this *before* passing through TaskRun labels. If the TaskRun
	// has a managed-by label, it should override this default.

	// Copy through the TaskRun's labels to the underlying Pod's.
	labels[ManagedByLabelKey] = ManagedByLabelValue
	for k, v := range s.ObjectMeta.Labels {
		labels[k] = v
	}

	// NB: Set this *after* passing through TaskRun Labels. If the TaskRun
	// specifies this label, it should be overridden by this value.
	labels[taskRunLabelKey] = s.Name
	return labels
}

// zeroNonMaxResourceRequests zeroes out the container's cpu, memory, or
// ephemeral storage resource requests if the container does not have the
// largest request out of all containers in the pod. This is done because Tekton
// overwrites each container's entrypoint to make containers effectively execute
// one at a time, so we want pods to only request the maximum resources needed
// at any single point in time. If no container has an explicit resource
// request, all requests are set to 0.
func zeroNonMaxResourceRequests(c *corev1.Container, stepIndex int, maxIndicesByResource map[corev1.ResourceName]int) {
	if c.Resources.Requests == nil {
		c.Resources.Requests = corev1.ResourceList{}
	}
	for name, maxIdx := range maxIndicesByResource {
		if maxIdx != stepIndex {
			c.Resources.Requests[name] = zeroQty
		}
	}
}

// findMaxResourceRequest returns the index of the container with the maximum
// request for the given resource from among the given set of containers.
func findMaxResourceRequest(steps []v1alpha1.Step, resourceNames ...corev1.ResourceName) map[corev1.ResourceName]int {
	maxIdxs := make(map[corev1.ResourceName]int, len(resourceNames))
	maxReqs := make(map[corev1.ResourceName]resource.Quantity, len(resourceNames))
	for _, name := range resourceNames {
		maxIdxs[name] = -1
		maxReqs[name] = zeroQty
	}
	for i, s := range steps {
		for _, name := range resourceNames {
			maxReq := maxReqs[name]
			req, exists := s.Container.Resources.Requests[name]
			if exists && req.Cmp(maxReq) > 0 {
				maxIdxs[name] = i
				maxReqs[name] = req
			}
		}
	}
	return maxIdxs
}
