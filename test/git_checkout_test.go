// +build e2e

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

package test

import (
	"context"
	"strings"
	"testing"

	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1alpha1"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	knativetest "knative.dev/pkg/test"
)

const (
	gitSourceResourceName  = "git-source-resource"
	gitTestPipelineRunName = "git-check-pipeline-run"
)

// TestGitPipelineRun is an integration test that will verify the source code
// is either fetched or pulled successfully under different resource
// parameters.
func TestGitPipelineRun(t *testing.T) {
	skipIfExcluded(t)

	for _, tc := range []struct {
		name      string
		repo      string
		revision  string
		refspec   string
		sslVerify string
	}{{
		name:     "tekton @ main",
		repo:     "https://github.com/tektoncd/pipeline",
		revision: "main",
	}, {
		name:     "tekton @ commit",
		repo:     "https://github.com/tektoncd/pipeline",
		revision: "c15aced0e5aaee6456fbe6f7a7e95e0b5b3b2b2f",
	}, {
		name:     "tekton @ release",
		repo:     "https://github.com/tektoncd/pipeline",
		revision: "release-0.1",
	}, {
		name:     "tekton @ tag",
		repo:     "https://github.com/tektoncd/pipeline",
		revision: "v0.1.0",
	}, {
		name:     "tekton @ PR ref",
		repo:     "https://github.com/tektoncd/pipeline",
		revision: "refs/pull/347/head",
	}, {
		name:     "tekton @ main with refspec",
		repo:     "https://github.com/tektoncd/pipeline",
		revision: "main",
		refspec:  "refs/tags/v0.1.0:refs/tags/v0.1.0 refs/heads/main:refs/heads/main",
	}, {
		name:     "tekton @ commit with PR refspec",
		repo:     "https://github.com/tektoncd/pipeline",
		revision: "968d5d37a61bfb85426c885dc1090c1cc4b33436",
		refspec:  "refs/pull/1009/head",
	}, {
		name:     "tekton @ main with PR refspec",
		repo:     "https://github.com/tektoncd/pipeline",
		revision: "main",
		refspec:  "refs/pull/1009/head:refs/heads/main",
	}, {
		name:      "tekton @ main with sslverify=false",
		repo:      "https://github.com/tektoncd/pipeline",
		revision:  "main",
		sslVerify: "false",
	}, {
		name:     "non-master repo with default revision",
		repo:     "https://github.com/tektoncd/results",
		revision: "",
	}, {
		name:     "non-master repo with main revision",
		repo:     "https://github.com/tektoncd/results",
		revision: "main",
	}} {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()
			c, namespace := setup(ctx, t)
			knativetest.CleanupOnInterrupt(func() { tearDown(ctx, t, c, namespace) }, t.Logf)
			defer tearDown(ctx, t, c, namespace)

			t.Logf("Creating Git PipelineResource %s", gitSourceResourceName)
			if _, err := c.PipelineResourceClient.Create(ctx, &v1alpha1.PipelineResource{
				ObjectMeta: metav1.ObjectMeta{Name: gitSourceResourceName},
				Spec: v1alpha1.PipelineResourceSpec{
					Type: v1alpha1.PipelineResourceTypeGit,
					Params: []v1alpha1.ResourceParam{
						{Name: "Url", Value: tc.repo},
						{Name: "Revision", Value: tc.revision},
						{Name: "Refspec", Value: tc.refspec},
						{Name: "sslVerify", Value: tc.sslVerify},
					},
				},
			}, metav1.CreateOptions{}); err != nil {
				t.Fatalf("Failed to create Pipeline Resource `%s`: %s", gitSourceResourceName, err)
			}

			t.Logf("Creating PipelineRun %s", gitTestPipelineRunName)
			if _, err := c.PipelineRunClient.Create(ctx, &v1beta1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{Name: gitTestPipelineRunName},
				Spec: v1beta1.PipelineRunSpec{
					Resources: []v1beta1.PipelineResourceBinding{{
						Name:        "git-repo",
						ResourceRef: &v1beta1.PipelineResourceRef{Name: gitSourceResourceName},
					}},
					PipelineSpec: &v1beta1.PipelineSpec{
						Resources: []v1beta1.PipelineDeclaredResource{{
							Name: "git-repo", Type: v1alpha1.PipelineResourceTypeGit,
						}},
						Tasks: []v1beta1.PipelineTask{{
							Name: "git-check",
							TaskSpec: &v1beta1.EmbeddedTask{TaskSpec: v1beta1.TaskSpec{
								Resources: &v1beta1.TaskResources{
									Inputs: []v1beta1.TaskResource{{ResourceDeclaration: v1beta1.ResourceDeclaration{
										Name: "gitsource", Type: v1alpha1.PipelineResourceTypeGit,
									}}},
								},
								Steps: []v1beta1.Step{{Container: corev1.Container{
									Image: "alpine/git",
									Args:  []string{"--git-dir=/workspace/gitsource/.git", "show"},
								}}},
							}},
							Resources: &v1beta1.PipelineTaskResources{
								Inputs: []v1beta1.PipelineTaskInputResource{{
									Name:     "gitsource",
									Resource: "git-repo",
								}},
							},
						}},
					},
				},
			}, metav1.CreateOptions{}); err != nil {
				t.Fatalf("Failed to create PipelineRun %q: %s", gitTestPipelineRunName, err)
			}

			if err := WaitForPipelineRunState(ctx, c, gitTestPipelineRunName, timeout, PipelineRunSucceed(gitTestPipelineRunName), "PipelineRunCompleted"); err != nil {
				t.Errorf("Error waiting for PipelineRun %s to finish: %s", gitTestPipelineRunName, err)
				t.Fatalf("PipelineRun execution failed")
			}
		})
	}
}

