// +build e2e

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

package test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1alpha1"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"github.com/tektoncd/pipeline/test/diff"
	"go.opencensus.io/trace"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	knativetest "knative.dev/pkg/test"
	"knative.dev/pkg/test/helpers"
)

const (
	apiVersion = "example.dev/v0"
	kind       = "Example"
)

var supportedFeatureGates = map[string]string{
	"enable-custom-tasks": "true",
	"enable-api-fields":   "alpha",
}

func TestCustomTask(t *testing.T) {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	c, namespace := setup(ctx, t, requireAnyGate(supportedFeatureGates))
	knativetest.CleanupOnInterrupt(func() { tearDown(ctx, t, c, namespace) }, t.Logf)
	defer tearDown(ctx, t, c, namespace)
	customTaskRawSpec := []byte(`{"field1":123,"field2":"value"}`)
	metadataLabel := map[string]string{"test-label": "test"}
	// Create a PipelineRun that runs a Custom Task.
	pipelineRunName := "custom-task-pipeline"
	if _, err := c.PipelineRunClient.Create(
		ctx,
		&v1beta1.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{Name: pipelineRunName},
			Spec: v1beta1.PipelineRunSpec{
				PipelineSpec: &v1beta1.PipelineSpec{
					Tasks: []v1beta1.PipelineTask{{
						Name: "custom-task-ref",
						TaskRef: &v1beta1.TaskRef{
							APIVersion: apiVersion,
							Kind:       kind,
						},
					}, {
						Name: "custom-task-spec",
						TaskSpec: &v1beta1.EmbeddedTask{
							TypeMeta: runtime.TypeMeta{
								APIVersion: apiVersion,
								Kind:       kind,
							},
							Metadata: v1beta1.PipelineTaskMetadata{Labels: metadataLabel},
							Spec: runtime.RawExtension{
								Raw: customTaskRawSpec,
							},
						},
					}, {
						Name: "result-consumer",
						Params: []v1beta1.Param{{
							Name: "input-result-from-custom-task-ref", Value: *v1beta1.NewArrayOrString("$(tasks.custom-task-ref.results.runResult)"),
						}, {
							Name: "input-result-from-custom-task-spec", Value: *v1beta1.NewArrayOrString("$(tasks.custom-task-spec.results.runResult)"),
						}},
						TaskSpec: &v1beta1.EmbeddedTask{TaskSpec: v1beta1.TaskSpec{
							Params: []v1beta1.ParamSpec{{
								Name: "input-result-from-custom-task-ref", Type: v1beta1.ParamTypeString,
							}, {
								Name: "input-result-from-custom-task-spec", Type: v1beta1.ParamTypeString,
							}},
							Steps: []v1beta1.Step{{Container: corev1.Container{
								Image:   "ubuntu",
								Command: []string{"/bin/bash"},
								Args:    []string{"-c", "echo $(input-result-from-custom-task-ref) $(input-result-from-custom-task-spec)"},
							}}},
						}},
					}},
					Results: []v1beta1.PipelineResult{{
						Name:  "prResult-ref",
						Value: "$(tasks.custom-task-ref.results.runResult)",
					}, {
						Name:  "prResult-spec",
						Value: "$(tasks.custom-task-spec.results.runResult)",
					}},
				},
			},
		},
		metav1.CreateOptions{}); err != nil {
		t.Fatalf("Failed to create PipelineRun %q: %v", pipelineRunName, err)
	}

	// Wait for the PipelineRun to start.
	if err := WaitForPipelineRunState(ctx, c, pipelineRunName, time.Minute, Running(pipelineRunName), "PipelineRunRunning"); err != nil {
		t.Fatalf("Waiting for PipelineRun to start running: %v", err)
	}

	// Get the status of the PipelineRun.
	pr, err := c.PipelineRunClient.Get(ctx, pipelineRunName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed to get PipelineRun %q: %v", pipelineRunName, err)
	}

	// Get the Run name.
	if len(pr.Status.Runs) != 2 {
		t.Fatalf("PipelineRun had unexpected .status.runs; got %d, want 2", len(pr.Status.Runs))
	}

	for runName := range pr.Status.Runs {
		// Get the Run.
		r, err := c.RunClient.Get(ctx, runName, metav1.GetOptions{})
		if err != nil {
			t.Fatalf("Failed to get Run %q: %v", runName, err)
		}
		if r.IsDone() {
			t.Fatalf("Run unexpectedly done: %v", r.Status.GetCondition(apis.ConditionSucceeded))
		}

		// Simulate a Custom Task controller updating the Run to done/successful.
		r.Status = v1alpha1.RunStatus{
			Status: duckv1.Status{
				Conditions: duckv1.Conditions{{
					Type:   apis.ConditionSucceeded,
					Status: corev1.ConditionTrue,
				}},
			},
			RunStatusFields: v1alpha1.RunStatusFields{
				Results: []v1alpha1.RunResult{{
					Name:  "runResult",
					Value: "aResultValue",
				}},
			},
		}

		if _, err := c.RunClient.UpdateStatus(ctx, r, metav1.UpdateOptions{}); err != nil {
			t.Fatalf("Failed to update Run to successful: %v", err)
		}

		// Get the Run.
		r, err = c.RunClient.Get(ctx, runName, metav1.GetOptions{})

		if strings.Contains(runName, "custom-task-spec") {
			if d := cmp.Diff(customTaskRawSpec, r.Spec.Spec.Spec.Raw); d != "" {
				t.Fatalf("Unexpected value of Spec.Raw: %s", diff.PrintWantGot(d))
			}
			if d := cmp.Diff(metadataLabel, r.Spec.Spec.Metadata.Labels); d != "" {
				t.Fatalf("Unexpected value of Metadata.Labels: %s", diff.PrintWantGot(d))
			}
		}
		if err != nil {
			t.Fatalf("Failed to get Run %q: %v", runName, err)
		}
		if !r.IsDone() {
			t.Fatalf("Run unexpectedly not done after update (UpdateStatus didn't work): %v", r.Status)
		}
	}
	// Wait for the PipelineRun to become done/successful.
	if err := WaitForPipelineRunState(ctx, c, pipelineRunName, time.Minute, PipelineRunSucceed(pipelineRunName), "PipelineRunCompleted"); err != nil {
		t.Fatalf("Waiting for PipelineRun to complete successfully: %v", err)
	}

	// Get the updated status of the PipelineRun.
	pr, err = c.PipelineRunClient.Get(ctx, pipelineRunName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed to get PipelineRun %q after it completed: %v", pipelineRunName, err)
	}

	// Get the TaskRun name.
	if len(pr.Status.TaskRuns) != 1 {
		t.Fatalf("PipelineRun had unexpected .status.taskRuns; got %d, want 1", len(pr.Status.TaskRuns))
	}
	var taskRunName string
	for k := range pr.Status.TaskRuns {
		taskRunName = k
		break
	}

	// Get the TaskRun.
	taskRun, err := c.TaskRunClient.Get(ctx, taskRunName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed to get TaskRun %q: %v", taskRunName, err)
	}

	// Validate the task's result reference to the custom task's result was resolved.
	expectedTaskRunParams := []v1beta1.Param{{
		Name: "input-result-from-custom-task-ref", Value: *v1beta1.NewArrayOrString("aResultValue"),
	}, {
		Name: "input-result-from-custom-task-spec", Value: *v1beta1.NewArrayOrString("aResultValue"),
	}}

	if d := cmp.Diff(expectedTaskRunParams, taskRun.Spec.Params); d != "" {
		t.Fatalf("Unexpected TaskRun Params: %s", diff.PrintWantGot(d))
	}

	// Validate that the pipeline's result reference to the custom task's result was resolved.

	expectedPipelineResults := []v1beta1.PipelineRunResult{{
		Name:  "prResult-ref",
		Value: "aResultValue",
	}, {
		Name:  "prResult-spec",
		Value: "aResultValue",
	}}

	if len(pr.Status.PipelineResults) != 2 {
		t.Fatalf("Expected 2 PipelineResults but there are %d.", len(pr.Status.PipelineResults))
	}
	if d := cmp.Diff(expectedPipelineResults, pr.Status.PipelineResults); d != "" {
		t.Fatalf("Unexpected PipelineResults: %s", diff.PrintWantGot(d))
	}
}

