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
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"github.com/tektoncd/pipeline/test/diff"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/apis"
	duckv1beta1 "knative.dev/pkg/apis/duck/v1beta1"
	"knative.dev/pkg/logging"
)

var ignoreVolatileTime = cmp.Comparer(func(_, _ apis.VolatileTime) bool { return true })

func TestMakeTaskRunStatus(t *testing.T) {
	for _, c := range []struct {
		desc      string
		podStatus corev1.PodStatus
		pod       corev1.Pod
		want      v1beta1.TaskRunStatus
	}{{
		desc:      "empty",
		podStatus: corev1.PodStatus{},

		want: v1beta1.TaskRunStatus{
			Status: statusRunning(),
			TaskRunStatusFields: v1beta1.TaskRunStatusFields{
				Steps:    []v1beta1.StepState{},
				Sidecars: []v1beta1.SidecarState{},
			},
		},
	}, {
		desc: "ignore-creds-init",
		podStatus: corev1.PodStatus{
			InitContainerStatuses: []corev1.ContainerStatus{{
				// creds-init; ignored
				ImageID: "ignored",
			}},
			ContainerStatuses: []corev1.ContainerStatus{{
				Name:    "step-state-name",
				ImageID: "",
				State: corev1.ContainerState{
					Terminated: &corev1.ContainerStateTerminated{
						ExitCode: 123,
					},
				},
			}},
		},
		want: v1beta1.TaskRunStatus{
			Status: statusRunning(),
			TaskRunStatusFields: v1beta1.TaskRunStatusFields{
				Steps: []v1beta1.StepState{{
					ContainerState: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							ExitCode: 123,
						}},
					Name:          "state-name",
					ContainerName: "step-state-name",
				}},
				Sidecars: []v1beta1.SidecarState{},
			},
		},
	}, {
		desc: "ignore-init-containers",
		podStatus: corev1.PodStatus{
			InitContainerStatuses: []corev1.ContainerStatus{{
				// creds-init; ignored.
				ImageID: "ignoreme",
			}, {
				// git-init; ignored.
				ImageID: "ignoreme",
			}},
			ContainerStatuses: []corev1.ContainerStatus{{
				Name:    "step-state-name",
				ImageID: "image-id",
				State: corev1.ContainerState{
					Terminated: &corev1.ContainerStateTerminated{
						ExitCode: 123,
					},
				},
			}},
		},
		want: v1beta1.TaskRunStatus{
			Status: statusRunning(),
			TaskRunStatusFields: v1beta1.TaskRunStatusFields{
				Steps: []v1beta1.StepState{{
					ContainerState: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							ExitCode: 123,
						}},
					Name:          "state-name",
					ContainerName: "step-state-name",
					ImageID:       "image-id",
				}},
				Sidecars: []v1beta1.SidecarState{},
			},
		},
	}, {
		desc: "success",
		podStatus: corev1.PodStatus{
			Phase: corev1.PodSucceeded,
			ContainerStatuses: []corev1.ContainerStatus{{
				Name: "step-step-push",
				State: corev1.ContainerState{
					Terminated: &corev1.ContainerStateTerminated{
						ExitCode: 0,
					},
				},
				ImageID: "image-id",
			}},
		},
		want: v1beta1.TaskRunStatus{
			Status: statusSuccess(),
			TaskRunStatusFields: v1beta1.TaskRunStatusFields{
				Steps: []v1beta1.StepState{{
					ContainerState: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							ExitCode: 0,
						}},
					Name:          "step-push",
					ContainerName: "step-step-push",
					ImageID:       "image-id",
				}},
				Sidecars: []v1beta1.SidecarState{},
				// We don't actually care about the time, just that it's not nil
				CompletionTime: &metav1.Time{Time: time.Now()},
			},
		},
	}, {
		desc: "running",
		podStatus: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{{
				Name: "step-running-step",
				State: corev1.ContainerState{
					Running: &corev1.ContainerStateRunning{},
				},
			}},
		},
		want: v1beta1.TaskRunStatus{
			Status: statusRunning(),
			TaskRunStatusFields: v1beta1.TaskRunStatusFields{
				Steps: []v1beta1.StepState{{
					ContainerState: corev1.ContainerState{
						Running: &corev1.ContainerStateRunning{},
					},
					Name:          "running-step",
					ContainerName: "step-running-step",
				}},
				Sidecars: []v1beta1.SidecarState{},
				// We don't actually care about the time, just that it's not nil
				RunAt: &metav1.Time{Time: time.Now()},
			},
		},
	}, {
		desc: "failure-terminated",
		podStatus: corev1.PodStatus{
			Phase: corev1.PodFailed,
			InitContainerStatuses: []corev1.ContainerStatus{{
				// creds-init status; ignored
				ImageID: "ignore-me",
			}},
			ContainerStatuses: []corev1.ContainerStatus{{
				Name:    "step-failure",
				ImageID: "image-id",
				State: corev1.ContainerState{
					Terminated: &corev1.ContainerStateTerminated{
						ExitCode: 123,
					},
				},
			}},
		},
		want: v1beta1.TaskRunStatus{
			Status: statusFailure("\"step-failure\" exited with code 123 (image: \"image-id\"); for logs run: kubectl -n foo logs pod -c step-failure\n"),
			TaskRunStatusFields: v1beta1.TaskRunStatusFields{
				Steps: []v1beta1.StepState{{
					ContainerState: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							ExitCode: 123,
						}},

					Name:          "failure",
					ContainerName: "step-failure",
					ImageID:       "image-id",
				}},
				Sidecars: []v1beta1.SidecarState{},
				// We don't actually care about the time, just that it's not nil
				CompletionTime: &metav1.Time{Time: time.Now()},
			},
		},
	}, {
		desc: "failure-message",
		podStatus: corev1.PodStatus{
			Phase:   corev1.PodFailed,
			Message: "boom",
		},
		want: v1beta1.TaskRunStatus{
			Status: statusFailure("boom"),
			TaskRunStatusFields: v1beta1.TaskRunStatusFields{
				Steps:    []v1beta1.StepState{},
				Sidecars: []v1beta1.SidecarState{},
				// We don't actually care about the time, just that it's not nil
				CompletionTime: &metav1.Time{Time: time.Now()},
			},
		},
	}, {
		desc: "failed with OOM",
		podStatus: corev1.PodStatus{
			Phase: corev1.PodSucceeded,
			ContainerStatuses: []corev1.ContainerStatus{{
				Name: "step-step-push",
				State: corev1.ContainerState{
					Terminated: &corev1.ContainerStateTerminated{
						Reason:   "OOMKilled",
						ExitCode: 0,
					},
				},
				ImageID: "image-id",
			}},
		},
		want: v1beta1.TaskRunStatus{
			Status: statusFailure("OOMKilled"),
			TaskRunStatusFields: v1beta1.TaskRunStatusFields{
				Steps: []v1beta1.StepState{{
					ContainerState: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							Reason:   "OOMKilled",
							ExitCode: 0,
						}},
					Name:          "step-push",
					ContainerName: "step-step-push",
					ImageID:       "image-id",
				}},
				Sidecars: []v1beta1.SidecarState{},
				// We don't actually care about the time, just that it's not nil
				CompletionTime: &metav1.Time{Time: time.Now()},
			},
		},
	}, {
		desc:      "failure-unspecified",
		podStatus: corev1.PodStatus{Phase: corev1.PodFailed},
		want: v1beta1.TaskRunStatus{
			Status: statusFailure("build failed for unspecified reasons."),
			TaskRunStatusFields: v1beta1.TaskRunStatusFields{
				Steps:    []v1beta1.StepState{},
				Sidecars: []v1beta1.SidecarState{},
				// We don't actually care about the time, just that it's not nil
				CompletionTime: &metav1.Time{Time: time.Now()},
			},
		},
	}, {
		desc: "pending-waiting-message",
		podStatus: corev1.PodStatus{
			Phase:                 corev1.PodPending,
			InitContainerStatuses: []corev1.ContainerStatus{{
				// creds-init status; ignored
			}},
			ContainerStatuses: []corev1.ContainerStatus{{
				Name: "step-status-name",
				State: corev1.ContainerState{
					Waiting: &corev1.ContainerStateWaiting{
						Message: "i'm pending",
					},
				},
			}},
		},
		want: v1beta1.TaskRunStatus{
			Status: statusPending("Pending", `build step "step-status-name" is pending with reason "i'm pending"`),
			TaskRunStatusFields: v1beta1.TaskRunStatusFields{
				Steps: []v1beta1.StepState{{
					ContainerState: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Message: "i'm pending",
						},
					},
					Name:          "status-name",
					ContainerName: "step-status-name",
				}},
				Sidecars: []v1beta1.SidecarState{},
			},
		},
	}, {
		desc: "pending-pod-condition",
		podStatus: corev1.PodStatus{
			Phase: corev1.PodPending,
			Conditions: []corev1.PodCondition{{
				Status:  corev1.ConditionUnknown,
				Type:    "the type",
				Message: "the message",
			}},
		},
		want: v1beta1.TaskRunStatus{
			Status: statusPending("Pending", `pod status "the type":"Unknown"; message: "the message"`),
			TaskRunStatusFields: v1beta1.TaskRunStatusFields{
				Steps:    []v1beta1.StepState{},
				Sidecars: []v1beta1.SidecarState{},
			},
		},
	}, {
		desc: "pending-message",
		podStatus: corev1.PodStatus{
			Phase:   corev1.PodPending,
			Message: "pod status message",
		},
		want: v1beta1.TaskRunStatus{
			Status: statusPending("Pending", "pod status message"),
			TaskRunStatusFields: v1beta1.TaskRunStatusFields{
				Steps:    []v1beta1.StepState{},
				Sidecars: []v1beta1.SidecarState{},
			},
		},
	}, {
		desc:      "pending-no-message",
		podStatus: corev1.PodStatus{Phase: corev1.PodPending},
		want: v1beta1.TaskRunStatus{
			Status: statusPending("Pending", "Pending"),
			TaskRunStatusFields: v1beta1.TaskRunStatusFields{
				Steps:    []v1beta1.StepState{},
				Sidecars: []v1beta1.SidecarState{},
			},
		},
	}, {
		desc: "pending-not-enough-node-resources",
		podStatus: corev1.PodStatus{
			Phase: corev1.PodPending,
			Conditions: []corev1.PodCondition{{
				Reason:  corev1.PodReasonUnschedulable,
				Message: "0/1 nodes are available: 1 Insufficient cpu.",
			}},
		},
		want: v1beta1.TaskRunStatus{
			Status: statusPending(ReasonExceededNodeResources, "TaskRun Pod exceeded available resources"),
			TaskRunStatusFields: v1beta1.TaskRunStatusFields{
				Steps:    []v1beta1.StepState{},
				Sidecars: []v1beta1.SidecarState{},
			},
		},
	}, {
		desc: "pending-CreateContainerConfigError",
		podStatus: corev1.PodStatus{
			Phase: corev1.PodPending,
			ContainerStatuses: []corev1.ContainerStatus{{
				State: corev1.ContainerState{
					Waiting: &corev1.ContainerStateWaiting{
						Reason: "CreateContainerConfigError",
					},
				},
			}},
		},
		want: v1beta1.TaskRunStatus{
			Status: statusPending(ReasonCreateContainerConfigError, "Pending"),
			TaskRunStatusFields: v1beta1.TaskRunStatusFields{
				Steps:    []v1beta1.StepState{},
				Sidecars: []v1beta1.SidecarState{},
			},
		},
	}, {
		desc: "with-sidecar-running",
		podStatus: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{{
				Name: "step-running-step",
				State: corev1.ContainerState{
					Running: &corev1.ContainerStateRunning{},
				},
			}, {
				Name:    "sidecar-running",
				ImageID: "image-id",
				State: corev1.ContainerState{
					Running: &corev1.ContainerStateRunning{},
				},
				Ready: true,
			}},
		},
		want: v1beta1.TaskRunStatus{
			Status: statusRunning(),
			TaskRunStatusFields: v1beta1.TaskRunStatusFields{
				Steps: []v1beta1.StepState{{
					ContainerState: corev1.ContainerState{
						Running: &corev1.ContainerStateRunning{},
					},
					Name:          "running-step",
					ContainerName: "step-running-step",
				}},
				Sidecars: []v1beta1.SidecarState{{
					ContainerState: corev1.ContainerState{
						Running: &corev1.ContainerStateRunning{},
					},
					Name:          "running",
					ImageID:       "image-id",
					ContainerName: "sidecar-running",
				}},
				// We don't actually care about the time, just that it's not nil
				RunAt: &metav1.Time{Time: time.Now()},
			},
		},
	}, {
		desc: "with-sidecar-waiting",
		podStatus: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{{
				Name: "step-waiting-step",
				State: corev1.ContainerState{
					Waiting: &corev1.ContainerStateWaiting{
						Reason:  "PodInitializing",
						Message: "PodInitializing",
					},
				},
			}, {
				Name:    "sidecar-waiting",
				ImageID: "image-id",
				State: corev1.ContainerState{
					Waiting: &corev1.ContainerStateWaiting{
						Reason:  "PodInitializing",
						Message: "PodInitializing",
					},
				},
				Ready: true,
			}},
		},
		want: v1beta1.TaskRunStatus{
			Status: statusRunning(),
			TaskRunStatusFields: v1beta1.TaskRunStatusFields{
				Steps: []v1beta1.StepState{{
					ContainerState: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason:  "PodInitializing",
							Message: "PodInitializing",
						},
					},
					Name:          "waiting-step",
					ContainerName: "step-waiting-step",
				}},
				Sidecars: []v1beta1.SidecarState{{
					ContainerState: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason:  "PodInitializing",
							Message: "PodInitializing",
						},
					},
					Name:          "waiting",
					ImageID:       "image-id",
					ContainerName: "sidecar-waiting",
				}},
				// We don't actually care about the time, just that it's not nil
				RunAt: &metav1.Time{Time: time.Now()},
			},
		},
	}, {
		desc: "with-sidecar-terminated",
		podStatus: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{{
				Name: "step-running-step",
				State: corev1.ContainerState{
					Running: &corev1.ContainerStateRunning{},
				},
			}, {
				Name:    "sidecar-error",
				ImageID: "image-id",
				State: corev1.ContainerState{
					Terminated: &corev1.ContainerStateTerminated{
						ExitCode: 1,
						Reason:   "Error",
						Message:  "Error",
					},
				},
				Ready: true,
			}},
		},
		want: v1beta1.TaskRunStatus{
			Status: statusRunning(),
			TaskRunStatusFields: v1beta1.TaskRunStatusFields{
				Steps: []v1beta1.StepState{{
					ContainerState: corev1.ContainerState{
						Running: &corev1.ContainerStateRunning{},
					},
					Name:          "running-step",
					ContainerName: "step-running-step",
				}},
				Sidecars: []v1beta1.SidecarState{{
					ContainerState: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							ExitCode: 1,
							Reason:   "Error",
							Message:  "Error",
						},
					},
					Name:          "error",
					ImageID:       "image-id",
					ContainerName: "sidecar-error",
				}},
				// We don't actually care about the time, just that it's not nil
				RunAt: &metav1.Time{Time: time.Now()},
			},
		},
	}, {
		desc: "image resource updated",
		podStatus: corev1.PodStatus{
			Phase: corev1.PodSucceeded,
			ContainerStatuses: []corev1.ContainerStatus{{
				Name: "step-foo",
				State: corev1.ContainerState{
					Terminated: &corev1.ContainerStateTerminated{
						Message: `[{"key":"digest","value":"sha256:12345","resourceRef":{"name":"source-image"}}]`,
					},
				},
			}},
		},
		want: v1beta1.TaskRunStatus{
			Status: statusSuccess(),
			TaskRunStatusFields: v1beta1.TaskRunStatusFields{
				Steps: []v1beta1.StepState{{
					ContainerState: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							Message: `[{"key":"digest","value":"sha256:12345","resourceRef":{"name":"source-image"}}]`,
						}},
					Name:          "foo",
					ContainerName: "step-foo",
				}},
				Sidecars: []v1beta1.SidecarState{},
				ResourcesResult: []v1beta1.PipelineResourceResult{{
					Key:         "digest",
					Value:       "sha256:12345",
					ResourceRef: &v1beta1.PipelineResourceRef{Name: "source-image"},
				}},
				// We don't actually care about the time, just that it's not nil
				CompletionTime: &metav1.Time{Time: time.Now()},
			},
		},
	}, {
		desc: "test result with pipeline result",
		podStatus: corev1.PodStatus{
			Phase: corev1.PodSucceeded,
			ContainerStatuses: []corev1.ContainerStatus{{
				Name: "step-bar",
				State: corev1.ContainerState{
					Terminated: &corev1.ContainerStateTerminated{
						Message: `[{"key":"resultName","value":"resultValue", "type": "TaskRunResult"}, {"key":"digest","value":"sha256:1234","resourceRef":{"name":"source-image"}}]`,
					},
				},
			}},
		},
		want: v1beta1.TaskRunStatus{
			Status: statusSuccess(),
			TaskRunStatusFields: v1beta1.TaskRunStatusFields{
				Steps: []v1beta1.StepState{{
					ContainerState: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							Message: `[{"key":"digest","value":"sha256:1234","resourceRef":{"name":"source-image"}},{"key":"resultName","value":"resultValue","type":"TaskRunResult"}]`,
						}},
					Name:          "bar",
					ContainerName: "step-bar",
				}},
				Sidecars: []v1beta1.SidecarState{},
				ResourcesResult: []v1beta1.PipelineResourceResult{{
					Key:         "digest",
					Value:       "sha256:1234",
					ResourceRef: &v1beta1.PipelineResourceRef{Name: "source-image"},
				}},
				TaskRunResults: []v1beta1.TaskRunResult{{
					Name:  "resultName",
					Value: "resultValue",
				}},
				// We don't actually care about the time, just that it's not nil
				CompletionTime: &metav1.Time{Time: time.Now()},
			},
		},
	}, {
		desc: "test result with pipeline result - no result type",
		podStatus: corev1.PodStatus{
			Phase: corev1.PodSucceeded,
			ContainerStatuses: []corev1.ContainerStatus{{
				Name: "step-banana",
				State: corev1.ContainerState{
					Terminated: &corev1.ContainerStateTerminated{
						Message: `[{"key":"resultName","value":"resultValue", "type": "TaskRunResult"}, {"key":"digest","value":"sha256:1234","resourceRef":{"name":"source-image"}}]`,
					},
				},
			}},
		},
		want: v1beta1.TaskRunStatus{
			Status: statusSuccess(),
			TaskRunStatusFields: v1beta1.TaskRunStatusFields{
				Steps: []v1beta1.StepState{{
					ContainerState: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							Message: `[{"key":"digest","value":"sha256:1234","resourceRef":{"name":"source-image"}},{"key":"resultName","value":"resultValue","type":"TaskRunResult"}]`,
						}},
					Name:          "banana",
					ContainerName: "step-banana",
				}},
				Sidecars: []v1beta1.SidecarState{},
				ResourcesResult: []v1beta1.PipelineResourceResult{{
					Key:         "digest",
					Value:       "sha256:1234",
					ResourceRef: &v1beta1.PipelineResourceRef{Name: "source-image"},
				}},
				TaskRunResults: []v1beta1.TaskRunResult{{
					Name:  "resultName",
					Value: "resultValue",
				}},
				// We don't actually care about the time, just that it's not nil
				CompletionTime: &metav1.Time{Time: time.Now()},
			},
		},
	}, {
		desc: "two test results",
		podStatus: corev1.PodStatus{
			Phase: corev1.PodSucceeded,
			ContainerStatuses: []corev1.ContainerStatus{{
				Name: "step-one",
				State: corev1.ContainerState{
					Terminated: &corev1.ContainerStateTerminated{
						Message: `[{"key":"resultNameOne","value":"resultValueOne", "type": "TaskRunResult"}, {"key":"resultNameTwo","value":"resultValueTwo", "type": "TaskRunResult"}]`,
					},
				},
			}, {
				Name: "step-two",
				State: corev1.ContainerState{
					Terminated: &corev1.ContainerStateTerminated{
						Message: `[{"key":"resultNameOne","value":"resultValueThree","type":"TaskRunResult"},{"key":"resultNameTwo","value":"resultValueTwo","type":"TaskRunResult"}]`,
					},
				},
			}},
		},
		want: v1beta1.TaskRunStatus{
			Status: statusSuccess(),
			TaskRunStatusFields: v1beta1.TaskRunStatusFields{
				Steps: []v1beta1.StepState{{
					ContainerState: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							Message: `[{"key":"resultNameOne","value":"resultValueOne","type":"TaskRunResult"},{"key":"resultNameTwo","value":"resultValueTwo","type":"TaskRunResult"}]`,
						}},
					Name:          "one",
					ContainerName: "step-one",
				}, {
					ContainerState: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							Message: `[{"key":"resultNameOne","value":"resultValueThree","type":"TaskRunResult"},{"key":"resultNameTwo","value":"resultValueTwo","type":"TaskRunResult"}]`,
						}},
					Name:          "two",
					ContainerName: "step-two",
				}},
				Sidecars: []v1beta1.SidecarState{},
				TaskRunResults: []v1beta1.TaskRunResult{{
					Name:  "resultNameOne",
					Value: "resultValueThree",
				}, {
					Name:  "resultNameTwo",
					Value: "resultValueTwo",
				}},
				// We don't actually care about the time, just that it's not nil
				CompletionTime: &metav1.Time{Time: time.Now()},
			},
		},
	}, {
		desc: "taskrun status set to failed if task fails",
		podStatus: corev1.PodStatus{
			Phase: corev1.PodFailed,
			ContainerStatuses: []corev1.ContainerStatus{{
				Name: "step-mango",
				State: corev1.ContainerState{
					Terminated: &corev1.ContainerStateTerminated{},
				},
			}},
		},
		want: v1beta1.TaskRunStatus{
			Status: statusFailure("build failed for unspecified reasons."),
			TaskRunStatusFields: v1beta1.TaskRunStatusFields{
				Steps: []v1beta1.StepState{{
					ContainerState: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{}},
					Name:          "mango",
					ContainerName: "step-mango",
				}},
				Sidecars: []v1beta1.SidecarState{},
				// We don't actually care about the time, just that it's not nil
				CompletionTime: &metav1.Time{Time: time.Now()},
			},
		},
	}, {
		desc: "termination message not adhering to pipelineresourceresult format is filtered from taskrun termination message",
		podStatus: corev1.PodStatus{
			Phase: corev1.PodSucceeded,
			ContainerStatuses: []corev1.ContainerStatus{{
				Name: "step-pineapple",
				State: corev1.ContainerState{
					Terminated: &corev1.ContainerStateTerminated{
						Message: `[{"invalid":"resultName","invalid":"resultValue"}]`,
					},
				},
			}},
		},
		want: v1beta1.TaskRunStatus{
			Status: statusSuccess(),
			TaskRunStatusFields: v1beta1.TaskRunStatusFields{
				Steps: []v1beta1.StepState{{
					ContainerState: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{}},
					Name:          "pineapple",
					ContainerName: "step-pineapple",
				}},
				Sidecars: []v1beta1.SidecarState{},
				// We don't actually care about the time, just that it's not nil
				CompletionTime: &metav1.Time{Time: time.Now()},
			},
		},
	}, {
		desc: "filter internaltektonresult",
		podStatus: corev1.PodStatus{
			Phase: corev1.PodSucceeded,
			ContainerStatuses: []corev1.ContainerStatus{{
				Name: "step-pear",
				State: corev1.ContainerState{
					Terminated: &corev1.ContainerStateTerminated{
						Message: `[{"key":"resultNameOne","value":"","type":"PipelineResourceResult"}, {"key":"resultNameTwo","value":"","type":"InternalTektonResult"}, {"key":"resultNameThree","value":"","type":"TaskRunResult"}]`,
					},
				},
			}},
		},
		want: v1beta1.TaskRunStatus{
			Status: statusSuccess(),
			TaskRunStatusFields: v1beta1.TaskRunStatusFields{
				Steps: []v1beta1.StepState{{
					ContainerState: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							Message: `[{"key":"resultNameOne","value":"","type":"PipelineResourceResult"},{"key":"resultNameThree","value":"","type":"TaskRunResult"}]`,
						}},
					Name:          "pear",
					ContainerName: "step-pear",
				}},
				Sidecars: []v1beta1.SidecarState{},
				ResourcesResult: []v1beta1.PipelineResourceResult{{
					Key:        "resultNameOne",
					Value:      "",
					ResultType: "PipelineResourceResult",
				}},
				TaskRunResults: []v1beta1.TaskRunResult{{
					Name:  "resultNameThree",
					Value: "",
				}},
				// We don't actually care about the time, just that it's not nil
				CompletionTime: &metav1.Time{Time: time.Now()},
			},
		},
	}, {
		desc: "correct TaskRun status step order regardless of pod container status order",
		pod: corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "pod",
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{
					Name: "step-first",
				}, {
					Name: "step-second",
				}, {
					Name: "step-third",
				}, {
					Name: "step-",
				}, {
					Name: "step-fourth",
				}},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodSucceeded,
				ContainerStatuses: []corev1.ContainerStatus{{
					Name: "step-second",
					State: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{},
					},
				}, {
					Name: "step-fourth",
					State: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{},
					},
				}, {
					Name: "step-",
					State: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{},
					},
				}, {
					Name: "step-first",
					State: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{},
					},
				}, {
					Name: "step-third",
					State: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{},
					},
				}},
			},
		},
		want: v1beta1.TaskRunStatus{
			Status: statusSuccess(),
			TaskRunStatusFields: v1beta1.TaskRunStatusFields{
				Steps: []v1beta1.StepState{{
					ContainerState: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{},
					},
					Name:          "first",
					ContainerName: "step-first",
				}, {
					ContainerState: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{}},
					Name:          "second",
					ContainerName: "step-second",
				}, {
					ContainerState: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{}},
					Name:          "third",
					ContainerName: "step-third",
				}, {
					ContainerState: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{}},
					Name:          "",
					ContainerName: "step-",
				}, {
					ContainerState: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{}},
					Name:          "fourth",
					ContainerName: "step-fourth",
				}},
				Sidecars: []v1beta1.SidecarState{},
				// We don't actually care about the time, just that it's not nil
				CompletionTime: &metav1.Time{Time: time.Now()},
			},
		},
	}} {
		t.Run(c.desc, func(t *testing.T) {
			now := metav1.Now()
			if cmp.Diff(c.pod, corev1.Pod{}) == "" {
				c.pod = corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "pod",
						Namespace:         "foo",
						CreationTimestamp: now,
					},
					Status: c.podStatus,
				}
			}

			startTime := time.Date(2010, 1, 1, 1, 1, 1, 1, time.UTC)
			tr := v1beta1.TaskRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "task-run",
					Namespace: "foo",
				},
				Status: v1beta1.TaskRunStatus{
					TaskRunStatusFields: v1beta1.TaskRunStatusFields{
						StartTime: &metav1.Time{Time: startTime},
					},
				},
			}

			logger, _ := logging.NewLogger("", "status")
			got, err := MakeTaskRunStatus(logger, tr, &c.pod)
			if err != nil {
				t.Errorf("MakeTaskRunResult: %s", err)
			}

			// Common traits, set for test case brevity.
			c.want.PodName = "pod"
			c.want.StartTime = &metav1.Time{Time: startTime}

			ensureTimeNotNil := cmp.Comparer(func(x, y *metav1.Time) bool {
				if x == nil {
					return y == nil
				}
				return y != nil
			})
			if d := cmp.Diff(c.want, got, ignoreVolatileTime, ensureTimeNotNil); d != "" {
				t.Errorf("Diff %s", diff.PrintWantGot(d))
			}
			if tr.Status.StartTime.Time != c.want.StartTime.Time {
				t.Errorf("Expected TaskRun startTime to be unchanged but was %s", tr.Status.StartTime)
			}
		})
	}
}

