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

package v1alpha1_test

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	apisconfig "github.com/tektoncd/pipeline/pkg/apis/config"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1alpha1"
	v1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"github.com/tektoncd/pipeline/pkg/client/clientset/versioned/scheme"
	"github.com/tektoncd/pipeline/test/diff"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
)

func TestGetParams(t *testing.T) {
	for _, c := range []struct {
		desc string
		spec v1alpha1.RunSpec
		name string
		want *v1beta1.Param
	}{{
		desc: "no params",
		spec: v1alpha1.RunSpec{},
		name: "anything",
		want: nil,
	}, {
		desc: "found",
		spec: v1alpha1.RunSpec{
			Params: []v1beta1.Param{{
				Name:  "first",
				Value: *v1beta1.NewArrayOrString("blah"),
			}, {
				Name:  "foo",
				Value: *v1beta1.NewArrayOrString("bar"),
			}},
		},
		name: "foo",
		want: &v1beta1.Param{
			Name:  "foo",
			Value: *v1beta1.NewArrayOrString("bar"),
		},
	}, {
		desc: "not found",
		spec: v1alpha1.RunSpec{
			Params: []v1beta1.Param{{
				Name:  "first",
				Value: *v1beta1.NewArrayOrString("blah"),
			}, {
				Name:  "foo",
				Value: *v1beta1.NewArrayOrString("bar"),
			}},
		},
		name: "bar",
		want: nil,
	}, {
		// This shouldn't happen since it's invalid, but just in
		// case, GetParams just returns the first param it finds with
		// the specified name.
		desc: "multiple with same name",
		spec: v1alpha1.RunSpec{
			Params: []v1beta1.Param{{
				Name:  "first",
				Value: *v1beta1.NewArrayOrString("blah"),
			}, {
				Name:  "foo",
				Value: *v1beta1.NewArrayOrString("bar"),
			}, {
				Name:  "foo",
				Value: *v1beta1.NewArrayOrString("second bar"),
			}},
		},
		name: "foo",
		want: &v1beta1.Param{
			Name:  "foo",
			Value: *v1beta1.NewArrayOrString("bar"),
		},
	}} {
		t.Run(c.desc, func(t *testing.T) {
			got := c.spec.GetParam(c.name)
			if d := cmp.Diff(c.want, got); d != "" {
				t.Fatalf("Diff(-want,+got): %v", d)
			}
		})
	}
}

func TestRunHasStarted(t *testing.T) {
	params := []struct {
		name          string
		runStatus     v1alpha1.RunStatus
		expectedValue bool
	}{{
		name:          "runWithNoStartTime",
		runStatus:     v1alpha1.RunStatus{},
		expectedValue: false,
	}, {
		name: "runWithStartTime",
		runStatus: v1alpha1.RunStatus{
			RunStatusFields: v1alpha1.RunStatusFields{
				StartTime: &metav1.Time{Time: time.Now()},
			},
		},
		expectedValue: true,
	}, {
		name: "runWithZeroStartTime",
		runStatus: v1alpha1.RunStatus{
			RunStatusFields: v1alpha1.RunStatusFields{
				StartTime: &metav1.Time{},
			},
		},
		expectedValue: false,
	}}
	for _, tc := range params {
		t.Run(tc.name, func(t *testing.T) {
			run := v1alpha1.Run{}
			run.Status = tc.runStatus
			if run.HasStarted() != tc.expectedValue {
				t.Fatalf("Expected run HasStarted() to return %t but got %t", tc.expectedValue, run.HasStarted())
			}
		})
	}
}

func TestRunIsDone(t *testing.T) {
	run := v1alpha1.Run{
		Status: v1alpha1.RunStatus{
			Status: duckv1.Status{
				Conditions: duckv1.Conditions{{
					Type:   apis.ConditionSucceeded,
					Status: corev1.ConditionFalse,
				}},
			},
		},
	}

	if !run.IsDone() {
		t.Fatal("Expected run status to be done")
	}
}

func TestRunIsCancelled(t *testing.T) {
	run := v1alpha1.Run{
		Spec: v1alpha1.RunSpec{
			Status: v1alpha1.RunSpecStatusCancelled,
		},
	}
	if !run.IsCancelled() {
		t.Fatal("Expected run status to be cancelled")
	}
}

// TestRunStatusExtraFields tests that extraFields in a RunStatus can be parsed
// from YAML.
func TestRunStatus(t *testing.T) {
	in := `apiVersion: tekton.dev/v1alpha1
kind: Run
metadata:
  name: run
spec:
  ref:
    apiVersion: example.dev/v0
    kind: Example
status:
  conditions:
  - type: "Succeeded"
    status: "True"
  results:
  - name: foo
    value: bar
  extraFields:
    simple: 'hello'
    complex:
      hello: ['w', 'o', 'r', 'l', 'd']
`
	var r v1alpha1.Run
	if _, _, err := scheme.Codecs.UniversalDeserializer().Decode([]byte(in), nil, &r); err != nil {
		t.Fatalf("Decode YAML: %v", err)
	}

	want := &v1alpha1.Run{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "tekton.dev/v1alpha1",
			Kind:       "Run",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "run",
		},
		Spec: v1alpha1.RunSpec{
			Ref: &v1alpha1.TaskRef{
				APIVersion: "example.dev/v0",
				Kind:       "Example",
			},
		},
		Status: v1alpha1.RunStatus{
			Status: duckv1.Status{
				Conditions: duckv1.Conditions{{
					Type:   apis.ConditionSucceeded,
					Status: corev1.ConditionTrue,
				}},
			},
			RunStatusFields: v1alpha1.RunStatusFields{
				// Results are parsed correctly.
				Results: []v1alpha1.RunResult{{
					Name:  "foo",
					Value: "bar",
				}},
				// Any extra fields are simply stored as JSON bytes.
				ExtraFields: runtime.RawExtension{
					Raw: []byte(`{"complex":{"hello":["w","o","r","l","d"]},"simple":"hello"}`),
				},
			},
		},
	}
	if d := cmp.Diff(want, &r); d != "" {
		t.Fatalf("Diff(-want,+got): %s", d)
	}
}

