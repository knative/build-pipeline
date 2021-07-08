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

package resources_test

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/tektoncd/pipeline/pkg/apis/config"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"github.com/tektoncd/pipeline/pkg/apis/resource"
	resourcev1alpha1 "github.com/tektoncd/pipeline/pkg/apis/resource/v1alpha1"
	"github.com/tektoncd/pipeline/pkg/reconciler/taskrun/resources"
	"github.com/tektoncd/pipeline/pkg/workspace"
	"github.com/tektoncd/pipeline/test/diff"
	"github.com/tektoncd/pipeline/test/names"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/apis"
	duckv1beta1 "knative.dev/pkg/apis/duck/v1beta1"
)

var (
	images = pipeline.Images{
		EntrypointImage:          "override-with-entrypoint:latest",
		NopImage:                 "override-with-nop:latest",
		GitImage:                 "override-with-git:latest",
		KubeconfigWriterImage:    "override-with-kubeconfig-writer-image:latest",
		ShellImage:               "busybox",
		GsutilImage:              "gcr.io/google.com/cloudsdktool/cloud-sdk",
		PRImage:                  "override-with-pr:latest",
		ImageDigestExporterImage: "override-with-imagedigest-exporter-image:latest",
	}

	simpleTaskSpec = &v1beta1.TaskSpec{
		Sidecars: []v1beta1.Sidecar{{
			Container: corev1.Container{
				Name:  "foo",
				Image: "$(inputs.params.myimage)",
				Env: []corev1.EnvVar{{
					Name:  "foo",
					Value: "$(inputs.params.FOO)",
				}},
			},
		}},
		StepTemplate: &corev1.Container{
			Env: []corev1.EnvVar{{
				Name:  "template-var",
				Value: "$(inputs.params.FOO)",
			}},
		},
		Steps: []v1beta1.Step{{Container: corev1.Container{
			Name:  "foo",
			Image: "$(inputs.params.myimage)",
		}}, {Container: corev1.Container{
			Name:       "baz",
			Image:      "bat",
			WorkingDir: "$(inputs.resources.workspace.path)",
			Args:       []string{"$(inputs.resources.workspace.url)"},
		}}, {Container: corev1.Container{
			Name:  "qux",
			Image: "$(inputs.params.something)",
			Args:  []string{"$(outputs.resources.imageToUse.url)"},
		}}, {Container: corev1.Container{
			Name:  "foo",
			Image: "$(inputs.params.myimage)",
		}}, {Container: corev1.Container{
			Name:       "baz",
			Image:      "$(inputs.params.somethingelse)",
			WorkingDir: "$(inputs.resources.workspace.path)",
			Args:       []string{"$(inputs.resources.workspace.url)"},
		}}, {Container: corev1.Container{
			Name:  "qux",
			Image: "quux",
			Args:  []string{"$(outputs.resources.imageToUse.url)"},
		}}, {Container: corev1.Container{
			Name:  "foo",
			Image: "busybox:$(inputs.params.FOO)",
			VolumeMounts: []corev1.VolumeMount{{
				Name:      "$(inputs.params.FOO)",
				MountPath: "path/to/$(inputs.params.FOO)",
				SubPath:   "sub/$(inputs.params.FOO)/path",
			}},
		}}, {Container: corev1.Container{
			Name:  "foo",
			Image: "busybox:$(inputs.params.FOO)",
			Env: []corev1.EnvVar{{
				Name:  "foo",
				Value: "value-$(inputs.params.FOO)",
			}, {
				Name: "bar",
				ValueFrom: &corev1.EnvVarSource{
					ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: "config-$(inputs.params.FOO)"},
						Key:                  "config-key-$(inputs.params.FOO)",
					},
				},
			}, {
				Name: "baz",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: "secret-$(inputs.params.FOO)"},
						Key:                  "secret-key-$(inputs.params.FOO)",
					},
				},
			}},
			EnvFrom: []corev1.EnvFromSource{{
				Prefix: "prefix-0-$(inputs.params.FOO)",
				ConfigMapRef: &corev1.ConfigMapEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: "config-$(inputs.params.FOO)"},
				},
			}, {
				Prefix: "prefix-1-$(inputs.params.FOO)",
				SecretRef: &corev1.SecretEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: "secret-$(inputs.params.FOO)"},
				},
			}},
		}}, {Container: corev1.Container{
			Name:  "outputs-resources-path-ab",
			Image: "$(outputs.resources.imageToUse-ab.path)",
		}}, {Container: corev1.Container{
			Name:  "outputs-resources-path-re",
			Image: "$(outputs.resources.imageToUse-re.path)",
		}}},
		Volumes: []corev1.Volume{{
			Name: "$(inputs.params.FOO)",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "$(inputs.params.FOO)",
					},
					Items: []corev1.KeyToPath{{
						Key:  "$(inputs.params.FOO)",
						Path: "$(inputs.params.FOO)",
					}},
				},
			},
		}, {
			Name: "some-secret",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: "$(inputs.params.FOO)",
					Items: []corev1.KeyToPath{{
						Key:  "$(inputs.params.FOO)",
						Path: "$(inputs.params.FOO)",
					}},
				},
			},
		}, {
			Name: "some-pvc",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: "$(inputs.params.FOO)",
				},
			},
		}, {
			Name: "some-projected-volumes",
			VolumeSource: corev1.VolumeSource{
				Projected: &corev1.ProjectedVolumeSource{
					Sources: []corev1.VolumeProjection{{
						ConfigMap: &corev1.ConfigMapProjection{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "$(inputs.params.FOO)",
							},
						},
						Secret: &corev1.SecretProjection{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "$(inputs.params.FOO)",
							},
						},
						ServiceAccountToken: &corev1.ServiceAccountTokenProjection{
							Audience: "$(inputs.params.FOO)",
						},
					}},
				},
			},
		}, {
			Name: "some-csi",
			VolumeSource: corev1.VolumeSource{
				CSI: &corev1.CSIVolumeSource{
					VolumeAttributes: map[string]string{
						"secretProviderClass": "$(inputs.params.FOO)",
					},
					NodePublishSecretRef: &corev1.LocalObjectReference{
						Name: "$(inputs.params.FOO)",
					},
				},
			},
		}},
		Resources: &v1beta1.TaskResources{
			Inputs: []v1beta1.TaskResource{{
				ResourceDeclaration: v1beta1.ResourceDeclaration{
					Name: "workspace",
				},
			}},
			Outputs: []v1beta1.TaskResource{{
				ResourceDeclaration: v1beta1.ResourceDeclaration{
					Name:       "imageToUse-ab",
					TargetPath: "/foo/builtImage",
				},
			}, {
				ResourceDeclaration: v1beta1.ResourceDeclaration{
					Name:       "imageToUse-re",
					TargetPath: "foo/builtImage",
				},
			}},
		},
	}

	gcsTaskSpec = &v1beta1.TaskSpec{
		Steps: []v1beta1.Step{{Container: corev1.Container{
			Name:  "foobar",
			Image: "someImage",
			Args:  []string{"$(outputs.resources.bucket.path)"},
		}}},
		Resources: &v1beta1.TaskResources{
			Outputs: []v1beta1.TaskResource{{
				ResourceDeclaration: v1beta1.ResourceDeclaration{
					Name: "bucket",
				},
			}},
		},
	}

	arrayParamTaskSpec = &v1beta1.TaskSpec{
		Steps: []v1beta1.Step{{Container: corev1.Container{
			Name:  "simple-image",
			Image: "some-image",
		}}, {Container: corev1.Container{
			Name:    "image-with-c-specified",
			Image:   "some-other-image",
			Command: []string{"echo"},
			Args:    []string{"first", "second", "$(inputs.params.array-param)", "last"},
		}}},
	}

	arrayAndStringParamTaskSpec = &v1beta1.TaskSpec{
		Steps: []v1beta1.Step{{Container: corev1.Container{
			Name:  "simple-image",
			Image: "some-image",
		}}, {Container: corev1.Container{
			Name:    "image-with-c-specified",
			Image:   "some-other-image",
			Command: []string{"echo"},
			Args:    []string{"$(inputs.params.normal-param)", "second", "$(inputs.params.array-param)", "last"},
		}}},
	}

	multipleArrayParamsTaskSpec = &v1beta1.TaskSpec{
		Steps: []v1beta1.Step{{Container: corev1.Container{
			Name:  "simple-image",
			Image: "some-image",
		}}, {Container: corev1.Container{
			Name:    "image-with-c-specified",
			Image:   "some-other-image",
			Command: []string{"cmd", "$(inputs.params.another-array-param)"},
			Args:    []string{"first", "second", "$(inputs.params.array-param)", "last"},
		}}},
	}

	multipleArrayAndStringsParamsTaskSpec = &v1beta1.TaskSpec{
		Steps: []v1beta1.Step{{Container: corev1.Container{
			Name:  "simple-image",
			Image: "image-$(inputs.params.string-param2)",
		}}, {Container: corev1.Container{
			Name:    "image-with-c-specified",
			Image:   "some-other-image",
			Command: []string{"cmd", "$(inputs.params.array-param1)"},
			Args:    []string{"$(inputs.params.array-param2)", "second", "$(inputs.params.array-param1)", "$(inputs.params.string-param1)", "last"},
		}}},
	}

	arrayTaskRun0Elements = &v1beta1.TaskRun{
		Spec: v1beta1.TaskRunSpec{
			Params: []v1beta1.Param{{
				Name: "array-param",
				Value: v1beta1.ArrayOrString{
					Type:     v1beta1.ParamTypeArray,
					ArrayVal: []string{},
				}},
			},
		},
	}

	arrayTaskRun1Elements = &v1beta1.TaskRun{
		Spec: v1beta1.TaskRunSpec{
			Params: []v1beta1.Param{{
				Name:  "array-param",
				Value: *v1beta1.NewArrayOrString("foo"),
			}},
		},
	}

	arrayTaskRun3Elements = &v1beta1.TaskRun{
		Spec: v1beta1.TaskRunSpec{
			Params: []v1beta1.Param{{
				Name:  "array-param",
				Value: *v1beta1.NewArrayOrString("foo", "bar", "third"),
			}},
		},
	}

	arrayTaskRunMultipleArrays = &v1beta1.TaskRun{
		Spec: v1beta1.TaskRunSpec{
			Params: []v1beta1.Param{{
				Name:  "array-param",
				Value: *v1beta1.NewArrayOrString("foo", "bar", "third"),
			}, {
				Name:  "another-array-param",
				Value: *v1beta1.NewArrayOrString("part1", "part2"),
			}},
		},
	}

	arrayTaskRunWith1StringParam = &v1beta1.TaskRun{
		Spec: v1beta1.TaskRunSpec{
			Params: []v1beta1.Param{{
				Name:  "array-param",
				Value: *v1beta1.NewArrayOrString("middlefirst", "middlesecond"),
			}, {
				Name:  "normal-param",
				Value: *v1beta1.NewArrayOrString("foo"),
			}},
		},
	}

	arrayTaskRunMultipleArraysAndStrings = &v1beta1.TaskRun{
		Spec: v1beta1.TaskRunSpec{
			Params: []v1beta1.Param{{
				Name:  "array-param1",
				Value: *v1beta1.NewArrayOrString("1-param1", "2-param1", "3-param1", "4-param1"),
			}, {
				Name:  "array-param2",
				Value: *v1beta1.NewArrayOrString("1-param2", "2-param2", "2-param3"),
			}, {
				Name:  "string-param1",
				Value: *v1beta1.NewArrayOrString("foo"),
			}, {
				Name:  "string-param2",
				Value: *v1beta1.NewArrayOrString("bar"),
			}},
		},
	}

	inputs = map[string]v1beta1.PipelineResourceInterface{
		"workspace": gitResource,
	}

	outputs = map[string]v1beta1.PipelineResourceInterface{
		"imageToUse": imageResource,
		"bucket":     gcsResource,
	}

	gitResource, _ = resource.FromType("git-resource", &resourcev1alpha1.PipelineResource{
		ObjectMeta: metav1.ObjectMeta{
			Name: "git-resource",
		},
		Spec: resourcev1alpha1.PipelineResourceSpec{
			Type: resourcev1alpha1.PipelineResourceTypeGit,
			Params: []resourcev1alpha1.ResourceParam{{
				Name:  "URL",
				Value: "https://git-repo",
			}},
		},
	}, images)

	imageResource, _ = resource.FromType("image-resource", &resourcev1alpha1.PipelineResource{
		ObjectMeta: metav1.ObjectMeta{
			Name: "image-resource",
		},
		Spec: resourcev1alpha1.PipelineResourceSpec{
			Type: resourcev1alpha1.PipelineResourceTypeImage,
			Params: []resourcev1alpha1.ResourceParam{{
				Name:  "URL",
				Value: "gcr.io/hans/sandwiches",
			}},
		},
	}, images)

	gcsResource, _ = resource.FromType("gcs-resource", &resourcev1alpha1.PipelineResource{
		ObjectMeta: metav1.ObjectMeta{
			Name: "gcs-resource",
		},
		Spec: resourcev1alpha1.PipelineResourceSpec{
			Type: resourcev1alpha1.PipelineResourceTypeStorage,
			Params: []resourcev1alpha1.ResourceParam{{
				Name:  "type",
				Value: "gcs",
			}, {
				Name:  "location",
				Value: "theCloud?",
			}},
		},
	}, images)
)

