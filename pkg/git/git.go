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
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	homedir "github.com/mitchellh/go-homedir"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline"
	"go.uber.org/zap"
)

const (
	sshKnownHostsUserPath          = ".ssh/known_hosts"
	sshMissingKnownHostsSSHCommand = "ssh -o StrictHostKeyChecking=accept-new"
)

var (
	// sshURLRegexFormat matches the url of SSH git repository
	sshURLRegexFormat = regexp.MustCompile(`(ssh://[\w\d\.]+|.+@?.+\..+:)(:[\d]+){0,1}/*(.*)`)
)

func run(logger *zap.SugaredLogger, dir string, args ...string) (string, error) {
	c := exec.Command("git", args...)
	var output bytes.Buffer
	c.Stderr = &output
	c.Stdout = &output
	// This is the optional working directory. If not set, it defaults to the current
	// working directory of the process.
	if dir != "" {
		c.Dir = dir
	}
	if err := c.Run(); err != nil {
		logger.Errorf("Error running git %v: %v\n%v", args, err, output.String())
		return "", err
	}
	return output.String(), nil
}

// FetchSpec describes how to initialize and fetch from a Git repository.
type FetchSpec struct {
	URL                       string
	Revision                  string
	Refspec                   string
	Path                      string
	Dir                       string
	Depth                     uint
	Submodules                bool
	SSLVerify                 bool
	HTTPProxy                 string
	HTTPSProxy                string
	NOProxy                   string
	SparseCheckoutDirectories string
}

// Fetch fetches the specified git repository at the revision into path, using the refspec to fetch if provided.
func Fetch(logger *zap.SugaredLogger, spec FetchSpec) error {
	if err := ensureHomeEnv(logger); err != nil {
		return err
	}
	validateGitAuth(logger, pipeline.CredsDir, spec.URL)

	dir := spec.Dir
	if spec.Path != "" {
		dir = spec.Path
		if _, err := run(logger, dir, "init", dir); err != nil {
			return err
		}
		if err := os.Chdir(dir); err != nil {
			return fmt.Errorf("failed to change directory with path %s; err: %w", dir, err)
		}
	} else if _, err := run(logger, dir, "init"); err != nil {
		return err
	}
	if err := configSparseCheckout(logger, spec); err != nil {
		return err
	}
	trimmedURL := strings.TrimSpace(spec.URL)
	if _, err := run(logger, dir, "remote", "add", "origin", trimmedURL); err != nil {
		return err
	}

	hasKnownHosts, err := userHasKnownHostsFile(logger)
	if err != nil {
		return fmt.Errorf("error checking for known_hosts file: %w", err)
	}
	if !hasKnownHosts {
		if _, err := run(logger, dir, "config", "core.sshCommand", sshMissingKnownHostsSSHCommand); err != nil {
			err = fmt.Errorf("error disabling strict host key checking: %w", err)
			logger.Warnf(err.Error())
			return err
		}
	}
	if _, err := run(logger, dir, "config", "http.sslVerify", strconv.FormatBool(spec.SSLVerify)); err != nil {
		logger.Warnf("Failed to set http.sslVerify in git config: %s", err)
		return err
	}
	if spec.Revision == "" {
		spec.Revision = "HEAD"
		if _, err := run(logger, dir, "symbolic-ref", spec.Revision, "refs/remotes/origin/HEAD"); err != nil {
			return err
		}
	}

	fetchArgs := []string{"fetch"}
	if spec.Submodules {
		fetchArgs = append(fetchArgs, "--recurse-submodules=yes")
	}
	if spec.Depth > 0 {
		fetchArgs = append(fetchArgs, fmt.Sprintf("--depth=%d", spec.Depth))
	}

	// Fetch the revision and verify with FETCH_HEAD
	fetchParam := []string{spec.Revision}
	checkoutParam := "FETCH_HEAD"

	if spec.Refspec != "" {
		// if refspec is specified, fetch the refspec and verify with provided revision
		fetchParam = strings.Split(spec.Refspec, " ")
		checkoutParam = spec.Revision
	}

	// git-init always creates and checks out an empty master branch. When the user requests
	// "master" as the revision, git-fetch will refuse to update the HEAD of the branch it is
	// currently on. The --update-head-ok parameter tells git-fetch that it is ok to update
	// the current (empty) HEAD on initial fetch.
	// The --force parameter tells git-fetch that its ok to update an existing HEAD in a
	// non-fast-forward manner (though this cannot be possible on initial fetch, it can help
	// when the refspec specifies the same destination twice)
	fetchArgs = append(fetchArgs, "origin", "--update-head-ok", "--force")
	fetchArgs = append(fetchArgs, fetchParam...)
	if _, err := run(logger, dir, fetchArgs...); err != nil {
		return fmt.Errorf("failed to fetch %v: %v", fetchParam, err)
	}
	// After performing a fetch, verify that the item to checkout is actually valid
	if _, err := ShowCommit(logger, checkoutParam, dir); err != nil {
		return fmt.Errorf("error parsing %s after fetching refspec %s", checkoutParam, spec.Refspec)
	}

	if _, err := run(logger, dir, "checkout", "-f", checkoutParam); err != nil {
		return fmt.Errorf("failed to checkout %s: %v", checkoutParam, err)
	}

	commit, err := ShowCommit(logger, "HEAD", dir)
	if err != nil {
		return err
	}
	ref, err := showRef(logger, "HEAD", dir)
	if err != nil {
		return err
	}
	logger.Infof("Successfully cloned %s @ %s (%s) in path %s", trimmedURL, commit, ref, dir)
	if spec.Submodules {
		if err := submoduleFetch(logger, spec); err != nil {
			return err
		}
	}
	return nil
}