func TestEncodeDecodeExtraFields(t *testing.T) {
	type Mystatus struct {
		S string
		I int
	}
	status := Mystatus{S: "one", I: 1}
	r := &v1alpha1.RunStatus{}
	if err := r.EncodeExtraFields(&status); err != nil {
		t.Fatalf("EncodeExtraFields failed: %s", err)
	}
	newStatus := Mystatus{}
	if err := r.DecodeExtraFields(&newStatus); err != nil {
		t.Fatalf("DecodeExtraFields failed: %s", err)
	}
	if d := cmp.Diff(status, newStatus); d != "" {
		t.Fatalf("Diff(-want,+got): %s", d)
	}
}

func TestRunGetTimeOut(t *testing.T) {
	testCases := []struct {
		name          string
		run           v1alpha1.Run
		expectedValue time.Duration
	}{{
		name:          "runWithNoTimeout",
		run:           v1alpha1.Run{},
		expectedValue: apisconfig.DefaultTimeoutMinutes * time.Minute,
	}, {
		name: "runWithTimeout",
		run: v1alpha1.Run{
			TypeMeta: metav1.TypeMeta{Kind: "kind", APIVersion: "apiVersion"},
			Spec:     v1alpha1.RunSpec{Timeout: &metav1.Duration{10 * time.Second}},
		},
		expectedValue: 10 * time.Second,
	}}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.run.GetTimeout()
			if d := cmp.Diff(result, tc.expectedValue); d != "" {
				t.Fatalf(diff.PrintWantGot(d))
			}
		})
	}
}

func TestRunHasTimedOut(t *testing.T) {
	testCases := []struct {
		name          string
		run           v1alpha1.Run
		expectedValue bool
	}{{
		name:          "runWithNoStartTimeNoTimeout",
		run:           v1alpha1.Run{},
		expectedValue: false,
	}, {
		name: "runWithStartTimeNoTimeout",
		run: v1alpha1.Run{
			TypeMeta: metav1.TypeMeta{Kind: "kind", APIVersion: "apiVersion"},
			Status: v1alpha1.RunStatus{
				RunStatusFields: v1alpha1.RunStatusFields{
					StartTime: &metav1.Time{Time: time.Now()},
				},
			}},
		expectedValue: false,
	}, {
		name: "runWithStartTimeNoTimeout2",
		run: v1alpha1.Run{
			TypeMeta: metav1.TypeMeta{Kind: "kind", APIVersion: "apiVersion"},
			Status: v1alpha1.RunStatus{
				RunStatusFields: v1alpha1.RunStatusFields{
					StartTime: &metav1.Time{Time: time.Now().Add(-1 * time.Hour)},
				},
			}},
		expectedValue: true,
	}, {
		name: "runWithStartTimeAndTimeout",
		run: v1alpha1.Run{
			TypeMeta: metav1.TypeMeta{Kind: "kind", APIVersion: "apiVersion"},
			Spec:     v1alpha1.RunSpec{Timeout: &metav1.Duration{10 * time.Second}},
			Status: v1alpha1.RunStatus{RunStatusFields: v1alpha1.RunStatusFields{
				StartTime: &metav1.Time{Time: time.Now().Add(-1 * time.Hour)},
			}}},
		expectedValue: true,
	}, {
		name: "runWithNoStartTimeAndTimeout",
		run: v1alpha1.Run{
			TypeMeta: metav1.TypeMeta{Kind: "kind", APIVersion: "apiVersion"},
			Spec:     v1alpha1.RunSpec{Timeout: &metav1.Duration{1 * time.Second}},
		},
		expectedValue: false,
	}, {
		name: "runWithStartTimeAndTimeout2",
		run: v1alpha1.Run{
			TypeMeta: metav1.TypeMeta{Kind: "kind", APIVersion: "apiVersion"},
			Spec:     v1alpha1.RunSpec{Timeout: &metav1.Duration{10 * time.Second}},
			Status: v1alpha1.RunStatus{RunStatusFields: v1alpha1.RunStatusFields{
				StartTime: &metav1.Time{Time: time.Now()},
			}}},
		expectedValue: false,
	}}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.run.HasTimedOut()
			if d := cmp.Diff(result, tc.expectedValue); d != "" {
				t.Fatalf(diff.PrintWantGot(d))
			}
		})
	}
}
