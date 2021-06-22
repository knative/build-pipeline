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

type PipelineRunResolverReconciler struct {
	// Implements reconciler.LeaderAware
	reconciler.LeaderAwareFuncs

	kubeClientSet     kubernetes.Interface
	pipelineClientSet clientset.Interface
	pipelinerunLister listers.PipelineRunLister
	configStore       reconciler.ConfigStore
}

var _ controller.Reconciler = &PipelineRunResolverReconciler{}

func (r *PipelineRunResolverReconciler) Reconcile(ctx context.Context, key string) error {
	if r.configStore != nil {
		ctx = r.configStore.ToContext(ctx)
	}

	logger := logging.FromContext(ctx)

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return fmt.Errorf("invalid resource key %q: %v", key, err)
	}

	pr, err := r.pipelinerunLister.PipelineRuns(namespace).Get(name)
	if err != nil {
		return fmt.Errorf("error getting pipelinerun %s/%s: %w", namespace, name, err)
	}

	req := resolution.PipelineRunResolutionRequest{
		KubeClientSet:     r.kubeClientSet,
		PipelineClientSet: r.pipelineClientSet,
		PipelineRun:       pr,
	}

	// TODO(sbws): At some point I'd like to add a new status reason. Something
	// like Succeeded/Unknown with reason:PipelineResolving. This would improve on the
	// existing situation where PipelineRuns are stored with a blank status until
	// bundle / cluster resolution is completed. For now, though, that does not match
	// behaviour with the existing reconcilers so it is not added yet.

	err = req.Resolve(ctx)
	switch {
	case pr == nil:
		// The pipelinerun we were told to resolve doesn't exist, therefore
		// we can't update it with a failure condition.
		// TODO(sbws): Does this match our reconciler's current behaviour?
		return controller.NewPermanentError(err)
	case err == resolution.ErrorPipelineRunAlreadyResolved:
		// Nothing to do: another process has already resolved the pipelinerun.
		return nil
	case err != nil:
		reason := resolution.ReasonPipelineRunResolutionFailed
		if e, ok := err.(*resolution.Error); ok {
			reason = e.Reason
		}
		pr.Status.MarkFailed(reason, err.Error())
		logger.Error(err)
		return err
	}

	resolution.CopyPipelineMetaToPipelineRun(req.ResolvedPipelineMeta, pr)

	pr.Status.PipelineSpec = req.ResolvedPipelineSpec

	// Update() because we want any labels and annotations from the pipeline's
	// meta persisted in the PipelineRun. Matches expectations/behaviour of the
	// current PipelineRun reconciler.
	newPR, err := r.pipelineClientSet.TektonV1beta1().PipelineRuns(pr.Namespace).Update(ctx, pr, metav1.UpdateOptions{})
	if err != nil {
		err = fmt.Errorf("error committing updated pipeline spec to pipelinerun %s/%s: %w", namespace, name, err)
		pr.Status.MarkFailed(resolution.ReasonPipelineRunResolutionFailed, err.Error())
		logger.Error(err)
		return fmt.Errorf("error updating pipelinerun spec: %w", err)
	}

	// UpdateStatus() because we want to store the PipelineSpec in-lined in the
	// PipelineRun Status. Matches expectations/behaviour of the current
	// PipelineRun reconciler.
	newPR.Status.PipelineSpec = pr.Status.PipelineSpec
	newPR, err = r.pipelineClientSet.TektonV1beta1().PipelineRuns(namespace).UpdateStatus(ctx, pr, metav1.UpdateOptions{})
	if err != nil {
		err = fmt.Errorf("error committing updated pipeline spec to pipelinerun %s/%s: %w", namespace, name, err)
		pr.Status.MarkFailed(resolution.ReasonPipelineRunResolutionFailed, err.Error())
		logger.Error(err)
		return fmt.Errorf("error updating pipelinerun status: %v", err)
	}
	return err
}
