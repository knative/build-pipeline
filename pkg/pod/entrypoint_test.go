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
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"github.com/tektoncd/pipeline/pkg/entrypoint"
	"github.com/tektoncd/pipeline/test/diff"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakek8s "k8s.io/client-go/kubernetes/fake"
)

var volumeMount = corev1.VolumeMount{
	Name:      "my-mount",
	MountPath: "/mount/point",
}

func TestOrderContainers(t *testing.T) {
	steps := []corev1.Container{{
		Image:   "step-1",
		Command: []string{"cmd"},
		Args:    []string{"arg1", "arg2"},
	}, {
		Image:        "step-2",
		Command:      []string{"cmd1", "cmd2", "cmd3"}, // multiple cmd elements
		Args:         []string{"arg1", "arg2"},
		VolumeMounts: []corev1.VolumeMount{volumeMount}, // pre-existing volumeMount
	}, {
		Image:   "step-3",
		Command: []string{"cmd"},
		Args:    []string{"arg1", "arg2"},
	}}
	want := []corev1.Container{{
		Image:   "step-1",
		Command: []string{entrypointBinary},
		Args: []string{
			"-wait_file", "/tekton/downward/ready",
			"-wait_file_content",
			"-post_file", "/tekton/tools/0",
			"-termination_path", "/tekton/termination",
			"-step_path", "/tekton/steps/step-unnamed-0",
			"-step_path_link", "/tekton/steps/0",
			"-entrypoint", "cmd", "--",
			"arg1", "arg2",
		},
		VolumeMounts:           []corev1.VolumeMount{toolsMount, downwardMount},
		TerminationMessagePath: "/tekton/termination",
	}, {
		Image:   "step-2",
		Command: []string{entrypointBinary},
		Args: []string{
			"-wait_file", "/tekton/tools/0",
			"-post_file", "/tekton/tools/1",
			"-termination_path", "/tekton/termination",
			"-step_path", "/tekton/steps/step-unnamed-1",
			"-step_path_link", "/tekton/steps/1",
			"-entrypoint", "cmd1", "--",
			"cmd2", "cmd3",
			"arg1", "arg2",
		},
		VolumeMounts:           []corev1.VolumeMount{volumeMount, toolsMount},
		TerminationMessagePath: "/tekton/termination",
	}, {
		Image:   "step-3",
		Command: []string{entrypointBinary},
		Args: []string{
			"-wait_file", "/tekton/tools/1",
			"-post_file", "/tekton/tools/2",
			"-termination_path", "/tekton/termination",
			"-step_path", "/tekton/steps/step-unnamed-2",
			"-step_path_link", "/tekton/steps/2",
			"-entrypoint", "cmd", "--",
			"arg1", "arg2",
		},
		VolumeMounts:           []corev1.VolumeMount{toolsMount},
		TerminationMessagePath: "/tekton/termination",
	}}
	gotInit, got, err := orderContainers(images.EntrypointImage, []string{}, steps, nil, nil)
	if err != nil {
		t.Fatalf("orderContainers: %v", err)
	}
	if d := cmp.Diff(want, got); d != "" {
		t.Errorf("Diff %s", diff.PrintWantGot(d))
	}

	wantInit := corev1.Container{
		Name:         "place-tools",
		Image:        images.EntrypointImage,
		WorkingDir:   "/",
		Command:      []string{"/ko-app/entrypoint", "cp", "/ko-app/entrypoint", entrypointBinary},
		VolumeMounts: []corev1.VolumeMount{toolsMount},
	}
	if d := cmp.Diff(wantInit, gotInit); d != "" {
		t.Errorf("Init Container Diff %s", diff.PrintWantGot(d))
	}
}

func TestOrderContainersWithDebugOnFailure(t *testing.T) {
	steps := []corev1.Container{{
		Image:   "step-1",
		Command: []string{"cmd"},
		Args:    []string{"arg1", "arg2"},
	}}
	want := []corev1.Container{{
		Image:   "step-1",
		Command: []string{entrypointBinary},
		Args: []string{
			"-wait_file", "/tekton/downward/ready",
			"-wait_file_content",
			"-post_file", "/tekton/tools/0",
			"-termination_path", "/tekton/termination",
			"-step_path", "/tekton/steps/step-unnamed-0",
			"-step_path_link", "/tekton/steps/0",
			"-breakpoint_on_failure",
			"-entrypoint", "cmd", "--",
			"arg1", "arg2",
		},
		VolumeMounts:           []corev1.VolumeMount{toolsMount, downwardMount},
		TerminationMessagePath: "/tekton/termination",
	}}
	taskRunDebugConfig := &v1beta1.TaskRunDebug{
		Breakpoint: []string{"onFailure"},
	}
	_, got, err := orderContainers(images.EntrypointImage, []string{}, steps, nil, taskRunDebugConfig)
	if err != nil {
		t.Fatalf("orderContainers: %v", err)
	}
	if d := cmp.Diff(want, got); d != "" {
		t.Errorf("Diff %s", diff.PrintWantGot(d))
	}
}

