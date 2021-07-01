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
	"context"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/tektoncd/pipeline/pkg/apis/config"
	"github.com/tektoncd/pipeline/test/diff"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/selection"
	"knative.dev/pkg/apis"
	logtesting "knative.dev/pkg/logging/testing"
)

func TestPipeline_Validate_Success(t *testing.T) {
	tests := []struct {
		name string
		p    *Pipeline
		wc   func(context.Context) context.Context
	}{{
		name: "valid metadata",
		p: &Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "pipeline"},
			Spec: PipelineSpec{
				Tasks: []PipelineTask{{Name: "foo", TaskRef: &TaskRef{Name: "foo-task"}}},
			},
		},
	}, {
		name: "pipelinetask custom task references",
		p: &Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "pipeline"},
			Spec: PipelineSpec{
				Tasks: []PipelineTask{{Name: "foo", TaskRef: &TaskRef{APIVersion: "example.dev/v0", Kind: "Example", Name: ""}}},
			},
		},
		wc: enableFeatures(t, []string{"enable-custom-tasks"}),
	}, {
		name: "pipelinetask custom task spec",
		p: &Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "pipeline"},
			Spec: PipelineSpec{
				Tasks: []PipelineTask{{Name: "foo",
					TaskSpec: &EmbeddedTask{
						TypeMeta: runtime.TypeMeta{
							APIVersion: "example.dev/v0",
							Kind:       "Example",
						},
						Spec: runtime.RawExtension{
							Raw: []byte(`{"field1":123,"field2":"value"}`),
						}},
				}},
			},
		},
		wc: enableFeatures(t, []string{"enable-custom-tasks"}),
	}, {
		name: "valid pipeline with params, resources, workspaces, task results, and pipeline results",
		p: &Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "pipeline"},
			Spec: PipelineSpec{
				Description: "this is a valid pipeline with all possible fields initialized",
				Resources: []PipelineDeclaredResource{{
					Name:     "app-repo",
					Type:     "git",
					Optional: false,
				}, {
					Name:     "app-image",
					Type:     "git",
					Optional: false,
				}},
				Tasks: []PipelineTask{{
					Name:    "my-task",
					TaskRef: &TaskRef{Name: "foo-task"},
					Retries: 5,
					Resources: &PipelineTaskResources{
						Inputs: []PipelineTaskInputResource{{
							Name:     "task-app-repo",
							Resource: "app-repo",
						}},
						Outputs: []PipelineTaskOutputResource{{
							Name:     "task-app-image",
							Resource: "app-image",
						}},
					},
					Params: []Param{{
						Name:  "param1",
						Value: ArrayOrString{},
					}},
					Workspaces: []WorkspacePipelineTaskBinding{{
						Name:      "task-shared-workspace",
						Workspace: "shared-workspace",
					}},
					Timeout: &metav1.Duration{Duration: 5 * time.Minute},
				}},
				Params: []ParamSpec{{
					Name:        "param1",
					Type:        ParamType("string"),
					Description: "this is my param",
					Default: &ArrayOrString{
						Type:      ParamType("string"),
						StringVal: "pipeline-default",
					},
				}},
				Workspaces: []PipelineWorkspaceDeclaration{{
					Name:        "shared-workspace",
					Description: "this is my shared workspace",
				}},
				Results: []PipelineResult{{
					Name:        "pipeline-result",
					Description: "this is my pipeline result",
					Value:       "pipeline-result-default",
				}},
			},
		},
	}, {
		name: "do not validate spec on delete",
		p: &Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "pipeline"},
		},
		wc: apis.WithinDelete,
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.wc != nil {
				ctx = tt.wc(ctx)
			}
			err := tt.p.Validate(ctx)
			if err != nil {
				t.Errorf("Pipeline.Validate() returned error for valid Pipeline: %v", err)
			}
		})
	}
}

func TestPipeline_Validate_Failure(t *testing.T) {
	tests := []struct {
		name          string
		p             *Pipeline
		expectedError apis.FieldError
		wc            func(context.Context) context.Context
	}{{
		name: "comma in name",
		p: &Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "pipe,line"},
			Spec: PipelineSpec{
				Tasks: []PipelineTask{{Name: "foo", TaskRef: &TaskRef{Name: "foo-task"}}},
			},
		},
		expectedError: apis.FieldError{
			Message: `invalid resource name "pipe,line": must be a valid DNS label`,
			Paths:   []string{"metadata.name"},
		},
	}, {
		name: "pipeline name too long",
		p: &Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "asdf123456789012345678901234567890123456789012345678901234567890"},
			Spec: PipelineSpec{
				Tasks: []PipelineTask{{Name: "foo", TaskRef: &TaskRef{Name: "foo-task"}}},
			},
		},
		expectedError: apis.FieldError{
			Message: "Invalid resource name: length must be no more than 63 characters",
			Paths:   []string{"metadata.name"},
		},
	}, {
		name: "pipeline spec missing",
		p: &Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "pipeline"},
		},
		expectedError: apis.FieldError{
			Message: `expected at least one, got none`,
			Paths:   []string{"spec.description", "spec.params", "spec.resources", "spec.tasks", "spec.workspaces"},
		},
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.wc != nil {
				ctx = tt.wc(ctx)
			}
			err := tt.p.Validate(ctx)
			if err == nil {
				t.Error("Pipeline.Validate() did not return error for invalid pipeline")
			}
			if d := cmp.Diff(tt.expectedError.Error(), err.Error(), cmpopts.IgnoreUnexported(apis.FieldError{})); d != "" {
				t.Errorf("Pipeline.Validate() errors diff %s", diff.PrintWantGot(d))
			}
		})
	}
}

