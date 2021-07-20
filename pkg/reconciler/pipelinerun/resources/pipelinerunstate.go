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

package resources

import (
	"fmt"

	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"github.com/tektoncd/pipeline/pkg/reconciler/pipeline/dag"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"knative.dev/pkg/apis"
)

const (
	// ReasonUnknown indicates that the execution status of a pipelineTask is unknown
	PipelineTaskStateNone = "None"
	// PipelineTaskStatusPrefix is a prefix of the param representing execution state of pipelineTask
	PipelineTaskStatusPrefix = "tasks."
	// PipelineTaskStatusSuffix is a suffix of the param representing execution state of pipelineTask
	PipelineTaskStatusSuffix = ".status"
)

// PipelineRunState is a slice of ResolvedPipelineRunTasks the represents the current execution
// state of the PipelineRun.
type PipelineRunState []*ResolvedPipelineRunTask

// PipelineRunFacts holds the state of all the components that make up the Pipeline graph that are used to track the
// PipelineRun state without passing all these components separately. It helps simplify our implementation for getting
// and scheduling the next tasks. It is a collection of list of ResolvedPipelineTask, graph of DAG tasks, graph of
// finally tasks, cache of skipped tasks, and the scope of when expressions.
type PipelineRunFacts struct {
	State           PipelineRunState
	SpecStatus      v1beta1.PipelineRunSpecStatus
	TasksGraph      *dag.Graph
	FinalTasksGraph *dag.Graph

	// SkipCache is a hash of PipelineTask names that stores whether a task will be
	// executed or not, because it's either not reachable via the DAG due to the pipeline
	// state, or because it has failed conditions.
	// We cache this data along the state, because it's expensive to compute, it requires
	// traversing potentially the whole graph; this way it can built incrementally, when
	// needed, via the `Skip` method in pipelinerunresolution.go
	// The skip data is sensitive to changes in the state. The ResetSkippedCache method
	// can be used to clean the cache and force re-computation when needed.
	SkipCache                  map[string]TaskSkipStatus
	ScopeWhenExpressionsToTask bool
}

// pipelineRunStatusCount holds the count of successful, failed, cancelled, skipped, and incomplete tasks
type pipelineRunStatusCount struct {
	// skipped tasks count
	Skipped int
	// successful tasks count
	Succeeded int
	// failed tasks count
	Failed int
	// cancelled tasks count
	Cancelled int
	// number of tasks which are still pending, have not executed
	Incomplete int
}

// ResetSkippedCache resets the skipped cache in the facts map
func (facts *PipelineRunFacts) ResetSkippedCache() {
	facts.SkipCache = make(map[string]TaskSkipStatus)
}

// ToMap returns a map that maps pipeline task name to the resolved pipeline run task
func (state PipelineRunState) ToMap() map[string]*ResolvedPipelineRunTask {
	m := make(map[string]*ResolvedPipelineRunTask)
	for _, rprt := range state {
		m[rprt.PipelineTask.Name] = rprt
	}
	return m
}

// IsBeforeFirstTaskRun returns true if the PipelineRun has not yet started its first TaskRun
func (state PipelineRunState) IsBeforeFirstTaskRun() bool {
	for _, t := range state {
		if t.IsCustomTask() && t.Run != nil {
			return false
		} else if t.TaskRun != nil {
			return false
		}
	}
	return true
}

// AdjustStartTime adjusts potential drift in the PipelineRun's start time.
//
// The StartTime will only adjust earlier, so that the PipelineRun's StartTime
// is no later than any of its constituent TaskRuns.
//
// This drift could be due to us either failing to record the Run's start time
// previously, or our own failure to observe a prior update before reconciling
// the resource again.
func (state PipelineRunState) AdjustStartTime(unadjustedStartTime *metav1.Time) *metav1.Time {
	adjustedStartTime := unadjustedStartTime
	for _, rprt := range state {
		if rprt.TaskRun == nil {
			if rprt.Run != nil {
				if rprt.Run.CreationTimestamp.Time.Before(adjustedStartTime.Time) {
					adjustedStartTime = &rprt.Run.CreationTimestamp
				}
			}
		} else {
			if rprt.TaskRun.CreationTimestamp.Time.Before(adjustedStartTime.Time) {
				adjustedStartTime = &rprt.TaskRun.CreationTimestamp
			}
		}
	}
	return adjustedStartTime.DeepCopy()
}