func TestEntryPointResults(t *testing.T) {
	taskSpec := v1beta1.TaskSpec{
		Results: []v1beta1.TaskResult{{
			Name:        "sum",
			Description: "This is the sum result of the task",
		}, {
			Name:        "sub",
			Description: "This is the sub result of the task",
		}},
	}

	steps := []corev1.Container{{
		Image:   "step-1",
		Command: []string{"cmd"},
		Args:    []string{"arg1", "arg2"},
	}, {
		Image:        "step-2",
		Command:      []string{"cmd1", "cmd2", "cmd3"}, // multiple cmd elements
		Args:         []string{"arg1", "arg2"},
		VolumeMounts: []corev1.VolumeMount{volumeMount}, // pre-existing volumeMount
	}, {
		Image:   "step-3",
		Command: []string{"cmd"},
		Args:    []string{"arg1", "arg2"},
	}}
	want := []corev1.Container{{
		Image:   "step-1",
		Command: []string{entrypointBinary},
		Args: []string{
			"-wait_file", "/tekton/downward/ready",
			"-wait_file_content",
			"-post_file", "/tekton/tools/0",
			"-termination_path", "/tekton/termination",
			"-step_path", "/tekton/steps/step-unnamed-0",
			"-step_path_link", "/tekton/steps/0",
			"-results", "sum,sub",
			"-entrypoint", "cmd", "--",
			"arg1", "arg2",
		},
		VolumeMounts:           []corev1.VolumeMount{toolsMount, downwardMount},
		TerminationMessagePath: "/tekton/termination",
	}, {
		Image:   "step-2",
		Command: []string{entrypointBinary},
		Args: []string{
			"-wait_file", "/tekton/tools/0",
			"-post_file", "/tekton/tools/1",
			"-termination_path", "/tekton/termination",
			"-step_path", "/tekton/steps/step-unnamed-1",
			"-step_path_link", "/tekton/steps/1",
			"-results", "sum,sub",
			"-entrypoint", "cmd1", "--",
			"cmd2", "cmd3",
			"arg1", "arg2",
		},
		VolumeMounts:           []corev1.VolumeMount{volumeMount, toolsMount},
		TerminationMessagePath: "/tekton/termination",
	}, {
		Image:   "step-3",
		Command: []string{entrypointBinary},
		Args: []string{
			"-wait_file", "/tekton/tools/1",
			"-post_file", "/tekton/tools/2",
			"-termination_path", "/tekton/termination",
			"-step_path", "/tekton/steps/step-unnamed-2",
			"-step_path_link", "/tekton/steps/2",
			"-results", "sum,sub",
			"-entrypoint", "cmd", "--",
			"arg1", "arg2",
		},
		VolumeMounts:           []corev1.VolumeMount{toolsMount},
		TerminationMessagePath: "/tekton/termination",
	}}
	_, got, err := orderContainers(images.EntrypointImage, []string{}, steps, &taskSpec, nil)
	if err != nil {
		t.Fatalf("orderContainers: %v", err)
	}
	if d := cmp.Diff(want, got); d != "" {
		t.Errorf("Diff %s", diff.PrintWantGot(d))
	}
}