func TestPipelineSpec_Validate_Failure(t *testing.T) {
	tests := []struct {
		name          string
		ps            *PipelineSpec
		expectedError apis.FieldError
	}{{
		name: "invalid pipeline with one pipeline task having taskRef and taskSpec both",
		ps: &PipelineSpec{
			Description: "this is an invalid pipeline with invalid pipeline task",
			Tasks: []PipelineTask{{
				Name:    "valid-pipeline-task",
				TaskRef: &TaskRef{Name: "foo-task"},
			}, {
				Name:     "invalid-pipeline-task",
				TaskRef:  &TaskRef{Name: "foo-task"},
				TaskSpec: &EmbeddedTask{TaskSpec: getTaskSpec()},
			}},
		},
		expectedError: apis.FieldError{
			Message: `expected exactly one, got both`,
			Paths:   []string{"tasks[1].taskRef", "tasks[1].taskSpec"},
		},
	}, {
		name: "invalid pipeline with one pipeline task having both conditions and when expressions",
		ps: &PipelineSpec{
			Description: "this is an invalid pipeline with invalid pipeline task",
			Tasks: []PipelineTask{{
				Name:    "invalid-pipeline-task",
				TaskRef: &TaskRef{Name: "foo-task"},
				WhenExpressions: []WhenExpression{{
					Input:    "foo",
					Operator: selection.In,
					Values:   []string{"bar"},
				}},
				Conditions: []PipelineTaskCondition{{
					ConditionRef: "some-condition",
				}},
			}},
		},
		expectedError: apis.FieldError{
			Message: `expected exactly one, got both`,
			Paths:   []string{"tasks[0].conditions", "tasks[0].when"},
		},
	}, {
		name: "invalid pipeline with one pipeline task having when expression with invalid operator (not In/NotIn)",
		ps: &PipelineSpec{
			Description: "this is an invalid pipeline with invalid pipeline task",
			Tasks: []PipelineTask{{
				Name:    "invalid-pipeline-task",
				TaskRef: &TaskRef{Name: "bar-task"},
				WhenExpressions: []WhenExpression{{
					Input:    "foo",
					Operator: selection.Exists,
					Values:   []string{"foo"},
				}},
			}},
		},
		expectedError: apis.FieldError{
			Message: `invalid value: operator "exists" is not recognized. valid operators: in,notin`,
			Paths:   []string{"tasks[0].when[0]"},
		},
	}, {
		name: "invalid pipeline with final task having when expression with invalid operator (not In/NotIn)",
		ps: &PipelineSpec{
			Description: "this is an invalid pipeline with invalid pipeline task",
			Tasks: []PipelineTask{{
				Name:    "invalid-pipeline-task",
				TaskRef: &TaskRef{Name: "bar-task"},
			}},
			Finally: []PipelineTask{{
				Name:    "invalid-pipeline-task-finally",
				TaskRef: &TaskRef{Name: "bar-task"},
				WhenExpressions: []WhenExpression{{
					Input:    "foo",
					Operator: selection.Exists,
					Values:   []string{"foo"},
				}},
			}},
		},
		expectedError: apis.FieldError{
			Message: `invalid value: operator "exists" is not recognized. valid operators: in,notin`,
			Paths:   []string{"finally[0].when[0]"},
		},
	}, {
		name: "invalid pipeline with dag task and final task having when expression with invalid operator (not In/NotIn)",
		ps: &PipelineSpec{
			Description: "this is an invalid pipeline with invalid pipeline task",
			Tasks: []PipelineTask{{
				Name:    "invalid-pipeline-task",
				TaskRef: &TaskRef{Name: "bar-task"},
				WhenExpressions: []WhenExpression{{
					Input:    "foo",
					Operator: selection.Exists,
					Values:   []string{"foo"},
				}},
			}},
			Finally: []PipelineTask{{
				Name:    "invalid-pipeline-task-finally",
				TaskRef: &TaskRef{Name: "bar-task"},
				WhenExpressions: []WhenExpression{{
					Input:    "foo",
					Operator: selection.Exists,
					Values:   []string{"foo"},
				}},
			}},
		},
		expectedError: apis.FieldError{
			Message: `invalid value: operator "exists" is not recognized. valid operators: in,notin`,
			Paths:   []string{"tasks[0].when[0]", "finally[0].when[0]"},
		},
	}, {
		name: "invalid pipeline with one pipeline task having when expression with invalid values (empty)",
		ps: &PipelineSpec{
			Description: "this is an invalid pipeline with invalid pipeline task",
			Tasks: []PipelineTask{{
				Name:    "invalid-pipeline-task",
				TaskRef: &TaskRef{Name: "foo-task"},
				WhenExpressions: []WhenExpression{{
					Input:    "foo",
					Operator: selection.In,
					Values:   []string{},
				}},
			}},
		},
		expectedError: apis.FieldError{
			Message: `invalid value: expecting non-empty values field`,
			Paths:   []string{"tasks[0].when[0]"},
		},
	}, {
		name: "invalid pipeline with final task having when expression with invalid values (empty)",
		ps: &PipelineSpec{
			Description: "this is an invalid pipeline with invalid pipeline task",
			Tasks: []PipelineTask{{
				Name:    "invalid-pipeline-task",
				TaskRef: &TaskRef{Name: "foo-task"},
			}},
			Finally: []PipelineTask{{
				Name:    "invalid-pipeline-task-finally",
				TaskRef: &TaskRef{Name: "foo-task"},
				WhenExpressions: []WhenExpression{{
					Input:    "foo",
					Operator: selection.In,
					Values:   []string{},
				}},
			}},
		},
		expectedError: apis.FieldError{
			Message: `invalid value: expecting non-empty values field`,
			Paths:   []string{"finally[0].when[0]"},
		},
	}, {
		name: "invalid pipeline with dag task and final task having when expression with invalid values (empty)",
		ps: &PipelineSpec{
			Description: "this is an invalid pipeline with invalid pipeline task",
			Tasks: []PipelineTask{{
				Name:    "invalid-pipeline-task",
				TaskRef: &TaskRef{Name: "foo-task"},
				WhenExpressions: []WhenExpression{{
					Input:    "foo",
					Operator: selection.In,
					Values:   []string{},
				}},
			}},
			Finally: []PipelineTask{{
				Name:    "invalid-pipeline-task-finally",
				TaskRef: &TaskRef{Name: "foo-task"},
				WhenExpressions: []WhenExpression{{
					Input:    "foo",
					Operator: selection.In,
					Values:   []string{},
				}},
			}},
		},
		expectedError: apis.FieldError{
			Message: `invalid value: expecting non-empty values field`,
			Paths:   []string{"tasks[0].when[0]", "finally[0].when[0]"},
		},
	}, {
		name: "invalid pipeline with one pipeline task having when expression with invalid operator (missing)",
		ps: &PipelineSpec{
			Description: "this is an invalid pipeline with invalid pipeline task",
			Tasks: []PipelineTask{{
				Name:    "invalid-pipeline-task",
				TaskRef: &TaskRef{Name: "foo-task"},
				WhenExpressions: []WhenExpression{{
					Input:  "foo",
					Values: []string{"foo"},
				}},
			}},
		},
		expectedError: apis.FieldError{
			Message: `invalid value: operator "" is not recognized. valid operators: in,notin`,
			Paths:   []string{"tasks[0].when[0]"},
		},
	}, {
		name: "invalid pipeline with final task having when expression with invalid operator (missing)",
		ps: &PipelineSpec{
			Description: "this is an invalid pipeline with invalid pipeline task",
			Tasks: []PipelineTask{{
				Name:    "invalid-pipeline-task",
				TaskRef: &TaskRef{Name: "foo-task"},
			}},
			Finally: []PipelineTask{{
				Name:    "invalid-pipeline-task-finally",
				TaskRef: &TaskRef{Name: "foo-task"},
				WhenExpressions: []WhenExpression{{
					Input:  "foo",
					Values: []string{"foo"},
				}},
			}},
		},
		expectedError: apis.FieldError{
			Message: `invalid value: operator "" is not recognized. valid operators: in,notin`,
			Paths:   []string{"finally[0].when[0]"},
		},
	}, {
		name: "invalid pipeline with dag task and final task having when expression with invalid operator (missing)",
		ps: &PipelineSpec{
			Description: "this is an invalid pipeline with invalid pipeline task",
			Tasks: []PipelineTask{{
				Name:    "invalid-pipeline-task",
				TaskRef: &TaskRef{Name: "foo-task"},
				WhenExpressions: []WhenExpression{{
					Input:  "foo",
					Values: []string{"foo"},
				}},
			}},
			Finally: []PipelineTask{{
				Name:    "invalid-pipeline-task-finally",
				TaskRef: &TaskRef{Name: "foo-task"},
				WhenExpressions: []WhenExpression{{
					Input:  "foo",
					Values: []string{"foo"},
				}},
			}},
		},
		expectedError: apis.FieldError{
			Message: `invalid value: operator "" is not recognized. valid operators: in,notin`,
			Paths:   []string{"tasks[0].when[0]", "finally[0].when[0]"},
		},
	}, {
		name: "invalid pipeline with one pipeline task having when expression with invalid values (missing)",
		ps: &PipelineSpec{
			Description: "this is an invalid pipeline with invalid pipeline task",
			Tasks: []PipelineTask{{
				Name:    "invalid-pipeline-task",
				TaskRef: &TaskRef{Name: "foo-task"},
				WhenExpressions: []WhenExpression{{
					Input:    "foo",
					Operator: selection.In,
				}},
			}},
		},
		expectedError: apis.FieldError{
			Message: `invalid value: expecting non-empty values field`,
			Paths:   []string{"tasks[0].when[0]"},
		},
	}, {
		name: "invalid pipeline with final task having when expression with invalid values (missing)",
		ps: &PipelineSpec{
			Description: "this is an invalid pipeline with invalid pipeline task",
			Tasks: []PipelineTask{{
				Name:    "invalid-pipeline-task",
				TaskRef: &TaskRef{Name: "foo-task"},
			}},
			Finally: []PipelineTask{{
				Name:    "invalid-pipeline-task-finally",
				TaskRef: &TaskRef{Name: "foo-task"},
				WhenExpressions: []WhenExpression{{
					Input:    "foo",
					Operator: selection.In,
				}},
			}},
		},
		expectedError: apis.FieldError{
			Message: `invalid value: expecting non-empty values field`,
			Paths:   []string{"finally[0].when[0]"},
		},
	}, {
		name: "invalid pipeline with dag task and final task having when expression with invalid values (missing)",
		ps: &PipelineSpec{
			Description: "this is an invalid pipeline with invalid pipeline task",
			Tasks: []PipelineTask{{
				Name:    "invalid-pipeline-task",
				TaskRef: &TaskRef{Name: "foo-task"},
				WhenExpressions: []WhenExpression{{
					Input:    "foo",
					Operator: selection.In,
				}},
			}},
			Finally: []PipelineTask{{
				Name:    "invalid-pipeline-task-finally",
				TaskRef: &TaskRef{Name: "foo-task"},
				WhenExpressions: []WhenExpression{{
					Input:    "foo",
					Operator: selection.In,
				}},
			}},
		},
		expectedError: apis.FieldError{
			Message: `invalid value: expecting non-empty values field`,
			Paths:   []string{"tasks[0].when[0]", "finally[0].when[0]"},
		},
	}, {
		name: "invalid pipeline with one pipeline task having when expression with misconfigured result reference",
		ps: &PipelineSpec{
			Description: "this is an invalid pipeline with invalid pipeline task",
			Tasks: []PipelineTask{{
				Name:    "valid-pipeline-task",
				TaskRef: &TaskRef{Name: "foo-task"},
			}, {
				Name:    "invalid-pipeline-task",
				TaskRef: &TaskRef{Name: "foo-task"},
				WhenExpressions: []WhenExpression{{
					Input:    "$(tasks.a-task.resultTypo.bResult)",
					Operator: selection.In,
					Values:   []string{"bar"},
				}},
			}},
		},
		expectedError: apis.FieldError{
			Message: `invalid value: expected all of the expressions [tasks.a-task.resultTypo.bResult] to be result expressions but only [] were`,
			Paths:   []string{"tasks[1].when[0]"},
		},
	}, {
		name: "invalid pipeline with final task having when expression with misconfigured result reference",
		ps: &PipelineSpec{
			Description: "this is an invalid pipeline with invalid pipeline task",
			Tasks: []PipelineTask{{
				Name:    "valid-pipeline-task",
				TaskRef: &TaskRef{Name: "foo-task"},
			}, {
				Name:    "invalid-pipeline-task",
				TaskRef: &TaskRef{Name: "foo-task"},
			}},
			Finally: []PipelineTask{{
				Name:    "invalid-pipeline-task-finally",
				TaskRef: &TaskRef{Name: "foo-task"},
				WhenExpressions: []WhenExpression{{
					Input:    "$(tasks.a-task.resultTypo.bResult)",
					Operator: selection.In,
					Values:   []string{"bar"},
				}},
			}},
		},
		expectedError: apis.FieldError{
			Message: `invalid value: expected all of the expressions [tasks.a-task.resultTypo.bResult] to be result expressions but only [] were`,
			Paths:   []string{"finally[0].when[0]"},
		},
	}, {
		name: "invalid pipeline with dag task and final task having when expression with misconfigured result reference",
		ps: &PipelineSpec{
			Description: "this is an invalid pipeline with invalid pipeline task",
			Tasks: []PipelineTask{{
				Name:    "valid-pipeline-task",
				TaskRef: &TaskRef{Name: "foo-task"},
			}, {
				Name:    "invalid-pipeline-task",
				TaskRef: &TaskRef{Name: "foo-task"},
				WhenExpressions: []WhenExpression{{
					Input:    "$(tasks.a-task.resultTypo.bResult)",
					Operator: selection.In,
					Values:   []string{"bar"},
				}},
			}},
			Finally: []PipelineTask{{
				Name:    "invalid-pipeline-task-finally",
				TaskRef: &TaskRef{Name: "foo-task"},
				WhenExpressions: []WhenExpression{{
					Input:    "$(tasks.a-task.resultTypo.bResult)",
					Operator: selection.In,
					Values:   []string{"bar"},
				}},
			}},
		},
		expectedError: apis.FieldError{
			Message: `invalid value: expected all of the expressions [tasks.a-task.resultTypo.bResult] to be result expressions but only [] were`,
			Paths:   []string{"tasks[1].when[0]", "finally[0].when[0]"},
		},
	}, {
		name: "invalid pipeline with one pipeline task having blank when expression",
		ps: &PipelineSpec{
			Description: "this is an invalid pipeline with invalid pipeline task",
			Tasks: []PipelineTask{{
				Name:    "valid-pipeline-task",
				TaskRef: &TaskRef{Name: "foo-task"},
			}, {
				Name:            "invalid-pipeline-task",
				TaskRef:         &TaskRef{Name: "foo-task"},
				WhenExpressions: []WhenExpression{{}},
			}},
		},
		expectedError: apis.FieldError{
			Message: `missing field(s)`,
			Paths:   []string{"tasks[1].when[0]"},
		},
	}, {
		name: "invalid pipeline with final task having blank when expression",
		ps: &PipelineSpec{
			Description: "this is an invalid pipeline with invalid pipeline task",
			Tasks: []PipelineTask{{
				Name:    "valid-pipeline-task",
				TaskRef: &TaskRef{Name: "foo-task"},
			}, {
				Name:    "invalid-pipeline-task",
				TaskRef: &TaskRef{Name: "foo-task"},
			}},
			Finally: []PipelineTask{{
				Name:            "invalid-pipeline-task-finally",
				TaskRef:         &TaskRef{Name: "foo-task"},
				WhenExpressions: []WhenExpression{{}},
			}},
		},
		expectedError: apis.FieldError{
			Message: `missing field(s)`,
			Paths:   []string{"finally[0].when[0]"},
		},
	}, {
		name: "invalid pipeline with dag task and final task having blank when expression",
		ps: &PipelineSpec{
			Description: "this is an invalid pipeline with invalid pipeline task",
			Tasks: []PipelineTask{{
				Name:    "valid-pipeline-task",
				TaskRef: &TaskRef{Name: "foo-task"},
			}, {
				Name:            "invalid-pipeline-task",
				TaskRef:         &TaskRef{Name: "foo-task"},
				WhenExpressions: []WhenExpression{{}},
			}},
			Finally: []PipelineTask{{
				Name:            "invalid-pipeline-task-finally",
				TaskRef:         &TaskRef{Name: "foo-task"},
				WhenExpressions: []WhenExpression{{}},
			}},
		},
		expectedError: apis.FieldError{
			Message: `missing field(s)`,
			Paths:   []string{"tasks[1].when[0]", "finally[0].when[0]"},
		},
	}, {
		name: "invalid pipeline with pipeline task having reference to resources which does not exist",
		ps: &PipelineSpec{
			Resources: []PipelineDeclaredResource{{
				Name: "great-resource", Type: PipelineResourceTypeGit,
			}, {
				Name: "wonderful-resource", Type: PipelineResourceTypeImage,
			}},
			Tasks: []PipelineTask{{
				Name:    "bar",
				TaskRef: &TaskRef{Name: "bar-task"},
				Resources: &PipelineTaskResources{
					Inputs: []PipelineTaskInputResource{{
						Name: "some-workspace", Resource: "missing-great-resource",
					}},
					Outputs: []PipelineTaskOutputResource{{
						Name: "some-imagee", Resource: "missing-wonderful-resource",
					}},
				},
				Conditions: []PipelineTaskCondition{{
					ConditionRef: "some-condition",
					Resources: []PipelineTaskInputResource{{
						Name: "some-workspace", Resource: "missing-great-resource",
					}},
				}},
			}, {
				Name:    "foo",
				TaskRef: &TaskRef{Name: "foo-task"},
				Resources: &PipelineTaskResources{
					Inputs: []PipelineTaskInputResource{{
						Name: "some-image", Resource: "wonderful-resource",
					}},
				},
				Conditions: []PipelineTaskCondition{{
					ConditionRef: "some-condition-2",
					Resources: []PipelineTaskInputResource{{
						Name: "some-image", Resource: "wonderful-resource",
					}},
				}},
			}},
		},
		expectedError: apis.FieldError{
			Message: `invalid value: pipeline declared resources didn't match usage in Tasks: Didn't provide required values: [missing-great-resource missing-wonderful-resource missing-great-resource]`,
			Paths:   []string{"resources"},
		},
	}, {
		name: "invalid pipeline spec - from referring to a pipeline task which does not exist",
		ps: &PipelineSpec{
			Resources: []PipelineDeclaredResource{{
				Name: "great-resource", Type: PipelineResourceTypeGit,
			}},
			Tasks: []PipelineTask{{
				Name: "baz", TaskRef: &TaskRef{Name: "baz-task"},
			}, {
				Name:    "foo",
				TaskRef: &TaskRef{Name: "foo-task"},
				Resources: &PipelineTaskResources{
					Inputs: []PipelineTaskInputResource{{
						Name: "the-resource", Resource: "great-resource", From: []string{"bar"},
					}},
				},
			}},
		},
		expectedError: *apis.ErrGeneric(`invalid value: couldn't add link between foo and bar: task foo depends on bar but bar wasn't present in Pipeline`, "tasks").Also(
			apis.ErrInvalidValue("expected resource great-resource to be from task bar, but task bar doesn't exist", "tasks[1].resources.inputs[0].from")),
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.ps.Validate(context.Background())
			if err == nil {
				t.Errorf("PipelineSpec.Validate() did not return error for invalid pipelineSpec")
			}
			if d := cmp.Diff(tt.expectedError.Error(), err.Error(), cmpopts.IgnoreUnexported(apis.FieldError{})); d != "" {
				t.Errorf("PipelineSpec.Validate() errors diff %s", diff.PrintWantGot(d))
			}
		})
	}
}

