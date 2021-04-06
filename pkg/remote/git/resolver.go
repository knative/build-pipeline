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

package git

import (
	"github.com/pkg/errors"
	gitclient "github.com/tektoncd/pipeline/pkg/git"
	"github.com/tektoncd/pipeline/pkg/remote"
	"github.com/tektoncd/pipeline/pkg/remote/file"
	"go.uber.org/zap"
	"io/ioutil"
	"k8s.io/apimachinery/pkg/runtime"
	"os"
)

// Resolver implements the Resolver interface using git.
type Resolver struct {
	options gitclient.FetchSpec
	logger  *zap.SugaredLogger
}

// NewResolver creates a git resolver
func NewResolver(logger *zap.SugaredLogger, opts gitclient.FetchSpec) remote.Resolver {
	return &Resolver{logger: logger, options: opts}
}

func (o *Resolver) List() ([]remote.ResolvedObject, error) {
	return nil, nil
}

func (o *Resolver) Get(kind, name string) (runtime.Object, error) {
	dir, err := ioutil.TempDir("", "git-remote-")
	defer os.RemoveAll(dir)

	gitURI, err := ParseGitURI(name)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse git URI: %s", name)
	}
	if gitURI == nil {
		return nil, errors.Errorf("could not parse git URI: %s", name)
	}
	if gitURI.CloneURL == "" {
		return nil, errors.Errorf("no git clone URI for git: %s", name)
	}

	// lets populate the spec making sure we clear anything from the options
	// that are not generic git settings
	spec := o.options
	spec.Refspec = ""
	spec.Path = ""
	spec.Dir = dir
	spec.Revision = gitURI.SHA

	// lets clear if HEAD so that we setup the remote symbolic-ref
	if spec.Revision == "HEAD" {
		spec.Revision = ""
	}
	spec.URL = gitURI.CloneURL
	path := gitURI.Path

	err = gitclient.Fetch(o.logger, spec)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to clone ")
	}

	r := file.NewResolver(dir)
	return r.Get(kind, path)
}
