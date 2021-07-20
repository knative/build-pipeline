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

package resources

import (
	"context"
	"fmt"
	"strconv"

	"github.com/tektoncd/pipeline/pkg/apis/config"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1alpha1"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	resourcev1alpha1 "github.com/tektoncd/pipeline/pkg/apis/resource/v1alpha1"
	"github.com/tektoncd/pipeline/pkg/contexts"
	"github.com/tektoncd/pipeline/pkg/list"
	"github.com/tektoncd/pipeline/pkg/names"
	"github.com/tektoncd/pipeline/pkg/reconciler/taskrun/resources"
	"k8s.io/apimachinery/pkg/api/errors"
	"knative.dev/pkg/apis"
)

const (
	// ReasonConditionCheckFailed indicates that the reason for the failure status is that the
	// condition check associated to the pipeline task evaluated to false
	ReasonConditionCheckFailed = "ConditionCheckFailed"
)

// TaskNotFoundError indicates that the resolution failed because a referenced Task couldn't be retrieved
type TaskNotFoundError struct {
	Name string
	Msg  string
}

func (e *TaskNotFoundError) Error() string {
	return fmt.Sprintf("Couldn't retrieve Task %q: %s", e.Name, e.Msg)
}

// ConditionNotFoundError is used to track failures to the
type ConditionNotFoundError struct {
	Name string
	Msg  string
}

func (e *ConditionNotFoundError) Error() string {
	return fmt.Sprintf("Couldn't retrieve Condition %q: %s", e.Name, e.Msg)
}

// ResolvedPipelineRunTask contains a Task and its associated TaskRun, if it
// exists. TaskRun can be nil to represent there being no TaskRun.
type ResolvedPipelineRunTask struct {
	TaskRunName string
	TaskRun     *v1beta1.TaskRun
	// If the PipelineTask is a Custom Task, RunName and Run will be set.
	CustomTask            bool
	RunName               string
	Run                   *v1alpha1.Run
	PipelineTask          *v1beta1.PipelineTask
	ResolvedTaskResources *resources.ResolvedTaskResources
	// ConditionChecks ~~TaskRuns but for evaling conditions
	ResolvedConditionChecks TaskConditionCheckState // Could also be a TaskRun or maybe just a Pod?
}

// IsDone returns true only if the task is skipped, succeeded or failed
func (t ResolvedPipelineRunTask) IsDone(facts *PipelineRunFacts) bool {
	return t.Skip(facts) || t.IsSuccessful() || t.IsFailure()
}

// IsRunning returns true only if the task is neither succeeded, cancelled nor failed
func (t ResolvedPipelineRunTask) IsRunning() bool {
	if t.IsCustomTask() {
		if t.Run == nil {
			return false
		}
	} else {
		if t.TaskRun == nil {
			return false
		}
	}
	return !t.IsSuccessful() && !t.IsFailure() && !t.IsCancelled()
}

// IsCustomTask returns true if the PipelineTask references a Custom Task.
func (t ResolvedPipelineRunTask) IsCustomTask() bool {
	return t.CustomTask
}

// IsSuccessful returns true only if the run has completed successfully
func (t ResolvedPipelineRunTask) IsSuccessful() bool {
	if t.IsCustomTask() {
		return t.Run != nil && t.Run.IsSuccessful()
	}
	return t.TaskRun != nil && t.TaskRun.IsSuccessful()
}

// IsFailure returns true only if the run has failed and will not be retried.
func (t ResolvedPipelineRunTask) IsFailure() bool {
	if t.IsCustomTask() {
		if t.Run == nil {
			return false
		}
		retriesDone := len(t.Run.Status.RetriesStatus)
		retries := t.PipelineTask.Retries
		return t.Run.IsDone() && !t.Run.IsSuccessful() && retriesDone >= retries
	}
	if t.TaskRun == nil {
		return false
	}
	c := t.TaskRun.Status.GetCondition(apis.ConditionSucceeded)
	retriesDone := len(t.TaskRun.Status.RetriesStatus)
	retries := t.PipelineTask.Retries
	return c.IsFalse() && retriesDone >= retries
}