// GetTaskRunsStatus returns a map of taskrun name and the taskrun
// ignore a nil taskrun in pipelineRunState, otherwise, capture taskrun object from PipelineRun Status
// update taskrun status based on the pipelineRunState before returning it in the map
func (state PipelineRunState) GetTaskRunsStatus(pr *v1beta1.PipelineRun) map[string]*v1beta1.PipelineRunTaskRunStatus {
	status := make(map[string]*v1beta1.PipelineRunTaskRunStatus)
	for _, rprt := range state {
		if rprt.IsCustomTask() {
			continue
		}
		if rprt.TaskRun == nil && rprt.ResolvedConditionChecks == nil {
			continue
		}

		var prtrs *v1beta1.PipelineRunTaskRunStatus
		if rprt.TaskRun != nil {
			prtrs = pr.Status.TaskRuns[rprt.TaskRun.Name]
		}
		if prtrs == nil {
			prtrs = &v1beta1.PipelineRunTaskRunStatus{
				PipelineTaskName: rprt.PipelineTask.Name,
				WhenExpressions:  rprt.PipelineTask.WhenExpressions,
			}
		}

		if rprt.TaskRun != nil {
			prtrs.Status = &rprt.TaskRun.Status
		}

		if len(rprt.ResolvedConditionChecks) > 0 {
			cStatus := make(map[string]*v1beta1.PipelineRunConditionCheckStatus)
			for _, c := range rprt.ResolvedConditionChecks {
				cStatus[c.ConditionCheckName] = &v1beta1.PipelineRunConditionCheckStatus{
					ConditionName: c.ConditionRegisterName,
				}
				if c.ConditionCheck != nil {
					cStatus[c.ConditionCheckName].Status = c.NewConditionCheckStatus()
				}
			}
			prtrs.ConditionChecks = cStatus
			if rprt.ResolvedConditionChecks.IsDone() && !rprt.ResolvedConditionChecks.IsSuccess() {
				if prtrs.Status == nil {
					prtrs.Status = &v1beta1.TaskRunStatus{}
				}
				prtrs.Status.SetCondition(&apis.Condition{
					Type:    apis.ConditionSucceeded,
					Status:  corev1.ConditionFalse,
					Reason:  ReasonConditionCheckFailed,
					Message: fmt.Sprintf("ConditionChecks failed for Task %s in PipelineRun %s", rprt.TaskRunName, pr.Name),
				})
			}
		}
		status[rprt.TaskRunName] = prtrs
	}
	return status
}

// GetRunsStatus returns a map of run name and the run.
// Ignore a nil run in pipelineRunState, otherwise, capture run object from PipelineRun Status.
// Update run status based on the pipelineRunState before returning it in the map.
func (state PipelineRunState) GetRunsStatus(pr *v1beta1.PipelineRun) map[string]*v1beta1.PipelineRunRunStatus {
	status := map[string]*v1beta1.PipelineRunRunStatus{}
	for _, rprt := range state {
		if !rprt.IsCustomTask() {
			continue
		}
		if rprt.Run == nil && rprt.ResolvedConditionChecks == nil {
			continue
		}

		var prrs *v1beta1.PipelineRunRunStatus
		if rprt.Run != nil {
			prrs = pr.Status.Runs[rprt.RunName]
		}

		if prrs == nil {
			prrs = &v1beta1.PipelineRunRunStatus{
				PipelineTaskName: rprt.PipelineTask.Name,
				WhenExpressions:  rprt.PipelineTask.WhenExpressions,
			}
		}

		if rprt.Run != nil {
			prrs.Status = &rprt.Run.Status
		}

		// TODO(#3133): Include any condition check statuses here too.
		status[rprt.RunName] = prrs
	}
	return status
}

// getNextTasks returns a list of tasks which should be executed next i.e.
// a list of tasks from candidateTasks which aren't yet indicated in state to be running and
// a list of cancelled/failed tasks from candidateTasks which haven't exhausted their retries
func (state PipelineRunState) getNextTasks(candidateTasks sets.String) []*ResolvedPipelineRunTask {
	tasks := []*ResolvedPipelineRunTask{}
	for _, t := range state {
		if _, ok := candidateTasks[t.PipelineTask.Name]; ok {
			if t.TaskRun == nil && t.Run == nil {
				tasks = append(tasks, t)
			} else if t.TaskRun != nil { // only TaskRun currently supports retry
				status := t.TaskRun.Status.GetCondition(apis.ConditionSucceeded)
				if status != nil && status.IsFalse() {
					if !(t.TaskRun.IsCancelled() || status.Reason == v1beta1.TaskRunReasonCancelled.String() || status.Reason == ReasonConditionCheckFailed) {
						if len(t.TaskRun.Status.RetriesStatus) < t.PipelineTask.Retries {
							tasks = append(tasks, t)
						}
					}
				}
			}
		}
	}
	return tasks
}

