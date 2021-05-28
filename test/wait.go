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

/*
Poll Pipeline resources

After creating Pipeline resources or making changes to them, you will need to
wait for the system to realize those changes. You can use polling methods to
check the resources reach the desired state.

The WaitFor* functions use the kubernetes
wait package (https://godoc.org/k8s.io/apimachinery/pkg/util/wait). To poll
they use
PollImmediate (https://godoc.org/k8s.io/apimachinery/pkg/util/wait#PollImmediate)
and the return values of the function you provide behave the same as
ConditionFunc (https://godoc.org/k8s.io/apimachinery/pkg/util/wait#ConditionFunc):
a boolean to indicate if the function should stop or continue polling, and an
error to indicate if there has been an error.


For example, you can poll a TaskRun object to wait for it to have a Status.Condition:

	err = WaitForTaskRunState(c, hwTaskRunName, func(tr *v1alpha1.TaskRun) (bool, error) {
		if len(tr.Status.Conditions) > 0 {
			return true, nil
		}
		return false, nil
	}, "TaskRunHasCondition")

*/
package test

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"go.opencensus.io/trace"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"knative.dev/pkg/apis"
)

const (
	interval = 1 * time.Second
	timeout  = 10 * time.Minute
)

// ConditionAccessorFn is a condition function used polling functions
type ConditionAccessorFn func(ca apis.ConditionAccessor) (bool, error)

func pollImmediateWithContext(ctx context.Context, fn func() (bool, error)) error {
	return wait.PollImmediate(interval, timeout, func() (bool, error) {
		select {
		case <-ctx.Done():
			return true, ctx.Err()
		default:
		}
		return fn()
	})
}

// WaitForTaskRunState polls the status of the TaskRun called name from client every
// interval until inState returns `true` indicating it is done, returns an
// error or timeout. desc will be used to name the metric that is emitted to
// track how long it took for name to get into the state checked by inState.
func WaitForTaskRunState(ctx context.Context, c *clients, name string, inState ConditionAccessorFn, desc string) error {
	metricName := fmt.Sprintf("WaitForTaskRunState/%s/%s", name, desc)
	_, span := trace.StartSpan(context.Background(), metricName)
	defer span.End()

	return pollImmediateWithContext(ctx, func() (bool, error) {
		r, err := c.TaskRunClient.Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return true, err
		}
		return inState(&r.Status)
	})
}

// WaitForDeploymentState polls the status of the Deployment called name
// from client every interval until inState returns `true` indicating it is done,
// returns an  error on timeout. desc will be used to name the metric that is emitted to
// track how long it took for name to get into the state checked by inState.
func WaitForDeploymentState(ctx context.Context, c *clients, name string, namespace string, inState func(d *appsv1.Deployment) (bool, error), desc string) error {
	metricName := fmt.Sprintf("WaitForDeploymentState/%s/%s", name, desc)
	_, span := trace.StartSpan(context.Background(), metricName)
	defer span.End()

	return pollImmediateWithContext(ctx, func() (bool, error) {
		d, err := c.KubeClient.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return true, err
		}
		return inState(d)
	})
}

// WaitForPodState polls the status of the Pod called name from client every
// interval until inState returns `true` indicating it is done, returns an
// error or timeout. desc will be used to name the metric that is emitted to
// track how long it took for name to get into the state checked by inState.
func WaitForPodState(ctx context.Context, c *clients, name string, namespace string, inState func(r *corev1.Pod) (bool, error), desc string) error {
	metricName := fmt.Sprintf("WaitForPodState/%s/%s", name, desc)
	_, span := trace.StartSpan(context.Background(), metricName)
	defer span.End()

	return pollImmediateWithContext(ctx, func() (bool, error) {
		r, err := c.KubeClient.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return true, err
		}
		return inState(r)
	})
}

// WaitForPipelineRunState polls the status of the PipelineRun called name from client every
// interval until inState returns `true` indicating it is done, returns an
// error or timeout. desc will be used to name the metric that is emitted to
// track how long it took for name to get into the state checked by inState.
func WaitForPipelineRunState(ctx context.Context, c *clients, name string, polltimeout time.Duration, inState ConditionAccessorFn, desc string) error {
	metricName := fmt.Sprintf("WaitForPipelineRunState/%s/%s", name, desc)
	_, span := trace.StartSpan(context.Background(), metricName)
	defer span.End()

	ctx, cancel := context.WithTimeout(ctx, polltimeout)
	defer cancel()
	return pollImmediateWithContext(ctx, func() (bool, error) {
		r, err := c.PipelineRunClient.Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return true, err
		}
		return inState(&r.Status)
	})
}

// WaitForServiceExternalIPState polls the status of the a k8s Service called name from client every
// interval until an external ip is assigned indicating it is done, returns an
// error or timeout. desc will be used to name the metric that is emitted to
// track how long it took for name to get into the state checked by inState.
func WaitForServiceExternalIPState(ctx context.Context, c *clients, namespace, name string, inState func(s *corev1.Service) (bool, error), desc string) error {
	metricName := fmt.Sprintf("WaitForServiceExternalIPState/%s/%s", name, desc)
	_, span := trace.StartSpan(context.Background(), metricName)
	defer span.End()

	return pollImmediateWithContext(ctx, func() (bool, error) {
		r, err := c.KubeClient.CoreV1().Services(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return true, err
		}
		return inState(r)
	})
}

