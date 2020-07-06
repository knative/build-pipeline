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

package validation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1alpha1"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"github.com/tektoncd/pipeline/pkg/authorization"
	clientset "github.com/tektoncd/pipeline/pkg/client/clientset/versioned"

	"github.com/markbates/inflect"
	"go.uber.org/zap"

	admissionv1 "k8s.io/api/admission/v1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	authorizationv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	admissionlisters "k8s.io/client-go/listers/admissionregistration/v1"
	corelisters "k8s.io/client-go/listers/core/v1"

	"knative.dev/pkg/apis"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/kmp"
	"knative.dev/pkg/logging"
	"knative.dev/pkg/ptr"
	"knative.dev/pkg/system"
	"knative.dev/pkg/webhook"
	certresources "knative.dev/pkg/webhook/certificates/resources"
	"knative.dev/pkg/webhook/resourcesemantics"
)

var errMissingNewObject = errors.New("the new object may not be nil")

// reconciler implements the AdmissionController for resources
type reconciler struct {
	name     string
	path     string
	handlers map[schema.GroupVersionKind]resourcesemantics.GenericCRD

	withContext func(context.Context) context.Context

	client       kubernetes.Interface
	vwhlister    admissionlisters.ValidatingWebhookConfigurationLister
	secretlister corelisters.SecretLister
	tektonClient clientset.Interface

	disallowUnknownFields bool
	secretName            string
}

var _ controller.Reconciler = (*reconciler)(nil)
var _ webhook.AdmissionController = (*reconciler)(nil)

// Reconcile implements controller.Reconciler
func (ac *reconciler) Reconcile(ctx context.Context, key string) error {
	logger := logging.FromContext(ctx)

	// Look up the webhook secret, and fetch the CA cert bundle.
	secret, err := ac.secretlister.Secrets(system.Namespace()).Get(ac.secretName)
	if err != nil {
		logger.Errorw("Error fetching secret", zap.Error(err))
		return err
	}
	caCert, ok := secret.Data[certresources.CACert]
	if !ok {
		return fmt.Errorf("secret %q is missing %q key", ac.secretName, certresources.CACert)
	}

	// Reconcile the webhook configuration.
	return ac.reconcileValidatingWebhook(ctx, caCert)
}

// Path implements AdmissionController
func (ac *reconciler) Path() string {
	return ac.path
}

