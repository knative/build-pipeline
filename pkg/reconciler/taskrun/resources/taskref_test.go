/*
 Copyright 2020 The Tekton Authors

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
	"github.com/tektoncd/pipeline/pkg/remote/git"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-containerregistry/pkg/registry"
	tb "github.com/tektoncd/pipeline/internal/builder/v1beta1"
	"github.com/tektoncd/pipeline/pkg/apis/config"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1alpha1"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"github.com/tektoncd/pipeline/pkg/client/clientset/versioned/fake"
	"github.com/tektoncd/pipeline/pkg/reconciler/taskrun/resources"
	"github.com/tektoncd/pipeline/test"
	"github.com/tektoncd/pipeline/test/diff"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	fakek8s "k8s.io/client-go/kubernetes/fake"
	logtesting "knative.dev/pkg/logging/testing"
)

func TestLocalTaskRef(t *testing.T) {
	testcases := []struct {
		name     string
		tasks    []runtime.Object
		ref      *v1beta1.TaskRef
		expected runtime.Object
		wantErr  bool
	}{
		{
			name: "local-task",
			tasks: []runtime.Object{
				tb.Task("simple", tb.TaskNamespace("default")),
				tb.Task("dummy", tb.TaskNamespace("default")),
			},
			ref: &v1beta1.TaskRef{
				Name: "simple",
			},
			expected: tb.Task("simple", tb.TaskNamespace("default")),
			wantErr:  false,
		},
		{
			name: "local-clustertask",
			tasks: []runtime.Object{
				tb.ClusterTask("cluster-task"),
				tb.ClusterTask("dummy-task"),
			},
			ref: &v1beta1.TaskRef{
				Name: "cluster-task",
				Kind: "ClusterTask",
			},
			expected: tb.ClusterTask("cluster-task"),
			wantErr:  false,
		},
		{
			name:  "task-not-found",
			tasks: []runtime.Object{},
			ref: &v1beta1.TaskRef{
				Name: "simple",
			},
			expected: nil,
			wantErr:  true,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()
			tektonclient := fake.NewSimpleClientset(tc.tasks...)

			lc := &resources.LocalTaskRefResolver{
				Namespace:    "default",
				Kind:         tc.ref.Kind,
				Tektonclient: tektonclient,
			}

			task, err := lc.GetTask(ctx, tc.ref.Name)
			if tc.wantErr && err == nil {
				t.Fatal("Expected error but found nil instead")
			} else if !tc.wantErr && err != nil {
				t.Fatalf("Received unexpected error ( %#v )", err)
			}

			if d := cmp.Diff(task, tc.expected); tc.expected != nil && d != "" {
				t.Error(diff.PrintWantGot(d))
			}
		})
	}
}

func TestGetTaskFunc(t *testing.T) {
	// Set up a fake registry to push an image to.
	s := httptest.NewServer(registry.New())
	defer s.Close()
	u, err := url.Parse(s.URL)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	cfg := config.NewStore(logtesting.TestLogger(t))
	cfg.OnConfigChanged(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: config.GetFeatureFlagsConfigName()},
		Data: map[string]string{
			"enable-tekton-oci-bundles": "true",
		},
	})
	ctx = cfg.ToContext(ctx)

	testcases := []struct {
		name         string
		localTasks   []runtime.Object
		remoteTasks  []runtime.Object
		ref          *v1beta1.TaskRef
		expected     runtime.Object
		expectedKind v1beta1.TaskKind
	}{
		{
			name: "remote-task",
			localTasks: []runtime.Object{
				tb.Task("simple", tb.TaskType, tb.TaskNamespace("default"), tb.TaskSpec(tb.Step("something"))),
			},
			remoteTasks: []runtime.Object{
				tb.Task("simple", tb.TaskType),
				tb.Task("dummy", tb.TaskType),
			},
			ref: &v1beta1.TaskRef{
				Name:   "simple",
				Bundle: u.Host + "/remote-task",
			},
			expected:     tb.Task("simple", tb.TaskType),
			expectedKind: v1beta1.NamespacedTaskKind,
		}, {
			name: "local-task",
			localTasks: []runtime.Object{
				tb.Task("simple", tb.TaskType, tb.TaskNamespace("default"), tb.TaskSpec(tb.Step("something"))),
			},
			remoteTasks: []runtime.Object{
				tb.Task("simple", tb.TaskType),
				tb.Task("dummy", tb.TaskType),
			},
			ref: &v1beta1.TaskRef{
				Name: "simple",
			},
			expected:     tb.Task("simple", tb.TaskType, tb.TaskNamespace("default"), tb.TaskSpec(tb.Step("something"))),
			expectedKind: v1beta1.NamespacedTaskKind,
		}, {
			name: "remote-cluster-task",
			localTasks: []runtime.Object{
				&v1beta1.ClusterTask{
					TypeMeta:   metav1.TypeMeta{APIVersion: "v1alpha1", Kind: "ClusterTask"},
					ObjectMeta: metav1.ObjectMeta{Name: "simple"},
					Spec:       v1beta1.TaskSpec{Params: []v1beta1.ParamSpec{{Name: "foo"}}},
				},
			},
			remoteTasks: []runtime.Object{
				&v1beta1.ClusterTask{
					TypeMeta:   metav1.TypeMeta{APIVersion: "tekton.dev/v1alpha1", Kind: "ClusterTask"},
					ObjectMeta: metav1.ObjectMeta{Name: "simple"},
				},
				&v1beta1.ClusterTask{
					TypeMeta:   metav1.TypeMeta{APIVersion: "tekton.dev/v1alpha1", Kind: "ClusterTask"},
					ObjectMeta: metav1.ObjectMeta{Name: "dummy"},
				},
			},
			ref: &v1beta1.TaskRef{
				Name:   "simple",
				Kind:   v1alpha1.ClusterTaskKind,
				Bundle: u.Host + "/remote-cluster-task",
			},
			expected:     tb.ClusterTask("simple"),
			expectedKind: v1beta1.ClusterTaskKind,
		}, {
			name: "local-cluster-task",
			localTasks: []runtime.Object{
				tb.ClusterTask("simple", tb.ClusterTaskType, tb.ClusterTaskSpec(tb.Step("something"))),
			},
			remoteTasks: []runtime.Object{
				tb.ClusterTask("simple", tb.ClusterTaskType),
				tb.ClusterTask("dummy", tb.ClusterTaskType),
			},
			ref: &v1alpha1.TaskRef{
				Name: "simple",
				Kind: v1alpha1.ClusterTaskKind,
			},
			expected:     tb.ClusterTask("simple", tb.ClusterTaskType, tb.ClusterTaskSpec(tb.Step("something"))),
			expectedKind: v1beta1.ClusterTaskKind,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			tektonclient := fake.NewSimpleClientset(tc.localTasks...)
			kubeclient := fakek8s.NewSimpleClientset(&v1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "default",
				},
			})
			gitFactory := &git.Factory{}

			_, err := test.CreateImage(u.Host+"/"+tc.name, tc.remoteTasks...)
			if err != nil {
				t.Fatalf("failed to upload test image: %s", err.Error())
			}

			fn, kind, err := resources.GetTaskFunc(ctx, kubeclient, tektonclient, gitFactory, tc.ref, "default", "default")
			if err != nil {
				t.Fatalf("failed to get task fn: %s", err.Error())
			}

			if string(tc.expectedKind) != string(kind) {
				t.Errorf("expected kind %s did not match actual kind %s", tc.expectedKind, kind)
			}

			task, err := fn(ctx, tc.ref.Name)
			if err != nil {
				t.Fatalf("failed to call taskfn: %s", err.Error())
			}

			if diff := cmp.Diff(task, tc.expected); tc.expected != nil && diff != "" {
				t.Error(diff)
			}
		})
	}
}