func applyMutation(ts *v1beta1.TaskSpec, f func(*v1beta1.TaskSpec)) *v1beta1.TaskSpec {
	ts = ts.DeepCopy()
	f(ts)
	return ts
}

func TestApplyArrayParameters(t *testing.T) {
	type args struct {
		ts *v1beta1.TaskSpec
		tr *v1beta1.TaskRun
		dp []v1beta1.ParamSpec
	}
	tests := []struct {
		name string
		args args
		want *v1beta1.TaskSpec
	}{{
		name: "array parameter with 0 elements",
		args: args{
			ts: arrayParamTaskSpec,
			tr: arrayTaskRun0Elements,
		},
		want: applyMutation(arrayParamTaskSpec, func(spec *v1beta1.TaskSpec) {
			spec.Steps[1].Args = []string{"first", "second", "last"}
		}),
	}, {
		name: "array parameter with 1 element",
		args: args{
			ts: arrayParamTaskSpec,
			tr: arrayTaskRun1Elements,
		},
		want: applyMutation(arrayParamTaskSpec, func(spec *v1beta1.TaskSpec) {
			spec.Steps[1].Args = []string{"first", "second", "foo", "last"}
		}),
	}, {
		name: "array parameter with 3 elements",
		args: args{
			ts: arrayParamTaskSpec,
			tr: arrayTaskRun3Elements,
		},
		want: applyMutation(arrayParamTaskSpec, func(spec *v1beta1.TaskSpec) {
			spec.Steps[1].Args = []string{"first", "second", "foo", "bar", "third", "last"}
		}),
	}, {
		name: "multiple arrays",
		args: args{
			ts: multipleArrayParamsTaskSpec,
			tr: arrayTaskRunMultipleArrays,
		},
		want: applyMutation(multipleArrayParamsTaskSpec, func(spec *v1beta1.TaskSpec) {
			spec.Steps[1].Command = []string{"cmd", "part1", "part2"}
			spec.Steps[1].Args = []string{"first", "second", "foo", "bar", "third", "last"}
		}),
	}, {
		name: "array and normal string parameter",
		args: args{
			ts: arrayAndStringParamTaskSpec,
			tr: arrayTaskRunWith1StringParam,
		},
		want: applyMutation(arrayAndStringParamTaskSpec, func(spec *v1beta1.TaskSpec) {
			spec.Steps[1].Args = []string{"foo", "second", "middlefirst", "middlesecond", "last"}
		}),
	}, {
		name: "several arrays and strings",
		args: args{
			ts: multipleArrayAndStringsParamsTaskSpec,
			tr: arrayTaskRunMultipleArraysAndStrings,
		},
		want: applyMutation(multipleArrayAndStringsParamsTaskSpec, func(spec *v1beta1.TaskSpec) {
			spec.Steps[0].Image = "image-bar"
			spec.Steps[1].Command = []string{"cmd", "1-param1", "2-param1", "3-param1", "4-param1"}
			spec.Steps[1].Args = []string{"1-param2", "2-param2", "2-param3", "second", "1-param1", "2-param1", "3-param1", "4-param1", "foo", "last"}
		}),
	}, {
		name: "default array parameter",
		args: args{
			ts: arrayParamTaskSpec,
			tr: &v1beta1.TaskRun{},
			dp: []v1beta1.ParamSpec{{
				Name:    "array-param",
				Default: v1beta1.NewArrayOrString("defaulted", "value!"),
			}},
		},
		want: applyMutation(arrayParamTaskSpec, func(spec *v1beta1.TaskSpec) {
			spec.Steps[1].Args = []string{"first", "second", "defaulted", "value!", "last"}
		}),
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resources.ApplyParameters(tt.args.ts, tt.args.tr, tt.args.dp...)
			if d := cmp.Diff(tt.want, got); d != "" {
				t.Errorf("ApplyParameters() got diff %s", diff.PrintWantGot(d))
			}
		})
	}
}