func TestPipelineSpec_Validate_Failure_CycleDAG(t *testing.T) {
	name := "invalid pipeline spec with DAG having cyclic dependency"
	ps := &PipelineSpec{
		Tasks: []PipelineTask{{
			Name: "foo", TaskRef: &TaskRef{Name: "foo-task"}, RunAfter: []string{"bar"},
		}, {
			Name: "bar", TaskRef: &TaskRef{Name: "bar-task"}, RunAfter: []string{"foo"},
		}},
	}
	err := ps.Validate(context.Background())
	if err == nil {
		t.Errorf("PipelineSpec.Validate() did not return error for invalid pipelineSpec: %s", name)
	}
}

func TestValidatePipelineTasks_Failure(t *testing.T) {
	tests := []struct {
		name          string
		tasks         []PipelineTask
		finalTasks    []PipelineTask
		expectedError apis.FieldError
	}{{
		name: "pipeline tasks invalid (duplicate tasks)",
		tasks: []PipelineTask{
			{Name: "foo", TaskRef: &TaskRef{Name: "foo-task"}},
			{Name: "foo", TaskRef: &TaskRef{Name: "foo-task"}},
		},
		expectedError: apis.FieldError{
			Message: `expected exactly one, got both`,
			Paths:   []string{"tasks[1].name"},
		},
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePipelineTasks(context.Background(), tt.tasks, tt.finalTasks)
			if err == nil {
				t.Error("ValidatePipelineTasks() did not return error for invalid pipeline tasks")
			}
			if d := cmp.Diff(tt.expectedError.Error(), err.Error(), cmpopts.IgnoreUnexported(apis.FieldError{})); d != "" {
				t.Errorf("ValidatePipelineTasks() errors diff %s", diff.PrintWantGot(d))
			}
		})
	}
}

func TestValidateFrom_Success(t *testing.T) {
	desc := "valid pipeline task - from resource referring to valid output resource of the pipeline task"
	tasks := []PipelineTask{{
		Name:    "bar",
		TaskRef: &TaskRef{Name: "bar-task"},
		Resources: &PipelineTaskResources{
			Inputs: []PipelineTaskInputResource{{
				Name: "some-resource", Resource: "some-resource",
			}},
			Outputs: []PipelineTaskOutputResource{{
				Name: "output-resource", Resource: "output-resource",
			}},
		},
	}, {
		Name:    "foo",
		TaskRef: &TaskRef{Name: "foo-task"},
		Resources: &PipelineTaskResources{
			Inputs: []PipelineTaskInputResource{{
				Name: "wow-image", Resource: "output-resource", From: []string{"bar"},
			}},
		},
	}}
	t.Run(desc, func(t *testing.T) {
		err := validateFrom(tasks)
		if err != nil {
			t.Errorf("Pipeline.validateFrom() returned error for: %v", err)
		}
	})
}

func TestValidateFrom_Failure(t *testing.T) {
	tests := []struct {
		name          string
		tasks         []PipelineTask
		expectedError apis.FieldError
	}{{
		name: "invalid pipeline task - from in a pipeline with single pipeline task",
		tasks: []PipelineTask{{
			Name:    "foo",
			TaskRef: &TaskRef{Name: "foo-task"},
			Resources: &PipelineTaskResources{
				Inputs: []PipelineTaskInputResource{{
					Name: "the-resource", Resource: "great-resource", From: []string{"bar"},
				}},
			},
		}},
		expectedError: apis.FieldError{
			Message: `invalid value: expected resource great-resource to be from task bar, but task bar doesn't exist`,
			Paths:   []string{"tasks[0].resources.inputs[0].from"},
		},
	}, {
		name: "invalid pipeline task - from referencing pipeline task which does not exist",
		tasks: []PipelineTask{{
			Name: "baz", TaskRef: &TaskRef{Name: "baz-task"},
		}, {
			Name:    "foo",
			TaskRef: &TaskRef{Name: "foo-task"},
			Resources: &PipelineTaskResources{
				Inputs: []PipelineTaskInputResource{{
					Name: "the-resource", Resource: "great-resource", From: []string{"bar"},
				}},
			},
		}},
		expectedError: apis.FieldError{
			Message: `invalid value: expected resource great-resource to be from task bar, but task bar doesn't exist`,
			Paths:   []string{"tasks[1].resources.inputs[0].from"},
		},
	}, {
		name: "invalid pipeline task - pipeline task condition resource does not exist",
		tasks: []PipelineTask{{
			Name: "foo", TaskRef: &TaskRef{Name: "foo-task"},
		}, {
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			Conditions: []PipelineTaskCondition{{
				ConditionRef: "some-condition",
				Resources: []PipelineTaskInputResource{{
					Name: "some-workspace", Resource: "missing-resource", From: []string{"foo"},
				}},
			}},
		}},
		expectedError: apis.FieldError{
			Message: `invalid value: the resource missing-resource from foo must be an output but is an input`,
			Paths:   []string{"tasks[1].resources.inputs[0].from"},
		},
	}, {
		name: "invalid pipeline task - from resource referring to a pipeline task which has no output",
		tasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			Resources: &PipelineTaskResources{
				Inputs: []PipelineTaskInputResource{{
					Name: "some-resource", Resource: "great-resource",
				}},
			},
		}, {
			Name:    "foo",
			TaskRef: &TaskRef{Name: "foo-task"},
			Resources: &PipelineTaskResources{
				Inputs: []PipelineTaskInputResource{{
					Name: "wow-image", Resource: "wonderful-resource", From: []string{"bar"},
				}},
			},
		}},
		expectedError: apis.FieldError{
			Message: `invalid value: the resource wonderful-resource from bar must be an output but is an input`,
			Paths:   []string{"tasks[1].resources.inputs[0].from"},
		},
	}, {
		name: "invalid pipeline task - from resource referring to input resource of the pipeline task instead of output",
		tasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			Resources: &PipelineTaskResources{
				Inputs: []PipelineTaskInputResource{{
					Name: "some-resource", Resource: "great-resource",
				}},
				Outputs: []PipelineTaskOutputResource{{
					Name: "output-resource", Resource: "great-output-resource",
				}},
			},
		}, {
			Name:    "foo",
			TaskRef: &TaskRef{Name: "foo-task"},
			Resources: &PipelineTaskResources{
				Inputs: []PipelineTaskInputResource{{
					Name: "wow-image", Resource: "some-resource", From: []string{"bar"},
				}},
			},
		}},
		expectedError: apis.FieldError{
			Message: `invalid value: the resource some-resource from bar must be an output but is an input`,
			Paths:   []string{"tasks[1].resources.inputs[0].from"},
		},
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFrom(tt.tasks)
			if err == nil {
				t.Error("Pipeline.validateFrom() did not return error for invalid pipeline task resources")
			}
			if d := cmp.Diff(tt.expectedError.Error(), err.Error(), cmpopts.IgnoreUnexported(apis.FieldError{})); d != "" {
				t.Errorf("PipelineSpec.Validate() errors diff %s", diff.PrintWantGot(d))
			}
		})
	}
}

func TestValidateDeclaredResources_Success(t *testing.T) {
	tests := []struct {
		name      string
		resources []PipelineDeclaredResource
		tasks     []PipelineTask
	}{{
		name: "valid resource declarations and usage",
		resources: []PipelineDeclaredResource{{
			Name: "great-resource", Type: PipelineResourceTypeGit,
		}, {
			Name: "wonderful-resource", Type: PipelineResourceTypeImage,
		}},
		tasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			Resources: &PipelineTaskResources{
				Inputs: []PipelineTaskInputResource{{
					Name: "some-workspace", Resource: "great-resource",
				}},
				Outputs: []PipelineTaskOutputResource{{
					Name: "some-imagee", Resource: "wonderful-resource",
				}},
			},
			Conditions: []PipelineTaskCondition{{
				ConditionRef: "some-condition",
				Resources: []PipelineTaskInputResource{{
					Name: "some-workspace", Resource: "great-resource",
				}},
			}},
		}, {
			Name:    "foo",
			TaskRef: &TaskRef{Name: "foo-task"},
			Resources: &PipelineTaskResources{
				Inputs: []PipelineTaskInputResource{{
					Name: "some-image", Resource: "wonderful-resource", From: []string{"bar"},
				}},
			},
			Conditions: []PipelineTaskCondition{{
				ConditionRef: "some-condition-2",
				Resources: []PipelineTaskInputResource{{
					Name: "some-image", Resource: "wonderful-resource", From: []string{"bar"},
				}},
			}},
		}},
	}, {
		name: "valid resource declaration with single reference in the pipeline task condition",
		resources: []PipelineDeclaredResource{{
			Name: "great-resource", Type: PipelineResourceTypeGit,
		}},
		tasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			Conditions: []PipelineTaskCondition{{
				ConditionRef: "some-condition",
				Resources: []PipelineTaskInputResource{{
					Name: "some-workspace", Resource: "great-resource",
				}},
			}},
		}},
	}, {
		name: "valid resource declarations with extra resources, not used in any pipeline task",
		resources: []PipelineDeclaredResource{{
			Name: "great-resource", Type: PipelineResourceTypeGit,
		}, {
			Name: "awesome-resource", Type: PipelineResourceTypeImage,
		}, {
			Name: "yet-another-great-resource", Type: PipelineResourceTypeGit,
		}, {
			Name: "yet-another-awesome-resource", Type: PipelineResourceTypeImage,
		}},
		tasks: []PipelineTask{{
			Name:    "foo",
			TaskRef: &TaskRef{Name: "foo-task"},
			Resources: &PipelineTaskResources{
				Inputs: []PipelineTaskInputResource{{
					Name: "the-resource", Resource: "great-resource",
				}},
				Outputs: []PipelineTaskOutputResource{{
					Name: "the-awesome-resource", Resource: "awesome-resource",
				}},
			},
		}},
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDeclaredResources(tt.resources, tt.tasks, []PipelineTask{})
			if err != nil {
				t.Errorf("Pipeline.validateDeclaredResources() returned error for valid resource declarations: %v", err)
			}
		})
	}
}