// IsCancelled returns true only if the run is cancelled
func (t ResolvedPipelineRunTask) IsCancelled() bool {
	if t.IsCustomTask() {
		if t.Run == nil {
			return false
		}
		c := t.Run.Status.GetCondition(apis.ConditionSucceeded)
		return c != nil && c.IsFalse() && c.Reason == v1alpha1.RunReasonCancelled
	}
	if t.TaskRun == nil {
		return false
	}
	c := t.TaskRun.Status.GetCondition(apis.ConditionSucceeded)
	return c != nil && c.IsFalse() && c.Reason == v1beta1.TaskRunReasonCancelled.String()
}

// IsStarted returns true only if the PipelineRunTask itself has a TaskRun or
// Run associated that has a Succeeded-type condition.
func (t ResolvedPipelineRunTask) IsStarted() bool {
	if t.IsCustomTask() {
		return t.Run != nil && t.Run.Status.GetCondition(apis.ConditionSucceeded) != nil
	}
	return t.TaskRun != nil && t.TaskRun.Status.GetCondition(apis.ConditionSucceeded) != nil
}

// IsConditionStatusFalse returns true when a task has succeeded condition with status set to false
// it includes task failed after retries are exhausted, cancelled tasks, and time outs
func (t ResolvedPipelineRunTask) IsConditionStatusFalse() bool {
	if t.IsStarted() {
		if t.IsCustomTask() {
			return t.Run.Status.GetCondition(apis.ConditionSucceeded).IsFalse()
		}
		return t.TaskRun.Status.GetCondition(apis.ConditionSucceeded).IsFalse()
	}
	return false
}

func (t *ResolvedPipelineRunTask) checkParentsDone(facts *PipelineRunFacts) bool {
	if facts.isFinalTask(t.PipelineTask.Name) {
		return true
	}
	stateMap := facts.State.ToMap()
	node := facts.TasksGraph.Nodes[t.PipelineTask.Name]
	for _, p := range node.Prev {
		if !stateMap[p.Task.HashKey()].IsDone(facts) {
			return false
		}
	}
	return true
}

func (t *ResolvedPipelineRunTask) skip(facts *PipelineRunFacts) bool {
	if facts.isFinalTask(t.PipelineTask.Name) || t.IsStarted() {
		return false
	}

	if t.conditionsSkip() || t.whenExpressionsSkip(facts) || t.parentTasksSkip(facts) ||
		facts.IsStopping() || facts.IsGracefullyCancelled() || facts.IsGracefullyStopped() {
		return true
	}

	return false
}

// Skip returns true if a PipelineTask will not be run because
// (1) its When Expressions evaluated to false
// (2) its Condition Checks failed
// (3) its parent task was skipped
// (4) Pipeline is in stopping state (one of the PipelineTasks failed)
// Note that this means Skip returns false if a conditionCheck is in progress
func (t *ResolvedPipelineRunTask) Skip(facts *PipelineRunFacts) bool {
	if facts.SkipCache == nil {
		facts.SkipCache = make(map[string]bool)
	}
	if _, cached := facts.SkipCache[t.PipelineTask.Name]; !cached {
		facts.SkipCache[t.PipelineTask.Name] = t.skip(facts) // t.skip() is same as our existing t.Skip()
	}
	return facts.SkipCache[t.PipelineTask.Name]
}

func (t *ResolvedPipelineRunTask) conditionsSkip() bool {
	if len(t.ResolvedConditionChecks) > 0 {
		if t.ResolvedConditionChecks.IsDone() && !t.ResolvedConditionChecks.IsSuccess() {
			return true
		}
	}
	return false
}

func (t *ResolvedPipelineRunTask) whenExpressionsSkip(facts *PipelineRunFacts) bool {
	if t.checkParentsDone(facts) {
		if len(t.PipelineTask.WhenExpressions) > 0 {
			if !t.PipelineTask.WhenExpressions.HaveVariables() {
				if !t.PipelineTask.WhenExpressions.AllowsExecution() {
					return true
				}
			}
		}
	}
	return false
}