func TestMakeRunStatusJSONError(t *testing.T) {

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod",
			Namespace: "foo",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name: "step-non-json",
			}, {
				Name: "step-after-non-json",
			}, {
				Name: "step-this-step-might-panic",
			}, {
				Name: "step-foo",
			}},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodFailed,
			ContainerStatuses: []corev1.ContainerStatus{{
				Name:    "step-this-step-might-panic",
				ImageID: "image",
				State: corev1.ContainerState{
					Terminated: &corev1.ContainerStateTerminated{},
				},
			}, {
				Name:    "step-foo",
				ImageID: "image",
				State: corev1.ContainerState{
					Terminated: &corev1.ContainerStateTerminated{},
				},
			}, {
				Name:    "step-non-json",
				ImageID: "image",
				State: corev1.ContainerState{
					Terminated: &corev1.ContainerStateTerminated{
						ExitCode: 1,
						Message:  "this is a non-json termination message. dont panic!",
					},
				},
			}, {
				Name:    "step-after-non-json",
				ImageID: "image",
				State: corev1.ContainerState{
					Terminated: &corev1.ContainerStateTerminated{},
				},
			}},
		},
	}
	wantTr := v1beta1.TaskRunStatus{
		Status: statusFailure("\"step-non-json\" exited with code 1 (image: \"image\"); for logs run: kubectl -n foo logs pod -c step-non-json\n"),
		TaskRunStatusFields: v1beta1.TaskRunStatusFields{
			PodName: "pod",
			Steps: []v1beta1.StepState{{
				ContainerState: corev1.ContainerState{
					Terminated: &corev1.ContainerStateTerminated{
						ExitCode: 1,
						Message:  "this is a non-json termination message. dont panic!",
					}},
				Name:          "non-json",
				ContainerName: "step-non-json",
				ImageID:       "image",
			}, {
				ContainerState: corev1.ContainerState{
					Terminated: &corev1.ContainerStateTerminated{}},
				Name:          "after-non-json",
				ContainerName: "step-after-non-json",
				ImageID:       "image",
			}, {
				ContainerState: corev1.ContainerState{
					Terminated: &corev1.ContainerStateTerminated{}},
				Name:          "this-step-might-panic",
				ContainerName: "step-this-step-might-panic",
				ImageID:       "image",
			}, {
				ContainerState: corev1.ContainerState{
					Terminated: &corev1.ContainerStateTerminated{}},
				Name:          "foo",
				ContainerName: "step-foo",
				ImageID:       "image",
			}},
			Sidecars: []v1beta1.SidecarState{},
			// We don't actually care about the time, just that it's not nil
			CompletionTime: &metav1.Time{Time: time.Now()},
		},
	}
	tr := v1beta1.TaskRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "task-run",
			Namespace: "foo",
		},
	}

	logger, _ := logging.NewLogger("", "status")
	gotTr, err := MakeTaskRunStatus(logger, tr, pod)
	if err == nil {
		t.Error("Expected error, got nil")
	}

	ensureTimeNotNil := cmp.Comparer(func(x, y *metav1.Time) bool {
		if x == nil {
			return y == nil
		}
		return y != nil
	})
	if d := cmp.Diff(wantTr, gotTr, ignoreVolatileTime, ensureTimeNotNil); d != "" {
		t.Errorf("Diff %s", diff.PrintWantGot(d))
	}

}