func TestValidateDeclaredResources_Failure(t *testing.T) {
	tests := []struct {
		name          string
		resources     []PipelineDeclaredResource
		tasks         []PipelineTask
		expectedError apis.FieldError
	}{{
		name: "duplicate resource declaration - resource declarations must be unique",
		resources: []PipelineDeclaredResource{{
			Name: "duplicate-resource", Type: PipelineResourceTypeGit,
		}, {
			Name: "duplicate-resource", Type: PipelineResourceTypeGit,
		}},
		tasks: []PipelineTask{{
			Name:    "foo",
			TaskRef: &TaskRef{Name: "foo-task"},
			Resources: &PipelineTaskResources{
				Inputs: []PipelineTaskInputResource{{
					Name: "the-resource", Resource: "duplicate-resource",
				}},
			},
		}},
		expectedError: apis.FieldError{
			Message: `invalid value: resource with name "duplicate-resource" appears more than once`,
			Paths:   []string{"resources"},
		},
	}, {
		name: "output resource is missing from resource declarations",
		resources: []PipelineDeclaredResource{{
			Name: "great-resource", Type: PipelineResourceTypeGit,
		}},
		tasks: []PipelineTask{{
			Name:    "foo",
			TaskRef: &TaskRef{Name: "foo-task"},
			Resources: &PipelineTaskResources{
				Inputs: []PipelineTaskInputResource{{
					Name: "the-resource", Resource: "great-resource",
				}},
				Outputs: []PipelineTaskOutputResource{{
					Name: "the-magic-resource", Resource: "missing-resource",
				}},
			},
		}},
		expectedError: apis.FieldError{
			Message: `invalid value: pipeline declared resources didn't match usage in Tasks: Didn't provide required values: [missing-resource]`,
			Paths:   []string{"resources"},
		},
	}, {
		name: "input resource is missing from resource declarations",
		resources: []PipelineDeclaredResource{{
			Name: "great-resource", Type: PipelineResourceTypeGit,
		}},
		tasks: []PipelineTask{{
			Name:    "foo",
			TaskRef: &TaskRef{Name: "foo-task"},
			Resources: &PipelineTaskResources{
				Inputs: []PipelineTaskInputResource{{
					Name: "the-resource", Resource: "missing-resource",
				}},
				Outputs: []PipelineTaskOutputResource{{
					Name: "the-magic-resource", Resource: "great-resource",
				}},
			},
		}},
		expectedError: apis.FieldError{
			Message: `invalid value: pipeline declared resources didn't match usage in Tasks: Didn't provide required values: [missing-resource]`,
			Paths:   []string{"resources"},
		},
	}, {
		name: "invalid condition only resource -" +
			" pipeline task condition referring to a resource which is missing from resource declarations",
		tasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			Conditions: []PipelineTaskCondition{{
				ConditionRef: "some-condition",
				Resources: []PipelineTaskInputResource{{
					Name: "some-workspace", Resource: "missing-resource",
				}},
			}},
		}},
		expectedError: apis.FieldError{
			Message: `invalid value: pipeline declared resources didn't match usage in Tasks: Didn't provide required values: [missing-resource]`,
			Paths:   []string{"resources"},
		},
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDeclaredResources(tt.resources, tt.tasks, []PipelineTask{})
			if err == nil {
				t.Errorf("Pipeline.validateDeclaredResources() did not return error for invalid resource declarations")
			}
			if d := cmp.Diff(tt.expectedError.Error(), err.Error(), cmpopts.IgnoreUnexported(apis.FieldError{})); d != "" {
				t.Errorf("PipelineSpec.Validate() errors diff %s", diff.PrintWantGot(d))
			}
		})
	}
}

func TestValidateGraph_Success(t *testing.T) {
	desc := "valid dependency graph with multiple tasks"
	tasks := []PipelineTask{{
		Name: "foo", TaskRef: &TaskRef{Name: "foo-task"},
	}, {
		Name: "bar", TaskRef: &TaskRef{Name: "bar-task"},
	}, {
		Name: "foo1", TaskRef: &TaskRef{Name: "foo-task"}, RunAfter: []string{"foo"},
	}, {
		Name: "bar1", TaskRef: &TaskRef{Name: "bar-task"}, RunAfter: []string{"bar"},
	}, {
		Name: "foo-bar", TaskRef: &TaskRef{Name: "bar-task"}, RunAfter: []string{"foo1", "bar1"},
	}}
	if err := validateGraph(tasks); err != nil {
		t.Errorf("Pipeline.validateGraph() returned error for valid DAG of pipeline tasks: %s: %v", desc, err)
	}
}

func TestValidateGraph_Failure(t *testing.T) {
	desc := "invalid dependency graph between the tasks with cyclic dependency"
	tasks := []PipelineTask{{
		Name: "foo", TaskRef: &TaskRef{Name: "foo-task"}, RunAfter: []string{"bar"},
	}, {
		Name: "bar", TaskRef: &TaskRef{Name: "bar-task"}, RunAfter: []string{"foo"},
	}}
	if err := validateGraph(tasks); err == nil {
		t.Error("Pipeline.validateGraph() did not return error for invalid DAG of pipeline tasks:", desc)

	}
}

func TestValidateParamResults_Success(t *testing.T) {
	desc := "valid pipeline task referencing task result along with parameter variable"
	tasks := []PipelineTask{{
		TaskSpec: &EmbeddedTask{TaskSpec: TaskSpec{
			Results: []TaskResult{{
				Name: "output",
			}},
			Steps: []Step{{
				Container: corev1.Container{Name: "foo", Image: "bar"},
			}},
		}},
		Name: "a-task",
	}, {
		Name:    "foo",
		TaskRef: &TaskRef{Name: "foo-task"},
		Params: []Param{{
			Name: "a-param", Value: ArrayOrString{Type: ParamTypeString, StringVal: "$(params.foo) and $(tasks.a-task.results.output)"},
		}},
	}}
	if err := validateParamResults(tasks); err != nil {
		t.Errorf("Pipeline.validateParamResults() returned error for valid pipeline: %s: %v", desc, err)
	}
}

func TestValidateParamResults_Failure(t *testing.T) {
	desc := "invalid pipeline task referencing task results with malformed variable substitution expression"
	tasks := []PipelineTask{{
		Name: "a-task", TaskRef: &TaskRef{Name: "a-task"},
	}, {
		Name: "b-task", TaskRef: &TaskRef{Name: "b-task"},
		Params: []Param{{
			Name: "a-param", Value: ArrayOrString{Type: ParamTypeString, StringVal: "$(tasks.a-task.resultTypo.bResult)"}}},
	}}
	expectedError := apis.FieldError{
		Message: `invalid value: expected all of the expressions [tasks.a-task.resultTypo.bResult] to be result expressions but only [] were`,
		Paths:   []string{"tasks[1].params[a-param].value"},
	}
	err := validateParamResults(tasks)
	if err == nil {
		t.Errorf("Pipeline.validateParamResults() did not return error for invalid pipeline: %s", desc)
	}
	if d := cmp.Diff(expectedError.Error(), err.Error(), cmpopts.IgnoreUnexported(apis.FieldError{})); d != "" {
		t.Errorf("Pipeline.validateParamResults() errors diff %s", diff.PrintWantGot(d))
	}
}

func TestValidatePipelineResults_Success(t *testing.T) {
	desc := "valid pipeline with valid pipeline results syntax"
	results := []PipelineResult{{
		Name:        "my-pipeline-result",
		Description: "this is my pipeline result",
		Value:       "$(tasks.a-task.results.output)",
	}}
	if err := validatePipelineResults(results); err != nil {
		t.Errorf("Pipeline.validatePipelineResults() returned error for valid pipeline: %s: %v", desc, err)
	}
}

func TestValidatePipelineResults_Failure(t *testing.T) {
	desc := "invalid pipeline result reference"
	results := []PipelineResult{{
		Name:        "my-pipeline-result",
		Description: "this is my pipeline result",
		Value:       "$(tasks.a-task.results.output.output)",
	}}
	expectedError := apis.FieldError{
		Message: `invalid value: expected all of the expressions [tasks.a-task.results.output.output] to be result expressions but only [] were`,
		Paths:   []string{"results[0].value"},
	}
	err := validatePipelineResults(results)
	if err == nil {
		t.Errorf("Pipeline.validatePipelineResults() did not return for invalid pipeline: %s", desc)
	}
	if d := cmp.Diff(expectedError.Error(), err.Error(), cmpopts.IgnoreUnexported(apis.FieldError{})); d != "" {
		t.Errorf("Pipeline.validateParamResults() errors diff %s", diff.PrintWantGot(d))
	}
}

