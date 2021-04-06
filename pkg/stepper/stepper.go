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

package stepper

import (
	"context"
	"github.com/pkg/errors"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1alpha1"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"strconv"
)

type Resolver struct {
	ResolveRemote func(ctx context.Context, uses *v1beta1.Uses) (runtime.Object, error)
}

// NewResolver creates a new stepper resolver
func NewResolver(opts *Options) *Resolver {
	usesResolver := NewResourceLoader(opts)

	return &Resolver{
		ResolveRemote: usesResolver,
	}
}

// NewResourceLoader the default resolver of uses resources
func NewResourceLoader(opts *Options) func(ctx context.Context, uses *v1beta1.Uses) (runtime.Object, error) {
	// lets lazy create the logger
	opts.GitFactory.GetLogger()

	cachingResolver := NewCachingRemoter(opts.CreateRemote)
	usesResolver := cachingResolver.CreateRemote
	return usesResolver
}

// Resolve will resolve any known kind
func (r *Resolver) Resolve(ctx context.Context, obj runtime.Object) (runtime.Object, error) {
	if pr, ok := obj.(*v1beta1.PipelineRun); ok {
		return pr, r.ResolvePipelineRun(ctx, pr)
	}
	if pr, ok := obj.(*v1alpha1.PipelineRun); ok {
		betaPR := &v1beta1.PipelineRun{}
		err := pr.ConvertTo(ctx, betaPR)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to convert v1alpha1.PipelineRun")
		}
		err = r.ResolvePipelineRun(ctx, betaPR)
		return betaPR, err
	}

	// Pipelines
	if pipeline, ok := obj.(*v1beta1.Pipeline); ok {
		return pipeline, r.ResolvePipelineSpec(ctx, nil, &pipeline.Spec)
	}
	if pipeline, ok := obj.(*v1alpha1.Pipeline); ok {
		betaPipeline := &v1beta1.Pipeline{}
		err := pipeline.ConvertTo(ctx, betaPipeline)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to convert v1alpha1.Pipeline")
		}
		return betaPipeline, r.ResolvePipelineSpec(ctx, nil, &betaPipeline.Spec)
	}

	// TaskRun
	if tr, ok := obj.(*v1beta1.TaskRun); ok {
		return tr, r.ResolveTaskSpec(ctx, tr.Name, &tr.Spec, tr.Spec.TaskSpec)
	}
	if tr, ok := obj.(*v1alpha1.TaskRun); ok {
		betaTaskRun := &v1beta1.TaskRun{}
		err := tr.ConvertTo(ctx, betaTaskRun)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to convert v1alpha1.TaskRun")
		}
		return betaTaskRun, r.ResolveTaskSpec(ctx, betaTaskRun.Name, &betaTaskRun.Spec, betaTaskRun.Spec.TaskSpec)
	}

	// Tasks
	if task, ok := obj.(*v1beta1.Task); ok {
		return task, r.ResolveTaskSpec(ctx, task.Name, nil, &task.Spec)
	}
	if task, ok := obj.(*v1alpha1.Task); ok {
		betaTask := &v1beta1.Task{}
		err := task.ConvertTo(ctx, betaTask)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to convert v1alpha1.Task")
		}
		return betaTask, r.ResolveTaskSpec(ctx, betaTask.TaskMetadata().Name, nil, &betaTask.Spec)
	}

	return obj, errors.Errorf("unsupported kind %v", obj)
}

// ResolvePipelineRun will resolve any `uses` structs on any steps in any tasks in the PipelineRun
func (r *Resolver) ResolvePipelineRun(ctx context.Context, prs *v1beta1.PipelineRun) error {
	return r.ResolvePipelineSpec(ctx, &prs.Spec, prs.Spec.PipelineSpec)
}

// ResolvePipeline will resolve any `uses` structs on any steps in any tasks in the Pipeline
func (r *Resolver) ResolvePipeline(ctx context.Context, p *v1beta1.Pipeline) error {
	return r.ResolvePipelineSpec(ctx, nil, &p.Spec)
}

// ResolvePipelineSpec will resolve any `uses` structs on any steps in any tasks in the PipelineSpec
func (r *Resolver) ResolvePipelineSpec(ctx context.Context, prs *v1beta1.PipelineRunSpec, ps *v1beta1.PipelineSpec) error {
	if ps == nil {
		return nil
	}
	for i := range ps.Tasks {
		pt := &ps.Tasks[i]
		if pt.TaskSpec != nil {
			ts := &pt.TaskSpec.TaskSpec
			taskName := pt.Name
			if taskName == "" {
				taskName = strconv.Itoa(i)
			}
			err := r.resolveTaskSpec(ctx, &UseLocation{
				PipelineRunSpec: prs,
				PipelineSpec:    ps,
				TaskName:        taskName,
				TaskSpec:        ts,
			})
			if err != nil {
				return err
			}
		}
	}
	err := ps.Validate(ctx)
	if err != nil {
		return errors.Wrapf(err, "failed to validate Pipeline")
	}
	return nil
}

// ResolveTaskSpec will resolve any `uses` structs on any steps in any tasks in the TaskSpec
func (r *Resolver) ResolveTaskSpec(ctx context.Context, taskName string, trs *v1beta1.TaskRunSpec, ts *v1beta1.TaskSpec) error {
	return r.resolveTaskSpec(ctx, &UseLocation{
		TaskName:    taskName,
		TaskRunSpec: trs,
		TaskSpec:    ts,
	})
}

