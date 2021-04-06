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
	"reflect"
	"testing"
)

func TestParseGitURI(t *testing.T) {
	testCases := []struct {
		text       string
		want       *git.GitURI
		wantErr    bool
		wantString string
	}{
		{
			text: "myowner/myrepo/task.yaml@v1",
			want: &git.GitURI{
				CloneURL:   "https://github.com/myowner/myrepo.git",
				Server:     "https://github.com",
				Owner:      "myowner",
				Repository: "myrepo",
				Path:       "task.yaml",
				SHA:        "v1",
			},
			wantString: "https://github.com/myowner/myrepo.git/task.yaml@v1",
		},
		{
			text: "myowner/myrepo/myfile.yaml@v1",
			want: &git.GitURI{
				CloneURL:   "https://github.com/myowner/myrepo.git",
				Server:     "https://github.com",
				Owner:      "myowner",
				Repository: "myrepo",
				Path:       "myfile.yaml",
				SHA:        "v1",
			},
			wantString: "https://github.com/myowner/myrepo.git/myfile.yaml@v1",
		},
		{
			text: "myowner/myrepo/javascript/pullrequest.yaml@v1",
			want: &git.GitURI{
				CloneURL:   "https://github.com/myowner/myrepo.git",
				Server:     "https://github.com",
				Owner:      "myowner",
				Repository: "myrepo",
				Path:       "javascript/pullrequest.yaml",
				SHA:        "v1",
			},
			wantString: "https://github.com/myowner/myrepo.git/javascript/pullrequest.yaml@v1",
		},
		{
			text:    "foo.yaml",
			wantErr: true,
		},
		{
			text:    "foo/bar.yaml",
			wantErr: true,
		},
		{
			text: "foo/bar/thingy.yaml",
			want: &git.GitURI{
				CloneURL:   "https://github.com/foo/bar.git",
				Server:     "https://github.com",
				Owner:      "foo",
				Repository: "bar",
				Path:       "thingy.yaml",
				SHA:        "",
			},
			wantString: "https://github.com/foo/bar.git/thingy.yaml",
		},
		{
			text: "https://mybitbucket.com/scm/myowner/myrepo.git/thingy.yaml",
			want: &git.GitURI{
				CloneURL: "https://mybitbucket.com/scm/myowner/myrepo.git",
				Path:     "thingy.yaml",
			},
			wantString: "https://mybitbucket.com/scm/myowner/myrepo.git/thingy.yaml",
		},
		{
			text: "https://mybitbucket.com/scm/myowner/myrepo.git/thingy.yaml@mybranch",
			want: &git.GitURI{
				CloneURL: "https://mybitbucket.com/scm/myowner/myrepo.git",
				Path:     "thingy.yaml",
				SHA:      "mybranch",
			},
			wantString: "https://mybitbucket.com/scm/myowner/myrepo.git/thingy.yaml@mybranch",
		},
		{
			text: "git@github.com:bar/foo.git/thingy.yaml@mybranch",
			want: &git.GitURI{
				CloneURL: "git@github.com:bar/foo.git",
				Path:     "thingy.yaml",
				SHA:      "mybranch",
			},
			wantString: "git@github.com:bar/foo.git/thingy.yaml@mybranch",
		},
		{
			text: "git://host.xz/org/repo.git/some/path.yaml@v1.2.3",
			want: &git.GitURI{
				CloneURL: "git://host.xz/org/repo.git",
				Path:     "some/path.yaml",
				SHA:      "v1.2.3",
			},
			wantString: "git://host.xz/org/repo.git/some/path.yaml@v1.2.3",
		},
	}

	for _, tc := range testCases {
		text := tc.text
		got, err := git.ParseGitURI(text)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("should have failed to parse %s", text)
			} else {
				t.Logf("parsing %s got want error: %s\n", text, err.Error())
			}
		} else {
			if err != nil {
				t.Fatalf("failed to parse %s", text)
			}
			if !reflect.DeepEqual(tc.want, got) {
				t.Fatalf("ParseGitURI(%s) got %#v ; want %#v", text, got, tc.want)
			}
			if tc.want != nil {
				gotString := tc.want.String()
				wantString := tc.wantString
				if wantString == "" {
					wantString = text
				}

				if gotString != wantString {
					t.Fatalf("ParseGitURI(%s).String() got %s ; want %s", text, gotString, tc.wantString)
				}
			}
		}
	}
}