func (t *ResolvedPipelineRunTask) parentTasksSkip(facts *PipelineRunFacts) bool {
	stateMap := facts.State.ToMap()
	node := facts.TasksGraph.Nodes[t.PipelineTask.Name]
	for _, p := range node.Prev {
		if stateMap[p.Task.HashKey()].Skip(facts) {
			return true
		}
	}
	return false
}

// IsFinalTask returns true if a task is a finally task
func (t *ResolvedPipelineRunTask) IsFinalTask(facts *PipelineRunFacts) bool {
	return facts.isFinalTask(t.PipelineTask.Name)
}

// IsFinallySkipped returns true if a finally task is not executed and skipped due to task result validation failure
func (t *ResolvedPipelineRunTask) IsFinallySkipped(facts *PipelineRunFacts) bool {
	if t.IsStarted() {
		return false
	}
	if facts.checkDAGTasksDone() && facts.isFinalTask(t.PipelineTask.Name) {
		if _, err := ResolveResultRef(facts.State, t); err != nil {
			return true
		}
		if t.whenExpressionsSkip(facts) {
			return true
		}
	}
	return false
}

// GetRun is a function that will retrieve a Run by name.
type GetRun func(name string) (*v1alpha1.Run, error)

// GetResourcesFromBindings will retrieve all Resources bound in PipelineRun pr and return a map
// from the declared name of the PipelineResource (which is how the PipelineResource will
// be referred to in the PipelineRun) to the PipelineResource, obtained via getResource.
func GetResourcesFromBindings(pr *v1beta1.PipelineRun, getResource resources.GetResource) (map[string]*resourcev1alpha1.PipelineResource, error) {
	rs := map[string]*resourcev1alpha1.PipelineResource{}
	for _, resource := range pr.Spec.Resources {
		r, err := resources.GetResourceFromBinding(resource, getResource)
		if err != nil {
			return rs, err
		}
		rs[resource.Name] = r
	}
	return rs, nil
}

// ValidateResourceBindings validate that the PipelineResources declared in Pipeline p are bound in PipelineRun.
func ValidateResourceBindings(p *v1beta1.PipelineSpec, pr *v1beta1.PipelineRun) error {
	required := make([]string, 0, len(p.Resources))
	optional := make([]string, 0, len(p.Resources))
	for _, resource := range p.Resources {
		if resource.Optional {
			// create a list of optional resources
			optional = append(optional, resource.Name)
		} else {
			// create a list of required resources
			required = append(required, resource.Name)
		}
	}
	provided := make([]string, 0, len(pr.Spec.Resources))
	for _, resource := range pr.Spec.Resources {
		provided = append(provided, resource.Name)
	}
	// verify that the list of required resources exists in the provided resources
	missing := list.DiffLeft(required, provided)
	if len(missing) > 0 {
		return fmt.Errorf("Pipeline's declared required resources are missing from the PipelineRun: %s", missing)
	}
	// verify that the list of provided resources does not have any extra resources (outside of required and optional resources combined)
	extra := list.DiffLeft(provided, append(required, optional...))
	if len(extra) > 0 {
		return fmt.Errorf("PipelineRun's declared resources didn't match usage in Pipeline: %s", extra)
	}
	return nil
}

// ValidateWorkspaceBindings validates that the Workspaces expected by a Pipeline are provided by a PipelineRun.
func ValidateWorkspaceBindings(p *v1beta1.PipelineSpec, pr *v1beta1.PipelineRun) error {
	pipelineRunWorkspaces := make(map[string]v1beta1.WorkspaceBinding)
	for _, binding := range pr.Spec.Workspaces {
		pipelineRunWorkspaces[binding.Name] = binding
	}

	for _, ws := range p.Workspaces {
		if ws.Optional {
			continue
		}
		if _, ok := pipelineRunWorkspaces[ws.Name]; !ok {
			return fmt.Errorf("pipeline requires workspace with name %q be provided by pipelinerun", ws.Name)
		}
	}
	return nil
}

