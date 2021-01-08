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

package taskrun

import (
	"testing"
	"time"

	tb "github.com/tektoncd/pipeline/internal/builder/v1beta1"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	faketaskruninformer "github.com/tektoncd/pipeline/pkg/client/injection/informers/pipeline/v1beta1/taskrun/fake"
	"github.com/tektoncd/pipeline/pkg/names"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/apis"
	duckv1beta1 "knative.dev/pkg/apis/duck/v1beta1"
	"knative.dev/pkg/metrics/metricstest"
	_ "knative.dev/pkg/metrics/testing"
	rtesting "knative.dev/pkg/reconciler/testing"
)

var (
	startTime      = metav1.Now()
	completionTime = metav1.NewTime(startTime.Time.Add(time.Minute))
)

func TestUninitializedMetrics(t *testing.T) {
	metrics := Recorder{}

	if err := metrics.DurationAndCount(&v1beta1.TaskRun{}); err == nil {
		t.Error("DurationCount recording expected to return error but got nil")
	}
	if err := metrics.RunningTaskRuns(nil); err == nil {
		t.Error("Current TaskRunsCount recording expected to return error but got nil")
	}
	if err := metrics.RecordPodLatency(nil, nil); err == nil {
		t.Error("Pod Latency recording expected to return error but got nil")
	}
	if err := metrics.CloudEvents(&v1beta1.TaskRun{}); err == nil {
		t.Error("Cloud Events recording expected to return error but got nil")
	}
}