func TestSidecarsReady(t *testing.T) {
	for _, c := range []struct {
		desc     string
		statuses []corev1.ContainerStatus
		want     bool
	}{{
		desc: "no sidecars",
		statuses: []corev1.ContainerStatus{
			{Name: "step-ignore-me"},
			{Name: "step-ignore-me"},
			{Name: "step-ignore-me"},
		},
		want: true,
	}, {
		desc: "both sidecars ready",
		statuses: []corev1.ContainerStatus{
			{Name: "step-ignore-me"},
			{
				Name:  "sidecar-bar",
				Ready: true,
				State: corev1.ContainerState{
					Running: &corev1.ContainerStateRunning{
						StartedAt: metav1.NewTime(time.Now()),
					},
				},
			},
			{Name: "step-ignore-me"},
			{
				Name: "sidecar-stopped-baz",
				State: corev1.ContainerState{
					Terminated: &corev1.ContainerStateTerminated{
						ExitCode: 99,
					},
				},
			},
			{Name: "step-ignore-me"},
		},
		want: true,
	}, {
		desc: "one sidecar ready, one not running",
		statuses: []corev1.ContainerStatus{
			{Name: "step-ignore-me"},
			{
				Name:  "sidecar-ready",
				Ready: true,
				State: corev1.ContainerState{
					Running: &corev1.ContainerStateRunning{
						StartedAt: metav1.NewTime(time.Now()),
					},
				},
			},
			{Name: "step-ignore-me"},
			{
				Name: "sidecar-unready",
				State: corev1.ContainerState{
					Waiting: &corev1.ContainerStateWaiting{},
				},
			},
			{Name: "step-ignore-me"},
		},
		want: false,
	}, {
		desc: "one sidecar running but not ready",
		statuses: []corev1.ContainerStatus{
			{Name: "step-ignore-me"},
			{
				Name:  "sidecar-running-not-ready",
				Ready: false, // Not ready.
				State: corev1.ContainerState{
					Running: &corev1.ContainerStateRunning{
						StartedAt: metav1.NewTime(time.Now()),
					},
				},
			},
			{Name: "step-ignore-me"},
		},
		want: false,
	}} {
		t.Run(c.desc, func(t *testing.T) {
			got := SidecarsReady(corev1.PodStatus{
				Phase:             corev1.PodRunning,
				ContainerStatuses: c.statuses,
			})
			if got != c.want {
				t.Errorf("SidecarsReady got %t, want %t", got, c.want)
			}
		})
	}
}

