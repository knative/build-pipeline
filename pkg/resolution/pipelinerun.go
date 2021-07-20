package resolution

import (
	"context"
	"fmt"

	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	clientset "github.com/tektoncd/pipeline/pkg/client/clientset/versioned"
	listers "github.com/tektoncd/pipeline/pkg/client/listers/pipeline/v1beta1"
	"github.com/tektoncd/pipeline/pkg/reconciler/pipelinerun/resources"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type PipelineRunResolutionRequest struct {
	KubeClientSet     kubernetes.Interface
	PipelineClientSet clientset.Interface
	PipelineRunLister listers.PipelineRunLister
	PipelineRun       *v1beta1.PipelineRun

	ResolvedPipelineMeta *metav1.ObjectMeta
	ResolvedPipelineSpec *v1beta1.PipelineSpec
}

// PipelineRunResolutionRequest.Resolve implements the default resolution behaviour
// for a Tekton Pipelines pipelinerun. It resolves the pipeline associated with the pipelinerun
// from one of three places:
//
// - in-line in the pipelinerun's spec.pipelineSpec
// - from the cluster via the pipelinerun's spec.pipelineRef
// - (when relevant feature flag enabled) from a Tekton Bundle indicated by the pipelinerun's spec.pipelineRef.bundle field.
//
// If a pipeline is resolved correctly from the pipelinerun then the ResolvedPipelineMeta
// and ResolvedPipelineSpec fields of the PipelineRunResolutionRequest will be
// populated after this method returns.
//
// If an error occurs during any part in the resolution process it will be
// returned with both a human-readable Message and a machine-readable Reason
// embedded in a *resolution.Error.
func (req *PipelineRunResolutionRequest) Resolve(ctx context.Context) error {
	pipelinerunKey := fmt.Sprintf("%s/%s", req.PipelineRun.Namespace, req.PipelineRun.Name)

	getPipelineFunc, err := resources.GetPipelineFunc(ctx, req.KubeClientSet, req.PipelineClientSet, req.PipelineRun)
	if err != nil {
		// TODO(sbws): What's the error reason to return here?
		// Encapsulate in a resolution.Error
		return fmt.Errorf("Failed to fetch pipeline func for pipelinerun %q: %w", pipelinerunKey, err)
	}

	// TODO(sbws): We're assuming that GetPipelineData will always return meta
	// if it successfully returns spec. This is the behaviour of our current
	// reconciler (I think). Double-check and decide if there is a
	// different error we should be returning when pipelineMeta is nil.
	pipelineMeta, pipelineSpec, err := resources.GetPipelineData(ctx, req.PipelineRun, getPipelineFunc)
	if err != nil {
		// TODO(sbws): Check with the pipelinerun reconciler that this
		// is the current message and reason and update this.
		return NewError(ReasonCouldntGetPipeline, fmt.Errorf("error getting pipeline func for pipelinerun %q: %w", pipelinerunKey, err))
	} else if pipelineSpec == nil {
		return NewError(ReasonCouldntGetPipeline, fmt.Errorf("no pipeline found for pipelinerun %q", pipelinerunKey))
	}

	req.ResolvedPipelineMeta = pipelineMeta
	req.ResolvedPipelineSpec = pipelineSpec

	return nil
}
