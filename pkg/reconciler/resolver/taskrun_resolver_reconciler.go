/*
Copyright 2021 The Tekton Authors

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

package resolver

import (
	"context"
	"fmt"

	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	clientset "github.com/tektoncd/pipeline/pkg/client/clientset/versioned"
	listers "github.com/tektoncd/pipeline/pkg/client/listers/pipeline/v1beta1"
	"github.com/tektoncd/pipeline/pkg/resolution"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/logging"
	"knative.dev/pkg/reconciler"
)

type TaskRunResolverReconciler struct {
	// Implements reconciler.LeaderAware
	reconciler.LeaderAwareFuncs

	kubeClientSet     kubernetes.Interface
	pipelineClientSet clientset.Interface
	taskrunLister     listers.TaskRunLister
	configStore       reconciler.ConfigStore
}

var _ controller.Reconciler = &TaskRunResolverReconciler{}

func (r *TaskRunResolverReconciler) Reconcile(ctx context.Context, key string) error {
	if r.configStore != nil {
		ctx = r.configStore.ToContext(ctx)
	}

	logger := logging.FromContext(ctx)

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return fmt.Errorf("invalid resource key %q: %v", key, err)
	}

	tr, err := r.taskrunLister.TaskRuns(namespace).Get(name)
	if err != nil {
		return fmt.Errorf("error getting taskrun %s/%s: %w", namespace, name, err)
	}

	req := resolution.TaskRunResolutionRequest{
		KubeClientSet:     r.kubeClientSet,
		PipelineClientSet: r.pipelineClientSet,
		TaskRun:           tr,
	}

	// TODO(sbws): At some point I'd like to add a new status reason. Something
	// like Succeeded/Unknown with reason:TaskResolving. This would improve on the
	// existing situation where TaskRuns are stored with a blank status until
	// bundle / cluster resolution is completed. For now, though, that does not match
	// behaviour with the existing reconcilers so it is not added yet.

	err = req.Resolve(ctx)
	switch {
	case tr == nil:
		// The taskrun we were told to resolve doesn't exist, therefore
		// we can't update it with a failure condition.
		// TODO(sbws): Does this match our reconciler's current behaviour?
		return controller.NewPermanentError(err)
	case err == resolution.ErrorTaskRunAlreadyResolved:
		// Nothing to do: another process has already resolved the taskrun.
		return nil
	case err != nil:
		reason := resolution.ReasonTaskRunResolutionFailed
		if e, ok := err.(*resolution.Error); ok {
			reason = e.Reason
		}
		tr.Status.MarkResourceFailed(v1beta1.TaskRunReason(reason), err)
		logger.Error(err)
		return err
	}

	resolution.CopyTaskMetaToTaskRun(req.ResolvedTaskMeta, tr)

	tr.Status.TaskSpec = req.ResolvedTaskSpec

	// Update() because we wants any labels and annotations from the task's
	// meta persisted in the taskrun. Matches expectations/behaviour of the
	// current TaskRun reconciler.
	newTR, err := r.pipelineClientSet.TektonV1beta1().TaskRuns(tr.Namespace).Update(ctx, tr, metav1.UpdateOptions{})
	if err != nil {
		err = fmt.Errorf("error committing updated task spec to taskrun %s/%s: %w", namespace, name, err)
		tr.Status.MarkResourceFailed(resolution.ReasonTaskRunResolutionFailed, err)
		logger.Error(err)
		return fmt.Errorf("error updating taskrun spec: %w", err)
	}

	// UpdateStatus() because we want to store the TaskSpec in-lined in the
	// taskrun Status. Matches expectations/behaviour of the current
	// TaskRun reconciler.
	newTR.Status.TaskSpec = tr.Status.TaskSpec
	newTR, err = r.pipelineClientSet.TektonV1beta1().TaskRuns(namespace).UpdateStatus(ctx, tr, metav1.UpdateOptions{})
	if err != nil {
		err = fmt.Errorf("error committing updated task spec to taskrun %s/%s: %w", namespace, name, err)
		tr.Status.MarkResourceFailed(resolution.ReasonTaskRunResolutionFailed, err)
		logger.Error(err)
		return fmt.Errorf("error updating taskrun status: %v", err)
	}
	return err
}