// IsStopping returns true if the PipelineRun won't be scheduling any new Task because
// at least one task already failed or was cancelled in the specified dag
func (facts *PipelineRunFacts) IsStopping() bool {
	for _, t := range facts.State {
		if facts.isDAGTask(t.PipelineTask.Name) {
			if t.IsCancelled() {
				return true
			}
			if t.IsFailure() {
				return true
			}
		}
	}
	return false
}

// IsRunning returns true if the PipelineRun is still running tasks in the specified dag
func (facts *PipelineRunFacts) IsRunning() bool {
	for _, t := range facts.State {
		if facts.isDAGTask(t.PipelineTask.Name) {
			if t.IsRunning() {
				return true
			}
		}
	}
	return false
}

// IsGracefullyCancelled returns true if the PipelineRun won't be scheduling any new Task because it was gracefully cancelled
func (facts *PipelineRunFacts) IsGracefullyCancelled() bool {
	return facts.SpecStatus == v1beta1.PipelineRunSpecStatusCancelledRunFinally
}

// IsGracefullyStopped returns true if the PipelineRun won't be scheduling any new Task because it was gracefully stopped
func (facts *PipelineRunFacts) IsGracefullyStopped() bool {
	return facts.SpecStatus == v1beta1.PipelineRunSpecStatusStoppedRunFinally
}

// DAGExecutionQueue returns a list of DAG tasks which needs to be scheduled next
func (facts *PipelineRunFacts) DAGExecutionQueue() (PipelineRunState, error) {
	tasks := PipelineRunState{}
	// when pipeline run is stopping, gracefully cancelled or stopped, do not schedule any new task and only
	// wait for all running tasks to complete and report their status
	if !facts.IsStopping() && !facts.IsGracefullyCancelled() && !facts.IsGracefullyStopped() {
		// candidateTasks is initialized to DAG root nodes to start pipeline execution
		// candidateTasks is derived based on successfully finished tasks and/or skipped tasks
		candidateTasks, err := dag.GetSchedulable(facts.TasksGraph, facts.successfulOrSkippedDAGTasks()...)
		if err != nil {
			return tasks, err
		}
		tasks = facts.State.getNextTasks(candidateTasks)
	}
	return tasks, nil
}

// GetFinalTasks returns a list of final tasks without any taskRun associated with it
// GetFinalTasks returns final tasks only when all DAG tasks have finished executing successfully or skipped or
// any one DAG task resulted in failure
func (facts *PipelineRunFacts) GetFinalTasks() PipelineRunState {
	tasks := PipelineRunState{}
	finalCandidates := sets.NewString()
	// check either pipeline has finished executing all DAG pipelineTasks
	// or any one of the DAG pipelineTask has failed
	if facts.checkDAGTasksDone() {
		// return list of tasks with all final tasks
		for _, t := range facts.State {
			if facts.isFinalTask(t.PipelineTask.Name) && !t.IsSuccessful() {
				finalCandidates.Insert(t.PipelineTask.Name)
			}
		}
		tasks = facts.State.getNextTasks(finalCandidates)
	}
	return tasks
}

