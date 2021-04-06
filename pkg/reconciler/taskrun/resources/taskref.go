/*
Copyright 2020 The Tekton Authors

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

package resources

import (
	"context"
	"fmt"
	"github.com/tektoncd/pipeline/pkg/remote"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn/k8schain"
	"github.com/tektoncd/pipeline/pkg/apis/config"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1alpha1"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	clientset "github.com/tektoncd/pipeline/pkg/client/clientset/versioned"
	"github.com/tektoncd/pipeline/pkg/remote/git"
	"github.com/tektoncd/pipeline/pkg/remote/oci"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// GetTaskFunc is a factory function that will use the given TaskRef as context to return a valid GetTask function. It
// also requires a kubeclient, tektonclient, namespace, and service account in case it needs to find that task in
// cluster or authorize against an external repositroy. It will figure out whether it needs to look in the cluster or in
// a remote image to fetch the  reference. It will also return the "kind" of the task being referenced.
func GetTaskFunc(ctx context.Context, k8s kubernetes.Interface, tekton clientset.Interface, gitFactory *git.Factory, tr *v1beta1.TaskRef, namespace, saName string) (GetTask, v1beta1.TaskKind, error) {
	cfg := config.FromContextOrDefaults(ctx)
	kind := v1alpha1.NamespacedTaskKind
	if tr != nil && tr.Kind != "" {
		kind = tr.Kind
	}

	switch {
	case cfg.FeatureFlags.EnableTektonOCIBundles && tr != nil && tr.Bundle != "":
		// Return an inline function that implements GetTask by calling Resolver.Get with the specified task type and
		// casting it to a TaskObject.
		return func(ctx context.Context, name string) (v1beta1.TaskObject, error) {
			// If there is a bundle url at all, construct an OCI resolver to fetch the task.
			kc, err := k8schain.New(ctx, k8s, k8schain.Options{
				Namespace:          namespace,
				ServiceAccountName: saName,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to get keychain: %w", err)
			}
			resolver := oci.NewResolver(tr.Bundle, kc)

			return getRemoteTask(ctx, resolver, kind, name)
		}, kind, nil
	case tr != nil && tr.Git != "":
		// Return an inline function that implements GetTask by calling Resolver.Get with the specified task type and
		// casting it to a TaskObject.
		return func(ctx context.Context, name string) (v1beta1.TaskObject, error) {
			resolver := gitFactory.CreateResolver(ctx)
			return getRemoteTask(ctx, resolver, kind, name)
		}, kind, nil
	default:
		// Even if there is no task ref, we should try to return a local resolver.
		local := &LocalTaskRefResolver{
			Namespace:    namespace,
			Kind:         kind,
			Tektonclient: tekton,
		}
		return local.GetTask, kind, nil
	}
}

func getRemoteTask(ctx context.Context, resolver remote.Resolver, kind v1alpha1.TaskKind, name string) (v1beta1.TaskObject, error) {
	// Because the resolver will only return references with the same kind (eg ClusterTask), this will ensure we
	// don't accidentally return a Task with the same name but different kind.
	obj, err := resolver.Get(strings.ToLower(string(kind)), name)
	if err != nil {
		return nil, err
	}

	// If the resolved object is already a v1beta1.{Cluster}Task, it should be returnable as a
	// v1beta1.TaskObject.
	if ti, ok := obj.(v1beta1.TaskObject); ok {
		return ti, nil
	}

	// If this object is not already a v1beta1 object, figure out what type it is actually and try to coerce it
	// into a v1beta1.TaskInterface compatible object.
	switch tt := obj.(type) {
	case *v1alpha1.Task:
		betaTask := &v1beta1.Task{}
		err := tt.ConvertTo(ctx, betaTask)
		return betaTask, err
	case *v1alpha1.ClusterTask:
		betaTask := &v1beta1.ClusterTask{}
		err := tt.ConvertTo(ctx, betaTask)
		return betaTask, err
	}

	return nil, fmt.Errorf("failed to convert obj %s into Task", obj.GetObjectKind().GroupVersionKind().String())
}

// LocalTaskRefResolver uses the current cluster to resolve a task reference.
type LocalTaskRefResolver struct {
	Namespace    string
	Kind         v1beta1.TaskKind
	Tektonclient clientset.Interface
}

// GetTask will resolve either a Task or ClusterTask from the local cluster using a versioned Tekton client. It will
// return an error if it can't find an appropriate Task for any reason.
func (l *LocalTaskRefResolver) GetTask(ctx context.Context, name string) (v1beta1.TaskObject, error) {
	if l.Kind == v1beta1.ClusterTaskKind {
		task, err := l.Tektonclient.TektonV1beta1().ClusterTasks().Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		return task, nil
	}

	// If we are going to resolve this reference locally, we need a namespace scope.
	if l.Namespace == "" {
		return nil, fmt.Errorf("Must specify namespace to resolve reference to task %s", name)
	}
	return l.Tektonclient.TektonV1beta1().Tasks(l.Namespace).Get(ctx, name, metav1.GetOptions{})
}
