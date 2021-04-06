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

package git_test

import (
	"fmt"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/registry"
	tb "github.com/tektoncd/pipeline/internal/builder/v1beta1"
	"github.com/tektoncd/pipeline/pkg/remote"
	"github.com/tektoncd/pipeline/pkg/remote/oci"
	"github.com/tektoncd/pipeline/test"
	"github.com/tektoncd/pipeline/test/diff"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestGitResolver(t *testing.T) {
	// Set up a fake registry to push an image to.
	s := httptest.NewServer(registry.New())
	defer s.Close()
	u, err := url.Parse(s.URL)
	if err != nil {
		t.Fatal(err)
	}

	testcases := []struct {
		name         string
		objs         []runtime.Object
		listExpected []remote.ResolvedObject
		wantErr      string
	}{
		{
			name: "single-task",
			objs: []runtime.Object{
				tb.Task("simple-task", tb.TaskType),
			},
			listExpected: []remote.ResolvedObject{{Kind: "task", APIVersion: "v1beta1", Name: "simple-task"}},
		},
		{
			name: "cluster-task",
			objs: []runtime.Object{
				tb.ClusterTask("simple-task", tb.ClusterTaskType),
			},
			listExpected: []remote.ResolvedObject{{Kind: "clustertask", APIVersion: "v1beta1", Name: "simple-task"}},
		},
		{
			name: "multiple-tasks",
			objs: []runtime.Object{
				tb.Task("first-task", tb.TaskType),
				tb.Task("second-task", tb.TaskType),
			},
			listExpected: []remote.ResolvedObject{
				{Kind: "task", APIVersion: "v1beta1", Name: "first-task"},
				{Kind: "task", APIVersion: "v1beta1", Name: "second-task"},
			},
		},
		{
			name: "too-many-objects",
			objs: []runtime.Object{
				tb.Task("first-task", tb.TaskType),
				tb.Task("second-task", tb.TaskType),
				tb.Task("third-task", tb.TaskType),
				tb.Task("fourth-task", tb.TaskType),
				tb.Task("fifth-task", tb.TaskType),
				tb.Task("sixth-task", tb.TaskType),
				tb.Task("seventh-task", tb.TaskType),
				tb.Task("eighth-task", tb.TaskType),
				tb.Task("ninth-task", tb.TaskType),
				tb.Task("tenth-task", tb.TaskType),
				tb.Task("eleventh-task", tb.TaskType),
			},
			listExpected: []remote.ResolvedObject{},
			wantErr:      "contained more than the maximum 10 allow objects",
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a new image with the objects.
			ref, err := test.CreateImage(fmt.Sprintf("%s/testociresolve/%s", u.Host, tc.name), tc.objs...)
			if err != nil {
				t.Fatalf("could not push image: %#v", err)
			}

			resolver := oci.NewResolver(ref, authn.DefaultKeychain)
			listActual, err := resolver.List()
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("want error containing %q but got: %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error listing contents of image: %v", err)
			}

			// The contents of the image are in a specific order so we can expect this iteration to be consistent.
			for idx, actual := range listActual {
				if d := cmp.Diff(actual, tc.listExpected[idx]); d != "" {
					t.Error(diff.PrintWantGot(d))
				}
			}

			for _, obj := range tc.objs {
				actual, err := resolver.Get(strings.ToLower(obj.GetObjectKind().GroupVersionKind().Kind), getObjectName(obj))
				if err != nil {
					t.Fatalf("could not retrieve object from image: %#v", err)
				}

				if d := cmp.Diff(actual, obj); d != "" {
					t.Error(diff.PrintWantGot(d))
				}
			}
		})
	}
}

func getObjectName(obj runtime.Object) string {
	return reflect.Indirect(reflect.ValueOf(obj)).FieldByName("ObjectMeta").FieldByName("Name").String()
}
