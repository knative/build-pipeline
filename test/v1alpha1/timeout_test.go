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
	"sync"
	"testing"
	"time"

	tb "github.com/tektoncd/pipeline/internal/builder/v1alpha1"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/apis"
	knativetest "knative.dev/pkg/test"
	"knative.dev/pkg/test/helpers"
)

// TestPipelineRunTimeout is an integration test that will
// verify that pipelinerun timeout works and leads to the correct TaskRun statuses
// and pod deletions.
func TestPipelineRunTimeout(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), timeout+2*time.Minute)
	defer cancel()
	c, namespace := setup(ctx, t)

	knativetest.CleanupOnInterrupt(func() { tearDown(context.Background(), t, c, namespace) }, t.Logf)
	defer tearDown(context.Background(), t, c, namespace)

	t.Logf("Creating Task in namespace %s", namespace)
	task := tb.Task(helpers.ObjectNameForTest(t), tb.TaskSpec(
		tb.Step("busybox", tb.StepCommand("/bin/sh"), tb.StepArgs("-c", "sleep 10"))))
	if _, err := c.TaskClient.Create(ctx, task, metav1.CreateOptions{}); err != nil {
		t.Fatalf("Failed to create Task %q: %s", task.Name, err)
	}

	pipeline := tb.Pipeline(helpers.ObjectNameForTest(t),
		tb.PipelineSpec(tb.PipelineTask("foo", task.Name)),
	)
	pipelineRun := tb.PipelineRun(helpers.ObjectNameForTest(t), tb.PipelineRunSpec(pipeline.Name,
		tb.PipelineRunTimeout(5*time.Second),
	))
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

	taskrunList, err := c.TaskRunClient.List(ctx, metav1.ListOptions{LabelSelector: fmt.Sprintf("tekton.dev/pipelineRun=%s", pipelineRun.Name)})
	if err != nil {
		t.Fatalf("Error listing TaskRuns for PipelineRun %s: %s", pipelineRun.Name, err)
	}

	t.Logf("Waiting for TaskRuns from PipelineRun %s in namespace %s to be running", pipelineRun.Name, namespace)
	errChan := make(chan error, len(taskrunList.Items))
	defer close(errChan)

	for _, taskrunItem := range taskrunList.Items {
		go func(name string) {
			err := WaitForTaskRunState(ctx, c, name, Running(name), "TaskRunRunning")
			errChan <- err
		}(taskrunItem.Name)
	}

	for i := 1; i <= len(taskrunList.Items); i++ {
		if <-errChan != nil {
			t.Errorf("Error waiting for TaskRun %s to be running: %s", taskrunList.Items[i-1].Name, err)
		}
	}

	if _, err := c.PipelineRunClient.Get(ctx, pipelineRun.Name, metav1.GetOptions{}); err != nil {
		t.Fatalf("Failed to get PipelineRun `%s`: %s", pipelineRun.Name, err)
	}

	t.Logf("Waiting for PipelineRun %s in namespace %s to be timed out", pipelineRun.Name, namespace)
	if err := WaitForPipelineRunState(ctx, c, pipelineRun.Name, timeout, FailedWithReason("PipelineRunTimeout", pipelineRun.Name), "PipelineRunTimedOut"); err != nil {
		t.Errorf("Error waiting for PipelineRun %s to finish: %s", pipelineRun.Name, err)
	}

	t.Logf("Waiting for TaskRuns from PipelineRun %s in namespace %s to be cancelled", pipelineRun.Name, namespace)
	var wg sync.WaitGroup
	for _, taskrunItem := range taskrunList.Items {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			err := WaitForTaskRunState(ctx, c, name, FailedWithReason("TaskRunTimeout", name), "TaskRunTimeout")
			if err != nil {
				t.Errorf("Error waiting for TaskRun %s to timeout: %s", name, err)
			}
		}(taskrunItem.Name)
	}
	wg.Wait()

	if _, err := c.PipelineRunClient.Get(ctx, pipelineRun.Name, metav1.GetOptions{}); err != nil {
		t.Fatalf("Failed to get PipelineRun `%s`: %s", pipelineRun.Name, err)
	}

	// Verify that we can create a second Pipeline using the same Task without a Pipeline-level timeout that will not
	// time out
	secondPipeline := tb.Pipeline(helpers.ObjectNameForTest(t),
		tb.PipelineSpec(tb.PipelineTask("foo", task.Name)))
	secondPipelineRun := tb.PipelineRun(helpers.ObjectNameForTest(t), tb.PipelineRunSpec(secondPipeline.Name))
	if _, err := c.PipelineClient.Create(ctx, secondPipeline, metav1.CreateOptions{}); err != nil {
		t.Fatalf("Failed to create Pipeline `%s`: %s", secondPipeline.Name, err)
	}
	if _, err := c.PipelineRunClient.Create(ctx, secondPipelineRun, metav1.CreateOptions{}); err != nil {
		t.Fatalf("Failed to create PipelineRun `%s`: %s", secondPipelineRun.Name, err)
	}

	t.Logf("Waiting for PipelineRun %s in namespace %s to complete", secondPipelineRun.Name, namespace)
	if err := WaitForPipelineRunState(ctx, c, secondPipelineRun.Name, timeout, PipelineRunSucceed(secondPipelineRun.Name), "PipelineRunSuccess"); err != nil {
		t.Fatalf("Error waiting for PipelineRun %s to finish: %s", secondPipelineRun.Name, err)
	}
}

