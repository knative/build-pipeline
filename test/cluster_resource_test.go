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
	"testing"

	"github.com/knative/build-pipeline/pkg/apis/pipeline/v1alpha1"
	duckv1alpha1 "github.com/knative/pkg/apis/duck/v1alpha1"
	knativetest "github.com/knative/pkg/test"
	"github.com/knative/pkg/test/logging"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestClusterResource(t *testing.T) {
	secretName := "hw-secret"
	configName := "hw-config"
	resourceName := "helloworld-cluster"
	taskName := "helloworld-cluster-task"
	taskRunName := "helloworld-cluster-taskrun"

	logger := logging.GetContextLogger(t.Name())
	c, namespace := setup(t, logger)

	knativetest.CleanupOnInterrupt(func() { tearDown(t, logger, c, namespace) }, logger)
	defer tearDown(t, logger, c, namespace)

	logger.Infof("Creating secret %s", secretName)
	if _, err := c.KubeClient.Kube.CoreV1().Secrets(namespace).Create(getClusterResourceTaskSecret(namespace, secretName)); err != nil {
		t.Fatalf("Failed to create Secret `%s`: %s", secretName, err)
	}

	logger.Infof("Creating configMap %s", configName)
	if _, err := c.KubeClient.Kube.CoreV1().ConfigMaps(namespace).Create(getClusterConfigMap(namespace, configName)); err != nil {
		t.Fatalf("Failed to create configMap `%s`: %s", configName, err)
	}

	logger.Infof("Creating cluster PipelineResource %s", resourceName)
	if _, err := c.PipelineResourceClient.Create(getClusterResource(namespace, resourceName, secretName)); err != nil {
		t.Fatalf("Failed to create cluster Pipeline Resource `%s`: %s", resourceName, err)
	}

	logger.Infof("Creating Task %s", taskName)
	if _, err := c.TaskClient.Create(getClusterResourceTask(namespace, taskName, resourceName, configName)); err != nil {
		t.Fatalf("Failed to create Task `%s`: %s", taskName, err)
	}

	logger.Infof("Creating TaskRun %s", taskRunName)
	if _, err := c.TaskRunClient.Create(getClusterResourceTaskRun(namespace, taskRunName, taskName, resourceName)); err != nil {
		t.Fatalf("Failed to create Taskrun `%s`: %s", taskRunName, err)
	}

	// Verify status of TaskRun (wait for it)
	if err := WaitForTaskRunState(c, taskRunName, func(tr *v1alpha1.TaskRun) (bool, error) {
		c := tr.Status.GetCondition(duckv1alpha1.ConditionSucceeded)
		if c != nil {
			if c.Status == corev1.ConditionTrue {
				return true, nil
			} else if c.Status == corev1.ConditionFalse {
				return true, fmt.Errorf("task run %s failed", taskRunName)
			}
		}
		return false, nil
	}, "TaskRunCompleted"); err != nil {
		t.Errorf("Error waiting for TaskRun %s to finish: %s", taskRunName, err)
	}
}

func getClusterResource(namespace, name, sname string) *v1alpha1.PipelineResource {
	return &v1alpha1.PipelineResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: v1alpha1.PipelineResourceSpec{
			Type: v1alpha1.PipelineResourceTypeCluster,
			Params: []v1alpha1.Param{{
				Name:  "Url",
				Value: "https://1.1.1.1",
			}, {
				Name:  "username",
				Value: "test-user",
			}, {
				Name:  "password",
				Value: "test-password",
			}},
			SecretParams: []v1alpha1.SecretParam{{
				FieldName:  "cadata",
				SecretKey:  "cadatakey",
				SecretName: sname,
			}, {
				FieldName:  "token",
				SecretKey:  "tokenkey",
				SecretName: sname,
			}},
		},
	}
}

func getClusterResourceTaskSecret(namespace, name string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"cadatakey": []byte("Y2EtY2VydAo="), //ca-cert
			"tokenkey":  []byte("dG9rZW4K"),     //token
		},
	}
}

func getClusterResourceTask(namespace, name, resName, configName string) *v1alpha1.Task {
	return &v1alpha1.Task{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Spec: v1alpha1.TaskSpec{
			Inputs: &v1alpha1.Inputs{
				Resources: []v1alpha1.TaskResource{
					v1alpha1.TaskResource{
						Name: "target-cluster",
						Type: v1alpha1.PipelineResourceTypeCluster,
					},
				},
				Params: []v1alpha1.TaskParam{},
			},
			Volumes: []corev1.Volume{{
				Name: "config-vol",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: configName,
						},
					},
				},
			}},
			Steps: []corev1.Container{{
				Name:    "check-file-existence",
				Image:   "ubuntu",
				Command: []string{"cat"},
				Args:    []string{"/workspace/helloworld-cluster/kubeconfig"},
			}, {
				Name:    "check-config-data",
				Image:   "ubuntu",
				Command: []string{"cat"},
				Args:    []string{"/config/test.data"},
				VolumeMounts: []corev1.VolumeMount{{
					Name:      "config-vol",
					MountPath: "/config",
				}},
			}, {
				Name:    "check-contents",
				Image:   "ubuntu",
				Command: []string{"bash"},
				Args:    []string{"-c", "cmp -b /workspace/helloworld-cluster/kubeconfig /config/test.data"},
				VolumeMounts: []corev1.VolumeMount{{
					Name:      "config-vol",
					MountPath: "/config",
				}},
			}},
		},
	}
}

func getClusterResourceTaskRun(namespace, name, taskName, resName string) *v1alpha1.TaskRun {
	return &v1alpha1.TaskRun{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Spec: v1alpha1.TaskRunSpec{
			TaskRef: v1alpha1.TaskRef{
				Name: taskName,
			},
			Trigger: v1alpha1.TaskTrigger{
				TriggerRef: v1alpha1.TaskTriggerRef{
					Type: v1alpha1.TaskTriggerTypeManual,
				},
			},
			Inputs: v1alpha1.TaskRunInputs{
				Resources: []v1alpha1.TaskRunResourceVersion{
					{
						Name: "target-cluster",
						ResourceRef: v1alpha1.PipelineResourceRef{
							Name: resName,
						},
					},
				},
			},
		},
	}
}

func getClusterConfigMap(namespace, name string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Data: map[string]string{
			"test.data": `apiVersion: v1
clusters:
- cluster:
    certificate-authority-data: WTJFdFkyVnlkQW89
    server: https://1.1.1.1
  name: helloworld-cluster
contexts:
- context:
    cluster: helloworld-cluster
    user: test-user
  name: helloworld-cluster
current-context: helloworld-cluster
kind: Config
preferences: {}
users:
- name: test-user
  user:
    token: dG9rZW4K
`,
		},
	}
}
