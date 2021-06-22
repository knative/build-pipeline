package resolver

import (
	"github.com/tektoncd/pipeline/pkg/client/listers/pipeline/v1beta1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"knative.dev/pkg/reconciler"
)

// buildTaskRunLeaderAwareFuncs constructs a LeaderAwareFuncs object to embed in a
// TaskRun-aware controller so that it can meet knative's controller.LeaderAware interface.
func buildTaskRunLeaderAwareFuncs(lister v1beta1.TaskRunLister) reconciler.LeaderAwareFuncs {
	return reconciler.LeaderAwareFuncs{
		PromoteFunc: func(bkt reconciler.Bucket, enq func(reconciler.Bucket, types.NamespacedName)) error {
			all, err := lister.List(labels.Everything())
			if err != nil {
				return err
			}
			for _, elt := range all {
				enq(bkt, types.NamespacedName{
					Namespace: elt.GetNamespace(),
					Name:      elt.GetName(),
				})
			}
			return nil
		},
	}
}

// buildPipelineRunLeaderAwareFuncs constructs a LeaderAwareFuncs object to embed in a
// PipelineRun-aware controller so that it can meet knative's controller.LeaderAware interface.
func buildPipelineRunLeaderAwareFuncs(lister v1beta1.PipelineRunLister) reconciler.LeaderAwareFuncs {
	return reconciler.LeaderAwareFuncs{
		PromoteFunc: func(bkt reconciler.Bucket, enq func(reconciler.Bucket, types.NamespacedName)) error {
			all, err := lister.List(labels.Everything())
			if err != nil {
				return err
			}
			for _, elt := range all {
				enq(bkt, types.NamespacedName{
					Namespace: elt.GetNamespace(),
					Name:      elt.GetName(),
				})
			}
			return nil
		},
	}
}