// Succeed provides a poll condition function that checks if the ConditionAccessor
// resource has successfully completed or not.
func Succeed(name string) ConditionAccessorFn {
	return func(ca apis.ConditionAccessor) (bool, error) {
		c := ca.GetCondition(apis.ConditionSucceeded)
		if c != nil {
			if c.Status == corev1.ConditionTrue {
				return true, nil
			} else if c.Status == corev1.ConditionFalse {
				return true, fmt.Errorf("%q failed", name)
			}
		}
		return false, nil
	}
}

// Failed provides a poll condition function that checks if the ConditionAccessor
// resource has failed or not.
func Failed(name string) ConditionAccessorFn {
	return func(ca apis.ConditionAccessor) (bool, error) {
		c := ca.GetCondition(apis.ConditionSucceeded)
		if c != nil {
			if c.Status == corev1.ConditionTrue {
				return true, fmt.Errorf("%q succeeded", name)
			} else if c.Status == corev1.ConditionFalse {
				return true, nil
			}
		}
		return false, nil
	}
}

// FailedWithReason provides a poll function that checks if the ConditionAccessor
// resource has failed with the TimeoudOut reason
func FailedWithReason(reason, name string) ConditionAccessorFn {
	return func(ca apis.ConditionAccessor) (bool, error) {
		c := ca.GetCondition(apis.ConditionSucceeded)
		if c != nil {
			if c.Status == corev1.ConditionFalse {
				if c.Reason == reason {
					return true, nil
				}
				return true, fmt.Errorf("%q completed with the wrong reason: %s (message: %s)", name, c.Reason, c.Message)
			} else if c.Status == corev1.ConditionTrue {
				return true, fmt.Errorf("%q completed successfully, should have been failed with reason %q", name, reason)
			}
		}
		return false, nil
	}
}

// FailedWithMessage provides a poll function that checks if the ConditionAccessor
// resource has failed with the TimeoudOut reason
func FailedWithMessage(message, name string) ConditionAccessorFn {
	return func(ca apis.ConditionAccessor) (bool, error) {
		c := ca.GetCondition(apis.ConditionSucceeded)
		if c != nil {
			if c.Status == corev1.ConditionFalse {
				if strings.Contains(c.Message, message) {
					return true, nil
				}
				return true, fmt.Errorf("%q completed with the wrong message: %s", name, c.Message)
			} else if c.Status == corev1.ConditionTrue {
				return true, fmt.Errorf("%q completed successfully, should have been failed with message %q", name, message)
			}
		}
		return false, nil
	}
}

// Running provides a poll condition function that checks if the ConditionAccessor
// resource is currently running.
func Running(name string) ConditionAccessorFn {
	return func(ca apis.ConditionAccessor) (bool, error) {
		c := ca.GetCondition(apis.ConditionSucceeded)
		if c != nil {
			if c.Status == corev1.ConditionTrue || c.Status == corev1.ConditionFalse {
				return true, fmt.Errorf(`%q already finished`, name)
			} else if c.Status == corev1.ConditionUnknown && (c.Reason == "Running" || c.Reason == "Pending") {
				return true, nil
			}
		}
		return false, nil
	}
}

// TaskRunSucceed provides a poll condition function that checks if the TaskRun
// has successfully completed.
func TaskRunSucceed(name string) ConditionAccessorFn {
	return Succeed(name)
}

// TaskRunFailed provides a poll condition function that checks if the TaskRun
// has failed.
func TaskRunFailed(name string) ConditionAccessorFn {
	return Failed(name)
}

// PipelineRunSucceed provides a poll condition function that checks if the PipelineRun
// has successfully completed.
func PipelineRunSucceed(name string) ConditionAccessorFn {
	return Succeed(name)
}

// PipelineRunFailed provides a poll condition function that checks if the PipelineRun
// has failed.
func PipelineRunFailed(name string) ConditionAccessorFn {
	return Failed(name)
}

// PipelineRunPending provides a poll condition function that checks if the PipelineRun
// has been marked pending by the Tekton controller.
func PipelineRunPending(name string) ConditionAccessorFn {
	running := Running(name)

	return func(ca apis.ConditionAccessor) (bool, error) {
		c := ca.GetCondition(apis.ConditionSucceeded)
		if c != nil {
			if c.Status == corev1.ConditionUnknown && c.Reason == string(v1beta1.PipelineRunReasonPending) {
				return true, nil
			}
		}
		status, err := running(ca)
		if status {
			reason := ""
			// c _should_ never be nil if we get here, but we have this check just in case.
			if c != nil {
				reason = c.Reason
			}
			return false, fmt.Errorf("status should be %s, but it is %s", v1beta1.PipelineRunReasonPending, reason)
		}
		return status, err
	}
}

// Chain allows multiple ConditionAccessorFns to be chained together, checking the condition of each in order.
func Chain(fns ...ConditionAccessorFn) ConditionAccessorFn {
	return func(ca apis.ConditionAccessor) (bool, error) {
		for _, fn := range fns {
			status, err := fn(ca)
			if err != nil || !status {
				return status, err
			}
		}
		return true, nil
	}
}