func TestApplyParameters(t *testing.T) {
	tr := &v1beta1.TaskRun{
		Spec: v1beta1.TaskRunSpec{
			Params: []v1beta1.Param{{
				Name:  "myimage",
				Value: *v1beta1.NewArrayOrString("bar"),
			}, {
				Name:  "FOO",
				Value: *v1beta1.NewArrayOrString("world"),
			}},
		},
	}
	dp := []v1beta1.ParamSpec{{
		Name:    "something",
		Default: v1beta1.NewArrayOrString("mydefault"),
	}, {
		Name:    "somethingelse",
		Default: v1beta1.NewArrayOrString(""),
	}}
	want := applyMutation(simpleTaskSpec, func(spec *v1beta1.TaskSpec) {
		spec.StepTemplate.Env[0].Value = "world"

		spec.Steps[0].Image = "bar"
		spec.Steps[2].Image = "mydefault"
		spec.Steps[3].Image = "bar"
		spec.Steps[4].Image = ""

		spec.Steps[6].VolumeMounts[0].Name = "world"
		spec.Steps[6].VolumeMounts[0].SubPath = "sub/world/path"
		spec.Steps[6].VolumeMounts[0].MountPath = "path/to/world"
		spec.Steps[6].Image = "busybox:world"

		spec.Steps[7].Env[0].Value = "value-world"
		spec.Steps[7].Env[1].ValueFrom.ConfigMapKeyRef.LocalObjectReference.Name = "config-world"
		spec.Steps[7].Env[1].ValueFrom.ConfigMapKeyRef.Key = "config-key-world"
		spec.Steps[7].Env[2].ValueFrom.SecretKeyRef.LocalObjectReference.Name = "secret-world"
		spec.Steps[7].Env[2].ValueFrom.SecretKeyRef.Key = "secret-key-world"
		spec.Steps[7].EnvFrom[0].Prefix = "prefix-0-world"
		spec.Steps[7].EnvFrom[0].ConfigMapRef.LocalObjectReference.Name = "config-world"
		spec.Steps[7].EnvFrom[1].Prefix = "prefix-1-world"
		spec.Steps[7].EnvFrom[1].SecretRef.LocalObjectReference.Name = "secret-world"
		spec.Steps[7].Image = "busybox:world"
		spec.Steps[8].Image = "$(outputs.resources.imageToUse-ab.path)"
		spec.Steps[9].Image = "$(outputs.resources.imageToUse-re.path)"

		spec.Volumes[0].Name = "world"
		spec.Volumes[0].VolumeSource.ConfigMap.LocalObjectReference.Name = "world"
		spec.Volumes[0].VolumeSource.ConfigMap.Items[0].Key = "world"
		spec.Volumes[0].VolumeSource.ConfigMap.Items[0].Path = "world"
		spec.Volumes[1].VolumeSource.Secret.SecretName = "world"
		spec.Volumes[1].VolumeSource.Secret.Items[0].Key = "world"
		spec.Volumes[1].VolumeSource.Secret.Items[0].Path = "world"
		spec.Volumes[2].VolumeSource.PersistentVolumeClaim.ClaimName = "world"
		spec.Volumes[3].VolumeSource.Projected.Sources[0].ConfigMap.Name = "world"
		spec.Volumes[3].VolumeSource.Projected.Sources[0].Secret.Name = "world"
		spec.Volumes[3].VolumeSource.Projected.Sources[0].ServiceAccountToken.Audience = "world"
		spec.Volumes[4].VolumeSource.CSI.VolumeAttributes["secretProviderClass"] = "world"
		spec.Volumes[4].VolumeSource.CSI.NodePublishSecretRef.Name = "world"

		spec.Sidecars[0].Container.Image = "bar"
		spec.Sidecars[0].Container.Env[0].Value = "world"
	})
	got := resources.ApplyParameters(simpleTaskSpec, tr, dp...)
	if d := cmp.Diff(want, got); d != "" {
		t.Errorf("ApplyParameters() got diff %s", diff.PrintWantGot(d))
	}
}