func TestValidatePipelineParameterVariables_Success(t *testing.T) {
	tests := []struct {
		name   string
		params []ParamSpec
		tasks  []PipelineTask
	}{{
		name: "valid string parameter variables",
		params: []ParamSpec{{
			Name: "baz", Type: ParamTypeString,
		}, {
			Name: "foo-is-baz", Type: ParamTypeString,
		}},
		tasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			Params: []Param{{
				Name: "a-param", Value: ArrayOrString{StringVal: "$(baz) and $(foo-is-baz)"},
			}},
		}},
	}, {
		name: "valid string parameter variables in when expression",
		params: []ParamSpec{{
			Name: "baz", Type: ParamTypeString,
		}, {
			Name: "foo-is-baz", Type: ParamTypeString,
		}},
		tasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			WhenExpressions: []WhenExpression{{
				Input:    "$(params.baz)",
				Operator: selection.In,
				Values:   []string{"foo"},
			}, {
				Input:    "baz",
				Operator: selection.In,
				Values:   []string{"$(params.foo-is-baz)"},
			}},
		}},
	}, {
		name: "valid string parameter variables in input, array reference in values in when expression",
		params: []ParamSpec{{
			Name: "baz", Type: ParamTypeString,
		}, {
			Name: "foo", Type: ParamTypeArray, Default: &ArrayOrString{Type: ParamTypeArray, ArrayVal: []string{"anarray", "elements"}},
		}},
		tasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			WhenExpressions: []WhenExpression{{
				Input:    "$(params.baz)",
				Operator: selection.In,
				Values:   []string{"$(params.foo[*])"},
			}},
		}},
	}, {
		name: "valid array parameter variables",
		params: []ParamSpec{{
			Name: "baz", Type: ParamTypeArray, Default: &ArrayOrString{Type: ParamTypeArray, ArrayVal: []string{"some", "default"}},
		}, {
			Name: "foo-is-baz", Type: ParamTypeArray,
		}},
		tasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			Params: []Param{{
				Name: "a-param", Value: ArrayOrString{ArrayVal: []string{"$(baz)", "and", "$(foo-is-baz)"}},
			}},
		}},
	}, {
		name: "valid star array parameter variables",
		params: []ParamSpec{{
			Name: "baz", Type: ParamTypeArray, Default: &ArrayOrString{Type: ParamTypeArray, ArrayVal: []string{"some", "default"}},
		}, {
			Name: "foo-is-baz", Type: ParamTypeArray,
		}},
		tasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			Params: []Param{{
				Name: "a-param", Value: ArrayOrString{ArrayVal: []string{"$(baz[*])", "and", "$(foo-is-baz[*])"}},
			}},
		}},
	}, {
		name: "pipeline parameter nested in task parameter",
		params: []ParamSpec{{
			Name: "baz", Type: ParamTypeString,
		}},
		tasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			Params: []Param{{
				Name: "a-param", Value: ArrayOrString{StringVal: "$(input.workspace.$(baz))"},
			}},
		}},
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePipelineParameterVariables(tt.tasks, tt.params)
			if err != nil {
				t.Errorf("Pipeline.validatePipelineParameterVariables() returned error for valid pipeline parameters: %v", err)
			}
		})
	}
}

func TestValidatePipelineParameterVariables_Failure(t *testing.T) {
	tests := []struct {
		name          string
		params        []ParamSpec
		tasks         []PipelineTask
		expectedError apis.FieldError
	}{{
		name: "invalid pipeline task with a parameter which is missing from the param declarations",
		tasks: []PipelineTask{{
			Name:    "foo",
			TaskRef: &TaskRef{Name: "foo-task"},
			Params: []Param{{
				Name: "a-param", Value: ArrayOrString{Type: ParamTypeString, StringVal: "$(params.does-not-exist)"},
			}},
		}},
		expectedError: apis.FieldError{
			Message: `non-existent variable in "$(params.does-not-exist)"`,
			Paths:   []string{"[0].params[a-param]"},
		},
	}, {
		name: "invalid string parameter variables in when expression, missing input param from the param declarations",
		tasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			WhenExpressions: []WhenExpression{{
				Input:    "$(params.baz)",
				Operator: selection.In,
				Values:   []string{"foo"},
			}},
		}},
		expectedError: apis.FieldError{
			Message: `non-existent variable in "$(params.baz)"`,
			Paths:   []string{"[0].when[0].input"},
		},
	}, {
		name: "invalid string parameter variables in when expression, missing values param from the param declarations",
		tasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			WhenExpressions: []WhenExpression{{
				Input:    "bax",
				Operator: selection.In,
				Values:   []string{"$(params.foo-is-baz)"},
			}},
		}},
		expectedError: apis.FieldError{
			Message: `non-existent variable in "$(params.foo-is-baz)"`,
			Paths:   []string{"[0].when[0].values"},
		},
	}, {
		name: "invalid string parameter variables in when expression, array reference in input",
		params: []ParamSpec{{
			Name: "foo", Type: ParamTypeArray, Default: &ArrayOrString{Type: ParamTypeArray, ArrayVal: []string{"anarray", "elements"}},
		}},
		tasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			WhenExpressions: []WhenExpression{{
				Input:    "$(params.foo)",
				Operator: selection.In,
				Values:   []string{"foo"},
			}},
		}},
		expectedError: apis.FieldError{
			Message: `variable type invalid in "$(params.foo)"`,
			Paths:   []string{"[0].when[0].input"},
		},
	}, {
		name: "Invalid array parameter variable in when expression, array reference in input with array notation [*]",
		params: []ParamSpec{{
			Name: "foo", Type: ParamTypeArray, Default: &ArrayOrString{Type: ParamTypeArray, ArrayVal: []string{"anarray", "elements"}},
		}},
		tasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			WhenExpressions: []WhenExpression{{
				Input:    "$(params.foo)[*]",
				Operator: selection.In,
				Values:   []string{"$(params.foo[*])"},
			}},
		}},
		expectedError: apis.FieldError{
			Message: `variable type invalid in "$(params.foo)[*]"`,
			Paths:   []string{"[0].when[0].input"},
		},
	}, {
		name: "invalid pipeline task with a parameter combined with missing param from the param declarations",
		params: []ParamSpec{{
			Name: "foo", Type: ParamTypeString,
		}},
		tasks: []PipelineTask{{
			Name:    "foo-task",
			TaskRef: &TaskRef{Name: "foo-task"},
			Params: []Param{{
				Name: "a-param", Value: ArrayOrString{Type: ParamTypeString, StringVal: "$(params.foo) and $(params.does-not-exist)"},
			}},
		}},
		expectedError: apis.FieldError{
			Message: `non-existent variable in "$(params.foo) and $(params.does-not-exist)"`,
			Paths:   []string{"[0].params[a-param]"},
		},
	}, {
		name: "invalid pipeline task with two parameters and one of them missing from the param declarations",
		params: []ParamSpec{{
			Name: "foo", Type: ParamTypeString,
		}},
		tasks: []PipelineTask{{
			Name:    "foo-task",
			TaskRef: &TaskRef{Name: "foo-task"},
			Params: []Param{{
				Name: "a-param", Value: ArrayOrString{Type: ParamTypeString, StringVal: "$(params.foo)"},
			}, {
				Name: "b-param", Value: ArrayOrString{Type: ParamTypeString, StringVal: "$(params.does-not-exist)"},
			}},
		}},
		expectedError: apis.FieldError{
			Message: `non-existent variable in "$(params.does-not-exist)"`,
			Paths:   []string{"[0].params[b-param]"},
		},
	}, {
		name: "invalid parameter type",
		params: []ParamSpec{{
			Name: "foo", Type: "invalidtype",
		}},
		tasks: []PipelineTask{{
			Name:    "foo",
			TaskRef: &TaskRef{Name: "foo-task"},
		}},
		expectedError: apis.FieldError{
			Message: `invalid value: invalidtype`,
			Paths:   []string{"params[foo].type"},
		},
	}, {
		name: "array parameter mismatching default type",
		params: []ParamSpec{{
			Name: "foo", Type: ParamTypeArray, Default: &ArrayOrString{Type: ParamTypeString, StringVal: "astring"},
		}},
		tasks: []PipelineTask{{
			Name:    "foo",
			TaskRef: &TaskRef{Name: "foo-task"},
		}},
		expectedError: apis.FieldError{
			Message: `"array" type does not match default value's type: "string"`,
			Paths:   []string{"params[foo].default.type", "params[foo].type"},
		},
	}, {
		name: "string parameter mismatching default type",
		params: []ParamSpec{{
			Name: "foo", Type: ParamTypeString, Default: &ArrayOrString{Type: ParamTypeArray, ArrayVal: []string{"anarray", "elements"}},
		}},
		tasks: []PipelineTask{{
			Name:    "foo",
			TaskRef: &TaskRef{Name: "foo-task"},
		}},
		expectedError: apis.FieldError{
			Message: `"string" type does not match default value's type: "array"`,
			Paths:   []string{"params[foo].default.type", "params[foo].type"},
		},
	}, {
		name: "array parameter used as string",
		params: []ParamSpec{{
			Name: "baz", Type: ParamTypeString, Default: &ArrayOrString{Type: ParamTypeArray, ArrayVal: []string{"anarray", "elements"}},
		}},
		tasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			Params: []Param{{
				Name: "a-param", Value: ArrayOrString{Type: ParamTypeString, StringVal: "$(params.baz)"},
			}},
		}},
		expectedError: apis.FieldError{
			Message: `"string" type does not match default value's type: "array"`,
			Paths:   []string{"params[baz].default.type", "params[baz].type"},
		},
	}, {
		name: "star array parameter used as string",
		params: []ParamSpec{{
			Name: "baz", Type: ParamTypeString, Default: &ArrayOrString{Type: ParamTypeArray, ArrayVal: []string{"anarray", "elements"}},
		}},
		tasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			Params: []Param{{
				Name: "a-param", Value: ArrayOrString{Type: ParamTypeString, StringVal: "$(params.baz[*])"},
			}},
		}},
		expectedError: apis.FieldError{
			Message: `"string" type does not match default value's type: "array"`,
			Paths:   []string{"params[baz].default.type", "params[baz].type"},
		},
	}, {
		name: "array parameter string template not isolated",
		params: []ParamSpec{{
			Name: "baz", Type: ParamTypeString, Default: &ArrayOrString{Type: ParamTypeArray, ArrayVal: []string{"anarray", "elements"}},
		}},
		tasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			Params: []Param{{
				Name: "a-param", Value: ArrayOrString{Type: ParamTypeArray, ArrayVal: []string{"value: $(params.baz)", "last"}},
			}},
		}},
		expectedError: apis.FieldError{
			Message: `"string" type does not match default value's type: "array"`,
			Paths:   []string{"params[baz].default.type", "params[baz].type"},
		},
	}, {
		name: "star array parameter string template not isolated",
		params: []ParamSpec{{
			Name: "baz", Type: ParamTypeString, Default: &ArrayOrString{Type: ParamTypeArray, ArrayVal: []string{"anarray", "elements"}},
		}},
		tasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			Params: []Param{{
				Name: "a-param", Value: ArrayOrString{Type: ParamTypeArray, ArrayVal: []string{"value: $(params.baz[*])", "last"}},
			}},
		}},
		expectedError: apis.FieldError{
			Message: `"string" type does not match default value's type: "array"`,
			Paths:   []string{"params[baz].default.type", "params[baz].type"},
		},
	}, {
		name: "multiple string parameters with the same name",
		params: []ParamSpec{{
			Name: "baz", Type: ParamTypeString,
		}, {
			Name: "baz", Type: ParamTypeString,
		}},
		tasks: []PipelineTask{{
			Name:    "foo",
			TaskRef: &TaskRef{Name: "foo-task"},
		}},
		expectedError: apis.FieldError{
			Message: `parameter appears more than once`,
			Paths:   []string{"params[baz]"},
		},
	}, {
		name: "multiple array parameters with the same name",
		params: []ParamSpec{{
			Name: "baz", Type: ParamTypeArray,
		}, {
			Name: "baz", Type: ParamTypeArray,
		}},
		tasks: []PipelineTask{{
			Name:    "foo",
			TaskRef: &TaskRef{Name: "foo-task"},
		}},
		expectedError: apis.FieldError{
			Message: `parameter appears more than once`,
			Paths:   []string{"params[baz]"},
		},
	}, {
		name: "multiple different type parameters with the same name",
		params: []ParamSpec{{
			Name: "baz", Type: ParamTypeArray,
		}, {
			Name: "baz", Type: ParamTypeString,
		}},
		tasks: []PipelineTask{{
			Name:    "foo",
			TaskRef: &TaskRef{Name: "foo-task"},
		}},
		expectedError: apis.FieldError{
			Message: `parameter appears more than once`,
			Paths:   []string{"params[baz]"},
		},
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePipelineParameterVariables(tt.tasks, tt.params)
			if err == nil {
				t.Errorf("Pipeline.validatePipelineParameterVariables() did not return error for invalid pipeline parameters")
			}
			if d := cmp.Diff(tt.expectedError.Error(), err.Error(), cmpopts.IgnoreUnexported(apis.FieldError{})); d != "" {
				t.Errorf("PipelineSpec.Validate() errors diff %s", diff.PrintWantGot(d))
			}
		})
	}
}

