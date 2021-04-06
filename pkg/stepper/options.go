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
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	clientset "github.com/tektoncd/pipeline/pkg/client/clientset/versioned"
	"github.com/tektoncd/pipeline/pkg/reconciler/taskrun/resources"
	"github.com/tektoncd/pipeline/pkg/remote/git"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
)

type Options struct {
	KubeClientSet     kubernetes.Interface
	PipelineClientSet clientset.Interface
	Namespace         string
	ServiceAccount    string
	GitFactory        git.Factory
}

func (o *Options) CreateRemote(ctx context.Context, uses *v1beta1.Uses) (runtime.Object, error) {
	logger := o.GitFactory.GetLogger()
	taskRef := uses.ToTaskRef()
	getTaskfunc, _, err := resources.GetTaskFunc(ctx, o.KubeClientSet, o.PipelineClientSet, &o.GitFactory, &taskRef, o.Namespace, o.ServiceAccount)
	if err != nil {
		logger.Errorf("Failed to fetch task reference %s: %v", taskRef.Name, err)
		return nil, err
	}

	t, err := getTaskfunc(ctx, taskRef.Name)
	if err != nil {
		return nil, err
	}
	return &v1beta1.Task{
		ObjectMeta: t.TaskMetadata(),
		Spec:       t.TaskSpec(),
	}, nil
}