func TestEntryPointResultsSingleStep(t *testing.T) {
	taskSpec := v1beta1.TaskSpec{
		Results: []v1beta1.TaskResult{{
			Name:        "sum",
			Description: "This is the sum result of the task",
		}, {
			Name:        "sub",
			Description: "This is the sub result of the task",
		}},
	}

	steps := []corev1.Container{{
		Image:   "step-1",
		Command: []string{"cmd"},
		Args:    []string{"arg1", "arg2"},
	}}
	want := []corev1.Container{{
		Image:   "step-1",
		Command: []string{entrypointBinary},
		Args: []string{
			"-wait_file", "/tekton/downward/ready",
			"-wait_file_content",
			"-post_file", "/tekton/tools/0",
			"-termination_path", "/tekton/termination",
			"-step_path", "/tekton/steps/step-unnamed-0",
			"-step_path_link", "/tekton/steps/0",
			"-results", "sum,sub",
			"-entrypoint", "cmd", "--",
			"arg1", "arg2",
		},
		VolumeMounts:           []corev1.VolumeMount{toolsMount, downwardMount},
		TerminationMessagePath: "/tekton/termination",
	}}
	_, got, err := orderContainers(images.EntrypointImage, []string{}, steps, &taskSpec, nil)
	if err != nil {
		t.Fatalf("orderContainers: %v", err)
	}
	if d := cmp.Diff(want, got); d != "" {
		t.Errorf("Diff %s", diff.PrintWantGot(d))
	}
}
func TestEntryPointSingleResultsSingleStep(t *testing.T) {
	taskSpec := v1beta1.TaskSpec{
		Results: []v1beta1.TaskResult{{
			Name:        "sum",
			Description: "This is the sum result of the task",
		}},
	}

	steps := []corev1.Container{{
		Image:   "step-1",
		Command: []string{"cmd"},
		Args:    []string{"arg1", "arg2"},
	}}
	want := []corev1.Container{{
		Image:   "step-1",
		Command: []string{entrypointBinary},
		Args: []string{
			"-wait_file", "/tekton/downward/ready",
			"-wait_file_content",
			"-post_file", "/tekton/tools/0",
			"-termination_path", "/tekton/termination",
			"-step_path", "/tekton/steps/step-unnamed-0",
			"-step_path_link", "/tekton/steps/0",
			"-results", "sum",
			"-entrypoint", "cmd", "--",
			"arg1", "arg2",
		},
		VolumeMounts:           []corev1.VolumeMount{toolsMount, downwardMount},
		TerminationMessagePath: "/tekton/termination",
	}}
	_, got, err := orderContainers(images.EntrypointImage, []string{}, steps, &taskSpec, nil)
	if err != nil {
		t.Fatalf("orderContainers: %v", err)
	}
	if d := cmp.Diff(want, got); d != "" {
		t.Errorf("Diff %s", diff.PrintWantGot(d))
	}
}

func TestEntryPointOnError(t *testing.T) {
	taskSpec := v1beta1.TaskSpec{
		Steps: []v1beta1.Step{{
			OnError: entrypoint.ContinueOnError,
		}, {
			OnError: entrypoint.FailOnError,
		}},
	}

	steps := []corev1.Container{{
		Name:    "failing-step",
		Image:   "step-1",
		Command: []string{"cmd"},
	}, {
		Name:    "passing-step",
		Image:   "step-2",
		Command: []string{"cmd"},
	}}

	want := []corev1.Container{{
		Name:    "failing-step",
		Image:   "step-1",
		Command: []string{entrypointBinary},
		Args: []string{
			"-wait_file", "/tekton/downward/ready",
			"-wait_file_content",
			"-post_file", "/tekton/tools/0",
			"-termination_path", "/tekton/termination",
			"-step_path", "/tekton/steps/step-failing-step",
			"-step_path_link", "/tekton/steps/0",
			"-on_error", "continue",
			"-entrypoint", "cmd", "--",
		},
		VolumeMounts:           []corev1.VolumeMount{toolsMount, downwardMount},
		TerminationMessagePath: "/tekton/termination",
	}, {
		Name:    "passing-step",
		Image:   "step-2",
		Command: []string{entrypointBinary},
		Args: []string{
			"-wait_file", "/tekton/tools/0",
			"-post_file", "/tekton/tools/1",
			"-termination_path", "/tekton/termination",
			"-step_path", "/tekton/steps/step-passing-step",
			"-step_path_link", "/tekton/steps/1",
			"-on_error", "fail",
			"-entrypoint", "cmd", "--",
		},
		VolumeMounts:           []corev1.VolumeMount{toolsMount},
		TerminationMessagePath: "/tekton/termination",
	}}
	_, got, err := orderContainers(images.EntrypointImage, []string{}, steps, &taskSpec, nil)
	if err != nil {
		t.Fatalf("orderContainers: %v", err)
	}
	if d := cmp.Diff(want, got); d != "" {
		t.Errorf("Diff %s", diff.PrintWantGot(d))
	}
}

