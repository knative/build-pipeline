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
	"crypto/rand"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1alpha1"
	"github.com/tektoncd/pipeline/pkg/pod"
	"github.com/tektoncd/pipeline/test/names"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakek8s "k8s.io/client-go/kubernetes/fake"
)

var (
	resourceQuantityCmp = cmp.Comparer(func(x, y resource.Quantity) bool {
		return x.Cmp(y) == 0
	})
	credsImage = "override-with-creds:latest"
	shellImage = "busybox"
)

func TestMakePod(t *testing.T) {
	names.TestingSeed()

	secretsVolumeMount := corev1.VolumeMount{
		Name:      "secret-volume-multi-creds-9l9zj",
		MountPath: "/var/build-secrets/multi-creds",
	}
	secretsVolume := corev1.Volume{
		Name:         "secret-volume-multi-creds-9l9zj",
		VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: "multi-creds"}},
	}

	placeToolsInit := corev1.Container{
		Name:         "place-tools",
		Image:        images.EntrypointImage,
		Command:      []string{"cp", "/ko-app/entrypoint", "/builder/tools/entrypoint"},
		VolumeMounts: []corev1.VolumeMount{pod.ToolsMount},
	}

	runtimeClassName := "gvisor"

	randReader = strings.NewReader(strings.Repeat("a", 10000))
	defer func() { randReader = rand.Reader }()

	for _, c := range []struct {
		desc            string
		trs             v1alpha1.TaskRunSpec
		ts              v1alpha1.TaskSpec
		want            *corev1.PodSpec
		wantAnnotations map[string]string
	}{{
		desc: "simple",
		ts: v1alpha1.TaskSpec{
			Steps: []v1alpha1.Step{{Container: corev1.Container{
				Name:    "name",
				Image:   "image",
				Command: []string{"cmd"}, // avoid entrypoint lookup.
			}}},
		},
		want: &corev1.PodSpec{
			RestartPolicy:  corev1.RestartPolicyNever,
			InitContainers: []corev1.Container{placeToolsInit},
			Containers: []corev1.Container{{
				Name:    "step-name",
				Image:   "image",
				Command: []string{"/builder/tools/entrypoint"},
				Args: []string{
					"-wait_file",
					"/builder/downward/ready",
					"-wait_file_content",
					"-post_file",
					"/builder/tools/0",
					"-entrypoint",
					"cmd",
					"--",
				},
				Env:          implicitEnvVars,
				VolumeMounts: append([]corev1.VolumeMount{pod.ToolsMount, pod.DownwardMount}, implicitVolumeMounts...),
				WorkingDir:   workspaceDir,
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:              resource.MustParse("0"),
						corev1.ResourceMemory:           resource.MustParse("0"),
						corev1.ResourceEphemeralStorage: resource.MustParse("0"),
					},
				},
			}},
			Volumes: append(implicitVolumes, pod.ToolsVolume, pod.DownwardVolume),
		},
	}, {
		desc: "with service account",
		ts: v1alpha1.TaskSpec{
			Steps: []v1alpha1.Step{{Container: corev1.Container{
				Name:    "name",
				Image:   "image",
				Command: []string{"cmd"}, // avoid entrypoint lookup.
			}}},
		},
		trs: v1alpha1.TaskRunSpec{
			ServiceAccountName: "service-account",
		},
		want: &corev1.PodSpec{
			ServiceAccountName: "service-account",
			RestartPolicy:      corev1.RestartPolicyNever,
			InitContainers: []corev1.Container{{
				Name:    "credential-initializer-mz4c7",
				Image:   credsImage,
				Command: []string{"/ko-app/creds-init"},
				Args: []string{
					"-basic-docker=multi-creds=https://docker.io",
					"-basic-docker=multi-creds=https://us.gcr.io",
					"-basic-git=multi-creds=github.com",
					"-basic-git=multi-creds=gitlab.com",
				},
				VolumeMounts: append(implicitVolumeMounts, secretsVolumeMount),
				Env:          implicitEnvVars,
			},
				placeToolsInit,
			},
			Containers: []corev1.Container{{
				Name:    "step-name",
				Image:   "image",
				Command: []string{"/builder/tools/entrypoint"},
				Args: []string{
					"-wait_file",
					"/builder/downward/ready",
					"-wait_file_content",
					"-post_file",
					"/builder/tools/0",
					"-entrypoint",
					"cmd",
					"--",
				},
				Env:          implicitEnvVars,
				VolumeMounts: append([]corev1.VolumeMount{pod.ToolsMount, pod.DownwardMount}, implicitVolumeMounts...),
				WorkingDir:   workspaceDir,
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:              resource.MustParse("0"),
						corev1.ResourceMemory:           resource.MustParse("0"),
						corev1.ResourceEphemeralStorage: resource.MustParse("0"),
					},
				},
			}},
			Volumes: append(implicitVolumes, secretsVolume, pod.ToolsVolume, pod.DownwardVolume),
		},
	}, {
		desc: "with-pod-template",
		ts: v1alpha1.TaskSpec{
			Steps: []v1alpha1.Step{{Container: corev1.Container{
				Name:    "name",
				Image:   "image",
				Command: []string{"cmd"}, // avoid entrypoint lookup.
			}}},
		},
		trs: v1alpha1.TaskRunSpec{
			PodTemplate: v1alpha1.PodTemplate{
				SecurityContext: &corev1.PodSecurityContext{
					Sysctls: []corev1.Sysctl{
						{Name: "net.ipv4.tcp_syncookies", Value: "1"},
					},
				},
				RuntimeClassName: &runtimeClassName,
			},
		},
		want: &corev1.PodSpec{
			RestartPolicy:  corev1.RestartPolicyNever,
			InitContainers: []corev1.Container{placeToolsInit},
			Containers: []corev1.Container{{
				Name:    "step-name",
				Image:   "image",
				Command: []string{"/builder/tools/entrypoint"},
				Args: []string{
					"-wait_file",
					"/builder/downward/ready",
					"-wait_file_content",
					"-post_file",
					"/builder/tools/0",
					"-entrypoint",
					"cmd",
					"--",
				},
				Env:          implicitEnvVars,
				VolumeMounts: append([]corev1.VolumeMount{pod.ToolsMount, pod.DownwardMount}, implicitVolumeMounts...),
				WorkingDir:   workspaceDir,
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:              resource.MustParse("0"),
						corev1.ResourceMemory:           resource.MustParse("0"),
						corev1.ResourceEphemeralStorage: resource.MustParse("0"),
					},
				},
			}},
			Volumes: append(implicitVolumes, pod.ToolsVolume, pod.DownwardVolume),
			SecurityContext: &corev1.PodSecurityContext{
				Sysctls: []corev1.Sysctl{
					{Name: "net.ipv4.tcp_syncookies", Value: "1"},
				},
			},
			RuntimeClassName: &runtimeClassName,
		},
	}, {
		desc: "very long step name",
		ts: v1alpha1.TaskSpec{
			Steps: []v1alpha1.Step{{Container: corev1.Container{
				Name:    "a-very-very-long-character-step-name-to-trigger-max-len----and-invalid-characters",
				Image:   "image",
				Command: []string{"cmd"}, // avoid entrypoint lookup.
			}}},
		},
		want: &corev1.PodSpec{
			RestartPolicy:  corev1.RestartPolicyNever,
			InitContainers: []corev1.Container{placeToolsInit},
			Containers: []corev1.Container{{
				Name:    "step-a-very-very-long-character-step-name-to-trigger-max-len", // step name trimmed.
				Image:   "image",
				Command: []string{"/builder/tools/entrypoint"},
				Args: []string{
					"-wait_file",
					"/builder/downward/ready",
					"-wait_file_content",
					"-post_file",
					"/builder/tools/0",
					"-entrypoint",
					"cmd",
					"--",
				},
				Env:          implicitEnvVars,
				VolumeMounts: append([]corev1.VolumeMount{pod.ToolsMount, pod.DownwardMount}, implicitVolumeMounts...),
				WorkingDir:   workspaceDir,
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:              resource.MustParse("0"),
						corev1.ResourceMemory:           resource.MustParse("0"),
						corev1.ResourceEphemeralStorage: resource.MustParse("0"),
					},
				},
			}},
			Volumes: append(implicitVolumes, pod.ToolsVolume, pod.DownwardVolume),
		},
	}, {
		desc: "step name ends with non alphanumeric",
		ts: v1alpha1.TaskSpec{
			Steps: []v1alpha1.Step{{Container: corev1.Container{
				Name:    "ends-with-invalid-%%__$$",
				Image:   "image",
				Command: []string{"cmd"}, // avoid entrypoint lookup.
			}}},
		},
		want: &corev1.PodSpec{
			RestartPolicy:  corev1.RestartPolicyNever,
			InitContainers: []corev1.Container{placeToolsInit},
			Containers: []corev1.Container{{
				Name:    "step-ends-with-invalid", // invalid suffix removed.
				Image:   "image",
				Command: []string{"/builder/tools/entrypoint"},
				Args: []string{
					"-wait_file",
					"/builder/downward/ready",
					"-wait_file_content",
					"-post_file",
					"/builder/tools/0",
					"-entrypoint",
					"cmd",
					"--",
				},
				Env:          implicitEnvVars,
				VolumeMounts: append([]corev1.VolumeMount{pod.ToolsMount, pod.DownwardMount}, implicitVolumeMounts...),
				WorkingDir:   workspaceDir,
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:              resource.MustParse("0"),
						corev1.ResourceMemory:           resource.MustParse("0"),
						corev1.ResourceEphemeralStorage: resource.MustParse("0"),
					},
				},
			}},
			Volumes: append(implicitVolumes, pod.ToolsVolume, pod.DownwardVolume),
		},
	}, {
		desc: "workingDir in workspace",
		ts: v1alpha1.TaskSpec{
			Steps: []v1alpha1.Step{{Container: corev1.Container{
				Name:       "name",
				Image:      "image",
				Command:    []string{"cmd"}, // avoid entrypoint lookup.
				WorkingDir: filepath.Join(workspaceDir, "test"),
			}}},
		},
		want: &corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			InitContainers: []corev1.Container{{
				Name:         "working-dir-initializer-9l9zj",
				Image:        shellImage,
				Command:      []string{"sh"},
				Args:         []string{"-c", fmt.Sprintf("mkdir -p %s", filepath.Join(workspaceDir, "test"))},
				WorkingDir:   workspaceDir,
				VolumeMounts: implicitVolumeMounts,
			},
				placeToolsInit,
			},
			Containers: []corev1.Container{{
				Name:    "step-name",
				Image:   "image",
				Command: []string{"/builder/tools/entrypoint"},
				Args: []string{
					"-wait_file",
					"/builder/downward/ready",
					"-wait_file_content",
					"-post_file",
					"/builder/tools/0",
					"-entrypoint",
					"cmd",
					"--",
				},
				Env:          implicitEnvVars,
				VolumeMounts: append([]corev1.VolumeMount{pod.ToolsMount, pod.DownwardMount}, implicitVolumeMounts...),
				WorkingDir:   filepath.Join(workspaceDir, "test"),
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:              resource.MustParse("0"),
						corev1.ResourceMemory:           resource.MustParse("0"),
						corev1.ResourceEphemeralStorage: resource.MustParse("0"),
					},
				},
			}},
			Volumes: append(implicitVolumes, pod.ToolsVolume, pod.DownwardVolume),
		},
	}, {
		desc: "sidecar container",
		ts: v1alpha1.TaskSpec{
			Steps: []v1alpha1.Step{{Container: corev1.Container{
				Name:    "primary-name",
				Image:   "primary-image",
				Command: []string{"cmd"}, // avoid entrypoint lookup.
			}}},
			Sidecars: []corev1.Container{{
				Name:  "sc-name",
				Image: "sidecar-image",
			}},
		},
		wantAnnotations: map[string]string{},
		want: &corev1.PodSpec{
			RestartPolicy:  corev1.RestartPolicyNever,
			InitContainers: []corev1.Container{placeToolsInit},
			Containers: []corev1.Container{{
				Name:    "step-primary-name",
				Image:   "primary-image",
				Command: []string{"/builder/tools/entrypoint"},
				Args: []string{
					"-wait_file",
					"/builder/downward/ready",
					"-wait_file_content",
					"-post_file",
					"/builder/tools/0",
					"-entrypoint",
					"cmd",
					"--",
				},
				Env:          implicitEnvVars,
				VolumeMounts: append([]corev1.VolumeMount{pod.ToolsMount, pod.DownwardMount}, implicitVolumeMounts...),
				WorkingDir:   workspaceDir,
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:              resource.MustParse("0"),
						corev1.ResourceMemory:           resource.MustParse("0"),
						corev1.ResourceEphemeralStorage: resource.MustParse("0"),
					},
				},
			}, {
				Name:  "sidecar-sc-name",
				Image: "sidecar-image",
				Resources: corev1.ResourceRequirements{
					Requests: nil,
				},
			}},
			Volumes: append(implicitVolumes, pod.ToolsVolume, pod.DownwardVolume),
		},
	}, {
		desc: "resource request",
		ts: v1alpha1.TaskSpec{
			Steps: []v1alpha1.Step{{Container: corev1.Container{
				Image:   "image",
				Command: []string{"cmd"}, // avoid entrypoint lookup.
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("8"),
						corev1.ResourceMemory: resource.MustParse("10Gi"),
					},
				},
			}}, {Container: corev1.Container{
				Image:   "image",
				Command: []string{"cmd"}, // avoid entrypoint lookup.
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("1"),
						corev1.ResourceMemory: resource.MustParse("100Gi"),
					},
				},
			}}},
		},
		want: &corev1.PodSpec{
			RestartPolicy:  corev1.RestartPolicyNever,
			InitContainers: []corev1.Container{placeToolsInit},
			Containers: []corev1.Container{{
				Name:    "step-unnamed-0",
				Image:   "image",
				Command: []string{"/builder/tools/entrypoint"},
				Args: []string{
					"-wait_file",
					"/builder/downward/ready",
					"-wait_file_content",
					"-post_file",
					"/builder/tools/0",
					"-entrypoint",
					"cmd",
					"--",
				},
				Env:          implicitEnvVars,
				VolumeMounts: append([]corev1.VolumeMount{pod.ToolsMount, pod.DownwardMount}, implicitVolumeMounts...),
				WorkingDir:   workspaceDir,
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:              resource.MustParse("8"),
						corev1.ResourceMemory:           resource.MustParse("0"),
						corev1.ResourceEphemeralStorage: resource.MustParse("0"),
					},
				},
			}, {
				Name:    "step-unnamed-1",
				Image:   "image",
				Command: []string{"/builder/tools/entrypoint"},
				Args: []string{
					"-wait_file",
					"/builder/tools/0",
					"-post_file",
					"/builder/tools/1",
					"-entrypoint",
					"cmd",
					"--",
				},
				Env:          implicitEnvVars,
				VolumeMounts: append([]corev1.VolumeMount{pod.ToolsMount}, implicitVolumeMounts...),
				WorkingDir:   workspaceDir,
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:              resource.MustParse("0"),
						corev1.ResourceMemory:           resource.MustParse("100Gi"),
						corev1.ResourceEphemeralStorage: resource.MustParse("0"),
					},
				},
			}},
			Volumes: append(implicitVolumes, pod.ToolsVolume, pod.DownwardVolume),
		},
	}, {
		desc: "step with script",
		ts: v1alpha1.TaskSpec{
			Steps: []v1alpha1.Step{{
				Container: corev1.Container{
					Name:  "one",
					Image: "image",
				},
				Script: "echo hello from step one",
			}, {
				Container: corev1.Container{
					Name:         "two",
					Image:        "image",
					VolumeMounts: []corev1.VolumeMount{{Name: "i-have-a-volume-mount"}},
				},
				Script: `#!/usr/bin/env python
print("Hello from Python")`,
			}},
		},
		want: &corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			InitContainers: []corev1.Container{{
				Name:    "place-scripts-9l9zj",
				Image:   images.ShellImage,
				Command: []string{"sh"},
				TTY:     true,
				Args: []string{"-c", `tmpfile="/builder/scripts/script-0-mz4c7"
touch ${tmpfile} && chmod +x ${tmpfile}
cat > ${tmpfile} << 'script-heredoc-randomly-generated-mssqb'
echo hello from step one
script-heredoc-randomly-generated-mssqb
tmpfile="/builder/scripts/script-1-78c5n"
touch ${tmpfile} && chmod +x ${tmpfile}
cat > ${tmpfile} << 'script-heredoc-randomly-generated-6nl7g'
#!/usr/bin/env python
print("Hello from Python")
script-heredoc-randomly-generated-6nl7g
`},
				VolumeMounts: []corev1.VolumeMount{pod.ScriptsVolumeMount},
			}, {
				Name:         "place-tools",
				Image:        images.EntrypointImage,
				Command:      []string{"cp", "/ko-app/entrypoint", "/builder/tools/entrypoint"},
				VolumeMounts: []corev1.VolumeMount{pod.ToolsMount},
			}},
			Containers: []corev1.Container{{
				Name:    "step-one",
				Image:   "image",
				Command: []string{"/builder/tools/entrypoint"},
				Args: []string{
					"-wait_file",
					"/builder/downward/ready",
					"-wait_file_content",
					"-post_file",
					"/builder/tools/0",
					"-entrypoint",
					"/builder/scripts/script-0-mz4c7",
					"--",
				},
				Env:          implicitEnvVars,
				VolumeMounts: append([]corev1.VolumeMount{pod.ScriptsVolumeMount, pod.ToolsMount, pod.DownwardMount}, implicitVolumeMounts...),
				WorkingDir:   workspaceDir,
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:              resource.MustParse("0"),
						corev1.ResourceMemory:           resource.MustParse("0"),
						corev1.ResourceEphemeralStorage: resource.MustParse("0"),
					},
				},
			}, {
				Name:    "step-two",
				Image:   "image",
				Command: []string{"/builder/tools/entrypoint"},
				Args: []string{
					"-wait_file",
					"/builder/tools/0",
					"-post_file",
					"/builder/tools/1",
					"-entrypoint",
					"/builder/scripts/script-1-78c5n",
					"--",
				},
				Env:          implicitEnvVars,
				VolumeMounts: append([]corev1.VolumeMount{{Name: "i-have-a-volume-mount"}, pod.ScriptsVolumeMount, pod.ToolsMount}, implicitVolumeMounts...),
				WorkingDir:   workspaceDir,
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:              resource.MustParse("0"),
						corev1.ResourceMemory:           resource.MustParse("0"),
						corev1.ResourceEphemeralStorage: resource.MustParse("0"),
					},
				},
			}},
			Volumes: append(implicitVolumes, pod.ScriptsVolume, pod.ToolsVolume, pod.DownwardVolume),
		},
	}} {
		t.Run(c.desc, func(t *testing.T) {
			names.TestingSeed()
			kubeclient := fakek8s.NewSimpleClientset(
				&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "default"}},
				&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "service-account", Namespace: "default"},
					Secrets: []corev1.ObjectReference{{
						Name: "multi-creds",
					}},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: "multi-creds",
						Annotations: map[string]string{
							"tekton.dev/docker-0": "https://us.gcr.io",
							"tekton.dev/docker-1": "https://docker.io",
							"tekton.dev/git-0":    "github.com",
							"tekton.dev/git-1":    "gitlab.com",
						}},
					Type: "kubernetes.io/basic-auth",
					Data: map[string][]byte{
						"username": []byte("foo"),
						"password": []byte("BestEver"),
					},
				},
			)
			tr := &v1alpha1.TaskRun{
				ObjectMeta: metav1.ObjectMeta{
					Name: "taskrun-name",
				},
				Spec: c.trs,
			}
			entrypointCache, err := pod.NewEntrypointCache(kubeclient)
			if err != nil {
				t.Fatalf("NewEntrypointCache: %v", err)
			}
			got, err := MakePod(images, tr, c.ts, kubeclient, entrypointCache)
			if err != nil {
				t.Fatalf("MakePod: %v", err)
			}

			// Generated name from hexlifying a stream of 'a's.
			wantName := "taskrun-name-pod-616161"
			if got.Name != wantName {
				t.Errorf("Pod name got %q, want %q", got.Name, wantName)
			}

			if d := cmp.Diff(c.want, &got.Spec, resourceQuantityCmp); d != "" {
				t.Errorf("Diff(-want, +got):\n%s", d)
			}
		})
	}
}

func TestMakeLabels(t *testing.T) {
	taskRunName := "task-run-name"
	for _, c := range []struct {
		desc     string
		trLabels map[string]string
		want     map[string]string
	}{{
		desc: "taskrun labels pass through",
		trLabels: map[string]string{
			"foo":   "bar",
			"hello": "world",
		},
		want: map[string]string{
			taskRunLabelKey:   taskRunName,
			ManagedByLabelKey: ManagedByLabelValue,
			"foo":             "bar",
			"hello":           "world",
		},
	}, {
		desc: "taskrun managed-by overrides; taskrun label key doesn't",
		trLabels: map[string]string{
			"foo":             "bar",
			taskRunLabelKey:   "override-me",
			ManagedByLabelKey: "managed-by-something-else",
		},
		want: map[string]string{
			taskRunLabelKey:   taskRunName,
			ManagedByLabelKey: "managed-by-something-else",
			"foo":             "bar",
		},
	}} {
		t.Run(c.desc, func(t *testing.T) {
			got := makeLabels(&v1alpha1.TaskRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:   taskRunName,
					Labels: c.trLabels,
				},
			})
			if d := cmp.Diff(got, c.want); d != "" {
				t.Errorf("Diff labels:\n%s", d)
			}
		})
	}
}