// TestTaskRunTimeout is an integration test that will verify a TaskRun can be timed out.
func TestTaskRunTimeout(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), timeout+2*time.Minute)
	defer cancel()
	c, namespace := setup(ctx, t)

	knativetest.CleanupOnInterrupt(func() { tearDown(context.Background(), t, c, namespace) }, t.Logf)
	defer tearDown(context.Background(), t, c, namespace)

	t.Logf("Creating Task and TaskRun in namespace %s", namespace)
	task := tb.Task(helpers.ObjectNameForTest(t),
		tb.TaskSpec(tb.Step("busybox", tb.StepCommand("/bin/sh"), tb.StepArgs("-c", "sleep 3000"))))
	if _, err := c.TaskClient.Create(ctx, task, metav1.CreateOptions{}); err != nil {
		t.Fatalf("Failed to create Task `%s`: %s", task.Name, err)
	}

	taskRun := tb.TaskRun(helpers.ObjectNameForTest(t), tb.TaskRunSpec(tb.TaskRunTaskRef(task.Name),
		// Do not reduce this timeout. Taskrun e2e test is also verifying
		// if reconcile is triggered from timeout handler and not by pod informers
		tb.TaskRunTimeout(30*time.Second)))
	if _, err := c.TaskRunClient.Create(ctx, taskRun, metav1.CreateOptions{}); err != nil {
		t.Fatalf("Failed to create TaskRun `%s`: %s", taskRun.Name, err)
	}

	t.Logf("Waiting for TaskRun %s in namespace %s to complete", taskRun.Name, namespace)
	if err := WaitForTaskRunState(ctx, c, taskRun.Name, FailedWithReason("TaskRunTimeout", taskRun.Name), "TaskRunTimeout"); err != nil {
		t.Errorf("Error waiting for TaskRun %s to finish: %s", taskRun.Name, err)
	}
}

