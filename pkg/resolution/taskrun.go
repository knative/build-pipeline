package resolution

import (
	"context"
	"fmt"

	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	clientset "github.com/tektoncd/pipeline/pkg/client/clientset/versioned"
	resources "github.com/tektoncd/pipeline/pkg/reconciler/taskrun/resources"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type TaskRunResolutionRequest struct {
	KubeClientSet     kubernetes.Interface
	PipelineClientSet clientset.Interface
	TaskRun           *v1beta1.TaskRun

	ResolvedTaskMeta *metav1.ObjectMeta
	ResolvedTaskSpec *v1beta1.TaskSpec
}

// TaskRunResolutionRequest.Resolve implements the default resolution behaviour
// for a Tekton Pipelines taskrun. It resolves the task associated with the taskrun
// from one of three places:
//
// - in-line in the taskrun's spec.taskSpec
// - from the cluster via the taskrun's spec.taskRef
// - (when relevant feature flag enabled) from a Tekton Bundle indicated by the taskrun's spec.taskRef.bundle field.
//
// If a task is resolved correctly from the taskrun then the ResolvedTaskMeta
// and ResolvedTaskSpec fields of the TaskRunResolutionRequest will be
// populated after this method returns.
//
// If an error occurs during any part in the resolution process it will be
// returned with both a human-readable Message and a machine-readable Reason
// embedded in a *resolution.Error.
func (req *TaskRunResolutionRequest) Resolve(ctx context.Context) error {
	if req.TaskRun.Status.TaskSpec != nil {
		return ErrorTaskRunAlreadyResolved
	}

	taskrunKey := fmt.Sprintf("%s/%s", req.TaskRun.Namespace, req.TaskRun.Name)

	getTaskFunc, err := resources.GetTaskFuncFromTaskRun(ctx, req.KubeClientSet, req.PipelineClientSet, req.TaskRun)
	if err != nil {
		// TODO(sbws): What's the error reason to return here?
		// Encapsulate in a resolution.Error
		return fmt.Errorf("error getting task func for taskrun %q: %w", taskrunKey, err)
	}

	// TODO(sbws): We're assuming that GetTaskData will always return meta
	// if it successfully returns spec. This is the behaviour of our current
	// reconciler (I think). Double-check and decide if there is a
	// different error we should be returning when taskMeta is nil.
	taskMeta, taskSpec, err := resources.GetTaskData(ctx, req.TaskRun, getTaskFunc)
	if err != nil {
		// TODO(sbws): This is the wrong error message. we already have
		// the task func. find the appropriate message from our
		// existing reconciler and replace this message with it.
		return NewError(ReasonTaskRunResolutionFailed, fmt.Errorf("error getting task func for taskrun %q: %w", taskrunKey, err))
	} else if taskSpec == nil {
		return NewError(ReasonCouldntGetTask, fmt.Errorf("no task found for taskrun %q", taskrunKey))
	}

	req.ResolvedTaskMeta = taskMeta
	req.ResolvedTaskSpec = taskSpec

	return nil
}