func TestRecordTaskRunDurationCount(t *testing.T) {
	for _, c := range []struct {
		name              string
		taskRun           *v1beta1.TaskRun
		metricName        string // "taskrun_duration_seconds" or "pipelinerun_taskrun_duration_seconds"
		expectedTags      map[string]string
		expectedCountTags map[string]string
		expectedDuration  float64
		expectedCount     int64
	}{{
		name: "for succeeded task",
		taskRun: &v1beta1.TaskRun{
			ObjectMeta: metav1.ObjectMeta{Name: "taskrun-1", Namespace: "ns"},
			Spec: v1beta1.TaskRunSpec{
				TaskRef: &v1beta1.TaskRef{Name: "task-1"},
			},
			Status: v1beta1.TaskRunStatus{
				Status: duckv1beta1.Status{
					Conditions: duckv1beta1.Conditions{{
						Type:   apis.ConditionSucceeded,
						Status: corev1.ConditionTrue,
					}},
				},
				TaskRunStatusFields: v1beta1.TaskRunStatusFields{
					StartTime:      &startTime,
					CompletionTime: &completionTime,
				},
			},
		},
		metricName: "taskrun_duration_seconds",
		expectedTags: map[string]string{
			"task":      "task-1",
			"taskrun":   "taskrun-1",
			"namespace": "ns",
			"status":    "success",
		},
		expectedCountTags: map[string]string{
			"status": "success",
		},
		expectedDuration: 60,
		expectedCount:    1,
	}, {
		name: "for failed task",
		taskRun: &v1beta1.TaskRun{
			ObjectMeta: metav1.ObjectMeta{Name: "taskrun-1", Namespace: "ns"},
			Spec: v1beta1.TaskRunSpec{
				TaskRef: &v1beta1.TaskRef{Name: "task-1"},
			},
			Status: v1beta1.TaskRunStatus{
				Status: duckv1beta1.Status{
					Conditions: duckv1beta1.Conditions{{
						Type:   apis.ConditionSucceeded,
						Status: corev1.ConditionFalse,
					}},
				},
				TaskRunStatusFields: v1beta1.TaskRunStatusFields{
					StartTime:      &startTime,
					CompletionTime: &completionTime,
				},
			},
		},
		metricName: "taskrun_duration_seconds",
		expectedTags: map[string]string{
			"task":      "task-1",
			"taskrun":   "taskrun-1",
			"namespace": "ns",
			"status":    "failed",
		},
		expectedCountTags: map[string]string{
			"status": "failed",
		},
		expectedDuration: 60,
		expectedCount:    1,
	}, {
		name: "for succeeded taskrun in pipelinerun",
		taskRun: &v1beta1.TaskRun{
			ObjectMeta: metav1.ObjectMeta{
				Name: "taskrun-1", Namespace: "ns",
				Labels: map[string]string{
					pipeline.GroupName + pipeline.PipelineLabelKey:    "pipeline-1",
					pipeline.GroupName + pipeline.PipelineRunLabelKey: "pipelinerun-1",
				},
			},
			Spec: v1beta1.TaskRunSpec{
				TaskRef: &v1beta1.TaskRef{Name: "task-1"},
			},
			Status: v1beta1.TaskRunStatus{
				Status: duckv1beta1.Status{
					Conditions: duckv1beta1.Conditions{{
						Type:   apis.ConditionSucceeded,
						Status: corev1.ConditionTrue,
					}},
				},
				TaskRunStatusFields: v1beta1.TaskRunStatusFields{
					StartTime:      &startTime,
					CompletionTime: &completionTime,
				},
			},
		},
		metricName: "pipelinerun_taskrun_duration_seconds",
		expectedTags: map[string]string{
			"pipeline":    "pipeline-1",
			"pipelinerun": "pipelinerun-1",
			"task":        "task-1",
			"taskrun":     "taskrun-1",
			"namespace":   "ns",
			"status":      "success",
		},
		expectedCountTags: map[string]string{
			"status": "success",
		},
		expectedDuration: 60,
		expectedCount:    1,
	}, {
		name: "for failed taskrun in pipelinerun",
		taskRun: &v1beta1.TaskRun{
			ObjectMeta: metav1.ObjectMeta{
				Name: "taskrun-1", Namespace: "ns",
				Labels: map[string]string{
					pipeline.GroupName + pipeline.PipelineLabelKey:    "pipeline-1",
					pipeline.GroupName + pipeline.PipelineRunLabelKey: "pipelinerun-1",
				},
			},
			Spec: v1beta1.TaskRunSpec{
				TaskRef: &v1beta1.TaskRef{Name: "task-1"},
			},
			Status: v1beta1.TaskRunStatus{
				Status: duckv1beta1.Status{
					Conditions: duckv1beta1.Conditions{{
						Type:   apis.ConditionSucceeded,
						Status: corev1.ConditionFalse,
					}},
				},
				TaskRunStatusFields: v1beta1.TaskRunStatusFields{
					StartTime:      &startTime,
					CompletionTime: &completionTime,
				},
			},
		},
		metricName: "pipelinerun_taskrun_duration_seconds",
		expectedTags: map[string]string{
			"pipeline":    "pipeline-1",
			"pipelinerun": "pipelinerun-1",
			"task":        "task-1",
			"taskrun":     "taskrun-1",
			"namespace":   "ns",
			"status":      "failed",
		},
		expectedCountTags: map[string]string{
			"status": "failed",
		},
		expectedDuration: 60,
		expectedCount:    1,
	}} {
		t.Run(c.name, func(t *testing.T) {
			unregisterMetrics()

			metrics, err := NewRecorder()
			if err != nil {
				t.Fatalf("NewRecorder: %v", err)
			}

			if err := metrics.DurationAndCount(c.taskRun); err != nil {
				t.Errorf("DurationAndCount: %v", err)
			}
			metricstest.CheckDistributionData(t, c.metricName, c.expectedTags, 1, c.expectedDuration, c.expectedDuration)
			metricstest.CheckCountData(t, "taskrun_count", c.expectedCountTags, c.expectedCount)
		})
	}
}

