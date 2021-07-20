package resolution

import (
	"github.com/tektoncd/pipeline/pkg/apis/pipeline"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CopyTaskMetaToTaskRun implements the default Tekton Pipelines behaviour
// for copying meta information to a TaskRun from the Task it references.
//
// The labels and annotations from the task will all be copied to the taskrun
// verbatim. This matches the expectations/behaviour of the current TaskRun
// reconciler.
func CopyTaskMetaToTaskRun(meta *metav1.ObjectMeta, tr *v1beta1.TaskRun) {
	if tr.ObjectMeta.Labels == nil && len(meta.Labels) > 0 {
		tr.ObjectMeta.Labels = map[string]string{}
	}
	for key, value := range meta.Labels {
		tr.ObjectMeta.Labels[key] = value
	}

	if tr.ObjectMeta.Annotations == nil {
		tr.ObjectMeta.Annotations = make(map[string]string, len(meta.Annotations))
	}
	for key, value := range meta.Annotations {
		tr.ObjectMeta.Annotations[key] = value
	}

	if tr.Spec.TaskRef != nil {
		if tr.Spec.TaskRef.Kind == "ClusterTask" {
			tr.ObjectMeta.Labels[pipeline.GroupName+pipeline.ClusterTaskLabelKey] = meta.Name
		} else {
			tr.ObjectMeta.Labels[pipeline.GroupName+pipeline.TaskLabelKey] = meta.Name
		}
	}
}

// CopyPipelineMetaToPipelineRun implements the default Tekton Pipelines
// behaviour for copying meta information to a PipelineRun from the Pipeline it
// references.
//
// The labels and annotations from the pipeline will all be copied to the
// pipielinerun verbatim. This matches the expectations/behaviour of the
// current PipelineRun reconciler.
func CopyPipelineMetaToPipelineRun(meta *metav1.ObjectMeta, pr *v1beta1.PipelineRun) {
	if pr.ObjectMeta.Labels == nil {
		pr.ObjectMeta.Labels = make(map[string]string, len(meta.Labels)+1)
	}
	for key, value := range meta.Labels {
		pr.ObjectMeta.Labels[key] = value
	}
	pr.ObjectMeta.Labels[pipeline.GroupName+pipeline.PipelineLabelKey] = meta.Name

	if pr.ObjectMeta.Annotations == nil {
		pr.ObjectMeta.Annotations = make(map[string]string, len(meta.Annotations))
	}
	for key, value := range meta.Annotations {
		pr.ObjectMeta.Annotations[key] = value
	}
}