func TestApplyResources(t *testing.T) {
	tests := []struct {
		name string
		ts   *v1beta1.TaskSpec
		r    map[string]v1beta1.PipelineResourceInterface
		rStr string
		want *v1beta1.TaskSpec
	}{{
		name: "no replacements specified",
		ts:   simpleTaskSpec,
		r:    make(map[string]v1beta1.PipelineResourceInterface),
		rStr: "inputs",
		want: applyMutation(simpleTaskSpec, func(spec *v1beta1.TaskSpec) {
			spec.Steps[1].WorkingDir = "/workspace/workspace"
			spec.Steps[4].WorkingDir = "/workspace/workspace"
			spec.Steps[8].Image = "/foo/builtImage"
			spec.Steps[9].Image = "/workspace/foo/builtImage"
		}),
	}, {
		name: "input resource specified",
		ts:   simpleTaskSpec,
		r:    inputs,
		rStr: "inputs",
		want: applyMutation(simpleTaskSpec, func(spec *v1beta1.TaskSpec) {
			spec.Steps[1].WorkingDir = "/workspace/workspace"
			spec.Steps[1].Args = []string{"https://git-repo"}
			spec.Steps[4].WorkingDir = "/workspace/workspace"
			spec.Steps[4].Args = []string{"https://git-repo"}
			spec.Steps[8].Image = "/foo/builtImage"
			spec.Steps[9].Image = "/workspace/foo/builtImage"
		}),
	}, {
		name: "output resource specified",
		ts:   simpleTaskSpec,
		r:    outputs,
		rStr: "outputs",
		want: applyMutation(simpleTaskSpec, func(spec *v1beta1.TaskSpec) {
			spec.Steps[1].WorkingDir = "/workspace/workspace"
			spec.Steps[2].Args = []string{"gcr.io/hans/sandwiches"}
			spec.Steps[4].WorkingDir = "/workspace/workspace"
			spec.Steps[5].Args = []string{"gcr.io/hans/sandwiches"}
			spec.Steps[8].Image = "/foo/builtImage"
			spec.Steps[9].Image = "/workspace/foo/builtImage"
		}),
	}, {
		name: "output resource specified with path replacement",
		ts:   gcsTaskSpec,
		r:    outputs,
		rStr: "outputs",
		want: applyMutation(gcsTaskSpec, func(spec *v1beta1.TaskSpec) {
			spec.Steps[0].Args = []string{"/workspace/output/bucket"}
		}),
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resources.ApplyResources(tt.ts, tt.r, tt.rStr)
			if d := cmp.Diff(tt.want, got); d != "" {
				t.Errorf("ApplyResources() %s", diff.PrintWantGot(d))
			}
		})
	}
}