// TestGitPipelineRunFail is a test to ensure that the code extraction from
// github fails as expected when an invalid revision or https proxy is passed
// on the pipelineresource.
func TestGitPipelineRunFail(t *testing.T) {
	for _, tc := range []struct {
		name       string
		revision   string
		httpsproxy string
	}{{
		name:     "invalid revision",
		revision: "Idontexistrabbitmonkeydonkey",
	}, {
		name:       "invalid httpsproxy",
		httpsproxy: "invalid.https.proxy.example.com",
	}} {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()
			c, namespace := setup(ctx, t)
			knativetest.CleanupOnInterrupt(func() { tearDown(ctx, t, c, namespace) }, t.Logf)
			defer tearDown(ctx, t, c, namespace)

			t.Logf("Creating Git PipelineResource %s", gitSourceResourceName)
			if _, err := c.PipelineResourceClient.Create(ctx, &v1alpha1.PipelineResource{
				ObjectMeta: metav1.ObjectMeta{Name: gitSourceResourceName},
				Spec: v1alpha1.PipelineResourceSpec{
					Type: v1alpha1.PipelineResourceTypeGit,
					Params: []v1alpha1.ResourceParam{
						{Name: "Url", Value: "https://github.com/tektoncd/pipeline"},
						{Name: "Revision", Value: tc.revision},
						{Name: "httpsProxy", Value: tc.httpsproxy},
					},
				},
			}, metav1.CreateOptions{}); err != nil {
				t.Fatalf("Failed to create Pipeline Resource `%s`: %s", gitSourceResourceName, err)
			}

			t.Logf("Creating PipelineRun %s", gitTestPipelineRunName)
			if _, err := c.PipelineRunClient.Create(ctx, &v1beta1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{Name: gitTestPipelineRunName},
				Spec: v1beta1.PipelineRunSpec{
					Resources: []v1beta1.PipelineResourceBinding{{
						Name:        "git-repo",
						ResourceRef: &v1beta1.PipelineResourceRef{Name: gitSourceResourceName},
					}},
					PipelineSpec: &v1beta1.PipelineSpec{
						Resources: []v1beta1.PipelineDeclaredResource{{
							Name: "git-repo", Type: v1alpha1.PipelineResourceTypeGit,
						}},
						Tasks: []v1beta1.PipelineTask{{
							Name: "git-check",
							TaskSpec: &v1beta1.EmbeddedTask{TaskSpec: v1beta1.TaskSpec{
								Resources: &v1beta1.TaskResources{
									Inputs: []v1beta1.TaskResource{{ResourceDeclaration: v1beta1.ResourceDeclaration{
										Name: "gitsource", Type: v1alpha1.PipelineResourceTypeGit,
									}}},
								},
								Steps: []v1beta1.Step{{Container: corev1.Container{
									Image: "alpine/git",
									Args:  []string{"--git-dir=/workspace/gitsource/.git", "show"},
								}}},
							}},
							Resources: &v1beta1.PipelineTaskResources{
								Inputs: []v1beta1.PipelineTaskInputResource{{
									Name:     "gitsource",
									Resource: "git-repo",
								}},
							},
						}},
					},
				},
			}, metav1.CreateOptions{}); err != nil {
				t.Fatalf("Failed to create PipelineRun %q: %s", gitTestPipelineRunName, err)
			}

			if err := WaitForPipelineRunState(ctx, c, gitTestPipelineRunName, timeout, PipelineRunSucceed(gitTestPipelineRunName), "PipelineRunCompleted"); err != nil {
				taskruns, err := c.TaskRunClient.List(ctx, metav1.ListOptions{})
				if err != nil {
					t.Errorf("Error getting TaskRun list for PipelineRun %s %s", gitTestPipelineRunName, err)
				}
				for _, tr := range taskruns.Items {
					if tr.Status.PodName != "" {
						p, err := c.KubeClient.CoreV1().Pods(namespace).Get(ctx, tr.Status.PodName, metav1.GetOptions{})
						if err != nil {
							t.Fatalf("Error getting pod `%s` in namespace `%s`", tr.Status.PodName, namespace)
						}

						for _, stat := range p.Status.ContainerStatuses {
							if strings.HasPrefix(stat.Name, "step-git-source-"+gitSourceResourceName) {
								if stat.State.Terminated != nil {
									req := c.KubeClient.CoreV1().Pods(namespace).GetLogs(p.Name, &corev1.PodLogOptions{Container: stat.Name})
									logContent, err := req.Do(ctx).Raw()
									if err != nil {
										t.Fatalf("Error getting pod logs for pod `%s` and container `%s` in namespace `%s`", tr.Status.PodName, stat.Name, namespace)
									}
									// Check for failure messages from fetch and pull in the log file
									if strings.Contains(strings.ToLower(string(logContent)), "couldn't find remote ref idontexistrabbitmonkeydonkey") {
										t.Logf("Found exepected errors when retrieving non-existent git revision")
									} else {
										t.Logf("Container `%s` log File: %s", stat.Name, logContent)
										t.Fatalf("The git code extraction did not fail as expected.  Expected errors not found in log file.")
									}
								}
							}
						}
					}
				}
			} else {
				t.Fatalf("PipelineRun succeeded when should have failed")
			}
		})
	}
}