// resolveTaskSpec will resolve any `uses` structs on any steps in any tasks in the TaskSpec
func (r *Resolver) resolveTaskSpec(ctx context.Context, loc *UseLocation) error {
	ts := loc.TaskSpec
	if ts == nil {
		return nil
	}
	var steps []v1beta1.Step
	for j := range ts.Steps {
		step := ts.Steps[j]
		if step.Uses == nil {
			steps = append(steps, step)
			continue
		}
		stepName := step.Name
		if stepName == "" {
			stepName = strconv.Itoa(j)
		}
		replaceSteps, err := r.UsesSteps(ctx, step.Uses, loc, step)
		if err != nil {
			return errors.Wrapf(err, "failed to use %s for step %s/%s", step.Uses.String(), loc.TaskName, stepName)
		}
		if len(replaceSteps) == 0 {
			return errors.Errorf("no steps found for use %s on step %s/%s", step.Uses.String(), loc.TaskName, stepName)
		}
		steps = append(steps, replaceSteps...)
	}
	ts.Steps = steps
	err := ts.Validate(ctx)
	if err != nil {
		return errors.Wrapf(err, "failed to validate TaskSpec")
	}
	return nil
}

// UsesSteps lets resolve the uses.String() to a PipelineRun and find the step or steps
// for the given task name and/or step name then lets apply any overrides from the step
func (r *Resolver) UsesSteps(ctx context.Context, uses *v1beta1.Uses, loc *UseLocation, step v1beta1.Step) ([]v1beta1.Step, error) {
	obj, err := r.ResolveRemote(ctx, uses)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to resolve remote for uses %s", uses.String())
	}

	if obj == nil {
		return nil, errors.Errorf("no task found for uses %s", uses.String())
	}

	// PipelineRuns
	if pr, ok := obj.(*v1beta1.PipelineRun); ok {
		ps := pr.Spec.PipelineSpec
		if ps == nil {
			return nil, errors.Errorf("the PipelineRun %s for uses %s has no PipelineSpec", pr.Name, uses.String())
		}
		return r.findPipelineSteps(ctx, uses, ps, loc, step)
	}
	if pr, ok := obj.(*v1alpha1.PipelineRun); ok {
		betaPR := &v1beta1.PipelineRun{}
		err := pr.ConvertTo(ctx, betaPR)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to convert v1alpha1.PipelineRun")
		}
		ps := betaPR.Spec.PipelineSpec
		if ps == nil {
			return nil, errors.Errorf("the PipelineRun %s for uses %s has no PipelineSpec", pr.Name, uses.String())
		}
		return r.findPipelineSteps(ctx, uses, ps, loc, step)
	}

	// Pipelines
	if pipeline, ok := obj.(v1beta1.PipelineObject); ok {
		ps := pipeline.PipelineSpec()
		return r.findPipelineSteps(ctx, uses, &ps, loc, step)
	}
	if pipeline, ok := obj.(*v1alpha1.Pipeline); ok {
		betaPipeline := &v1beta1.Pipeline{}
		err := pipeline.ConvertTo(ctx, betaPipeline)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to convert v1alpha1.Pipeline")
		}
		return r.findPipelineSteps(ctx, uses, &betaPipeline.Spec, loc, step)
	}

	// Tasks
	if task, ok := obj.(v1beta1.TaskObject); ok {
		ts := task.TaskSpec()
		return r.findTaskSteps(ctx, uses, task.TaskMetadata().Name, &ts, loc, step)
	}
	if task, ok := obj.(*v1alpha1.Task); ok {
		betaTask := &v1beta1.Task{}
		err := task.ConvertTo(ctx, betaTask)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to convert v1alpha1.Task")
		}
		return r.findTaskSteps(ctx, uses, betaTask.Name, &betaTask.Spec, loc, step)
	}

	return nil, errors.Errorf("could not convert object %#v to task for uses %s", obj, uses.String())
}

func (r *Resolver) findPipelineSteps(ctx context.Context, uses *v1beta1.Uses, ps *v1beta1.PipelineSpec, loc *UseLocation, step v1beta1.Step) ([]v1beta1.Step, error) {
	taskName := uses.Task
	pipelineTasks := ps.Tasks
	for _, task := range pipelineTasks {
		if task.Name == taskName || taskName == "" {
			ts := task.TaskSpec
			if ts != nil {
				return r.findTaskSteps(ctx, uses, task.Name, &ts.TaskSpec, loc, step)
			}
		}
	}
	return nil, errors.Errorf("uses %s has no spec.pipelineSpec.tasks that match task name %s", uses.String(), taskName)
}

func (r *Resolver) findTaskSteps(ctx context.Context, uses *v1beta1.Uses, taskName string, ts *v1beta1.TaskSpec, loc *UseLocation, step v1beta1.Step) ([]v1beta1.Step, error) {
	err := UseParametersAndResults(ctx, loc, ts)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to ")
	}
	if ts == nil {
		return nil, errors.Errorf("uses %s has no task spec for task %s", uses.String(), taskName)
	}
	overrideAllSteps := false
	if uses.Step != "" {
		for i := range ts.Steps {
			s := &ts.Steps[i]
			if uses.Step == s.Name {
				replaceStep := *s
				err = OverrideStep(&replaceStep, &step, overrideAllSteps)
				if err != nil {
					return nil, errors.Wrapf(err, "failed to override step in task %s", taskName)
				}
				return []v1beta1.Step{replaceStep}, nil
			}
		}
		return nil, errors.Errorf("uses %s task %s has no step named %s", uses.String(), taskName, uses.Step)
	}

	overrideAllSteps = len(ts.Steps) > 1
	var steps []v1beta1.Step
	for i := range ts.Steps {
		s := &ts.Steps[i]
		replaceStep := *s
		err = OverrideStep(&replaceStep, &step, overrideAllSteps)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to override step in task %s", taskName)
		}
		steps = append(steps, replaceStep)
	}
	return steps, nil
}