// GetPipelineConditionStatus will return the Condition that the PipelineRun prName should be
// updated with, based on the status of the TaskRuns in state.
func (facts *PipelineRunFacts) GetPipelineConditionStatus(pr *v1beta1.PipelineRun, logger *zap.SugaredLogger) *apis.Condition {
	// We have 4 different states here:
	// 1. Timed out -> Failed
	// 2. All tasks are done and at least one has failed or has been cancelled -> Failed
	// 3. All tasks are done or are skipped (i.e. condition check failed).-> Success
	// 4. A Task or Condition is running right now or there are things left to run -> Running
	if pr.IsTimedOut() {
		return &apis.Condition{
			Type:    apis.ConditionSucceeded,
			Status:  corev1.ConditionFalse,
			Reason:  v1beta1.PipelineRunReasonTimedOut.String(),
			Message: fmt.Sprintf("PipelineRun %q failed to finish within %q", pr.Name, pr.Spec.Timeout.Duration.String()),
		}
	}

	// report the count in PipelineRun Status
	// get the count of successful tasks, failed tasks, cancelled tasks, skipped task, and incomplete tasks
	s := facts.getPipelineTasksCount()
	// completed task is a collection of successful, failed, cancelled tasks (skipped tasks are reported separately)
	cmTasks := s.Succeeded + s.Failed + s.Cancelled

	// The completion reason is set from the TaskRun completion reason
	// by default, set it to ReasonRunning
	reason := v1beta1.PipelineRunReasonRunning.String()

	// check if the pipeline is finished executing all tasks i.e. no incomplete tasks
	if s.Incomplete == 0 {
		status := corev1.ConditionTrue
		reason := v1beta1.PipelineRunReasonSuccessful.String()
		message := fmt.Sprintf("Tasks Completed: %d (Failed: %d, Cancelled %d), Skipped: %d",
			cmTasks, s.Failed, s.Cancelled, s.Skipped)
		// Set reason to ReasonCompleted - At least one is skipped
		if s.Skipped > 0 {
			reason = v1beta1.PipelineRunReasonCompleted.String()
		}

		switch {
		case s.Failed > 0:
			// Set reason to ReasonFailed - At least one failed
			reason = v1beta1.PipelineRunReasonFailed.String()
			status = corev1.ConditionFalse
		case pr.IsGracefullyCancelled() || pr.IsGracefullyStopped():
			// Set reason to ReasonCancelled - Cancellation requested
			reason = v1beta1.PipelineRunReasonCancelled.String()
			status = corev1.ConditionFalse
			message = fmt.Sprintf("PipelineRun %q was cancelled", pr.Name)
		case s.Cancelled > 0:
			// Set reason to ReasonCancelled - At least one is cancelled and no failure yet
			reason = v1beta1.PipelineRunReasonCancelled.String()
			status = corev1.ConditionFalse
		}
		logger.Infof("All TaskRuns have finished for PipelineRun %s so it has finished", pr.Name)
		return &apis.Condition{
			Type:    apis.ConditionSucceeded,
			Status:  status,
			Reason:  reason,
			Message: message,
		}
	}

	// Hasn't timed out; not all tasks have finished.... Must keep running then....
	switch {
	case pr.IsGracefullyCancelled():
		// Transition pipeline into running finally state, when graceful cancel is in progress
		reason = v1beta1.PipelineRunReasonCancelledRunningFinally.String()
	case pr.IsGracefullyStopped():
		// Transition pipeline into running finally state, when graceful stop is in progress
		reason = v1beta1.PipelineRunReasonStoppedRunningFinally.String()
	case s.Cancelled > 0 || (s.Failed > 0 && facts.checkFinalTasksDone()):
		// Transition pipeline into stopping state when one of the tasks(dag/final) cancelled or one of the dag tasks failed
		// for a pipeline with final tasks, single dag task failure does not transition to interim stopping state
		// pipeline stays in running state until all final tasks are done before transitioning to failed state
		reason = v1beta1.PipelineRunReasonStopping.String()
	}

	// return the status
	return &apis.Condition{
		Type:   apis.ConditionSucceeded,
		Status: corev1.ConditionUnknown,
		Reason: reason,
		Message: fmt.Sprintf("Tasks Completed: %d (Failed: %d, Cancelled %d), Incomplete: %d, Skipped: %d",
			cmTasks, s.Failed, s.Cancelled, s.Incomplete, s.Skipped),
	}
}

// GetSkippedTasks constructs a list of SkippedTask struct to be included in the PipelineRun Status
func (facts *PipelineRunFacts) GetSkippedTasks() []v1beta1.SkippedTask {
	var skipped []v1beta1.SkippedTask
	for _, rprt := range facts.State {
		if rprt.Skip(facts).IsSkipped {
			skippedTask := v1beta1.SkippedTask{
				Name:            rprt.PipelineTask.Name,
				WhenExpressions: rprt.PipelineTask.WhenExpressions,
			}
			skipped = append(skipped, skippedTask)
		}
		if rprt.IsFinallySkipped(facts).IsSkipped {
			skippedTask := v1beta1.SkippedTask{
				Name: rprt.PipelineTask.Name,
			}
			// include the when expressions only when the finally task was skipped because
			// its when expressions evaluated to false (not because results variables were missing)
			if !rprt.PipelineTask.WhenExpressions.HaveVariables() {
				skippedTask.WhenExpressions = rprt.PipelineTask.WhenExpressions
			}
			skipped = append(skipped, skippedTask)
		}
	}
	return skipped
}

