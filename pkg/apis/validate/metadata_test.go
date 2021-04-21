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

package validate_test

import (
	"strings"
	"testing"

	"github.com/tektoncd/pipeline/pkg/apis/pipeline"

	"github.com/tektoncd/pipeline/pkg/apis/validate"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestMetadata_Failure(t *testing.T) {

	invalidMetas := []*metav1.ObjectMeta{
		{Name: strings.Repeat("s", validate.MaxLength+1)},
		{Name: "bad.name"},
		{Labels: map[string]string{pipeline.GroupName: "mytekton"}},
		{Labels: map[string]string{pipeline.GroupName + pipeline.TaskLabelKey: "mytask"}},
	}
	for _, invalidMeta := range invalidMetas {
		if err := validate.ObjectMetadata(invalidMeta); err == nil {
			t.Errorf("Failed to validate object meta data: %s", err)
		}
	}
}