// ValidateTaskRunSpecs that the TaskRunSpecs defined by a PipelineRun are correct.
func ValidateTaskRunSpecs(p *v1beta1.PipelineSpec, pr *v1beta1.PipelineRun) error {
	pipelineTasks := make(map[string]string)
	for _, task := range p.Tasks {
		pipelineTasks[task.Name] = task.Name
	}

	for _, task := range p.Finally {
		pipelineTasks[task.Name] = task.Name
	}

	for _, taskrunSpec := range pr.Spec.TaskRunSpecs {
		if _, ok := pipelineTasks[taskrunSpec.PipelineTaskName]; !ok {
			return fmt.Errorf("PipelineRun's taskrunSpecs defined wrong taskName: %q, does not exist in Pipeline", taskrunSpec.PipelineTaskName)
		}
	}
	return nil
}

// ValidateServiceaccountMapping validates that the ServiceAccountNames defined by a PipelineRun are not correct.
func ValidateServiceaccountMapping(p *v1beta1.PipelineSpec, pr *v1beta1.PipelineRun) error {
	pipelineTasks := make(map[string]string)
	for _, task := range p.Tasks {
		pipelineTasks[task.Name] = task.Name
	}

	for _, task := range p.Finally {
		pipelineTasks[task.Name] = task.Name
	}

	for _, name := range pr.Spec.ServiceAccountNames {
		if _, ok := pipelineTasks[name.TaskName]; !ok {
			return fmt.Errorf("PipelineRun's ServiceAccountNames defined wrong taskName: %q, does not exist in Pipeline", name.TaskName)
		}
	}
	return nil
}

func isCustomTask(ctx context.Context, rprt ResolvedPipelineRunTask) bool {
	invalidSpec := rprt.PipelineTask.TaskRef != nil && rprt.PipelineTask.TaskSpec != nil
	isTaskRefCustomTask := rprt.PipelineTask.TaskRef != nil && rprt.PipelineTask.TaskRef.APIVersion != "" &&
		rprt.PipelineTask.TaskRef.Kind != ""
	isTaskSpecCustomTask := rprt.PipelineTask.TaskSpec != nil && rprt.PipelineTask.TaskSpec.APIVersion != "" &&
		rprt.PipelineTask.TaskSpec.Kind != ""
	cfg := config.FromContextOrDefaults(ctx)
	return cfg.FeatureFlags.EnableCustomTasks && !invalidSpec && (isTaskRefCustomTask || isTaskSpecCustomTask)
}