func ShowCommit(logger *zap.SugaredLogger, revision, path string) (string, error) {
	output, err := run(logger, path, "show", "-q", "--pretty=format:%H", revision)
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(output, "\n"), nil
}

func showRef(logger *zap.SugaredLogger, revision, path string) (string, error) {
	output, err := run(logger, path, "show", "-q", "--pretty=format:%D", revision)
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(output, "\n"), nil
}

func submoduleFetch(logger *zap.SugaredLogger, spec FetchSpec) error {
	if spec.Path != "" {
		if err := os.Chdir(spec.Path); err != nil {
			return fmt.Errorf("failed to change directory with path %s; err: %w", spec.Path, err)
		}
	}
	updateArgs := []string{"submodule", "update", "--recursive", "--init"}
	if spec.Depth > 0 {
		updateArgs = append(updateArgs, fmt.Sprintf("--depth=%d", spec.Depth))
	}
	if _, err := run(logger, spec.Dir, updateArgs...); err != nil {
		return err
	}
	logger.Infof("Successfully initialized and updated submodules in path %s", spec.Dir)
	return nil
}

// ensureHomeEnv works around an issue where ssh doesn't respect the HOME env variable. If HOME is set and
// different from the user's detected home directory then symlink .ssh from the home directory to the HOME env
// var. This way ssh will see the .ssh directory in the user's home directory even though it ignores
// the HOME env var.
func ensureHomeEnv(logger *zap.SugaredLogger) error {
	homepath, err := homedir.Dir()
	if err != nil {
		logger.Errorf("Unexpected error: getting the user home directory: %v", err)
		return err
	}
	homeenv := os.Getenv("HOME")
	euid := os.Geteuid()
	if _, err := os.Stat(filepath.Join(homeenv, ".ssh")); err != nil {
		// There's no $HOME/.ssh directory to access or the user doesn't have permissions
		// to read it, or something else; in any event there's no need to try creating a
		// symlink to it.
		return nil
	}
	if euid == 0 {
		ensureHomeEnvSSHLinkedFromPath(logger, homeenv, "/root")
	} else if homeenv != "" {
		ensureHomeEnvSSHLinkedFromPath(logger, homeenv, homepath)
	}
	return nil
}

func ensureHomeEnvSSHLinkedFromPath(logger *zap.SugaredLogger, homeenv string, homepath string) {
	if filepath.Clean(homeenv) != filepath.Clean(homepath) {
		homeEnvSSH := filepath.Join(homeenv, ".ssh")
		homePathSSH := filepath.Join(homepath, ".ssh")
		if _, err := os.Stat(homePathSSH); os.IsNotExist(err) {
			if err := os.Symlink(homeEnvSSH, homePathSSH); err != nil {
				// Only do a warning, in case we don't have a real home
				// directory writable in our image
				logger.Warnf("Unexpected error: creating symlink: %v", err)
			}
		}
	}
}

func userHasKnownHostsFile(logger *zap.SugaredLogger) (bool, error) {
	homepath, err := homedir.Dir()
	if err != nil {
		logger.Errorf("Unexpected error: getting the user home directory: %v", err)
		return false, err
	}
	f, err := os.Open(filepath.Join(homepath, sshKnownHostsUserPath))
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	f.Close()
	return true, nil
}

func validateGitAuth(logger *zap.SugaredLogger, credsDir, url string) {
	sshCred := true
	if _, err := os.Stat(credsDir + "/.ssh"); os.IsNotExist(err) {
		sshCred = false
	}
	urlSSHFormat := validateGitSSHURLFormat(url)
	if sshCred && !urlSSHFormat {
		logger.Warnf("SSH credentials have been provided but the URL(%q) is not a valid SSH URL. This warning can be safely ignored if the URL is for a public repo or you are using basic auth", url)
	} else if !sshCred && urlSSHFormat {
		logger.Warnf("URL(%q) appears to need SSH authentication but no SSH credentials have been provided", url)
	}
}

// validateGitSSHURLFormat validates the given URL format is SSH or not
func validateGitSSHURLFormat(url string) bool {
	if sshURLRegexFormat.MatchString(url) {
		return true
	}
	return false
}

func configSparseCheckout(logger *zap.SugaredLogger, spec FetchSpec) error {
	if spec.SparseCheckoutDirectories != "" {
		if _, err := run(logger, spec.Dir, "config", "core.sparsecheckout", "true"); err != nil {
			return err
		}

		dirPatterns := strings.Split(spec.SparseCheckoutDirectories, ",")

		cwd, err := os.Getwd()
		if err != nil {
			logger.Errorf("failed to get current directory: %v", err)
			return err
		}
		file, err := os.OpenFile(filepath.Join(cwd, ".git/info/sparse-checkout"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			logger.Errorf("failed to open sparse-checkout file: %v", err)
			return err
		}
		for _, pattern := range dirPatterns {
			if _, err := file.WriteString(pattern + "\n"); err != nil {
				file.Close()
				logger.Errorf("failed to write to sparse-checkout file: %v", err)
				return err
			}
		}
		if err := file.Close(); err != nil {
			logger.Errorf("failed to close sparse-checkout file: %v", err)
			return err
		}
	}
	return nil
}