func TestValidatePipelineWorkspaces_Success(t *testing.T) {
	desc := "unused pipeline spec workspaces do not cause an error"
	workspaces := []PipelineWorkspaceDeclaration{{
		Name: "foo",
	}, {
		Name: "bar",
	}}
	tasks := []PipelineTask{{
		Name: "foo", TaskRef: &TaskRef{Name: "foo"},
	}}
	t.Run(desc, func(t *testing.T) {
		err := validatePipelineWorkspaces(workspaces, tasks, []PipelineTask{})
		if err != nil {
			t.Errorf("Pipeline.validatePipelineWorkspaces() returned error for valid pipeline workspaces: %v", err)
		}
	})
}

func TestValidatePipelineWorkspaces_Failure(t *testing.T) {
	tests := []struct {
		name          string
		workspaces    []PipelineWorkspaceDeclaration
		tasks         []PipelineTask
		expectedError apis.FieldError
	}{{
		name: "workspace bindings relying on a non-existent pipeline workspace cause an error",
		workspaces: []PipelineWorkspaceDeclaration{{
			Name: "foo",
		}},
		tasks: []PipelineTask{{
			Name: "foo", TaskRef: &TaskRef{Name: "foo"},
			Workspaces: []WorkspacePipelineTaskBinding{{
				Name:      "taskWorkspaceName",
				Workspace: "pipelineWorkspaceName",
			}},
		}},
		expectedError: apis.FieldError{
			Message: `invalid value: pipeline task "foo" expects workspace with name "pipelineWorkspaceName" but none exists in pipeline spec`,
			Paths:   []string{"tasks[0].workspaces[0]"},
		},
	}, {
		name: "multiple workspaces sharing the same name are not allowed",
		workspaces: []PipelineWorkspaceDeclaration{{
			Name: "foo",
		}, {
			Name: "foo",
		}},
		tasks: []PipelineTask{{
			Name: "foo", TaskRef: &TaskRef{Name: "foo"},
		}},
		expectedError: apis.FieldError{
			Message: `invalid value: workspace with name "foo" appears more than once`,
			Paths:   []string{"workspaces[1]"},
		},
	}, {
		name: "workspace name must not be empty",
		workspaces: []PipelineWorkspaceDeclaration{{
			Name: "",
		}},
		tasks: []PipelineTask{{
			Name: "foo", TaskRef: &TaskRef{Name: "foo"},
		}},
		expectedError: apis.FieldError{
			Message: `invalid value: workspace 0 has empty name`,
			Paths:   []string{"workspaces[0]"},
		},
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePipelineWorkspaces(tt.workspaces, tt.tasks, []PipelineTask{})
			if err == nil {
				t.Errorf("Pipeline.validatePipelineWorkspaces() did not return error for invalid pipeline workspaces")
			}
			if d := cmp.Diff(tt.expectedError.Error(), err.Error(), cmpopts.IgnoreUnexported(apis.FieldError{})); d != "" {
				t.Errorf("PipelineSpec.Validate() errors diff %s", diff.PrintWantGot(d))
			}
		})
	}
}

func TestValidatePipelineWithFinalTasks_Success(t *testing.T) {
	tests := []struct {
		name string
		p    *Pipeline
	}{{
		name: "valid pipeline with final tasks",
		p: &Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "pipeline"},
			Spec: PipelineSpec{
				Tasks: []PipelineTask{{
					Name:    "non-final-task",
					TaskRef: &TaskRef{Name: "non-final-task"},
				}},
				Finally: []PipelineTask{{
					Name:    "final-task-1",
					TaskRef: &TaskRef{Name: "final-task"},
				}, {
					Name:     "final-task-2",
					TaskSpec: &EmbeddedTask{TaskSpec: getTaskSpec()},
				}},
			},
		},
	}, {
		name: "valid pipeline with resource declarations and their valid usage",
		p: &Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "pipeline"},
			Spec: PipelineSpec{
				Resources: []PipelineDeclaredResource{{
					Name: "great-resource", Type: PipelineResourceTypeGit,
				}, {
					Name: "wonderful-resource", Type: PipelineResourceTypeImage,
				}},
				Tasks: []PipelineTask{{
					Name:    "non-final-task",
					TaskRef: &TaskRef{Name: "bar-task"},
					Resources: &PipelineTaskResources{
						Inputs: []PipelineTaskInputResource{{
							Name: "some-workspace", Resource: "great-resource",
						}},
						Outputs: []PipelineTaskOutputResource{{
							Name: "some-image", Resource: "wonderful-resource",
						}},
					},
					Conditions: []PipelineTaskCondition{{
						ConditionRef: "some-condition",
						Resources: []PipelineTaskInputResource{{
							Name: "some-workspace", Resource: "great-resource",
						}},
					}},
				}},
				Finally: []PipelineTask{{
					Name:    "foo",
					TaskRef: &TaskRef{Name: "foo-task"},
					Resources: &PipelineTaskResources{
						Inputs: []PipelineTaskInputResource{{
							Name: "some-workspace", Resource: "great-resource",
						}},
						Outputs: []PipelineTaskOutputResource{{
							Name: "some-image", Resource: "wonderful-resource",
						}},
					},
				}},
			},
		},
	}, {
		name: "valid pipeline with final tasks referring to task results from a dag task",
		p: &Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "pipeline"},
			Spec: PipelineSpec{
				Tasks: []PipelineTask{{
					Name:    "non-final-task",
					TaskRef: &TaskRef{Name: "non-final-task"},
				}},
				Finally: []PipelineTask{{
					Name:    "final-task-1",
					TaskRef: &TaskRef{Name: "final-task"},
					Params: []Param{{
						Name: "param1", Value: ArrayOrString{Type: ParamTypeString, StringVal: "$(tasks.non-final-task.results.output)"},
					}},
				}},
			},
		},
	}, {
		name: "valid pipeline with final tasks referring to context variables",
		p: &Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "pipeline"},
			Spec: PipelineSpec{
				Tasks: []PipelineTask{{
					Name:    "non-final-task",
					TaskRef: &TaskRef{Name: "non-final-task"},
				}},
				Finally: []PipelineTask{{
					Name:    "final-task-1",
					TaskRef: &TaskRef{Name: "final-task"},
					Params: []Param{{
						Name: "param1", Value: ArrayOrString{Type: ParamTypeString, StringVal: "$(context.pipelineRun.name)"},
					}},
				}},
			},
		},
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.p.Validate(context.Background())
			if err != nil {
				t.Errorf("Pipeline.Validate() returned error for valid pipeline with finally: %v", err)
			}
		})
	}
}

func TestValidatePipelineWithFinalTasks_Failure(t *testing.T) {
	tests := []struct {
		name          string
		p             *Pipeline
		expectedError apis.FieldError
	}{{
		name: "invalid pipeline without any non-final task (tasks set to nil) but at least one final task",
		p: &Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "pipeline"},
			Spec: PipelineSpec{
				Tasks: nil,
				Finally: []PipelineTask{{
					Name:    "final-task",
					TaskRef: &TaskRef{Name: "final-task"},
				}},
			},
		},
		expectedError: apis.FieldError{
			Message: `invalid value: spec.tasks is empty but spec.finally has 1 tasks`,
			Paths:   []string{"spec.finally"},
		},
	}, {
		name: "invalid pipeline without any non-final task (tasks set to empty list of pipeline task) but at least one final task",
		p: &Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "pipeline"},
			Spec: PipelineSpec{
				Tasks: []PipelineTask{{}},
				Finally: []PipelineTask{{
					Name:    "final-task",
					TaskRef: &TaskRef{Name: "final-task"},
				}},
			},
		},
		expectedError: *apis.ErrMissingOneOf("spec.tasks[0].taskRef", "spec.tasks[0].taskSpec").Also(
			&apis.FieldError{
				Message: `invalid value ""`,
				Paths:   []string{"spec.tasks[0].name"},
				Details: "Pipeline Task name must be a valid DNS Label." +
					"For more info refer to https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names",
			}),
	}, {
		name: "invalid pipeline with valid non-final tasks but empty finally section",
		p: &Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "pipeline"},
			Spec: PipelineSpec{
				Tasks: []PipelineTask{{
					Name:    "non-final-task",
					TaskRef: &TaskRef{Name: "non-final-task"},
				}},
				Finally: []PipelineTask{{}},
			},
		},
		expectedError: *apis.ErrMissingOneOf("spec.finally[0].taskRef", "spec.finally[0].taskSpec").Also(
			&apis.FieldError{
				Message: `invalid value ""`,
				Paths:   []string{"spec.finally[0].name"},
				Details: "Pipeline Task name must be a valid DNS Label." +
					"For more info refer to https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names",
			}),
	}, {
		name: "invalid pipeline with duplicate final tasks",
		p: &Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "pipeline"},
			Spec: PipelineSpec{
				Tasks: []PipelineTask{{
					Name:    "non-final-task",
					TaskRef: &TaskRef{Name: "non-final-task"},
				}},
				Finally: []PipelineTask{{
					Name:    "final-task",
					TaskRef: &TaskRef{Name: "final-task"},
				}, {
					Name:    "final-task",
					TaskRef: &TaskRef{Name: "final-task"},
				}},
			},
		},
		expectedError: apis.FieldError{
			Message: `expected exactly one, got both`,
			Paths:   []string{"spec.finally[1].name"},
		},
	}, {
		name: "invalid pipeline with same task name for final and non final task",
		p: &Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "pipeline"},
			Spec: PipelineSpec{
				Tasks: []PipelineTask{{
					Name:    "common-task-name",
					TaskRef: &TaskRef{Name: "non-final-task"},
				}},
				Finally: []PipelineTask{{
					Name:    "common-task-name",
					TaskRef: &TaskRef{Name: "final-task"},
				}},
			},
		},
		expectedError: apis.FieldError{
			Message: `expected exactly one, got both`,
			Paths:   []string{"spec.finally[0].name"},
		},
	}, {
		name: "final task missing taskref and taskspec",
		p: &Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "pipeline"},
			Spec: PipelineSpec{
				Tasks: []PipelineTask{{
					Name:    "non-final-task",
					TaskRef: &TaskRef{Name: "non-final-task"},
				}},
				Finally: []PipelineTask{{
					Name: "final-task",
				}},
			},
		},
		expectedError: apis.FieldError{
			Message: `expected exactly one, got neither`,
			Paths:   []string{"spec.finally[0].taskRef", "spec.finally[0].taskSpec"},
		},
	}, {
		name: "final task with both tasfref and taskspec",
		p: &Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "pipeline"},
			Spec: PipelineSpec{
				Tasks: []PipelineTask{{
					Name:    "non-final-task",
					TaskRef: &TaskRef{Name: "non-final-task"},
				}},
				Finally: []PipelineTask{{
					Name:     "final-task",
					TaskRef:  &TaskRef{Name: "non-final-task"},
					TaskSpec: &EmbeddedTask{TaskSpec: getTaskSpec()},
				}},
			},
		},
		expectedError: apis.FieldError{
			Message: `expected exactly one, got both`,
			Paths:   []string{"spec.finally[0].taskRef", "spec.finally[0].taskSpec"},
		},
	}, {
		name: "extra parameter called final-param provided to final task which is not specified in the Pipeline",
		p: &Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "pipeline"},
			Spec: PipelineSpec{
				Params: []ParamSpec{{
					Name: "foo", Type: ParamTypeString,
				}},
				Tasks: []PipelineTask{{
					Name:    "non-final-task",
					TaskRef: &TaskRef{Name: "non-final-task"},
				}},
				Finally: []PipelineTask{{
					Name:    "final-task",
					TaskRef: &TaskRef{Name: "final-task"},
					Params: []Param{{
						Name: "final-param", Value: ArrayOrString{Type: ParamTypeString, StringVal: "$(params.foo) and $(params.does-not-exist)"},
					}},
				}},
			},
		},
		expectedError: apis.FieldError{
			Message: `non-existent variable in "$(params.foo) and $(params.does-not-exist)"`,
			Paths:   []string{"spec.finally[0].params[final-param]"},
		},
	}, {
		name: "invalid pipeline with invalid final tasks with runAfter and conditions",
		p: &Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "pipeline"},
			Spec: PipelineSpec{
				Tasks: []PipelineTask{{
					Name:    "non-final-task",
					TaskRef: &TaskRef{Name: "non-final-task"},
				}},
				Finally: []PipelineTask{{
					Name:     "final-task-1",
					TaskRef:  &TaskRef{Name: "final-task"},
					RunAfter: []string{"non-final-task"},
				}, {
					Name:    "final-task-2",
					TaskRef: &TaskRef{Name: "final-task"},
					Conditions: []PipelineTaskCondition{{
						ConditionRef: "some-condition",
					}},
				}},
			},
		},
		expectedError: apis.FieldError{
			Message: `invalid value: no runAfter allowed under spec.finally, final task final-task-1 has runAfter specified`,
			Paths:   []string{"spec.finally[0]"},
		},
	}, {
		name: "invalid pipeline - workspace bindings in final task relying on a non-existent pipeline workspace",
		p: &Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "pipeline"},
			Spec: PipelineSpec{
				Tasks: []PipelineTask{{
					Name: "non-final-task", TaskRef: &TaskRef{Name: "foo"},
				}},
				Finally: []PipelineTask{{
					Name: "final-task", TaskRef: &TaskRef{Name: "foo"},
					Workspaces: []WorkspacePipelineTaskBinding{{
						Name:      "shared-workspace",
						Workspace: "pipeline-shared-workspace",
					}},
				}},
				Workspaces: []WorkspacePipelineDeclaration{{
					Name: "foo",
				}},
			},
		},
		expectedError: apis.FieldError{
			Message: `invalid value: pipeline task "final-task" expects workspace with name "pipeline-shared-workspace" but none exists in pipeline spec`,
			Paths:   []string{"spec.finally[0].workspaces[0]"},
		},
	}, {
		name: "invalid pipeline with no tasks under tasks section and empty finally section",
		p: &Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "pipeline"},
			Spec: PipelineSpec{
				Finally: []PipelineTask{},
			},
		},
		expectedError: *apis.ErrGeneric("expected at least one, got none", "spec.description", "spec.params", "spec.resources", "spec.tasks", "spec.workspaces"),
	}, {
		name: "invalid pipeline with final tasks referring to invalid context variables",
		p: &Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "pipeline"},
			Spec: PipelineSpec{
				Tasks: []PipelineTask{{
					Name:    "non-final-task",
					TaskRef: &TaskRef{Name: "non-final-task"},
				}},
				Finally: []PipelineTask{{
					Name:    "final-task-1",
					TaskRef: &TaskRef{Name: "final-task"},
					Params: []Param{{
						Name: "param1", Value: ArrayOrString{Type: ParamTypeString, StringVal: "$(context.pipelineRun.missing)"},
					}},
				}},
			},
		},
		expectedError: apis.FieldError{
			Message: `non-existent variable in "$(context.pipelineRun.missing)"`,
			Paths:   []string{"spec.finally.value"},
		},
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.p.Validate(context.Background())
			if err == nil {
				t.Errorf("Pipeline.Validate() did not return error for invalid pipeline with finally")
			}
			if d := cmp.Diff(tt.expectedError.Error(), err.Error(), cmpopts.IgnoreUnexported(apis.FieldError{})); d != "" {
				t.Errorf("PipelineSpec.Validate() errors diff %s", diff.PrintWantGot(d))
			}
		})
	}
}