// ResolvePipelineRunTask retrieves a single Task's instance using the getTask to fetch
// the spec. If it is unable to retrieve an instance of a referenced Task, it  will return
// an error, otherwise it returns a list of all of the Tasks retrieved.  It will retrieve
// the Resources needed for the TaskRun using the mapping of providedResources.
func ResolvePipelineRunTask(
	ctx context.Context,
	pipelineRun v1beta1.PipelineRun,
	getTask resources.GetTask,
	getTaskRun resources.GetTaskRun,
	getRun GetRun,
	getCondition GetCondition,
	task v1beta1.PipelineTask,
	providedResources map[string]*resourcev1alpha1.PipelineResource,
) (*ResolvedPipelineRunTask, error) {

	rprt := ResolvedPipelineRunTask{
		PipelineTask: &task,
	}
	rprt.CustomTask = isCustomTask(ctx, rprt)
	if rprt.IsCustomTask() {
		rprt.RunName = getRunName(pipelineRun.Status.Runs, task.Name, pipelineRun.Name)
		run, err := getRun(rprt.RunName)
		if err != nil && !errors.IsNotFound(err) {
			return nil, fmt.Errorf("error retrieving Run %s: %w", rprt.RunName, err)
		}
		rprt.Run = run
	} else {
		rprt.TaskRunName = GetTaskRunName(pipelineRun.Status.TaskRuns, task.Name, pipelineRun.Name)

		// Find the Task that this PipelineTask is using
		var (
			t        v1beta1.TaskObject
			err      error
			spec     v1beta1.TaskSpec
			taskName string
			kind     v1beta1.TaskKind
		)

		taskRun, err := getTaskRun(rprt.TaskRunName)
		if err != nil {
			if !errors.IsNotFound(err) {
				return nil, fmt.Errorf("error retrieving TaskRun %s: %w", rprt.TaskRunName, err)
			}
		}
		if taskRun != nil {
			rprt.TaskRun = taskRun
		}

		if task.TaskRef != nil {
			// If the TaskRun has already a store TaskSpec in its status, use it as source of truth
			if taskRun != nil && taskRun.Status.TaskSpec != nil {
				spec = *taskRun.Status.TaskSpec
				taskName = task.TaskRef.Name
			} else {
				t, err = getTask(ctx, task.TaskRef.Name)
				if err != nil {
					return nil, &TaskNotFoundError{
						Name: task.TaskRef.Name,
						Msg:  err.Error(),
					}
				}
				spec = t.TaskSpec()
				taskName = t.TaskMetadata().Name
			}
			kind = task.TaskRef.Kind
		} else {
			spec = task.TaskSpec.TaskSpec
		}
		spec.SetDefaults(contexts.WithUpgradeViaDefaulting(ctx))
		rtr, err := resolvePipelineTaskResources(task, &spec, taskName, kind, providedResources)
		if err != nil {
			return nil, fmt.Errorf("couldn't match referenced resources with declared resources: %w", err)
		}

		rprt.ResolvedTaskResources = rtr

		// Get all conditions that this pipelineTask will be using, if any
		if len(task.Conditions) > 0 {
			rcc, err := resolveConditionChecks(&task, pipelineRun.Status.TaskRuns, rprt.TaskRunName, getTaskRun, getCondition, providedResources)
			if err != nil {
				return nil, err
			}
			rprt.ResolvedConditionChecks = rcc
		}
	}
	return &rprt, nil
}

// getConditionCheckName should return a unique name for a `ConditionCheck` if one has not already been defined, and the existing one otherwise.
func getConditionCheckName(taskRunStatus map[string]*v1beta1.PipelineRunTaskRunStatus, trName, conditionRegisterName string) string {
	trStatus, ok := taskRunStatus[trName]
	if ok && trStatus.ConditionChecks != nil {
		for k, v := range trStatus.ConditionChecks {
			// TODO(1022): Should  we allow multiple conditions of the same type?
			if conditionRegisterName == v.ConditionName {
				return k
			}
		}
	}
	return names.SimpleNameGenerator.RestrictLengthWithRandomSuffix(fmt.Sprintf("%s-%s", trName, conditionRegisterName))
}

// GetTaskRunName should return a unique name for a `TaskRun` if one has not already been defined, and the existing one otherwise.
func GetTaskRunName(taskRunsStatus map[string]*v1beta1.PipelineRunTaskRunStatus, ptName, prName string) string {
	for k, v := range taskRunsStatus {
		if v.PipelineTaskName == ptName {
			return k
		}
	}

	return names.SimpleNameGenerator.RestrictLengthWithRandomSuffix(fmt.Sprintf("%s-%s", prName, ptName))
}

// getRunName should return a unique name for a `Run` if one has not already
// been defined, and the existing one otherwise.
func getRunName(runsStatus map[string]*v1beta1.PipelineRunRunStatus, ptName, prName string) string {
	for k, v := range runsStatus {
		if v.PipelineTaskName == ptName {
			return k
		}
	}

	return names.SimpleNameGenerator.RestrictLengthWithRandomSuffix(fmt.Sprintf("%s-%s", prName, ptName))
}

