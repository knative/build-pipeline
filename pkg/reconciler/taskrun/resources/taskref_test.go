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
	"testing"

	"github.com/google/go-cmp/cmp"
	authorizationv1 "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/runtime"
	fakekubeclientset "k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"

	tb "github.com/tektoncd/pipeline/internal/builder/v1beta1"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1alpha1"
	"github.com/tektoncd/pipeline/pkg/client/clientset/versioned/fake"
	"github.com/tektoncd/pipeline/pkg/reconciler/taskrun/resources"
	"github.com/tektoncd/pipeline/test/diff"
)

func TestTaskRef(t *testing.T) {
	var acceptReactorFunc, denyReactorFunc ktesting.ReactionFunc
	acceptReactorFunc = func(action ktesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, &authorizationv1.SubjectAccessReview{Status: authorizationv1.SubjectAccessReviewStatus{Allowed: true}}, nil
	}
	denyReactorFunc = func(action ktesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, &authorizationv1.SubjectAccessReview{Status: authorizationv1.SubjectAccessReviewStatus{Allowed: false}}, nil
	}
	testcases := []struct {
		name     string
		tasks    []runtime.Object
		ref      *v1alpha1.TaskRef
		expected runtime.Object
		wantErr  bool
		reactor  ktesting.ReactionFunc
	}{
		{
			name: "local-task",
			tasks: []runtime.Object{
				tb.Task("simple", tb.TaskNamespace("default")),
				tb.Task("dummy", tb.TaskNamespace("default")),
			},
			ref: &v1alpha1.TaskRef{
				Name: "simple",
			},
			expected: tb.Task("simple", tb.TaskNamespace("default")),
			wantErr:  false,
			reactor:  acceptReactorFunc,
		},
		{
			name: "local-clustertask",
			tasks: []runtime.Object{
				tb.ClusterTask("cluster-task"),
				tb.ClusterTask("dummy-task"),
			},
			ref: &v1alpha1.TaskRef{
				Name: "cluster-task",
				Kind: "ClusterTask",
			},
			expected: tb.ClusterTask("cluster-task"),
			wantErr:  false,
			reactor:  acceptReactorFunc,
		},
		{
			name: "local-clustertask-not-allowed",
			tasks: []runtime.Object{
				tb.ClusterTask("cluster-task"),
				tb.ClusterTask("dummy-task"),
			},
			ref: &v1alpha1.TaskRef{
				Name: "cluster-task",
				Kind: "ClusterTask",
			},
			expected: nil,
			wantErr:  true,
			reactor:  denyReactorFunc,
		},
		{
			name:  "task-not-found",
			tasks: []runtime.Object{},
			ref: &v1alpha1.TaskRef{
				Name: "simple",
			},
			expected: nil,
			wantErr:  true,
			reactor:  acceptReactorFunc,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			tektonclient := fake.NewSimpleClientset(tc.tasks...)
			sarclient := fakekubeclientset.NewSimpleClientset()
			sarclient.PrependReactor("create", "subjectaccessreviews", tc.reactor)

			lc := &resources.LocalTaskRefResolver{
				Namespace:    "default",
				Kind:         tc.ref.Kind,
				TaskRunName:  tc.ref.Name,
				TaskRunSA:    "default",
				Tektonclient: tektonclient,
				SarClient:    sarclient.AuthorizationV1(),
			}

			task, err := lc.GetTask(tc.ref.Name)
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