func TestMarkStatusRunning(t *testing.T) {
	trs := v1beta1.TaskRunStatus{}
	MarkStatusRunning(&trs, v1beta1.TaskRunReasonRunning.String(), "Not all Steps in the Task have finished executing")

	expected := &apis.Condition{
		Type:    apis.ConditionSucceeded,
		Status:  corev1.ConditionUnknown,
		Reason:  v1beta1.TaskRunReasonRunning.String(),
		Message: "Not all Steps in the Task have finished executing",
	}

	if d := cmp.Diff(expected, trs.GetCondition(apis.ConditionSucceeded), cmpopts.IgnoreTypes(apis.Condition{}.LastTransitionTime.Inner.Time)); d != "" {
		t.Errorf("Unexpected status: %s", diff.PrintWantGot(d))
	}
}

func TestMarkStatusFailure(t *testing.T) {
	trs := v1beta1.TaskRunStatus{}
	MarkStatusFailure(&trs, "failure message")

	expected := &apis.Condition{
		Type:    apis.ConditionSucceeded,
		Status:  corev1.ConditionFalse,
		Reason:  v1beta1.TaskRunReasonFailed.String(),
		Message: "failure message",
	}

	if d := cmp.Diff(expected, trs.GetCondition(apis.ConditionSucceeded), cmpopts.IgnoreTypes(apis.Condition{}.LastTransitionTime.Inner.Time)); d != "" {
		t.Errorf("Unexpected status: %s", diff.PrintWantGot(d))
	}
}

