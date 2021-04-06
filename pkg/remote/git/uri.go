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

package git

import (
	"github.com/pkg/errors"
	"strings"
)

// GitURI for parsing the git URIs in uses
type GitURI struct {
	CloneURL   string
	Server     string
	Owner      string
	Repository string
	Path       string
	SHA        string
}

// ParseGitURI parses git source URIs or returns nil if its not a valid URI
//
// handles strings of the form "owner/repository(/path)@sha"
func ParseGitURI(text string) (*GitURI, error) {
	u := &GitURI{}
	parts := strings.SplitN(text, ".git/", 2)
	if len(parts) == 2 {
		u.CloneURL = parts[0] + ".git"
		u.Path = u.trimShaFromPath(parts[1])
	} else {
		if strings.Contains(text, ":") {
			return nil, errors.Errorf("invalid format of git URI: %s if not using github.com you need to separate the git URL from the path via '.git/' before the path", text)
		}

		// lets assume the github notation: `owner/repository/path[@version]`
		parts := strings.SplitN(text, "/", 3)
		if len(parts) < 3 {
			return nil, errors.Errorf("expecting github.com format 'owner/repository/path[@sha]' but got git URI %s", text)
		}
		u.Server = "https://github.com"
		u.Owner = parts[0]
		u.Repository = parts[1]
		u.Path = u.trimShaFromPath(parts[2])
		u.CloneURL = GitCloneURL(u.Server, u.Owner, u.Repository)
	}
	return u, nil
}

func (u *GitURI) trimShaFromPath(text string) string {
	idx := strings.Index(text, "@")
	if idx < 0 {
		return text
	}
	if idx+1 < len(text) {
		u.SHA = text[idx+1:]
	}
	return text[0:idx]
}

// String returns the string representation of the git URI
func (u *GitURI) String() string {
	path := u.CloneURL
	if u.Path != "" {
		if !strings.HasPrefix(u.Path, "/") {
			path += "/"
		}
		path += u.Path
	}
	path = strings.TrimSuffix(path, "/")
	if u.SHA == "" {
		return path
	}
	return path + "@" + u.SHA
}
