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

package v1alpha1_test

import (
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1alpha1"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/tektoncd/pipeline/test/names"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNewClusterResource(t *testing.T) {
	for _, c := range []struct {
		desc     string
		resource *v1alpha1.PipelineResource
		want     *v1alpha1.ClusterResource
	}{{
		desc: "basic cluster resource",
		resource: &v1alpha1.PipelineResource{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-clus ter-resource",
				Namespace: "foo",
			},
			Spec: v1alpha1.PipelineResourceSpec{
				Type: v1alpha1.PipelineResourceTypeCluster,
				Params: []v1alpha1.Param{{
					Name:  "name",
					Value: "test_cluster_resource",
				}, {
					Name:  "url",
					Value: "http://10.10.10.10",
				}, {
					Name:  "cadata",
					Value: "bXktY2x1c3Rlci1jZXJ0Cg",
				}, {
					Name:  "token",
					Value: "my-token",
				},
				},
			},
		},
		want: &v1alpha1.ClusterResource{
			Name:   "test_cluster_resource",
			Type:   v1alpha1.PipelineResourceTypeCluster,
			URL:    "http://10.10.10.10",
			CAData: []byte("my-cluster-cert"),
			Token:  "my-token",
		},
	}, {
		desc: "resource with password instead of token",
		resource: &v1alpha1.PipelineResource{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster-resource",
				Namespace: "foo",
			},
			Spec: v1alpha1.PipelineResourceSpec{
				Type: v1alpha1.PipelineResourceTypeCluster,
				Params: []v1alpha1.Param{{
					Name:  "name",
					Value: "test_cluster_resource",
				}, {
					Name:  "url",
					Value: "http://10.10.10.10",
				}, {
					Name:  "cadata",
					Value: "bXktY2x1c3Rlci1jZXJ0Cg",
				}, {
					Name:  "username",
					Value: "user",
				}, {
					Name:  "password",
					Value: "pass",
				},
				},
			},
		},
		want: &v1alpha1.ClusterResource{
			Name:     "test_cluster_resource",
			Type:     v1alpha1.PipelineResourceTypeCluster,
			URL:      "http://10.10.10.10",
			CAData:   []byte("my-cluster-cert"),
			Username: "user",
			Password: "pass",
		},
	}, {
		desc: "set insecure flag to true when there is no cert",
		resource: &v1alpha1.PipelineResource{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster-resource",
				Namespace: "foo",
			},
			Spec: v1alpha1.PipelineResourceSpec{
				Type: v1alpha1.PipelineResourceTypeCluster,
				Params: []v1alpha1.Param{{
					Name:  "Name",
					Value: "test.cluster.resource",
				}, {
					Name:  "url",
					Value: "http://10.10.10.10",
				}, {
					Name:  "token",
					Value: "my-token",
				},
				},
			},
		},
		want: &v1alpha1.ClusterResource{
			Name:     "test.cluster.resource",
			Type:     v1alpha1.PipelineResourceTypeCluster,
			URL:      "http://10.10.10.10",
			Token:    "my-token",
			Insecure: true,
		},
	}, {
		desc: "basic resource with secrets",
		resource: &v1alpha1.PipelineResource{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster-resource",
				Namespace: "foo",
			},
			Spec: v1alpha1.PipelineResourceSpec{
				Type: v1alpha1.PipelineResourceTypeCluster,
				Params: []v1alpha1.Param{{
					Name:  "name",
					Value: "test-cluster-resource",
				}, {
					Name:  "url",
					Value: "http://10.10.10.10",
				}},
				SecretParams: []v1alpha1.SecretParam{{
					FieldName:  "cadata",
					SecretKey:  "cadatakey",
					SecretName: "secret1",
				}, {
					FieldName:  "token",
					SecretKey:  "tokenkey",
					SecretName: "secret1",
				}},
			},
		},
		want: &v1alpha1.ClusterResource{
			Name: "test-cluster-resource",
			Type: v1alpha1.PipelineResourceTypeCluster,
			URL:  "http://10.10.10.10",
			Secrets: []v1alpha1.SecretParam{{
				FieldName:  "cadata",
				SecretKey:  "cadatakey",
				SecretName: "secret1",
			}, {
				FieldName:  "token",
				SecretKey:  "tokenkey",
				SecretName: "secret1",
			}},
		},
	}} {
		t.Run(c.desc, func(t *testing.T) {
			got, err := v1alpha1.NewClusterResource(c.resource)
			if err != nil {
				t.Errorf("Test: %q; TestNewClusterResource() error = %v", c.desc, err)
			}
			if d := cmp.Diff(got, c.want); d != "" {
				t.Errorf("Diff:\n%s", d)
			}
		})
	}
}

func Test_ClusterResource_GetDownloadContainerSpec(t *testing.T) {
	names.TestingSeed()
	testcases := []struct {
		name            string
		clusterResource *v1alpha1.ClusterResource
		wantContainers  []corev1.Container
		wantErr         bool
	}{{
		name: "valid cluster resource config",
		clusterResource: &v1alpha1.ClusterResource{
			Name: "test-cluster-resource",
			Type: v1alpha1.PipelineResourceTypeCluster,
			URL:  "http://10.10.10.10",
			Secrets: []v1alpha1.SecretParam{{
				FieldName:  "cadata",
				SecretKey:  "cadatakey",
				SecretName: "secret1",
			}},
		},
		wantContainers: []corev1.Container{{
			Name:    "kubeconfig-9l9zj",
			Image:   "override-with-kubeconfig-writer:latest",
			Command: []string{"/ko-app/kubeconfigwriter"},
			Args:    []string{"-clusterConfig", `{"name":"test-cluster-resource","type":"cluster","url":"http://10.10.10.10","revision":"","username":"","password":"","token":"","Insecure":false,"cadata":null,"secrets":[{"fieldName":"cadata","secretKey":"cadatakey","secretName":"secret1"}]}`},
			Env: []corev1.EnvVar{{
				Name: "CADATA",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "secret1",
						},
						Key: "cadatakey",
					},
				},
			}},
		}},
	}}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			gotContainers, err := tc.clusterResource.GetDownloadContainerSpec()
			if tc.wantErr && err == nil {
				t.Fatalf("Expected error to be %t but got %v:", tc.wantErr, err)
			}
			if d := cmp.Diff(gotContainers, tc.wantContainers); d != "" {
				t.Errorf("Error mismatch between download containers spec: %s", d)
			}
		})
	}
}