func TestRecordRunningTaskRunsCount(t *testing.T) {
	unregisterMetrics()
	newTaskRun := func(status corev1.ConditionStatus) *v1beta1.TaskRun {
		return &v1beta1.TaskRun{
			ObjectMeta: metav1.ObjectMeta{Name: names.SimpleNameGenerator.RestrictLengthWithRandomSuffix("taskrun-")},
			Status: v1beta1.TaskRunStatus{
				Status: duckv1beta1.Status{
					Conditions: duckv1beta1.Conditions{{
						Type:   apis.ConditionSucceeded,
						Status: status,
					}},
				},
			},
		}
	}

	ctx, _ := rtesting.SetupFakeContext(t)
	informer := faketaskruninformer.Get(ctx)
	// Add N randomly-named TaskRuns with differently-succeeded statuses.
	for _, tr := range []*v1beta1.TaskRun{
		newTaskRun(corev1.ConditionTrue),
		newTaskRun(corev1.ConditionUnknown),
		newTaskRun(corev1.ConditionFalse),
	} {
		if err := informer.Informer().GetIndexer().Add(tr); err != nil {
			t.Fatalf("Adding TaskRun to informer: %v", err)
		}
	}

	metrics, err := NewRecorder()
	if err != nil {
		t.Fatalf("NewRecorder: %v", err)
	}

	if err := metrics.RunningTaskRuns(informer.Lister()); err != nil {
		t.Errorf("RunningTaskRuns: %v", err)
	}
	metricstest.CheckLastValueData(t, "running_taskruns_count", map[string]string{}, 1)
}

func TestRecordPodLatency(t *testing.T) {
	creationTime := metav1.Now()

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-taskrun-pod-123456",
			Namespace:         "foo",
			CreationTimestamp: creationTime,
		},
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{{
				Type:               corev1.PodScheduled,
				LastTransitionTime: metav1.Time{Time: creationTime.Add(4 * time.Second)},
			}},
		},
	}
	for _, td := range []struct {
		name           string
		taskRun        *v1beta1.TaskRun
		expectedTags   map[string]string
		expectedValue  float64
		expectingError bool
	}{{
		name: "for run pod",
		taskRun: &v1beta1.TaskRun{
			ObjectMeta: metav1.ObjectMeta{Name: "test-taskrun", Namespace: "foo"},
			Spec: v1beta1.TaskRunSpec{
				TaskRef: &v1beta1.TaskRef{Name: "task-1"},
			},
			Status: v1beta1.TaskRunStatus{
				TaskRunStatusFields: v1beta1.TaskRunStatusFields{
					RunAt: &metav1.Time{Time: creationTime.Add(4 * time.Second)},
				},
			},
		},
		expectedTags: map[string]string{
			"pod":       "test-taskrun-pod-123456",
			"task":      "task-1",
			"taskrun":   "test-taskrun",
			"namespace": "foo",
		},
		expectedValue: 4,
	}, {
		name: "for non run pod",
		taskRun: &v1beta1.TaskRun{
			ObjectMeta: metav1.ObjectMeta{Name: "test-taskrun", Namespace: "foo"},
			Spec: v1beta1.TaskRunSpec{
				TaskRef: &v1beta1.TaskRef{Name: "task-1"},
			},
			Status: v1beta1.TaskRunStatus{},
		},
		expectingError: true,
	}} {
		t.Run(td.name, func(t *testing.T) {
			unregisterMetrics()

			metrics, err := NewRecorder()
			if err != nil {
				t.Fatalf("NewRecorder: %v", err)
			}

			if err := metrics.RecordPodLatency(pod, td.taskRun); td.expectingError && err == nil {
				t.Error("RecordPodLatency wanted error, got nil")
			} else if !td.expectingError {
				if err != nil {
					t.Errorf("RecordPodLatency: %v", err)
				}
				metricstest.CheckLastValueData(t, "taskruns_pod_latency", td.expectedTags, td.expectedValue)
			}
		})
	}

}

