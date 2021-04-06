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

package git_test

import (
	"github.com/tektoncd/pipeline/pkg/remote/git"
	"testing"
)

func TestCloneURL(t *testing.T) {
	testCases := []struct {
		server     string
		owner      string
		repository string
		want       string
	}{
		{
			server:     "github.com",
			owner:      "myorg",
			repository: "myrepo",
			want:       "https://github.com/myorg/myrepo.git",
		},
		{
			server:     "https://github.com",
			owner:      "myorg",
			repository: "myrepo",
			want:       "https://github.com/myorg/myrepo.git",
		},
		{
			server:     "https://github.com/",
			owner:      "myorg",
			repository: "myrepo",
			want:       "https://github.com/myorg/myrepo.git",
		},
	}

	for _, tc := range testCases {
		got := git.GitCloneURL(tc.server, tc.owner, tc.repository)

		if tc.want != got {
			t.Fatalf("GitCloneURL(%s, %s, %s) got %s ; want %s", tc.server, tc.owner, tc.repository, got, tc.want)
		}
	}
}
