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

package v1beta1

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/util/sets"
	"knative.dev/pkg/apis"
)

var validWhenOperators = []string{
	string(selection.In),
	string(selection.NotIn),
}

func (wes WhenExpressions) validate() *apis.FieldError {
	errs := wes.validateWhenExpressionsFields().ViaField("when")
	return errs.Also(wes.validateTaskResultsVariables().ViaField("when"))
}

func (wes WhenExpressions) validateWhenExpressionsFields() (errs *apis.FieldError) {
	for idx, we := range wes {
		errs = errs.Also(we.validateWhenExpressionFields().ViaIndex(idx))
	}
	return errs
}

func (we *WhenExpression) validateWhenExpressionFields() *apis.FieldError {
	if equality.Semantic.DeepEqual(we, &WhenExpression{}) || we == nil {
		return apis.ErrMissingField(apis.CurrentField)
	}
	if !sets.NewString(validWhenOperators...).Has(string(we.Operator)) {
		message := fmt.Sprintf("operator %q is not recognized. valid operators: %s", we.Operator, strings.Join(validWhenOperators, ","))
		return apis.ErrInvalidValue(message, apis.CurrentField)
	}
	if len(we.Values) == 0 {
		return apis.ErrInvalidValue("expecting non-empty values field", apis.CurrentField)
	}
	return nil
}

func (wes WhenExpressions) validateTaskResultsVariables() *apis.FieldError {
	for idx, we := range wes {
		expressions, ok := we.GetVarSubstitutionExpressions()
		if ok {
			if LooksLikeContainsResultRefs(expressions) {
				expressions = filter(expressions, looksLikeResultRef)
				resultRefs := NewResultRefs(expressions)
				if len(expressions) != len(resultRefs) {
					message := fmt.Sprintf("expected all of the expressions %v to be result expressions but only %v were", expressions, resultRefs)
					return apis.ErrInvalidValue(message, apis.CurrentField).ViaIndex(idx)
				}
			}
		}
	}
	return nil
}

func (wes WhenExpressions) validatePipelineParametersVariables(prefix string, paramNames sets.String, arrayParamNames sets.String) (errs *apis.FieldError) {
	for idx, we := range wes {
		errs = errs.Also(validateStringVariable(we.Input, prefix, paramNames, arrayParamNames).ViaField("input").ViaFieldIndex("when", idx))
		for _, val := range we.Values {
			// one of the values could be a reference to an array param, such as, $(params.foo[*])
			// extract the variable name from the pattern $(params.foo[*]), if the variable name matches with one of the array params
			// validate the param as an array variable otherwise, validate it as a string variable
			if arrayParamNames.Has(ArrayReference(val)) {
				errs = errs.Also(validateArrayVariable(val, prefix, paramNames, arrayParamNames).ViaField("values").ViaFieldIndex("when", idx))
			} else {
				errs = errs.Also(validateStringVariable(val, prefix, paramNames, arrayParamNames).ViaField("values").ViaFieldIndex("when", idx))
			}
		}
	}
	return errs
}