func TestRecordCloudEvents(t *testing.T) {
	for _, c := range []struct {
		name          string
		taskRun       *v1beta1.TaskRun
		expectedTags  map[string]string
		expectedCount float64
	}{{
		name: "for succeeded task",
		taskRun: tb.TaskRun("taskrun-1",
			tb.TaskRunNamespace("ns"),
			tb.TaskRunLabel(pipeline.GroupName+pipeline.PipelineLabelKey, "pipeline-1"),
			tb.TaskRunLabel(pipeline.GroupName+pipeline.PipelineRunLabelKey, "pipelinerun-1"),
			tb.TaskRunSpec(
				tb.TaskRunTaskRef("task-1"),
			),
			tb.TaskRunStatus(
				tb.TaskRunStartTime(time.Now()),
				tb.TaskRunCompletionTime(time.Now().Add(1*time.Minute)),
				tb.StatusCondition(apis.Condition{
					Type:   apis.ConditionSucceeded,
					Status: corev1.ConditionTrue,
				}),
				tb.TaskRunCloudEvent(
					"http://event_target",
					"",
					1,
					v1beta1.CloudEventConditionSent,
				),
			)),
		expectedTags: map[string]string{
			"pipeline":    "pipeline-1",
			"pipelinerun": "pipelinerun-1",
			"task":        "task-1",
			"taskrun":     "taskrun-1",
			"namespace":   "ns",
			"status":      "success",
		},
		expectedCount: 2,
	}, {
		name: "for failed task",
		taskRun: tb.TaskRun("taskrun-1",
			tb.TaskRunNamespace("ns"),
			tb.TaskRunLabel(pipeline.GroupName+pipeline.PipelineLabelKey, "pipeline-1"),
			tb.TaskRunLabel(pipeline.GroupName+pipeline.PipelineRunLabelKey, "pipelinerun-1"),
			tb.TaskRunSpec(
				tb.TaskRunTaskRef("task-1"),
			),
			tb.TaskRunStatus(
				tb.TaskRunStartTime(time.Now()),
				tb.TaskRunCompletionTime(time.Now().Add(1*time.Minute)),
				tb.StatusCondition(apis.Condition{
					Type:   apis.ConditionSucceeded,
					Status: corev1.ConditionFalse,
				}),
				tb.TaskRunCloudEvent(
					"http://event_target",
					"",
					2,
					v1beta1.CloudEventConditionFailed,
				),
			)),
		expectedTags: map[string]string{
			"pipeline":    "pipeline-1",
			"pipelinerun": "pipelinerun-1",
			"task":        "task-1",
			"taskrun":     "taskrun-1",
			"namespace":   "ns",
			"status":      "failed",
		},
		expectedCount: 3,
	}, {
		name: "for task not part of pipeline",
		taskRun: tb.TaskRun("taskrun-1",
			tb.TaskRunNamespace("ns"),
			tb.TaskRunSpec(
				tb.TaskRunTaskRef("task-1"),
			),
			tb.TaskRunStatus(
				tb.TaskRunStartTime(time.Now()),
				tb.TaskRunCompletionTime(time.Now().Add(1*time.Minute)),
				tb.StatusCondition(apis.Condition{
					Type:   apis.ConditionSucceeded,
					Status: corev1.ConditionTrue,
				}),
				tb.TaskRunCloudEvent(
					"http://event_target",
					"",
					1,
					v1beta1.CloudEventConditionSent,
				),
			)),
		expectedTags: map[string]string{
			"task":      "task-1",
			"taskrun":   "taskrun-1",
			"namespace": "ns",
			"status":    "success",
		},
		expectedCount: 2,
	}} {
		t.Run(c.name, func(t *testing.T) {
			unregisterMetrics()
			metrics, err := NewRecorder()
			if err != nil {
				t.Fatalf("NewRecorder: %v", err)
			}

			if err := metrics.CloudEvents(c.taskRun); err != nil {
				t.Fatalf("CloudEvents: %v", err)
			}
			metricstest.CheckSumData(t, "cloudevent_count", c.expectedTags, c.expectedCount)
		})
	}
}

func unregisterMetrics() {
	metricstest.Unregister("taskrun_duration_seconds", "pipelinerun_taskrun_duration_seconds", "taskrun_count", "running_taskruns_count", "taskruns_pod_latency", "cloudevent_count")
}
