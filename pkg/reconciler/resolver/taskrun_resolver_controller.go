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
	"reflect"
	"strings"

	"github.com/tektoncd/pipeline/pkg/apis/config"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	pipelineclient "github.com/tektoncd/pipeline/pkg/client/injection/client"
	taskruninformer "github.com/tektoncd/pipeline/pkg/client/injection/informers/pipeline/v1beta1/taskrun"
	"k8s.io/client-go/tools/cache"
	kubeclient "knative.dev/pkg/client/injection/kube/client"
	"knative.dev/pkg/configmap"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/logging"
)

// NewTaskRunResolverController returns a func that itself returns a knative
// controller implementation suitable for resolving Tekton TaskRuns.
// Resolving TaskRuns is the process of taking a TaskRun and determining
// which Task it is attempting to run.
func NewTaskRunResolverController() func(context.Context, configmap.Watcher) *controller.Impl {
	return func(ctx context.Context, cmw configmap.Watcher) *controller.Impl {
		logger := logging.FromContext(ctx)
		kubeclientset := kubeclient.Get(ctx)
		pipelineclientset := pipelineclient.Get(ctx)
		taskrunInformer := taskruninformer.Get(ctx)

		lister := taskrunInformer.Lister()

		r := &TaskRunResolverReconciler{
			LeaderAwareFuncs: buildTaskRunLeaderAwareFuncs(lister),

			kubeClientSet:     kubeclientset,
			pipelineClientSet: pipelineclientset,
			taskrunLister:     taskrunInformer.Lister(),
		}

		configStore := config.NewStore(logger.Named("config-store"))
		configStore.WatchConfigs(cmw)
		r.configStore = configStore

		ctrType := reflect.TypeOf(r).Elem()
		ctrTypeName := fmt.Sprintf("%s.%s", ctrType.PkgPath(), ctrType.Name())
		ctrTypeName = strings.ReplaceAll(ctrTypeName, "/", ".")
		impl := controller.NewImpl(r, logger, ctrTypeName)

		logger.Info("Setting up resolver controller event handlers")

		taskrunInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
			FilterFunc: acceptTaskRunWithUnpopulatedStatusSpec,
			Handler: cache.ResourceEventHandlerFuncs{
				AddFunc: impl.Enqueue,
			},
		})

		return impl
	}
}

// acceptTaskRunWithUnpopulatedStatusSpec is a filter func that is used to
// limit the taskruns that the resolver reconciler sees. Only the taskruns
// without a populated status.taskSpec field are passed to the resolver
// reconciler.
func acceptTaskRunWithUnpopulatedStatusSpec(obj interface{}) bool {
	tr, ok := obj.(*v1beta1.TaskRun)
	return ok && tr.Status.TaskSpec == nil
}
