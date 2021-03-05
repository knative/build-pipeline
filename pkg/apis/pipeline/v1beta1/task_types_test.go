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

package v1beta1

import (
	"testing"
)

func TestUses_Key(t *testing.T) {
	tests := []struct {
		name string
		uses *Uses
		want string
	}{{
		name: "empty",
		uses: &Uses{},
		want: "git/github.com",
	}, {
		name: "github-explicit",
		uses: &Uses{
			Server: "github.com",
			Git:    "tektoncd/catalog/task/git-clone/0.2/git-clone.yaml",
		},
		want: "git/github.com/tektoncd/catalog/task/git-clone/0.2/git-clone.yaml",
	}, {
		name: "my-git-server-explicit",
		uses: &Uses{
			Server: "my.git.server.com",
			Git:    "tektoncd/catalog/task/git-clone/0.2/git-clone.yaml",
		},
		want: "git/my.git.server.com/tektoncd/catalog/task/git-clone/0.2/git-clone.yaml",
	}, {
		name: "ref",
		uses: &Uses{
			TaskRef: &TaskRef{
				Name:       "my-task",
				Kind:       NamespacedTaskKind,
				APIVersion: "",
			},
		},
		want: "ref/Task/my-task",
	}, {
		name: "oci",
		uses: &Uses{
			TaskRef: &TaskRef{
				Name:       "my-task",
				Kind:       ClusterTaskKind,
				APIVersion: "",
				Bundle:     "docker.io/myrepo/mycatalog:1.2.3",
			},
		},
		want: "ref/ClusterTask/my-task/docker.io/myrepo/mycatalog:1.2.3",
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.uses.Key() != tt.want {
				t.Fatalf("Uses key mismatch: got %s ; expected %s", tt.uses.Key(), tt.want)
			}
		})
	}
}