func TestApplyWorkspaces(t *testing.T) {
	names.TestingSeed()
	ts := &v1beta1.TaskSpec{
		StepTemplate: &corev1.Container{
			Env: []corev1.EnvVar{{
				Name:  "template-var",
				Value: "$(workspaces.myws.volume)",
			}, {
				Name:  "pvc-name",
				Value: "$(workspaces.myws.claim)",
			}, {
				Name:  "non-pvc-name",
				Value: "$(workspaces.otherws.claim)",
			}},
		},
		Steps: []v1beta1.Step{{Container: corev1.Container{
			Name:       "$(workspaces.myws.volume)",
			Image:      "$(workspaces.otherws.volume)",
			WorkingDir: "$(workspaces.otherws.volume)",
			Args:       []string{"$(workspaces.myws.path)"},
		}}, {Container: corev1.Container{
			Name:  "foo",
			Image: "bar",
			VolumeMounts: []corev1.VolumeMount{{
				Name:      "$(workspaces.myws.volume)",
				MountPath: "path/to/$(workspaces.otherws.path)",
				SubPath:   "$(workspaces.myws.volume)",
			}},
		}}, {Container: corev1.Container{
			Name:  "foo",
			Image: "bar",
			Env: []corev1.EnvVar{{
				Name:  "foo",
				Value: "$(workspaces.myws.volume)",
			}, {
				Name: "baz",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: "$(workspaces.myws.volume)"},
						Key:                  "$(workspaces.myws.volume)",
					},
				},
			}},
			EnvFrom: []corev1.EnvFromSource{{
				Prefix: "$(workspaces.myws.volume)",
				ConfigMapRef: &corev1.ConfigMapEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: "$(workspaces.myws.volume)"},
				},
			}},
		}}},
		Volumes: []corev1.Volume{{
			Name: "$(workspaces.myws.volume)",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "$(workspaces.myws.volume)",
					},
				},
			}}, {
			Name: "some-secret",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: "$(workspaces.myws.volume)",
				},
			}}, {
			Name: "some-pvc",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: "$(workspaces.myws.volume)",
				},
			},
		}},
	}
	for _, tc := range []struct {
		name  string
		spec  *v1beta1.TaskSpec
		decls []v1beta1.WorkspaceDeclaration
		binds []v1beta1.WorkspaceBinding
		want  *v1beta1.TaskSpec
	}{{
		name: "workspace-variable-replacement",
		spec: ts.DeepCopy(),
		decls: []v1beta1.WorkspaceDeclaration{{
			Name: "myws",
		}, {
			Name:      "otherws",
			MountPath: "/foo",
		}},
		binds: []v1beta1.WorkspaceBinding{{
			Name: "myws",
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: "foo",
			},
		}, {
			Name:     "otherws",
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		}},
		want: applyMutation(ts, func(spec *v1beta1.TaskSpec) {
			spec.StepTemplate.Env[0].Value = "ws-9l9zj"
			spec.StepTemplate.Env[1].Value = "foo"
			spec.StepTemplate.Env[2].Value = ""

			spec.Steps[0].Name = "ws-9l9zj"
			spec.Steps[0].Image = "ws-mz4c7"
			spec.Steps[0].WorkingDir = "ws-mz4c7"
			spec.Steps[0].Args = []string{"/workspace/myws"}

			spec.Steps[1].VolumeMounts[0].Name = "ws-9l9zj"
			spec.Steps[1].VolumeMounts[0].MountPath = "path/to//foo"
			spec.Steps[1].VolumeMounts[0].SubPath = "ws-9l9zj"

			spec.Steps[2].Env[0].Value = "ws-9l9zj"
			spec.Steps[2].Env[1].ValueFrom.SecretKeyRef.LocalObjectReference.Name = "ws-9l9zj"
			spec.Steps[2].Env[1].ValueFrom.SecretKeyRef.Key = "ws-9l9zj"
			spec.Steps[2].EnvFrom[0].Prefix = "ws-9l9zj"
			spec.Steps[2].EnvFrom[0].ConfigMapRef.LocalObjectReference.Name = "ws-9l9zj"

			spec.Volumes[0].Name = "ws-9l9zj"
			spec.Volumes[0].VolumeSource.ConfigMap.LocalObjectReference.Name = "ws-9l9zj"
			spec.Volumes[1].VolumeSource.Secret.SecretName = "ws-9l9zj"
			spec.Volumes[2].VolumeSource.PersistentVolumeClaim.ClaimName = "ws-9l9zj"
		}),
	}, {
		name: "optional-workspace-provided-variable-replacement",
		spec: &v1beta1.TaskSpec{Steps: []v1beta1.Step{{
			Script: `test "$(workspaces.ows.bound)" = "true" && echo "$(workspaces.ows.path)"`,
		}}},
		decls: []v1beta1.WorkspaceDeclaration{{
			Name:     "ows",
			Optional: true,
		}},
		binds: []v1beta1.WorkspaceBinding{{
			Name:     "ows",
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		}},
		want: &v1beta1.TaskSpec{Steps: []v1beta1.Step{{
			Script: `test "true" = "true" && echo "/workspace/ows"`,
		}}},
	}, {
		name: "optional-workspace-omitted-variable-replacement",
		spec: &v1beta1.TaskSpec{Steps: []v1beta1.Step{{
			Script: `test "$(workspaces.ows.bound)" = "true" && echo "$(workspaces.ows.path)"`,
		}}},
		decls: []v1beta1.WorkspaceDeclaration{{
			Name:     "ows",
			Optional: true,
		}},
		binds: []v1beta1.WorkspaceBinding{}, // intentionally omitted ows binding
		want: &v1beta1.TaskSpec{Steps: []v1beta1.Step{{
			Script: `test "false" = "true" && echo ""`,
		}}},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			vols := workspace.CreateVolumes(tc.binds)
			got := resources.ApplyWorkspaces(context.Background(), tc.spec, tc.decls, tc.binds, vols)
			if d := cmp.Diff(tc.want, got); d != "" {
				t.Errorf("TestApplyWorkspaces() got diff %s", diff.PrintWantGot(d))
			}
		})
	}
}