func TestValidateTasksAndFinallySection_Success(t *testing.T) {
	tests := []struct {
		name string
		ps   *PipelineSpec
	}{{
		name: "pipeline with tasks and final tasks",
		ps: &PipelineSpec{
			Tasks: []PipelineTask{{
				Name: "non-final-task", TaskRef: &TaskRef{Name: "foo"},
			}},
			Finally: []PipelineTask{{
				Name: "final-task", TaskRef: &TaskRef{Name: "foo"},
			}},
		},
	}, {
		name: "valid pipeline with tasks and finally section without any tasks",
		ps: &PipelineSpec{
			Tasks: []PipelineTask{{
				Name: "my-task", TaskRef: &TaskRef{Name: "foo"},
			}},
			Finally: nil,
		},
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTasksAndFinallySection(tt.ps)
			if err != nil {
				t.Errorf("Pipeline.ValidateTasksAndFinallySection() returned error for valid pipeline with finally: %v", err)
			}
		})
	}
}

func TestValidateTasksAndFinallySection_Failure(t *testing.T) {
	desc := "invalid pipeline with empty tasks and a few final tasks"
	ps := &PipelineSpec{
		Tasks: nil,
		Finally: []PipelineTask{{
			Name: "final-task", TaskRef: &TaskRef{Name: "foo"},
		}},
	}
	expectedError := apis.FieldError{
		Message: `invalid value: spec.tasks is empty but spec.finally has 1 tasks`,
		Paths:   []string{"finally"},
	}
	err := validateTasksAndFinallySection(ps)
	if err == nil {
		t.Errorf("Pipeline.ValidateTasksAndFinallySection() did not return error for invalid pipeline with finally: %s", desc)
	}
	if d := cmp.Diff(expectedError.Error(), err.Error(), cmpopts.IgnoreUnexported(apis.FieldError{})); d != "" {
		t.Errorf("Pipeline.validateParamResults() errors diff %s", diff.PrintWantGot(d))
	}
}

func TestValidateFinalTasks_Failure(t *testing.T) {
	tests := []struct {
		name          string
		tasks         []PipelineTask
		finalTasks    []PipelineTask
		expectedError apis.FieldError
	}{{
		name: "invalid pipeline with final task specifying runAfter",
		finalTasks: []PipelineTask{{
			Name:     "final-task",
			TaskRef:  &TaskRef{Name: "final-task"},
			RunAfter: []string{"non-final-task"},
		}},
		expectedError: apis.FieldError{
			Message: `invalid value: no runAfter allowed under spec.finally, final task final-task has runAfter specified`,
			Paths:   []string{"finally[0]"},
		},
	}, {
		name: "invalid pipeline with final task specifying conditions",
		finalTasks: []PipelineTask{{
			Name:    "final-task",
			TaskRef: &TaskRef{Name: "final-task"},
			Conditions: []PipelineTaskCondition{{
				ConditionRef: "some-condition",
			}},
		}},
		expectedError: apis.FieldError{
			Message: `invalid value: no conditions allowed under spec.finally, final task final-task has conditions specified`,
			Paths:   []string{"finally[0]"},
		},
	}, {
		name: "invalid pipeline with final task output resources referring to other task input",
		finalTasks: []PipelineTask{{
			Name:    "final-task",
			TaskRef: &TaskRef{Name: "final-task"},
			Resources: &PipelineTaskResources{
				Inputs: []PipelineTaskInputResource{{
					Name: "final-input-2", Resource: "great-resource", From: []string{"task"},
				}},
			},
		}},
		expectedError: apis.FieldError{
			Message: `no from allowed under inputs, final task final-input-2 has from specified`,
			Paths:   []string{"finally[0].resources.inputs[0]"},
		},
	}, {
		name: "invalid pipeline with final tasks having task results reference from a final task",
		finalTasks: []PipelineTask{{
			Name:    "final-task-1",
			TaskRef: &TaskRef{Name: "final-task"},
		}, {
			Name:    "final-task-2",
			TaskRef: &TaskRef{Name: "final-task"},
			Params: []Param{{
				Name: "param1", Value: ArrayOrString{Type: ParamTypeString, StringVal: "$(tasks.final-task-1.results.output)"},
			}},
		}},
		expectedError: apis.FieldError{
			Message: `invalid value: invalid task result reference, final task param param1 has task result reference from a final task`,
			Paths:   []string{"finally[1].params"},
		},
	}, {
		name: "invalid pipeline with final tasks having task results reference from non existent dag task",
		finalTasks: []PipelineTask{{
			Name:    "final-task",
			TaskRef: &TaskRef{Name: "final-task"},
			Params: []Param{{
				Name: "param1", Value: ArrayOrString{Type: ParamTypeString, StringVal: "$(tasks.no-dag-task-1.results.output)"},
			}},
		}},
		expectedError: apis.FieldError{
			Message: `invalid value: invalid task result reference, final task param param1 has task result reference from a task which is not defined in the pipeline`,
			Paths:   []string{"finally[0].params"},
		},
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFinalTasks(tt.tasks, tt.finalTasks)
			if err == nil {
				t.Errorf("Pipeline.ValidateFinalTasks() did not return error for invalid pipeline")
			}
			if d := cmp.Diff(tt.expectedError.Error(), err.Error(), cmpopts.IgnoreUnexported(apis.FieldError{})); d != "" {
				t.Errorf("PipelineSpec.Validate() errors diff %s", diff.PrintWantGot(d))
			}
		})
	}
}

func TestContextValid(t *testing.T) {
	tests := []struct {
		name  string
		tasks []PipelineTask
	}{{
		name: "valid string context variable for task name",
		tasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			Params: []Param{{
				Name: "a-param", Value: ArrayOrString{StringVal: "$(context.pipeline.name)"},
			}},
		}},
	}, {
		name: "valid string context variable for taskrun name",
		tasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			Params: []Param{{
				Name: "a-param", Value: ArrayOrString{StringVal: "$(context.pipelineRun.name)"},
			}},
		}},
	}, {
		name: "valid string context variable for taskRun namespace",
		tasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			Params: []Param{{
				Name: "a-param", Value: ArrayOrString{StringVal: "$(context.pipelineRun.namespace)"},
			}},
		}},
	}, {
		name: "valid string context variable for taskRun uid",
		tasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			Params: []Param{{
				Name: "a-param", Value: ArrayOrString{StringVal: "$(context.pipelineRun.uid)"},
			}},
		}},
	}, {
		name: "valid array context variables for task and taskRun names",
		tasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			Params: []Param{{
				Name: "a-param", Value: ArrayOrString{ArrayVal: []string{"$(context.pipeline.name)", "and", "$(context.pipelineRun.name)"}},
			}},
		}},
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validatePipelineContextVariables(tt.tasks); err != nil {
				t.Errorf("Pipeline.validatePipelineContextVariables() returned error for valid pipeline context variables: %v", err)
			}
		})
	}
}