func TestPipelineTaskTimeout(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), timeout+2*time.Minute)
	defer cancel()
	c, namespace := setup(ctx, t)

	knativetest.CleanupOnInterrupt(func() { tearDown(context.Background(), t, c, namespace) }, t.Logf)
	defer tearDown(context.Background(), t, c, namespace)

	t.Logf("Creating Tasks in namespace %s", namespace)
	task1 := tb.Task(helpers.ObjectNameForTest(t), tb.TaskSpec(
		tb.Step("busybox", tb.StepCommand("sleep"), tb.StepArgs("1s"))))

	task2 := tb.Task(helpers.ObjectNameForTest(t), tb.TaskSpec(
		tb.Step("busybox", tb.StepCommand("sleep"), tb.StepArgs("10s"))))

	if _, err := c.TaskClient.Create(ctx, task1, metav1.CreateOptions{}); err != nil {
		t.Fatalf("Failed to create Task `%s`: %s", task1.Name, err)
	}
	if _, err := c.TaskClient.Create(ctx, task2, metav1.CreateOptions{}); err != nil {
		t.Fatalf("Failed to create Task `%s`: %s", task2.Name, err)
	}

	pipeline := tb.Pipeline(helpers.ObjectNameForTest(t),
		tb.PipelineSpec(
			tb.PipelineTask("pipelinetask1", task1.Name, tb.PipelineTaskTimeout(60*time.Second)),
			tb.PipelineTask("pipelinetask2", task2.Name, tb.PipelineTaskTimeout(5*time.Second)),
		),
	)

	pipelineRun := tb.PipelineRun(helpers.ObjectNameForTest(t), tb.PipelineRunSpec(pipeline.Name))

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

	taskrunList, err := c.TaskRunClient.List(ctx, metav1.ListOptions{LabelSelector: fmt.Sprintf("tekton.dev/pipelineRun=%s", pipelineRun.Name)})
	if err != nil {
		t.Fatalf("Error listing TaskRuns for PipelineRun %s: %s", pipelineRun.Name, err)
	}

	t.Logf("Waiting for TaskRuns from PipelineRun %s in namespace %s to be running", pipelineRun.Name, namespace)
	errChan := make(chan error, len(taskrunList.Items))
	defer close(errChan)

	for _, taskrunItem := range taskrunList.Items {
		go func(name string) {
			err := WaitForTaskRunState(ctx, c, name, Running(name), "TaskRunRunning")
			errChan <- err
		}(taskrunItem.Name)
	}

	for i := 1; i <= len(taskrunList.Items); i++ {
		if err := <-errChan; err != nil {
			t.Errorf("Error waiting for TaskRun %s to be running: %v", taskrunList.Items[i-1].Name, err)
		}
	}

	if _, err := c.PipelineRunClient.Get(ctx, pipelineRun.Name, metav1.GetOptions{}); err != nil {
		t.Fatalf("Failed to get PipelineRun `%s`: %s", pipelineRun.Name, err)
	}

	t.Logf("Waiting for PipelineRun %s with PipelineTask timeout in namespace %s to fail", pipelineRun.Name, namespace)
	if err := WaitForPipelineRunState(ctx, c, pipelineRun.Name, timeout, FailedWithReason("Failed", pipelineRun.Name), "PipelineRunTimedOut"); err != nil {
		t.Errorf("Error waiting for PipelineRun %s to finish: %s", pipelineRun.Name, err)
	}

	t.Logf("Waiting for TaskRun from PipelineRun %s in namespace %s to be timed out", pipelineRun.Name, namespace)
	var wg sync.WaitGroup
	for _, taskrunItem := range taskrunList.Items {
		wg.Add(1)
		go func(tr v1alpha1.TaskRun) {
			defer wg.Done()
			name := tr.Name
			err := WaitForTaskRunState(ctx, c, name, func(ca apis.ConditionAccessor) (bool, error) {
				cond := ca.GetCondition(apis.ConditionSucceeded)
				if cond != nil {
					if tr.Spec.TaskRef.Name == task1.Name && cond.Status == corev1.ConditionTrue {
						if cond.Reason == "Succeeded" {
							return true, nil
						}
						return true, fmt.Errorf("taskRun %q completed with the wrong reason: %s", task1.Name, cond.Reason)
					} else if tr.Spec.TaskRef.Name == task1.Name && cond.Status == corev1.ConditionFalse {
						return true, fmt.Errorf("taskRun %q failed, but should have been Succeeded", name)
					}

					if tr.Spec.TaskRef.Name == task2.Name && cond.Status == corev1.ConditionFalse {
						if cond.Reason == "TaskRunTimeout" {
							return true, nil
						}
						return true, fmt.Errorf("taskRun %q completed with the wrong reason: %s", task2.Name, cond.Reason)
					} else if tr.Spec.TaskRef.Name == task2.Name && cond.Status == corev1.ConditionTrue {
						return true, fmt.Errorf("taskRun %q should have timed out", name)
					}
				}
				return false, nil
			}, "TaskRunTimeout")
			if err != nil {
				t.Errorf("Error waiting for TaskRun %s to timeout: %s", name, err)
			}
		}(taskrunItem)
	}
	wg.Wait()
}