func TestApplyWorkspaces_IsolatedWorkspaces(t *testing.T) {
	for _, tc := range []struct {
		name  string
		spec  *v1beta1.TaskSpec
		decls []v1beta1.WorkspaceDeclaration
		binds []v1beta1.WorkspaceBinding
		want  *v1beta1.TaskSpec
	}{{
		name: "step-workspace-with-custom-mountpath",
		spec: &v1beta1.TaskSpec{Steps: []v1beta1.Step{{
			Script: `echo "$(workspaces.ws.path)"`,
			Workspaces: []v1beta1.WorkspaceUsage{{
				Name:      "ws",
				MountPath: "/foo",
			}},
		}, {
			Script: `echo "$(workspaces.ws.path)"`,
		}}, Sidecars: []v1beta1.Sidecar{{
			Script: `echo "$(workspaces.ws.path)"`,
		}}},
		decls: []v1beta1.WorkspaceDeclaration{{
			Name: "ws",
		}},
		want: &v1beta1.TaskSpec{Steps: []v1beta1.Step{{
			Script: `echo "/foo"`,
			Workspaces: []v1beta1.WorkspaceUsage{{
				Name:      "ws",
				MountPath: "/foo",
			}},
		}, {
			Script: `echo "/workspace/ws"`,
		}}, Sidecars: []v1beta1.Sidecar{{
			Script: `echo "/workspace/ws"`,
		}}},
	}, {
		name: "sidecar-workspace-with-custom-mountpath",
		spec: &v1beta1.TaskSpec{Steps: []v1beta1.Step{{
			Script: `echo "$(workspaces.ws.path)"`,
		}}, Sidecars: []v1beta1.Sidecar{{
			Script: `echo "$(workspaces.ws.path)"`,
			Workspaces: []v1beta1.WorkspaceUsage{{
				Name:      "ws",
				MountPath: "/bar",
			}},
		}}},
		decls: []v1beta1.WorkspaceDeclaration{{
			Name: "ws",
		}},
		want: &v1beta1.TaskSpec{Steps: []v1beta1.Step{{
			Script: `echo "/workspace/ws"`,
		}}, Sidecars: []v1beta1.Sidecar{{
			Script: `echo "/bar"`,
			Workspaces: []v1beta1.WorkspaceUsage{{
				Name:      "ws",
				MountPath: "/bar",
			}},
		}}},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := config.ToContext(context.Background(), &config.Config{
				FeatureFlags: &config.FeatureFlags{
					EnableAPIFields: "alpha",
				},
			})
			vols := workspace.CreateVolumes(tc.binds)
			got := resources.ApplyWorkspaces(ctx, tc.spec, tc.decls, tc.binds, vols)
			if d := cmp.Diff(tc.want, got); d != "" {
				t.Errorf("TestApplyWorkspaces() got diff %s", diff.PrintWantGot(d))
			}
		})
	}
}