func TestUpdateReady(t *testing.T) {
	for _, c := range []struct {
		desc            string
		pod             corev1.Pod
		wantAnnotations map[string]string
		wantErr         bool
	}{{
		desc: "Pod without any annotations fails",
		pod: corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "pod",
				Annotations: nil,
			},
		},
		wantErr: true, // Nothing to replace.
	}, {
		desc: "Pod without ready annotation adds it",
		pod: corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "pod",
				Annotations: map[string]string{
					"something": "else",
				},
			},
		},
		wantAnnotations: map[string]string{
			"something":     "else",
			readyAnnotation: "READY",
		},
	}, {
		desc: "Pod with empty annotation value has it replaced",
		pod: corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "pod",
				Annotations: map[string]string{
					"something":     "else",
					readyAnnotation: "",
				},
			},
		},
		wantAnnotations: map[string]string{
			"something":     "else",
			readyAnnotation: readyAnnotationValue,
		},
	}, {
		desc: "Pod with other annotation value has it replaced",
		pod: corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "pod",
				Annotations: map[string]string{
					"something":     "else",
					readyAnnotation: "something else",
				},
			},
		},
		wantAnnotations: map[string]string{
			"something":     "else",
			readyAnnotation: readyAnnotationValue,
		},
	}} {
		t.Run(c.desc, func(t *testing.T) {
			ctx := context.Background()
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()
			kubeclient := fakek8s.NewSimpleClientset(&c.pod)
			if err := UpdateReady(ctx, kubeclient, c.pod); (err != nil) != c.wantErr {
				t.Errorf("UpdateReady (wantErr=%t): %v", c.wantErr, err)
			}

			got, err := kubeclient.CoreV1().Pods(c.pod.Namespace).Get(ctx, c.pod.Name, metav1.GetOptions{})
			if err != nil {
				t.Errorf("Getting pod %q after update: %v", c.pod.Name, err)
			} else if d := cmp.Diff(c.wantAnnotations, got.Annotations); d != "" {
				t.Errorf("Annotations Diff %s", diff.PrintWantGot(d))
			}
		})
	}
}

const nopImage = "nop-image"

// TestStopSidecars tests stopping sidecars by updating their images to a nop
// image.
func TestStopSidecars(t *testing.T) {
	stepContainer := corev1.Container{
		Name:  stepPrefix + "my-step",
		Image: "foo",
	}
	sidecarContainer := corev1.Container{
		Name:  sidecarPrefix + "my-sidecar",
		Image: "original-image",
	}
	stoppedSidecarContainer := corev1.Container{
		Name:  sidecarContainer.Name,
		Image: nopImage,
	}

	// This is a container that doesn't start with the "sidecar-" prefix,
	// which indicates it was injected into the Pod by a Mutating Webhook
	// Admission Controller. Injected sidecars should also be stopped if
	// they're running.
	injectedSidecar := corev1.Container{
		Name:  "injected",
		Image: "original-injected-image",
	}
	stoppedInjectedSidecar := corev1.Container{
		Name:  injectedSidecar.Name,
		Image: nopImage,
	}

	for _, c := range []struct {
		desc           string
		pod            corev1.Pod
		wantContainers []corev1.Container
	}{{
		desc: "Running sidecars (incl injected) should be stopped",
		pod: corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-pod",
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{stepContainer, sidecarContainer, injectedSidecar},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{{
					// Step state doesn't matter.
				}, {
					Name: sidecarContainer.Name,
					// Sidecar is running.
					State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{StartedAt: metav1.NewTime(time.Now())}},
				}, {
					Name: injectedSidecar.Name,
					// Injected sidecar is running.
					State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{StartedAt: metav1.NewTime(time.Now())}},
				}},
			},
		},
		wantContainers: []corev1.Container{stepContainer, stoppedSidecarContainer, stoppedInjectedSidecar},
	}, {
		desc: "Pending Pod should not be updated",
		pod: corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{stepContainer, sidecarContainer},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodPending,
				// Container states don't matter.
			},
		},
		wantContainers: []corev1.Container{stepContainer, sidecarContainer},
	}, {
		desc: "Non-Running sidecars should not be stopped",
		pod: corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-pod",
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{stepContainer, sidecarContainer, injectedSidecar},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{{
					// Step state doesn't matter.
				}, {
					Name: sidecarContainer.Name,
					// Sidecar is waiting.
					State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{}},
				}, {
					Name: injectedSidecar.Name,
					// Injected sidecar is waiting.
					State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{}},
				}},
			},
		},
		wantContainers: []corev1.Container{stepContainer, sidecarContainer, injectedSidecar},
	}} {
		t.Run(c.desc, func(t *testing.T) {
			ctx := context.Background()
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()
			kubeclient := fakek8s.NewSimpleClientset(&c.pod)
			if got, err := StopSidecars(ctx, nopImage, kubeclient, c.pod.Namespace, c.pod.Name); err != nil {
				t.Errorf("error stopping sidecar: %v", err)
			} else if d := cmp.Diff(c.wantContainers, got.Spec.Containers); d != "" {
				t.Errorf("Containers Diff %s", diff.PrintWantGot(d))
			}
		})
	}
}