func TestMarkStatusSuccess(t *testing.T) {
	trs := v1beta1.TaskRunStatus{}
	MarkStatusSuccess(&trs)

	expected := &apis.Condition{
		Type:    apis.ConditionSucceeded,
		Status:  corev1.ConditionTrue,
		Reason:  v1beta1.TaskRunReasonSuccessful.String(),
		Message: "All Steps have completed executing",
	}

	if d := cmp.Diff(expected, trs.GetCondition(apis.ConditionSucceeded), cmpopts.IgnoreTypes(apis.Condition{}.LastTransitionTime.Inner.Time)); d != "" {
		t.Errorf("Unexpected status: %s", diff.PrintWantGot(d))
	}
}

func statusRunning() duckv1beta1.Status {
	var trs v1beta1.TaskRunStatus
	MarkStatusRunning(&trs, v1beta1.TaskRunReasonRunning.String(), "Not all Steps in the Task have finished executing")
	return trs.Status
}

func statusFailure(message string) duckv1beta1.Status {
	var trs v1beta1.TaskRunStatus
	MarkStatusFailure(&trs, message)
	return trs.Status
}

func statusSuccess() duckv1beta1.Status {
	var trs v1beta1.TaskRunStatus
	MarkStatusSuccess(&trs)
	return trs.Status
}

func statusPending(reason, message string) duckv1beta1.Status {
	return duckv1beta1.Status{
		Conditions: []apis.Condition{{
			Type:    apis.ConditionSucceeded,
			Status:  corev1.ConditionUnknown,
			Reason:  reason,
			Message: message,
		}},
	}
}