func TestContextInvalid(t *testing.T) {
	tests := []struct {
		name          string
		tasks         []PipelineTask
		expectedError apis.FieldError
	}{{
		name: "invalid string context variable for pipeline",
		tasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			Params: []Param{{
				Name: "a-param", Value: ArrayOrString{StringVal: "$(context.pipeline.missing)"},
			}},
		}},
		expectedError: apis.FieldError{
			Message: `non-existent variable in "$(context.pipeline.missing)"`,
			Paths:   []string{"value"},
		},
	}, {
		name: "invalid string context variable for pipelineRun",
		tasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			Params: []Param{{
				Name: "a-param", Value: ArrayOrString{StringVal: "$(context.pipelineRun.missing)"},
			}},
		}},
		expectedError: apis.FieldError{
			Message: `non-existent variable in "$(context.pipelineRun.missing)"`,
			Paths:   []string{"value"},
		},
	}, {
		name: "invalid array context variables for pipeline and pipelineRun",
		tasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			Params: []Param{{
				Name: "a-param", Value: ArrayOrString{ArrayVal: []string{"$(context.pipeline.missing)", "and", "$(context.pipelineRun.missing)"}},
			}},
		}},
		expectedError: *apis.ErrGeneric(`non-existent variable in "$(context.pipeline.missing)"`, "value").Also(
			apis.ErrGeneric(`non-existent variable in "$(context.pipelineRun.missing)"`, "value")),
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePipelineContextVariables(tt.tasks)
			if err == nil {
				t.Errorf("Pipeline.validatePipelineContextVariables() did not return error for invalid pipeline parameters: %s", tt.tasks[0].Params)
			}
			if d := cmp.Diff(tt.expectedError.Error(), err.Error(), cmpopts.IgnoreUnexported(apis.FieldError{})); d != "" {
				t.Errorf("PipelineSpec.Validate() errors diff %s", diff.PrintWantGot(d))
			}
		})
	}
}

func TestPipelineTasksExecutionStatus(t *testing.T) {
	tests := []struct {
		name          string
		tasks         []PipelineTask
		finalTasks    []PipelineTask
		expectedError apis.FieldError
	}{{
		name: "valid string variable in finally accessing pipelineTask status",
		tasks: []PipelineTask{{
			Name: "foo",
		}},
		finalTasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			Params: []Param{{
				Name: "foo-status", Value: ArrayOrString{Type: ParamTypeString, StringVal: "$(tasks.foo.status)"},
			}, {
				Name: "tasks-status", Value: ArrayOrString{Type: ParamTypeString, StringVal: "$(tasks.status)"},
			}},
			WhenExpressions: WhenExpressions{{
				Input:    "$(tasks.foo.status)",
				Operator: selection.In,
				Values:   []string{"Failure"},
			}, {
				Input:    "$(tasks.status)",
				Operator: selection.In,
				Values:   []string{"Success"},
			}},
		}},
	}, {
		name: "valid task result reference with status as a variable must not cause validation failure",
		tasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			Params: []Param{{
				Name: "foo-status", Value: ArrayOrString{Type: ParamTypeString, StringVal: "$(tasks.foo.results.status)"},
			}},
			WhenExpressions: WhenExpressions{WhenExpression{
				Input:    "$(tasks.foo.results.status)",
				Operator: selection.In,
				Values:   []string{"Failure"},
			}},
		}},
	}, {
		name: "valid variable concatenated with extra string in finally accessing pipelineTask status",
		tasks: []PipelineTask{{
			Name: "foo",
		}},
		finalTasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			Params: []Param{{
				Name: "foo-status", Value: ArrayOrString{Type: ParamTypeString, StringVal: "Execution status of foo is $(tasks.foo.status)."},
			}},
		}},
	}, {
		name: "valid variable concatenated with other param in finally accessing pipelineTask status",
		tasks: []PipelineTask{{
			Name: "foo",
		}},
		finalTasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			Params: []Param{{
				Name: "foo-status", Value: ArrayOrString{Type: ParamTypeString, StringVal: "Execution status of $(tasks.taskname) is $(tasks.foo.status)."},
			}},
		}},
	}, {
		name: "invalid string variable in dag task accessing pipelineTask status",
		tasks: []PipelineTask{{
			Name:    "foo",
			TaskRef: &TaskRef{Name: "foo-task"},
			Params: []Param{{
				Name: "bar-status", Value: ArrayOrString{Type: ParamTypeString, StringVal: "$(tasks.bar.status)"},
			}},
			WhenExpressions: WhenExpressions{WhenExpression{
				Input:    "$(tasks.bar.status)",
				Operator: selection.In,
				Values:   []string{"foo"},
			}},
		}},
		expectedError: *apis.ErrGeneric("").Also(&apis.FieldError{
			Message: `invalid value: pipeline tasks can not refer to execution status of any other pipeline task or aggregate status of tasks`,
			Paths:   []string{"tasks[0].params[bar-status].value"},
		}).Also(&apis.FieldError{
			Message: `invalid value: when expressions in pipeline tasks can not refer to execution status of any other pipeline task or aggregate status of tasks`,
			Paths:   []string{"tasks[0].when[0]"},
		}),
	}, {
		name: "invalid string variable in dag task accessing aggregate status of tasks",
		tasks: []PipelineTask{{
			Name:    "foo",
			TaskRef: &TaskRef{Name: "foo-task"},
			Params: []Param{{
				Name: "tasks-status", Value: ArrayOrString{Type: ParamTypeString, StringVal: "$(tasks.status)"},
			}},
		}},
		expectedError: apis.FieldError{
			Message: `invalid value: pipeline tasks can not refer to execution status of any other pipeline task or aggregate status of tasks`,
			Paths:   []string{"tasks[0].params[tasks-status].value"},
		},
	}, {
		name: "invalid variable concatenated with extra string in dag task accessing pipelineTask status",
		tasks: []PipelineTask{{
			Name:    "foo",
			TaskRef: &TaskRef{Name: "foo-task"},
			Params: []Param{{
				Name: "bar-status", Value: ArrayOrString{Type: ParamTypeString, StringVal: "Execution status of bar is $(tasks.bar.status)"},
			}},
		}},
		expectedError: apis.FieldError{
			Message: `invalid value: pipeline tasks can not refer to execution status of any other pipeline task or aggregate status of tasks`,
			Paths:   []string{"tasks[0].params[bar-status].value"},
		},
	}, {
		name: "invalid array variable in dag task accessing pipelineTask status",
		tasks: []PipelineTask{{
			Name:    "foo",
			TaskRef: &TaskRef{Name: "foo-task"},
			Params: []Param{{
				Name: "bar-status", Value: ArrayOrString{Type: ParamTypeArray, ArrayVal: []string{"$(tasks.bar.status)"}},
			}},
		}},
		expectedError: apis.FieldError{
			Message: `invalid value: pipeline tasks can not refer to execution status of any other pipeline task or aggregate status of tasks`,
			Paths:   []string{"tasks[0].params[bar-status].value"},
		},
	}, {
		name: "invalid array variable in dag task accessing aggregate tasks status",
		tasks: []PipelineTask{{
			Name:    "foo",
			TaskRef: &TaskRef{Name: "foo-task"},
			Params: []Param{{
				Name: "tasks-status", Value: ArrayOrString{Type: ParamTypeArray, ArrayVal: []string{"$(tasks.status)"}},
			}},
		}},
		expectedError: apis.FieldError{
			Message: `invalid value: pipeline tasks can not refer to execution status of any other pipeline task or aggregate status of tasks`,
			Paths:   []string{"tasks[0].params[tasks-status].value"},
		},
	}, {
		name: "invalid string variable in finally accessing missing pipelineTask status",
		finalTasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			Params: []Param{{
				Name: "notask-status", Value: ArrayOrString{Type: ParamTypeString, StringVal: "$(tasks.notask.status)"},
			}},
		}},
		expectedError: apis.FieldError{
			Message: `invalid value: pipeline task notask is not defined in the pipeline`,
			Paths:   []string{"finally[0].params[notask-status].value"},
		},
	}, {
		name: "invalid string variable in finally accessing missing pipelineTask status in when expression",
		finalTasks: []PipelineTask{{
			Name:    "foo",
			TaskRef: &TaskRef{Name: "foo-task"},
			WhenExpressions: WhenExpressions{{
				Input:    "$(tasks.notask.status)",
				Operator: selection.In,
				Values:   []string{"Success"},
			}},
		}},
		expectedError: apis.FieldError{
			Message: `invalid value: pipeline task notask is not defined in the pipeline`,
			Paths:   []string{"finally[0].when[0]"},
		},
	}, {
		name: "invalid string variable in finally accessing missing pipelineTask status in params and when expression",
		finalTasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			Params: []Param{{
				Name: "notask-status", Value: ArrayOrString{Type: ParamTypeString, StringVal: "$(tasks.notask.status)"},
			}},
		}, {
			Name:    "foo",
			TaskRef: &TaskRef{Name: "foo-task"},
			WhenExpressions: WhenExpressions{{
				Input:    "$(tasks.notask.status)",
				Operator: selection.In,
				Values:   []string{"Success"},
			}},
		}},
		expectedError: apis.FieldError{
			Message: `invalid value: pipeline task notask is not defined in the pipeline`,
			Paths:   []string{"finally[0].params[notask-status].value", "finally[1].when[0]"},
		},
	}, {
		name: "invalid variable concatenated with extra string in finally accessing missing pipelineTask status",
		finalTasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			Params: []Param{{
				Name: "notask-status", Value: ArrayOrString{Type: ParamTypeString, StringVal: "Execution status of notask is $(tasks.notask.status)."},
			}},
		}},
		expectedError: apis.FieldError{
			Message: `invalid value: pipeline task notask is not defined in the pipeline`,
			Paths:   []string{"finally[0].params[notask-status].value"},
		},
	}, {
		name: "invalid variable concatenated with other params in finally accessing missing pipelineTask status",
		finalTasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			Params: []Param{{
				Name: "notask-status", Value: ArrayOrString{Type: ParamTypeString, StringVal: "Execution status of $(tasks.taskname) is $(tasks.notask.status)."},
			}},
		}},
		expectedError: apis.FieldError{
			Message: `invalid value: pipeline task notask is not defined in the pipeline`,
			Paths:   []string{"finally[0].params[notask-status].value"},
		},
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateExecutionStatusVariables(tt.tasks, tt.finalTasks)
			if len(tt.expectedError.Error()) == 0 {
				if err != nil {
					t.Errorf("Pipeline.validateExecutionStatusVariables() returned error for valid pipeline variable accessing execution status: %s: %v", tt.name, err)
				}
			} else {
				if err == nil {
					t.Errorf("Pipeline.validateExecutionStatusVariables() did not return error for invalid pipeline parameters accessing execution status: %s, %s", tt.name, tt.tasks[0].Params)
				}
				if d := cmp.Diff(tt.expectedError.Error(), err.Error(), cmpopts.IgnoreUnexported(apis.FieldError{})); d != "" {
					t.Errorf("PipelineSpec.Validate() errors diff %s", diff.PrintWantGot(d))
				}
			}
		})
	}
}

func getTaskSpec() TaskSpec {
	return TaskSpec{
		Steps: []Step{{
			Container: corev1.Container{Name: "foo", Image: "bar"},
		}},
	}
}

func enableFeatures(t *testing.T, features []string) func(context.Context) context.Context {
	return func(ctx context.Context) context.Context {
		s := config.NewStore(logtesting.TestLogger(t))
		data := make(map[string]string)
		for _, f := range features {
			data[f] = "true"
		}
		s.OnConfigChanged(&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: config.GetFeatureFlagsConfigName()},
			Data:       data,
		})
		return s.ToContext(ctx)
	}
}