func TestContext(t *testing.T) {
	for _, tc := range []struct {
		description string
		rtr         resources.ResolvedTaskResources
		tr          v1beta1.TaskRun
		spec        v1beta1.TaskSpec
		want        v1beta1.TaskSpec
	}{{
		description: "context taskName replacement without taskRun in spec container",
		rtr: resources.ResolvedTaskResources{
			TaskName: "Task1",
		},
		tr: v1beta1.TaskRun{},
		spec: v1beta1.TaskSpec{
			Steps: []v1beta1.Step{{
				Container: corev1.Container{
					Name:  "ImageName",
					Image: "$(context.task.name)-1",
				},
			}},
		},
		want: v1beta1.TaskSpec{
			Steps: []v1beta1.Step{{
				Container: corev1.Container{
					Name:  "ImageName",
					Image: "Task1-1",
				},
			}},
		},
	}, {
		description: "context taskName replacement with taskRun in spec container",
		rtr: resources.ResolvedTaskResources{
			TaskName: "Task1",
		},
		tr: v1beta1.TaskRun{
			ObjectMeta: metav1.ObjectMeta{
				Name: "taskrunName",
			},
		},
		spec: v1beta1.TaskSpec{
			Steps: []v1beta1.Step{{
				Container: corev1.Container{
					Name:  "ImageName",
					Image: "$(context.task.name)-1",
				},
			}},
		},
		want: v1beta1.TaskSpec{
			Steps: []v1beta1.Step{{
				Container: corev1.Container{
					Name:  "ImageName",
					Image: "Task1-1",
				},
			}},
		},
	}, {
		description: "context taskRunName replacement with defined taskRun in spec container",
		rtr: resources.ResolvedTaskResources{
			TaskName: "Task1",
		},
		tr: v1beta1.TaskRun{
			ObjectMeta: metav1.ObjectMeta{
				Name: "taskrunName",
			},
		},
		spec: v1beta1.TaskSpec{
			Steps: []v1beta1.Step{{
				Container: corev1.Container{
					Name:  "ImageName",
					Image: "$(context.taskRun.name)-1",
				},
			}},
		},
		want: v1beta1.TaskSpec{
			Steps: []v1beta1.Step{{
				Container: corev1.Container{
					Name:  "ImageName",
					Image: "taskrunName-1",
				},
			}},
		},
	}, {
		description: "context taskRunName replacement with no defined taskRun name in spec container",
		rtr: resources.ResolvedTaskResources{
			TaskName: "Task1",
		},
		tr: v1beta1.TaskRun{},
		spec: v1beta1.TaskSpec{
			Steps: []v1beta1.Step{{
				Container: corev1.Container{
					Name:  "ImageName",
					Image: "$(context.taskRun.name)-1",
				},
			}},
		},
		want: v1beta1.TaskSpec{
			Steps: []v1beta1.Step{{
				Container: corev1.Container{
					Name:  "ImageName",
					Image: "-1",
				},
			}},
		},
	}, {
		description: "context taskRun namespace replacement with no defined namepsace in spec container",
		rtr: resources.ResolvedTaskResources{
			TaskName: "Task1",
		},
		tr: v1beta1.TaskRun{},
		spec: v1beta1.TaskSpec{
			Steps: []v1beta1.Step{{
				Container: corev1.Container{
					Name:  "ImageName",
					Image: "$(context.taskRun.namespace)-1",
				},
			}},
		},
		want: v1beta1.TaskSpec{
			Steps: []v1beta1.Step{{
				Container: corev1.Container{
					Name:  "ImageName",
					Image: "-1",
				},
			}},
		},
	}, {
		description: "context taskRun namespace replacement with defined namepsace in spec container",
		rtr: resources.ResolvedTaskResources{
			TaskName: "Task1",
		},
		tr: v1beta1.TaskRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "taskrunName",
				Namespace: "trNamespace",
			},
		},
		spec: v1beta1.TaskSpec{
			Steps: []v1beta1.Step{{
				Container: corev1.Container{
					Name:  "ImageName",
					Image: "$(context.taskRun.namespace)-1",
				},
			}},
		},
		want: v1beta1.TaskSpec{
			Steps: []v1beta1.Step{{
				Container: corev1.Container{
					Name:  "ImageName",
					Image: "trNamespace-1",
				},
			}},
		},
	}, {
		description: "context taskRunName replacement with no defined taskName in spec container",
		rtr:         resources.ResolvedTaskResources{},
		tr:          v1beta1.TaskRun{},
		spec: v1beta1.TaskSpec{
			Steps: []v1beta1.Step{{
				Container: corev1.Container{
					Name:  "ImageName",
					Image: "$(context.task.name)-1",
				},
			}},
		},
		want: v1beta1.TaskSpec{
			Steps: []v1beta1.Step{{
				Container: corev1.Container{
					Name:  "ImageName",
					Image: "-1",
				},
			}},
		},
	}, {
		description: "context UID replacement",
		rtr: resources.ResolvedTaskResources{
			TaskName: "Task1",
		},
		tr: v1beta1.TaskRun{
			ObjectMeta: metav1.ObjectMeta{
				UID: "UID-1",
			},
		},
		spec: v1beta1.TaskSpec{
			Steps: []v1beta1.Step{{
				Container: corev1.Container{
					Name:  "ImageName",
					Image: "$(context.taskRun.uid)",
				},
			}},
		},
		want: v1beta1.TaskSpec{
			Steps: []v1beta1.Step{{
				Container: corev1.Container{
					Name:  "ImageName",
					Image: "UID-1",
				},
			}},
		},
	}, {
		description: "context retry count replacement",
		rtr:         resources.ResolvedTaskResources{},
		tr: v1beta1.TaskRun{
			Status: v1beta1.TaskRunStatus{
				TaskRunStatusFields: v1beta1.TaskRunStatusFields{
					RetriesStatus: []v1beta1.TaskRunStatus{{
						Status: duckv1beta1.Status{
							Conditions: []apis.Condition{{
								Type:   apis.ConditionSucceeded,
								Status: corev1.ConditionFalse,
							}},
						},
					}, {
						Status: duckv1beta1.Status{
							Conditions: []apis.Condition{{
								Type:   apis.ConditionSucceeded,
								Status: corev1.ConditionFalse,
							}},
						},
					}},
				},
			},
		},
		spec: v1beta1.TaskSpec{
			Steps: []v1beta1.Step{{
				Container: corev1.Container{
					Name:  "ImageName",
					Image: "$(context.task.retry-count)-1",
				},
			}},
		},
		want: v1beta1.TaskSpec{
			Steps: []v1beta1.Step{{
				Container: corev1.Container{
					Name:  "ImageName",
					Image: "2-1",
				},
			}},
		},
	}, {
		description: "context retry count replacement with task that never retries",
		rtr:         resources.ResolvedTaskResources{},
		tr:          v1beta1.TaskRun{},
		spec: v1beta1.TaskSpec{
			Steps: []v1beta1.Step{{
				Container: corev1.Container{
					Name:  "ImageName",
					Image: "$(context.task.retry-count)-1",
				},
			}},
		},
		want: v1beta1.TaskSpec{
			Steps: []v1beta1.Step{{
				Container: corev1.Container{
					Name:  "ImageName",
					Image: "0-1",
				},
			}},
		},
	}} {
		t.Run(tc.description, func(t *testing.T) {
			got := resources.ApplyContexts(&tc.spec, &tc.rtr, &tc.tr)
			if d := cmp.Diff(&tc.want, got); d != "" {
				t.Errorf(diff.PrintWantGot(d))
			}
		})
	}
}

