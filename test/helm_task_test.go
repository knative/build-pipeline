// +build e2e

/*
Copyright 2018 Knative Authors LLC
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
	"fmt"
	"net/http"
	"os"
	"testing"

	buildv1alpha1 "github.com/knative/build/pkg/apis/build/v1alpha1"
	duckv1alpha1 "github.com/knative/pkg/apis/duck/v1alpha1"
	knativetest "github.com/knative/pkg/test"
	"github.com/knative/pkg/test/logging"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/knative/build-pipeline/pkg/apis/pipeline/v1alpha1"
)

const (
	sourceResourceName        = "go-helloworld-git"
	sourceImageName           = "go-helloworld-image"
	createImageTaskName       = "create-image-task"
	helmDeployTaskName        = "helm-deploy-task"
	helmDeployPipelineName    = "helm-deploy-pipeline"
	helmDeployPipelineRunName = "helm-deploy-pipeline-run"
	helmDeployServiceName     = "gohelloworld-chart"
)

var imageName string

// TestHelmDeployPipelineRun is an integration test that will verify a pipeline build an image
// and then using helm to deploy it
func TestHelmDeployPipelineRun(t *testing.T) {
	logger := logging.GetContextLogger(t.Name())
	c, namespace := setup(t, logger)
	setupClusterBindingForHelm(c, t, namespace)

	knativetest.CleanupOnInterrupt(func() { tearDown(logger, c.KubeClient, namespace) }, logger)
	defer tearDown(logger, c.KubeClient, namespace)

	logger.Infof("Creating Git PipelineResource %s", sourceResourceName)
	if _, err := c.PipelineResourceClient.Create(getGoHelloworldGitResource(namespace)); err != nil {
		t.Fatalf("Failed to create Pipeline Resource `%s`: %s", sourceResourceName, err)
	}

	logger.Infof("Creating Task %s", createImageTaskName)
	if _, err := c.TaskClient.Create(getCreateImageTask(namespace, t)); err != nil {
		t.Fatalf("Failed to create Task `%s`: %s", createImageTaskName, err)
	}

	logger.Infof("Creating Task %s", helmDeployTaskName)
	if _, err := c.TaskClient.Create(getHelmDeployTask(namespace)); err != nil {
		t.Fatalf("Failed to create Task `%s`: %s", helmDeployTaskName, err)
	}

	logger.Infof("Creating Pipeline %s", helmDeployPipelineName)
	if _, err := c.PipelineClient.Create(getelmDeployPipeline(namespace)); err != nil {
		t.Fatalf("Failed to create Pipeline `%s`: %s", helmDeployPipelineName, err)
	}

	logger.Infof("Creating PipelineRun %s", helmDeployPipelineRunName)
	if _, err := c.PipelineRunClient.Create(getelmDeployPipelineRun(namespace)); err != nil {
		t.Fatalf("Failed to create Pipeline `%s`: %s", helmDeployPipelineRunName, err)
	}

	// Verify status of PipelineRun (wait for it)
	if err := WaitForPipelineRunState(c, helmDeployPipelineRunName, func(pr *v1alpha1.PipelineRun) (bool, error) {
		c := pr.Status.GetCondition(duckv1alpha1.ConditionSucceeded)
		if c != nil {
			if c.Status == corev1.ConditionTrue {
				return true, nil
			} else if c.Status == corev1.ConditionFalse {
				return true, fmt.Errorf("pipeline run %s failed!", helmDeployPipelineRunName)
			}
		}
		return false, nil
	}, "PipelineRunCompleted"); err != nil {
		t.Errorf("Error waiting for PipelineRun %s to finish: %s", helmDeployPipelineRunName, err)
	}

	k8sService, err := c.KubeClient.Kube.CoreV1().Services(namespace).Get(helmDeployServiceName, metav1.GetOptions{})
	if err != nil {
		t.Errorf("Error getting service at %s %s", helmDeployServiceName, err)
	}
	var serviceIp string
	ingress := k8sService.Status.LoadBalancer.Ingress
	if len(ingress) > 0 {
		serviceIp = ingress[0].IP
	}

	resp, err := http.Get(fmt.Sprintf("http://%s:8080", serviceIp))
	if err != nil {
		t.Errorf("Error reaching service at http://%s:8080 %s", serviceIp, err)
	}
	if resp != nil && resp.StatusCode != http.StatusOK {
		t.Errorf("Error from service at http://%s:8080 %s", serviceIp, err)
	}
}

func getGoHelloworldGitResource(namespace string) *v1alpha1.PipelineResource {
	return &v1alpha1.PipelineResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sourceResourceName,
			Namespace: namespace,
		},
		Spec: v1alpha1.PipelineResourceSpec{
			Type: v1alpha1.PipelineResourceTypeGit,
			Params: []v1alpha1.Param{
				v1alpha1.Param{
					Name:  "Url",
					Value: "https://github.com/pivotal-nader-ziada/gohelloworld",
				},
			},
		},
	}
}

func getCreateImageTask(namespace string, t *testing.T) *v1alpha1.Task {
	// according to knative/test-infra readme (https://github.com/knative/test-infra/blob/13055d769cc5e1756e605fcb3bcc1c25376699f1/scripts/README.md)
	// the KO_DOCKER_REPO will be set with according to the porject where the cluster is created
	// it is used here to dunamically get the docker registery to push the image to
	dockerRepo := os.Getenv("KO_DOCKER_REPO")
	if dockerRepo == "" {
		t.Fatalf("KO_DOCKER_REPO env variable is required")
	}

	imageName = fmt.Sprintf("%s/%s", dockerRepo, sourceImageName)

	return &v1alpha1.Task{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      createImageTaskName,
		},
		Spec: v1alpha1.TaskSpec{
			Inputs: &v1alpha1.Inputs{
				Resources: []v1alpha1.TaskResource{
					v1alpha1.TaskResource{
						Name: sourceResourceName,
						Type: v1alpha1.PipelineResourceTypeGit,
					},
				},
			},
			BuildSpec: &buildv1alpha1.BuildSpec{
				Steps: []corev1.Container{{
					Name:  "kaniko",
					Image: "gcr.io/kaniko-project/executor",
					Args: []string{"--dockerfile=/workspace/Dockerfile",
						fmt.Sprintf("--destination=%s", imageName),
					},
				}},
			},
		},
	}
}

func getHelmDeployTask(namespace string) *v1alpha1.Task {
	return &v1alpha1.Task{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      helmDeployTaskName,
		},
		Spec: v1alpha1.TaskSpec{
			Inputs: &v1alpha1.Inputs{
				Resources: []v1alpha1.TaskResource{
					v1alpha1.TaskResource{
						Name: sourceResourceName,
						Type: v1alpha1.PipelineResourceTypeGit,
					},
				},
				Params: []v1alpha1.TaskParam{{
					Name: "pathToHelmCharts",
				}, {
					Name: "image",
				}, {
					Name: "chartname",
				}},
			},
			BuildSpec: &buildv1alpha1.BuildSpec{
				Steps: []corev1.Container{{
					Name:  "helm-init",
					Image: "alpine/helm",
					Args:  []string{"init"},
				},
					{
						Name:  "helm-cleanup", //for local clusters, clean up from previous runs
						Image: "alpine/helm",
						Command: []string{"/bin/sh",
							"-c",
							"helm ls --short --all | xargs -n1 helm del --purge",
						},
					},
					{
						Name:  "helm-deploy",
						Image: "alpine/helm",
						Args: []string{"install",
							"--debug",
							"--name=${inputs.params.chartname}",
							"${inputs.params.pathToHelmCharts}",
							"--set",
							"image.repository=${inputs.params.image}",
						},
					}},
			},
		},
	}
}

func getelmDeployPipeline(namespace string) *v1alpha1.Pipeline {
	return &v1alpha1.Pipeline{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      helmDeployPipelineName,
		},
		Spec: v1alpha1.PipelineSpec{
			Tasks: []v1alpha1.PipelineTask{
				v1alpha1.PipelineTask{
					Name: "push-image",
					TaskRef: v1alpha1.TaskRef{
						Name: createImageTaskName,
					},
					InputSourceBindings: []v1alpha1.SourceBinding{{
						Name: "some-name",
						Key:  sourceResourceName,
						ResourceRef: v1alpha1.PipelineResourceRef{
							Name: sourceResourceName,
						},
					}},
				},
				v1alpha1.PipelineTask{
					Name: "helm-deploy",
					TaskRef: v1alpha1.TaskRef{
						Name: helmDeployTaskName,
					},
					InputSourceBindings: []v1alpha1.SourceBinding{{
						Name: "some-other-name",
						Key:  sourceResourceName,
						ResourceRef: v1alpha1.PipelineResourceRef{
							Name: sourceResourceName,
						},
					}},
					Params: []v1alpha1.Param{{
						Name:  "pathToHelmCharts",
						Value: "/workspace/gohelloworld-chart",
					}, {
						Name:  "chartname",
						Value: "gohelloworld",
					}, {
						Name:  "image",
						Value: imageName,
					}},
				},
			},
		},
	}
}

func getelmDeployPipelineRun(namespace string) *v1alpha1.PipelineRun {
	return &v1alpha1.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      helmDeployPipelineRunName,
		},
		Spec: v1alpha1.PipelineRunSpec{
			PipelineRef: v1alpha1.PipelineRef{
				Name: helmDeployPipelineName,
			},
			PipelineTriggerRef: v1alpha1.PipelineTriggerRef{
				Type: v1alpha1.PipelineTriggerTypeManual,
			},
		},
	}
}

func setupClusterBindingForHelm(c *clients, t *testing.T, namespace string) {
	defaultClusterRoleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: AppendRandomString("default-tiller"),
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "cluster-admin",
		},
		Subjects: []rbacv1.Subject{{
			Kind:      "ServiceAccount",
			Name:      "default",
			Namespace: namespace,
		}},
	}

	if _, err := c.KubeClient.Kube.RbacV1beta1().ClusterRoleBindings().Create(defaultClusterRoleBinding); err != nil {
		t.Fatalf("Failed to create default Service account for Helm in namespace: %s - %s", namespace, err)
	}
}
