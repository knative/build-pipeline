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

package pipelinerun

import (
	"context"
	"time"

	"github.com/tektoncd/pipeline/pkg/apis/config"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	pipelineclient "github.com/tektoncd/pipeline/pkg/client/injection/client"
	conditioninformer "github.com/tektoncd/pipeline/pkg/client/injection/informers/pipeline/v1alpha1/condition"
	runinformer "github.com/tektoncd/pipeline/pkg/client/injection/informers/pipeline/v1alpha1/run"
	clustertaskinformer "github.com/tektoncd/pipeline/pkg/client/injection/informers/pipeline/v1beta1/clustertask"
	pipelineinformer "github.com/tektoncd/pipeline/pkg/client/injection/informers/pipeline/v1beta1/pipeline"
	pipelineruninformer "github.com/tektoncd/pipeline/pkg/client/injection/informers/pipeline/v1beta1/pipelinerun"
	taskinformer "github.com/tektoncd/pipeline/pkg/client/injection/informers/pipeline/v1beta1/task"
	taskruninformer "github.com/tektoncd/pipeline/pkg/client/injection/informers/pipeline/v1beta1/taskrun"
	pipelinerunreconciler "github.com/tektoncd/pipeline/pkg/client/injection/reconciler/pipeline/v1beta1/pipelinerun"
	resourceinformer "github.com/tektoncd/pipeline/pkg/client/resource/injection/informers/resource/v1alpha1/pipelineresource"
	cloudeventclient "github.com/tektoncd/pipeline/pkg/reconciler/events/cloudevent"
	"github.com/tektoncd/pipeline/pkg/reconciler/volumeclaim"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	kubeclient "knative.dev/pkg/client/injection/kube/client"
	"knative.dev/pkg/configmap"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/kmeta"
	"knative.dev/pkg/logging"
	"knative.dev/pkg/tracker"
)

// NewController instantiates a new controller.Impl from knative.dev/pkg/controller
func NewController(namespace string, images pipeline.Images) func(context.Context, configmap.Watcher) *controller.Impl {
	return func(ctx context.Context, cmw configmap.Watcher) *controller.Impl {
		logger := logging.FromContext(ctx)
		kubeclientset := kubeclient.Get(ctx)
		pipelineclientset := pipelineclient.Get(ctx)
		taskRunInformer := taskruninformer.Get(ctx)
		runInformer := runinformer.Get(ctx)
		taskInformer := taskinformer.Get(ctx)
		clusterTaskInformer := clustertaskinformer.Get(ctx)
		pipelineRunInformer := pipelineruninformer.Get(ctx)
		pipelineInformer := pipelineinformer.Get(ctx)
		resourceInformer := resourceinformer.Get(ctx)
		conditionInformer := conditioninformer.Get(ctx)
		metrics, err := NewRecorder()
		if err != nil {
			logger.Errorf("Failed to create pipelinerun metrics recorder %v", err)
		}

		c := &Reconciler{
			KubeClientSet:     kubeclientset,
			PipelineClientSet: pipelineclientset,
			Images:            images,
			pipelineRunLister: pipelineRunInformer.Lister(),
			pipelineLister:    pipelineInformer.Lister(),
			taskLister:        taskInformer.Lister(),
			clusterTaskLister: clusterTaskInformer.Lister(),
			taskRunLister:     taskRunInformer.Lister(),
			runLister:         runInformer.Lister(),
			resourceLister:    resourceInformer.Lister(),
			conditionLister:   conditionInformer.Lister(),
			cloudEventClient:  cloudeventclient.Get(ctx),
			metrics:           metrics,
			pvcHandler:        volumeclaim.NewPVCHandler(kubeclientset, logger),
		}
		impl := pipelinerunreconciler.NewImpl(ctx, c, func(impl *controller.Impl) controller.Options {
			configStore := config.NewStore(logger.Named("config-store"))
			configStore.WatchConfigs(cmw)
			return controller.Options{
				AgentName:   pipeline.PipelineRunControllerName,
				ConfigStore: configStore,
			}
		})

		c.snooze = func(acc kmeta.Accessor, amnt time.Duration) {
			impl.EnqueueKeyAfter(types.NamespacedName{
				Namespace: acc.GetNamespace(),
				Name:      acc.GetName(),
			}, amnt)
		}

		logger.Info("Setting up event handlers")
		pipelineRunInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
			FilterFunc: filterPipelineRunsWithoutStatusPipelineSpec,
			Handler: cache.ResourceEventHandlerFuncs{
				AddFunc:    impl.Enqueue,
				UpdateFunc: controller.PassNew(impl.Enqueue),
				DeleteFunc: impl.Enqueue,
			},
		})

		c.tracker = tracker.New(impl.EnqueueKey, 30*time.Minute)
		taskRunInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
			UpdateFunc: controller.PassNew(impl.EnqueueControllerOf),
		})
		runInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
			UpdateFunc: controller.PassNew(impl.EnqueueControllerOf),
		})

		go metrics.ReportRunningPipelineRuns(ctx, pipelineRunInformer.Lister())

		return impl
	}
}

func filterPipelineRunsWithoutStatusPipelineSpec(obj interface{}) bool {
	pr, ok := obj.(*v1beta1.PipelineRun)
	if !ok {
		return false
	}
	return pr.Status.PipelineSpec != nil
}
