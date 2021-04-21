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

package validate

import (
	"strings"

	"github.com/tektoncd/pipeline/pkg/apis/pipeline"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/apis"
)

const MaxLength = 63

func ObjectMetadata(meta metav1.Object) *apis.FieldError {
	name := meta.GetName()

	if strings.Contains(name, ".") {
		return &apis.FieldError{
			Message: "Invalid resource name: special character . must not be present",
			Paths:   []string{"name"},
		}
	}

	if len(name) > MaxLength {
		return &apis.FieldError{
			Message: "Invalid resource name: length must be no more than 63 characters",
			Paths:   []string{"name"},
		}
	}

	// metadata must not contain any label with tekton.dev/*
	for k := range meta.GetLabels() {
		if strings.HasPrefix(k, pipeline.GroupName) {
			return &apis.FieldError{
				Message: "Invalid label key: can not change tekton resource name",
				Paths:   []string{"labels"},
			}
		}
	}
	return nil
}