func resolveConditionChecks(pt *v1beta1.PipelineTask, taskRunStatus map[string]*v1beta1.PipelineRunTaskRunStatus, taskRunName string, getTaskRun resources.GetTaskRun, getCondition GetCondition, providedResources map[string]*resourcev1alpha1.PipelineResource) ([]*ResolvedConditionCheck, error) {
	rccs := []*ResolvedConditionCheck{}
	for i := range pt.Conditions {
		ptc := pt.Conditions[i]
		cName := ptc.ConditionRef
		crName := fmt.Sprintf("%s-%s", cName, strconv.Itoa(i))
		c, err := getCondition(cName)
		if err != nil {
			return nil, &ConditionNotFoundError{
				Name: cName,
				Msg:  err.Error(),
			}
		}
		conditionCheckName := getConditionCheckName(taskRunStatus, taskRunName, crName)
		// TODO(#3133): Also handle Custom Task Runs (getRun here)
		cctr, err := getTaskRun(conditionCheckName)
		if err != nil {
			if !errors.IsNotFound(err) {
				return nil, fmt.Errorf("error retrieving ConditionCheck %s for taskRun name %s : %w", conditionCheckName, taskRunName, err)
			}
		}
		conditionResources := map[string]*resourcev1alpha1.PipelineResource{}
		for _, declared := range ptc.Resources {
			if r, ok := providedResources[declared.Resource]; ok {
				conditionResources[declared.Name] = r
			} else {
				for _, resource := range c.Spec.Resources {
					if declared.Name == resource.Name && !resource.Optional {
						return nil, fmt.Errorf("resources %s missing for condition %s in pipeline task %s", declared.Resource, cName, pt.Name)
					}
				}
			}
		}

		rcc := ResolvedConditionCheck{
			ConditionRegisterName: crName,
			Condition:             c,
			ConditionCheckName:    conditionCheckName,
			ConditionCheck:        v1beta1.NewConditionCheck(cctr),
			PipelineTaskCondition: &ptc,
			ResolvedResources:     conditionResources,
		}

		rccs = append(rccs, &rcc)
	}
	return rccs, nil
}

// resolvePipelineTaskResources matches PipelineResources referenced by pt inputs and outputs with the
// providedResources and returns an instance of ResolvedTaskResources.
func resolvePipelineTaskResources(pt v1beta1.PipelineTask, ts *v1beta1.TaskSpec, taskName string, kind v1beta1.TaskKind, providedResources map[string]*resourcev1alpha1.PipelineResource) (*resources.ResolvedTaskResources, error) {
	rtr := resources.ResolvedTaskResources{
		TaskName: taskName,
		TaskSpec: ts,
		Kind:     kind,
		Inputs:   map[string]*resourcev1alpha1.PipelineResource{},
		Outputs:  map[string]*resourcev1alpha1.PipelineResource{},
	}
	if pt.Resources != nil {
		for _, taskInput := range pt.Resources.Inputs {
			if resource, ok := providedResources[taskInput.Resource]; ok {
				rtr.Inputs[taskInput.Name] = resource
			} else {
				if ts.Resources == nil || ts.Resources.Inputs == nil {
					return nil, fmt.Errorf("pipelineTask tried to use input resource %s not present in declared resources", taskInput.Resource)
				}
				for _, r := range ts.Resources.Inputs {
					if r.Name == taskInput.Name && !r.Optional {
						return nil, fmt.Errorf("pipelineTask tried to use input resource %s not present in declared resources", taskInput.Resource)
					}
				}
			}
		}
		for _, taskOutput := range pt.Resources.Outputs {
			if resource, ok := providedResources[taskOutput.Resource]; ok {
				rtr.Outputs[taskOutput.Name] = resource
			} else {
				if ts.Resources == nil || ts.Resources.Outputs == nil {
					return nil, fmt.Errorf("pipelineTask tried to use output resource %s not present in declared resources", taskOutput.Resource)
				}
				for _, r := range ts.Resources.Outputs {
					if r.Name == taskOutput.Name && !r.Optional {
						return nil, fmt.Errorf("pipelineTask tried to use output resource %s not present in declared resources", taskOutput.Resource)
					}
				}
			}
		}
	}
	return &rtr, nil
}