// GetPipelineTaskStatus returns the status of a PipelineTask depending on its taskRun
// the checks are implemented such that the finally tasks are requesting status of the dag tasks
func (facts *PipelineRunFacts) GetPipelineTaskStatus() map[string]string {
	// construct a map of tasks.<pipelineTask>.status and its state
	tStatus := make(map[string]string)
	for _, t := range facts.State {
		if facts.isDAGTask(t.PipelineTask.Name) {
			var s string
			switch {
			// execution status is Succeeded when a task has succeeded condition with status set to true
			case t.IsSuccessful():
				s = v1beta1.TaskRunReasonSuccessful.String()
			// execution status is Failed when a task has succeeded condition with status set to false
			case t.IsConditionStatusFalse():
				s = v1beta1.TaskRunReasonFailed.String()
			default:
				// None includes skipped as well
				s = PipelineTaskStateNone
			}
			tStatus[PipelineTaskStatusPrefix+t.PipelineTask.Name+PipelineTaskStatusSuffix] = s
		}
	}
	// initialize aggregate status of all dag tasks to None
	aggregateStatus := PipelineTaskStateNone
	if facts.checkDAGTasksDone() {
		// all dag tasks are done, change the aggregate status to succeeded
		// will reset it to failed/skipped if needed
		aggregateStatus = v1beta1.PipelineRunReasonSuccessful.String()
		for _, t := range facts.State {
			if facts.isDAGTask(t.PipelineTask.Name) {
				// if any of the dag task failed, change the aggregate status to failed and return
				if t.IsConditionStatusFalse() {
					aggregateStatus = v1beta1.PipelineRunReasonFailed.String()
					break
				}
				// if any of the dag task skipped, change the aggregate status to completed
				// but continue checking for any other failure
				if t.Skip(facts).IsSkipped {
					aggregateStatus = v1beta1.PipelineRunReasonCompleted.String()
				}
			}
		}
	}
	tStatus[v1beta1.PipelineTasksAggregateStatus] = aggregateStatus
	return tStatus
}

// successfulOrSkippedTasks returns a list of the names of all of the PipelineTasks in state
// which have successfully completed or skipped
func (facts *PipelineRunFacts) successfulOrSkippedDAGTasks() []string {
	tasks := []string{}
	for _, t := range facts.State {
		if facts.isDAGTask(t.PipelineTask.Name) {
			if t.IsSuccessful() || t.Skip(facts).IsSkipped {
				tasks = append(tasks, t.PipelineTask.Name)
			}
		}
	}
	return tasks
}

// checkTasksDone returns true if all tasks from the specified graph are finished executing
// a task is considered done if it has failed/succeeded/skipped
func (facts *PipelineRunFacts) checkTasksDone(d *dag.Graph) bool {
	for _, t := range facts.State {
		if isTaskInGraph(t.PipelineTask.Name, d) {
			if !t.IsDone(facts) {
				return false
			}
		}
	}
	return true
}

// check if all DAG tasks done executing (succeeded, failed, or skipped)
func (facts *PipelineRunFacts) checkDAGTasksDone() bool {
	return facts.checkTasksDone(facts.TasksGraph)
}

// check if all finally tasks done executing (succeeded or failed)
func (facts *PipelineRunFacts) checkFinalTasksDone() bool {
	return facts.checkTasksDone(facts.FinalTasksGraph)
}

// getPipelineTasksCount returns the count of successful tasks, failed tasks, cancelled tasks, skipped task, and incomplete tasks
func (facts *PipelineRunFacts) getPipelineTasksCount() pipelineRunStatusCount {
	s := pipelineRunStatusCount{
		Skipped:    0,
		Succeeded:  0,
		Failed:     0,
		Cancelled:  0,
		Incomplete: 0,
	}
	for _, t := range facts.State {
		switch {
		// increment success counter since the task is successful
		case t.IsSuccessful():
			s.Succeeded++
		// increment cancelled counter since the task is cancelled
		case t.IsCancelled():
			s.Cancelled++
		// increment failure counter since the task has failed
		case t.IsFailure():
			s.Failed++
		// increment skip counter since the task is skipped
		case t.Skip(facts).IsSkipped:
			s.Skipped++
		// checking if any finally tasks were referring to invalid/missing task results
		case t.IsFinallySkipped(facts).IsSkipped:
			s.Skipped++
		// increment incomplete counter since the task is pending and not executed yet
		default:
			s.Incomplete++
		}
	}
	return s
}

// check if a specified pipelineTask is defined under tasks(DAG) section
func (facts *PipelineRunFacts) isDAGTask(pipelineTaskName string) bool {
	if _, ok := facts.TasksGraph.Nodes[pipelineTaskName]; ok {
		return true
	}
	return false
}

// check if a specified pipelineTask is defined under finally section
func (facts *PipelineRunFacts) isFinalTask(pipelineTaskName string) bool {
	if _, ok := facts.FinalTasksGraph.Nodes[pipelineTaskName]; ok {
		return true
	}
	return false
}

// Check if a PipelineTask belongs to the specified Graph
func isTaskInGraph(pipelineTaskName string, d *dag.Graph) bool {
	if _, ok := d.Nodes[pipelineTaskName]; ok {
		return true
	}
	return false
}