// WaitForRunSpecCancelled polls the spec.status of the Run until it is
// "RunCancelled", returns an error on timeout. desc will be used to name
// the metric that is emitted to track how long it took.
func WaitForRunSpecCancelled(ctx context.Context, c *clients, name string, desc string) error {
	metricName := fmt.Sprintf("WaitForRunSpecCancelled/%s/%s", name, desc)
	_, span := trace.StartSpan(context.Background(), metricName)
	defer span.End()

	return pollImmediateWithContext(ctx, func() (bool, error) {
		r, err := c.RunClient.Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return true, err
		}
		return r.Spec.Status == v1alpha1.RunSpecStatusCancelled, nil
	})
}

// TestPipelineRunCustomTaskTimeout is an integration test that will
// verify that pipelinerun timeout works and leads to the the correct Run Spec.status
func TestPipelineRunCustomTaskTimeout(t *testing.T) {
	// cancel the context after we have waited a suitable buffer beyond the given deadline.
	ctx, cancel := context.WithTimeout(context.Background(), timeout+2*time.Minute)
	defer cancel()
	c, namespace := setup(ctx, t, requireAnyGate(supportedFeatureGates))

	knativetest.CleanupOnInterrupt(func() { tearDown(context.Background(), t, c, namespace) }, t.Logf)
	defer tearDown(context.Background(), t, c, namespace)
	pipeline := &v1beta1.Pipeline{
		ObjectMeta: metav1.ObjectMeta{Name: helpers.ObjectNameForTest(t), Namespace: namespace},
		Spec: v1beta1.PipelineSpec{
			Tasks: []v1beta1.PipelineTask{{
				Name: "custom-task-ref",
				TaskRef: &v1beta1.TaskRef{
					APIVersion: apiVersion,
					Kind:       kind,
				},
			}},
		},
	}
	pipelineRun := &v1beta1.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{Name: helpers.ObjectNameForTest(t), Namespace: namespace},
		Spec: v1beta1.PipelineRunSpec{
			PipelineRef: &v1beta1.PipelineRef{Name: pipeline.Name},
			Timeout:     &metav1.Duration{Duration: 5 * time.Second},
		},
	}
	if _, err := c.PipelineClient.Create(ctx, pipeline, metav1.CreateOptions{}); err != nil {
		t.Fatalf("Failed to create Pipeline `%s`: %s", pipeline.Name, err)
	}
	if _, err := c.PipelineRunClient.Create(ctx, pipelineRun, metav1.CreateOptions{}); err != nil {
		t.Fatalf("Failed to create PipelineRun `%s`: %s", pipelineRun.Name, err)
	}

	t.Logf("Waiting for Pipelinerun %s in namespace %s to be started", pipelineRun.Name, namespace)
	if err := WaitForPipelineRunState(ctx, c, pipelineRun.Name, timeout, Running(pipelineRun.Name), "PipelineRunRunning"); err != nil {
		t.Fatalf("Error waiting for PipelineRun %s to be running: %s", pipelineRun.Name, err)
	}

	pr, err := c.PipelineRunClient.Get(ctx, pipelineRun.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed to get PipelineRun %q: %v", pipelineRun.Name, err)
	}

	// Get the Run name.
	if len(pr.Status.Runs) != 1 {
		t.Fatalf("PipelineRun had unexpected .status.runs; got %d, want 1", len(pr.Status.Runs))
	}

	for runName := range pr.Status.Runs {
		// Get the Run.
		r, err := c.RunClient.Get(ctx, runName, metav1.GetOptions{})
		if err != nil {
			t.Fatalf("Failed to get Run %q: %v", runName, err)
		}
		if r.IsDone() {
			t.Fatalf("Run unexpectedly done: %v", r.Status.GetCondition(apis.ConditionSucceeded))
		}

		// Simulate a Custom Task controller updating the Run to be started/running,
		// because, a run that has not started cannot timeout.
		r.Status = v1alpha1.RunStatus{
			RunStatusFields: v1alpha1.RunStatusFields{
				StartTime: &metav1.Time{Time: time.Now()},
			},
			Status: duckv1.Status{
				Conditions: []apis.Condition{{
					Type:   apis.ConditionSucceeded,
					Status: corev1.ConditionUnknown,
				}},
			},
		}
		if _, err := c.RunClient.UpdateStatus(ctx, r, metav1.UpdateOptions{}); err != nil {
			t.Fatalf("Failed to update Run to successful: %v", err)
		}
	}

	t.Logf("Waiting for PipelineRun %s in namespace %s to be timed out", pipelineRun.Name, namespace)
	if err := WaitForPipelineRunState(ctx, c, pipelineRun.Name, timeout, FailedWithReason(v1beta1.PipelineRunReasonTimedOut.String(), pipelineRun.Name), "PipelineRunTimedOut"); err != nil {
		t.Errorf("Error waiting for PipelineRun %s to finish: %s", pipelineRun.Name, err)
	}

	runList, err := c.RunClient.List(ctx, metav1.ListOptions{LabelSelector: fmt.Sprintf("tekton.dev/pipelineRun=%s", pipelineRun.Name)})
	if err != nil {
		t.Fatalf("Error listing Runs for PipelineRun %s: %s", pipelineRun.Name, err)
	}

	t.Logf("Runs from PipelineRun %s in namespace %s must be cancelled", pipelineRun.Name, namespace)
	var wg sync.WaitGroup
	for _, runItem := range runList.Items {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			err := WaitForRunSpecCancelled(ctx, c, name, "RunCancelled")
			if err != nil {
				t.Errorf("Error waiting for Run %s to cancel: %s", name, err)
			}
		}(runItem.Name)
	}
	wg.Wait()

	if _, err := c.PipelineRunClient.Get(ctx, pipelineRun.Name, metav1.GetOptions{}); err != nil {
		t.Fatalf("Failed to get PipelineRun `%s`: %s", pipelineRun.Name, err)
	}
}
