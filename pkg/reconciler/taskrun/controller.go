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

package taskrun

import (
	"context"
	"time"

	"github.com/tektoncd/pipeline/pkg/apis/pipeline"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1alpha1"
	pipelineclient "github.com/tektoncd/pipeline/pkg/client/injection/client"
	clustertaskinformer "github.com/tektoncd/pipeline/pkg/client/injection/informers/pipeline/v1alpha1/clustertask"
	resourceinformer "github.com/tektoncd/pipeline/pkg/client/injection/informers/pipeline/v1alpha1/pipelineresource"
	taskinformer "github.com/tektoncd/pipeline/pkg/client/injection/informers/pipeline/v1alpha1/task"
	taskruninformer "github.com/tektoncd/pipeline/pkg/client/injection/informers/pipeline/v1alpha1/taskrun"
	"github.com/tektoncd/pipeline/pkg/reconciler"
	"github.com/tektoncd/pipeline/pkg/reconciler/taskrun/entrypoint"
	cloudeventclient "github.com/tektoncd/pipeline/pkg/reconciler/taskrun/resources/cloudevent"
	"k8s.io/client-go/tools/cache"
        "k8s.io/client-go/util/workqueue"
	kubeclient "knative.dev/pkg/client/injection/kube/client"
	podinformer "knative.dev/pkg/client/injection/kube/informers/core/v1/pod"
	"knative.dev/pkg/configmap"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/logging"
	"knative.dev/pkg/tracker"
)

const (
	resyncPeriod = 10 * time.Hour
)

func NewController(images pipeline.Images) func(context.Context, configmap.Watcher) *controller.Impl {
	return func(ctx context.Context, cmw configmap.Watcher) *controller.Impl {
		logger := logging.FromContext(ctx)
		kubeclientset := kubeclient.Get(ctx)
		pipelineclientset := pipelineclient.Get(ctx)
		taskRunInformer := taskruninformer.Get(ctx)
		taskInformer := taskinformer.Get(ctx)
		clusterTaskInformer := clustertaskinformer.Get(ctx)
		podInformer := podinformer.Get(ctx)
		resourceInformer := resourceinformer.Get(ctx)
		timeoutHandler := reconciler.NewTimeoutHandler(ctx.Done(), logger)
		metrics, err := NewRecorder()
		if err != nil {
			logger.Errorf("Failed to create taskrun metrics recorder %v", err)
		}

		opt := reconciler.Options{
			KubeClientSet:     kubeclientset,
			PipelineClientSet: pipelineclientset,
			ConfigMapWatcher:  cmw,
			ResyncPeriod:      resyncPeriod,
			Logger:            logger,
		}

		c := &Reconciler{
			Base:              reconciler.NewBase(opt, taskRunAgentName, images),
			taskRunLister:     taskRunInformer.Lister(),
			taskLister:        taskInformer.Lister(),
			clusterTaskLister: clusterTaskInformer.Lister(),
			resourceLister:    resourceInformer.Lister(),
			timeoutHandler:    timeoutHandler,
			cloudEventClient:  cloudeventclient.Get(ctx),
			metrics:           metrics,
			queue:             workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "ttl_taskruns_to_delete"),
		}
		impl := controller.NewImpl(c, c.Logger, taskRunControllerName)

		timeoutHandler.SetTaskRunCallbackFunc(impl.Enqueue)
		timeoutHandler.CheckTimeouts(kubeclientset, pipelineclientset)

		c.Logger.Info("Setting up event handlers")
		taskRunInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc:    impl.Enqueue,
			UpdateFunc: controller.PassNew(impl.Enqueue),
		})

		AddTaskRun := func(obj interface{}) {
			tr := obj.(*v1alpha1.TaskRun)
			c.Logger.Infof("Adding TaskRun %s/%s", tr.Namespace, tr.Name)

			if tr.DeletionTimestamp == nil && taskRunCleanup(tr) {
				impl.Enqueue(tr)
			}
		}

		UpdateTaskRun := func(old, cur interface{}) {
			tr := cur.(*v1alpha1.TaskRun)
			c.Logger.Infof("Updating TaskRun %s/%s", tr.Namespace, tr.Name)

			if tr.DeletionTimestamp == nil && taskRunCleanup(tr) {
				impl.Enqueue(tr)
			}
		}

		taskRunInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc:    AddTaskRun,
			UpdateFunc: UpdateTaskRun,
		})

		c.ListerSynced = taskRunInformer.Informer().HasSynced

		c.tracker = tracker.New(impl.EnqueueKey, controller.GetTrackerLease(ctx))

		podInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
			FilterFunc: controller.Filter(v1alpha1.SchemeGroupVersion.WithKind("TaskRun")),
			Handler:    controller.HandleAll(impl.EnqueueControllerOf),
		})

		// FIXME(vdemeester) it was never set
		//entrypoint cache will be initialized by controller if not provided
		c.Logger.Info("Setting up Entrypoint cache")
		c.cache = nil
		if c.cache == nil {
			c.cache, _ = entrypoint.NewCache()
		}

		return impl
	}
}