func TestTaskResults(t *testing.T) {
	names.TestingSeed()
	ts := &v1beta1.TaskSpec{
		Results: []v1beta1.TaskResult{{
			Name:        "current-date-unix-timestamp",
			Description: "The current date in unix timestamp format",
		}, {
			Name:        "current-date-human-readable",
			Description: "The current date in humand readable format"},
		},
		Steps: []v1beta1.Step{{
			Container: corev1.Container{
				Name:  "print-date-unix-timestamp",
				Image: "bash:latest",
				Args:  []string{"$(results.current-date-unix-timestamp.path)"},
			},
			Script: "#!/usr/bin/env bash\ndate +%s | tee $(results.current-date-unix-timestamp.path)",
		}, {
			Container: corev1.Container{
				Name:  "print-date-human-readable",
				Image: "bash:latest",
			},
			Script: "#!/usr/bin/env bash\ndate | tee $(results.current-date-human-readable.path)",
		}},
	}
	want := applyMutation(ts, func(spec *v1beta1.TaskSpec) {
		spec.Steps[0].Script = "#!/usr/bin/env bash\ndate +%s | tee /tekton/results/current-date-unix-timestamp"
		spec.Steps[0].Args[0] = "/tekton/results/current-date-unix-timestamp"
		spec.Steps[1].Script = "#!/usr/bin/env bash\ndate | tee /tekton/results/current-date-human-readable"
	})
	got := resources.ApplyTaskResults(ts)
	if d := cmp.Diff(want, got); d != "" {
		t.Errorf("ApplyTaskResults() got diff %s", diff.PrintWantGot(d))
	}
}

func TestApplyStepExitCodePath(t *testing.T) {
	names.TestingSeed()
	ts := &v1beta1.TaskSpec{
		Steps: []v1beta1.Step{{
			Container: corev1.Container{
				Image: "bash:latest",
			},
			Script: "#!/usr/bin/env bash\nexit 11",
		}, {
			Container: corev1.Container{
				Name:  "failing-step",
				Image: "bash:latest",
			},
			Script: "#!/usr/bin/env bash\ncat $(steps.step-unnamed-0.exitCode.path)",
		}, {
			Container: corev1.Container{
				Name:  "check-failing-step",
				Image: "bash:latest",
			},
			Script: "#!/usr/bin/env bash\ncat $(steps.step-failing-step.exitCode.path)",
		}},
	}
	expected := applyMutation(ts, func(spec *v1beta1.TaskSpec) {
		spec.Steps[1].Script = "#!/usr/bin/env bash\ncat /tekton/steps/step-unnamed-0/exitCode"
		spec.Steps[2].Script = "#!/usr/bin/env bash\ncat /tekton/steps/step-failing-step/exitCode"
	})
	got := resources.ApplyStepExitCodePath(ts)
	if d := cmp.Diff(expected, got); d != "" {
		t.Errorf("ApplyStepExitCodePath() got diff %s", diff.PrintWantGot(d))
	}
}

func TestApplyCredentialsPath(t *testing.T) {
	for _, tc := range []struct {
		description string
		spec        v1beta1.TaskSpec
		path        string
		want        v1beta1.TaskSpec
	}{{
		description: "replacement in spec container",
		spec: v1beta1.TaskSpec{
			Steps: []v1beta1.Step{{
				Container: corev1.Container{
					Command: []string{"cp"},
					Args:    []string{"-R", "$(credentials.path)/", "$HOME"},
				},
			}},
		},
		path: "/tekton/creds",
		want: v1beta1.TaskSpec{
			Steps: []v1beta1.Step{{
				Container: corev1.Container{
					Command: []string{"cp"},
					Args:    []string{"-R", "/tekton/creds/", "$HOME"},
				},
			}},
		},
	}, {
		description: "replacement in spec Script",
		spec: v1beta1.TaskSpec{
			Steps: []v1beta1.Step{{
				Script: `cp -R "$(credentials.path)/" $HOME`,
			}},
		},
		path: "/tekton/home",
		want: v1beta1.TaskSpec{
			Steps: []v1beta1.Step{{
				Script: `cp -R "/tekton/home/" $HOME`,
			}},
		},
	}} {
		t.Run(tc.description, func(t *testing.T) {
			got := resources.ApplyCredentialsPath(&tc.spec, tc.path)
			if d := cmp.Diff(&tc.want, got); d != "" {
				t.Errorf(diff.PrintWantGot(d))
			}
		})
	}
}
