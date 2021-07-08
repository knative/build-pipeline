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

package main

import (
	"flag"
	"log"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/tektoncd/pipeline/cmd/entrypoint/subcommands"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline"
	"github.com/tektoncd/pipeline/pkg/credentials"
	"github.com/tektoncd/pipeline/pkg/credentials/dockercreds"
	"github.com/tektoncd/pipeline/pkg/credentials/gitcreds"
	"github.com/tektoncd/pipeline/pkg/entrypoint"
	"github.com/tektoncd/pipeline/pkg/termination"
)

var (
	ep                  = flag.String("entrypoint", "", "Original specified entrypoint to execute")
	waitFiles           = flag.String("wait_file", "", "Comma-separated list of paths to wait for")
	waitFileContent     = flag.Bool("wait_file_content", false, "If specified, expect wait_file to have content")
	postFile            = flag.String("post_file", "", "If specified, file to write upon completion")
	terminationPath     = flag.String("termination_path", "/tekton/termination", "If specified, file to write upon termination")
	results             = flag.String("results", "", "If specified, list of file names that might contain task results")
	timeout             = flag.Duration("timeout", time.Duration(0), "If specified, sets timeout for step")
	breakpointOnFailure = flag.Bool("breakpoint_on_failure", false, "If specified, expect steps to not skip on failure")
	onError             = flag.String("on_error", "", "Set to \"continue\" to ignore an error and "+
		"continue when a container terminates with a non-zero exit code. "+
		"Set to \"fail\" to declare a failure with a step error and stop executing the rest of the steps.")
	stepPath = flag.String("step_path", "", "Relative step path, creates the specified path under /tekton/steps/ which "+
		"can be used to store the step metadata e.g. <step-name> in /tekton/steps/<step-name>/")
	stepPathLink = flag.String("step_path_link", "", "Relative step path, creates a symbolic link to the "+
		"specified step path e.g. <step-index> in /tekton/steps/<step-index>")
)

const (
	defaultWaitPollingInterval = time.Second
	breakpointExitSuffix       = ".breakpointexit"
)

func checkForBreakpointOnFailure(e entrypoint.Entrypointer, breakpointExitPostFile string) {
	if e.BreakpointOnFailure {
		if waitErr := e.Waiter.Wait(breakpointExitPostFile, false, false); waitErr != nil {
			log.Println("error occurred while waiting for " + breakpointExitPostFile + " : " + waitErr.Error())
		}
		// get exitcode from .breakpointexit
		exitCode, readErr := e.BreakpointExitCode(breakpointExitPostFile)
		// if readErr exists, the exitcode with default to 0 as we would like
		// to encourage to continue running the next steps in the taskRun
		if readErr != nil {
			log.Println("error occurred while reading breakpoint exit code : " + readErr.Error())
		}
		os.Exit(exitCode)
	}
}

func main() {
	// Add credential flags originally introduced with our legacy credentials helper
	// image (creds-init).
	gitcreds.AddFlags(flag.CommandLine)
	dockercreds.AddFlags(flag.CommandLine)

	flag.Parse()

	if err := subcommands.Process(flag.Args()); err != nil {
		log.Println(err.Error())
		switch err.(type) {
		case subcommands.SubcommandSuccessful:
			return
		default:
			os.Exit(1)
		}
	}

	// Copy credentials we're expecting from the legacy credentials helper (creds-init)
	// from secret volume mounts to /tekton/creds. This is done to support the expansion
	// of a variable, $(credentials.path), that resolves to a single place with all the
	// stored credentials.
	builders := []credentials.Builder{dockercreds.NewBuilder(), gitcreds.NewBuilder()}
	for _, c := range builders {
		if err := c.Write(pipeline.CredsDir); err != nil {
			log.Printf("Error initializing credentials: %s", err)
		}
	}

	e := entrypoint.Entrypointer{
		Entrypoint:          *ep,
		WaitFiles:           strings.Split(*waitFiles, ","),
		WaitFileContent:     *waitFileContent,
		PostFile:            *postFile,
		TerminationPath:     *terminationPath,
		Args:                flag.Args(),
		Waiter:              &realWaiter{waitPollingInterval: defaultWaitPollingInterval, breakpointOnFailure: *breakpointOnFailure},
		Runner:              &realRunner{},
		PostWriter:          &realPostWriter{},
		Results:             strings.Split(*results, ","),
		Timeout:             timeout,
		BreakpointOnFailure: *breakpointOnFailure,
		OnError:             *onError,
		StepPath:            *stepPath,
		StepPathLink:        *stepPathLink,
	}

	// Copy any creds injected by the controller into the $HOME directory of the current
	// user so that they're discoverable by git / ssh.
	if err := credentials.CopyCredsToHome(credentials.CredsInitCredentials); err != nil {
		log.Printf("non-fatal error copying credentials: %q", err)
	}

	if err := e.Go(); err != nil {
		breakpointExitPostFile := e.PostFile + breakpointExitSuffix
		switch t := err.(type) {
		case skipError:
			log.Print("Skipping step because a previous step failed")
			os.Exit(1)
		case termination.MessageLengthError:
			log.Print(err.Error())
			os.Exit(1)
		case *exec.ExitError:
			// Copied from https://stackoverflow.com/questions/10385551/get-exit-code-go
			// This works on both Unix and Windows. Although
			// package syscall is generally platform dependent,
			// WaitStatus is defined for both Unix and Windows and
			// in both cases has an ExitStatus() method with the
			// same signature.
			if status, ok := t.Sys().(syscall.WaitStatus); ok {
				checkForBreakpointOnFailure(e, breakpointExitPostFile)
				// ignore a step error i.e. do not exit if a container terminates with a non-zero exit code when onError is set to "continue"
				if e.OnError != entrypoint.ContinueOnError {
					os.Exit(status.ExitStatus())
				}
			}
			// log and exit only if a step error must cause run failure
			if e.OnError != entrypoint.ContinueOnError {
				log.Fatalf("Error executing command (ExitError): %v", err)
			}
		default:
			checkForBreakpointOnFailure(e, breakpointExitPostFile)
			log.Fatalf("Error executing command: %v", err)
		}
	}
}
