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
	"k8s.io/apimachinery/pkg/runtime"
)

type cachingRemoter struct {
	resolveRemote func(ctx context.Context, uses *v1beta1.Uses) (runtime.Object, error)
	cache         map[string]runtime.Object
}

// NewCachingRemoter creates a new remote resolver which uses a cache of each kind of uses so we can optimise the
// amount of OCI bundle fetching or git cloning when processing multiple steps within a single tekton resource
func NewCachingRemoter(resolveRemote func(ctx context.Context, uses *v1beta1.Uses) (runtime.Object, error)) *cachingRemoter {
	return &cachingRemoter{
		resolveRemote: resolveRemote,
		cache:         map[string]runtime.Object{},
	}
}

func (o *cachingRemoter) CreateRemote(ctx context.Context, uses *v1beta1.Uses) (runtime.Object, error) {
	key := uses.Key()

	obj := o.cache[key]
	if obj != nil {
		return obj, nil
	}

	var err error
	obj, err = o.resolveRemote(ctx, uses)
	if err != nil {
		return nil, err
	}

	o.cache[key] = obj
	return obj, nil
}