// Admit implements AdmissionController
func (ac *reconciler) Admit(ctx context.Context, request *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {
	if ac.withContext != nil {
		ctx = ac.withContext(ctx)
	}

	logger := logging.FromContext(ctx)
	switch request.Operation {
	case admissionv1.Create, admissionv1.Update:
	default:
		logger.Infof("Unhandled webhook operation, letting it through %v", request.Operation)
		return &admissionv1.AdmissionResponse{Allowed: true}
	}

	if err := ac.validate(ctx, request); err != nil {
		return webhook.MakeErrorStatus("validation failed: %v", err)
	}

	return &admissionv1.AdmissionResponse{Allowed: true}
}

func (ac *reconciler) reconcileValidatingWebhook(ctx context.Context, caCert []byte) error {
	logger := logging.FromContext(ctx)

	var rules []admissionregistrationv1.RuleWithOperations
	for gvk := range ac.handlers {
		plural := strings.ToLower(inflect.Pluralize(gvk.Kind))

		rules = append(rules, admissionregistrationv1.RuleWithOperations{
			Operations: []admissionregistrationv1.OperationType{
				admissionregistrationv1.Create,
				admissionregistrationv1.Update,
			},
			Rule: admissionregistrationv1.Rule{
				APIGroups:   []string{gvk.Group},
				APIVersions: []string{gvk.Version},
				Resources:   []string{plural + "/*"},
			},
		})
	}

	// Sort the rules by Group, Version, Kind so that things are deterministically ordered.
	sort.Slice(rules, func(i, j int) bool {
		lhs, rhs := rules[i], rules[j]
		if lhs.APIGroups[0] != rhs.APIGroups[0] {
			return lhs.APIGroups[0] < rhs.APIGroups[0]
		}
		if lhs.APIVersions[0] != rhs.APIVersions[0] {
			return lhs.APIVersions[0] < rhs.APIVersions[0]
		}
		return lhs.Resources[0] < rhs.Resources[0]
	})

	configuredWebhook, err := ac.vwhlister.Get(ac.name)
	if err != nil {
		return fmt.Errorf("error retrieving webhook: %v", err)
	}

	webhook := configuredWebhook.DeepCopy()

	// Clear out any previous (bad) OwnerReferences.
	// See: https://github.com/knative/serving/issues/5845
	webhook.OwnerReferences = nil

	for i, wh := range webhook.Webhooks {
		if wh.Name != webhook.Name {
			continue
		}
		webhook.Webhooks[i].Rules = rules
		webhook.Webhooks[i].ClientConfig.CABundle = caCert
		if webhook.Webhooks[i].ClientConfig.Service == nil {
			return fmt.Errorf("missing service reference for webhook: %s", wh.Name)
		}
		webhook.Webhooks[i].ClientConfig.Service.Path = ptr.String(ac.Path())
	}

	ok, err := kmp.SafeEqual(configuredWebhook, webhook)
	switch {
	case err != nil:
		return fmt.Errorf("error diffing webhooks: %v", err)
	case !ok:
		logger.Info("Updating webhook")
		vwhclient := ac.client.AdmissionregistrationV1().ValidatingWebhookConfigurations()
		if _, err := vwhclient.Update(webhook); err != nil {
			return fmt.Errorf("failed to update webhook: %v", err)
		}
	default:
		logger.Info("Webhook is valid")
	}
	return nil
}

func (ac *reconciler) validate(ctx context.Context, req *admissionv1.AdmissionRequest) error {
	logger := logging.FromContext(ctx)

	if os.Getenv("ENABLE_CLUSTER_TASK_ACCESS_VALIDATION") != "true" {
		logger.Debug("ENABLE_CLUSTER_TASK_ACCESS_VALIDATION != true so returning")
		return nil
	}
	if os.Getenv("DISABLE_CLUSTER_TASK_ACCESS_VALIDATION") == "true" {
		logger.Debug("DISABLE_CLUSTER_TASK_ACCESS_VALIDATION == true so returning")
		return nil
	}

	kind := req.Kind
	newBytes := req.Object.Raw
	oldBytes := req.OldObject.Raw
	// Why, oh why are these different types...
	gvk := schema.GroupVersionKind{
		Group:   kind.Group,
		Version: kind.Version,
		Kind:    kind.Kind,
	}

	handler, ok := ac.handlers[gvk]
	if !ok {
		logger.Errorf("Unhandled kind: %v", gvk)
		return fmt.Errorf("unhandled kind: %v", gvk)
	}
	if !strings.EqualFold(strings.TrimSpace(req.Kind.Kind), "PipelineRun") &&
		!strings.EqualFold(strings.TrimSpace(req.Kind.Kind), "TaskRun") {
		return nil
	}

	// nil values denote absence of `old` (create) or `new` (delete) objects.
	var oldObj, newObj resourcesemantics.GenericCRD

	if len(newBytes) != 0 {
		newObj = handler.DeepCopyObject().(resourcesemantics.GenericCRD)
		newDecoder := json.NewDecoder(bytes.NewBuffer(newBytes))
		if ac.disallowUnknownFields {
			newDecoder.DisallowUnknownFields()
		}
		if err := newDecoder.Decode(&newObj); err != nil {
			return fmt.Errorf("cannot decode incoming new object: %v", err)
		}
	}
	if len(oldBytes) != 0 {
		oldObj = handler.DeepCopyObject().(resourcesemantics.GenericCRD)
		oldDecoder := json.NewDecoder(bytes.NewBuffer(oldBytes))
		if ac.disallowUnknownFields {
			oldDecoder.DisallowUnknownFields()
		}
		if err := oldDecoder.Decode(&oldObj); err != nil {
			return fmt.Errorf("cannot decode incoming old object: %v", err)
		}
	}

	// Set up the context for defaulting and validation
	if oldObj != nil {
		if req.SubResource == "" {
			ctx = apis.WithinUpdate(ctx, oldObj)
		} else {
			ctx = apis.WithinSubResourceUpdate(ctx, oldObj, req.SubResource)
		}
	} else {
		ctx = apis.WithinCreate(ctx)
	}
	ctx = apis.WithUserInfo(ctx, &req.UserInfo)

	// None of the validators will accept a nil value for newObj.
	if newObj == nil {
		return errMissingNewObject
	}

	if err := validate(ctx, newObj); err != nil {
		logger.Errorw("Failed the resource specific validation", zap.Error(err))
		// Return the error message as-is to give the validation callback
		// discretion over (our portion of) the message that the user sees.
		return err
	}

	switch v := newObj.(type) {
	case *v1alpha1.PipelineRun:
		pipelineTasks := []v1alpha1.PipelineTask{}
		switch {
		case v.Spec.PipelineRef != nil && v.Spec.PipelineRef.Name != "":
			pipeline, err := ac.tektonClient.TektonV1alpha1().Pipelines(v.Namespace).Get(v.Spec.PipelineRef.Name, metav1.GetOptions{})
			if err != nil {
				return err
			}
			pipelineTasks = pipeline.Spec.Tasks
		case v.Spec.PipelineSpec != nil:
			pipelineTasks = v.Spec.PipelineSpec.Tasks
		}
		for _, pt := range pipelineTasks {
			if pt.TaskRef == nil || pt.TaskRef.Kind == v1beta1.NamespacedTaskKind {
				continue
			}
			err := ac.runSar(v.Name, v.Namespace, v.Spec.ServiceAccountName, pt.TaskRef.Name)
			if err != nil {
				return err
			}
		}

	case *v1alpha1.TaskRun:
		if v.Spec.TaskRef != nil && v.Spec.TaskRef.Kind == v1alpha1.ClusterTaskKind {
			return ac.runSar(v.Name, v.Namespace, v.Spec.ServiceAccountName, v.Spec.TaskRef.Name)
		}

	case *v1beta1.PipelineRun:
		pipelineTasks := []v1beta1.PipelineTask{}
		switch {
		case v.Spec.PipelineRef != nil && v.Spec.PipelineRef.Name != "":
			pipeline, err := ac.tektonClient.TektonV1beta1().Pipelines(v.Namespace).Get(v.Spec.PipelineRef.Name, metav1.GetOptions{})
			if err != nil {
				return err
			}
			pipelineTasks = pipeline.Spec.Tasks
		case v.Spec.PipelineSpec != nil:
			pipelineTasks = v.Spec.PipelineSpec.Tasks
		}
		for _, pt := range pipelineTasks {
			if pt.TaskRef == nil || pt.TaskRef.Kind == v1beta1.NamespacedTaskKind {
				continue
			}
			err := ac.runSar(v.Name, v.Namespace, v.Spec.ServiceAccountName, pt.TaskRef.Name)
			if err != nil {
				return err
			}
		}

	case *v1beta1.TaskRun:
		tr := newObj.(*v1beta1.TaskRun)
		if tr.Spec.TaskRef != nil && tr.Spec.TaskRef.Kind == v1beta1.ClusterTaskKind {
			return ac.runSar(tr.Name, tr.Namespace, tr.Spec.ServiceAccountName, tr.Spec.TaskRef.Name)
		}

	}

	return nil
}

var cannotReadClusterTaskString = "%s/%s service account cannot access clustertask %s"

func (ac *reconciler) runSar(name, namespace, sa, clusterTaskName string) error {
	saToUse := "default"
	if len(sa) > 0 {
		saToUse = sa
	}
	if err := authorization.AuthorizeSAR(ac.client.AuthorizationV1().SubjectAccessReviews(),
		saToUse,
		namespace,
		&authorizationv1.ResourceAttributes{
			Verb:      "get",
			Group:     "tekton.dev",
			Namespace: namespace,
			Name:      clusterTaskName,
			Resource:  "clustertasks",
		}); err != nil {
		return fmt.Errorf(cannotReadClusterTaskString, namespace, saToUse, clusterTaskName)
	}
	return nil
}

// validate performs validation on the provided "new" CRD.
func validate(ctx context.Context, new apis.Validatable) error {
	// Can't just `return new.Validate()` because it doesn't properly nil-check.
	if err := new.Validate(ctx); err != nil {
		return err
	}

	return nil
}
