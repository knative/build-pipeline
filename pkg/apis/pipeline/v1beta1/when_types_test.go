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
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/tektoncd/pipeline/test/diff"
	"k8s.io/apimachinery/pkg/selection"
)

func TestAllowsExecution(t *testing.T) {
	tests := []struct {
		name            string
		whenExpressions WhenExpressions
		expected        bool
	}{{
		name: "in expression",
		whenExpressions: WhenExpressions{
			{
				Input:    "foo",
				Operator: selection.In,
				Values:   []string{"foo", "bar"},
			},
		},
		expected: true,
	}, {
		name: "notin expression",
		whenExpressions: WhenExpressions{
			{
				Input:    "foobar",
				Operator: selection.NotIn,
				Values:   []string{"foobar"},
			},
		},
		expected: false,
	}, {
		name: "multiple expressions - false",
		whenExpressions: WhenExpressions{
			{
				Input:    "foobar",
				Operator: selection.In,
				Values:   []string{"foobar"},
			}, {
				Input:    "foo",
				Operator: selection.In,
				Values:   []string{"bar"},
			},
		},
		expected: false,
	}, {
		name: "multiple expressions - true",
		whenExpressions: WhenExpressions{
			{
				Input:    "foobar",
				Operator: selection.In,
				Values:   []string{"foobar"},
			}, {
				Input:    "foo",
				Operator: selection.NotIn,
				Values:   []string{"bar"},
			},
		},
		expected: true,
	}}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.whenExpressions.AllowsExecution()
			if d := cmp.Diff(tc.expected, got); d != "" {
				t.Errorf("Error evaluating AllowsExecution() for When Expressions in test case %s", diff.PrintWantGot(d))
			}
		})
	}
}

func TestHaveVariables(t *testing.T) {
	tests := []struct {
		name            string
		whenExpressions WhenExpressions
		expected        bool
	}{{
		name: "doesn't have variable",
		whenExpressions: WhenExpressions{
			{
				Input:    "foo",
				Operator: selection.In,
				Values:   []string{"foo", "bar"},
			},
		},
		expected: false,
	}, {
		name: "have variable - input",
		whenExpressions: WhenExpressions{
			{
				Input:    "$(params.foobar)",
				Operator: selection.NotIn,
				Values:   []string{"foobar"},
			},
		},
		expected: true,
	}, {
		name: "have variable - values",
		whenExpressions: WhenExpressions{
			{
				Input:    "foobar",
				Operator: selection.In,
				Values:   []string{"$(params.foobar)"},
			},
		},
		expected: true,
	}, {
		name: "have variable - multiple when expressions",
		whenExpressions: WhenExpressions{
			{
				Input:    "foo",
				Operator: selection.NotIn,
				Values:   []string{"bar"},
			}, {
				Input:    "foobar",
				Operator: selection.In,
				Values:   []string{"$(params.foobar)"},
			},
		},
		expected: true,
	}}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.whenExpressions.HaveVariables()
			if d := cmp.Diff(tc.expected, got); d != "" {
				t.Errorf("Error evaluating HaveVariables() for When Expressions in test case %s", diff.PrintWantGot(d))
			}
		})
	}
}

func TestReplaceWhenExpressionsVariables(t *testing.T) {
	tests := []struct {
		name            string
		whenExpressions WhenExpressions
		replacements    map[string]string
		expected        WhenExpressions
	}{{
		name: "params replacement in input",
		whenExpressions: WhenExpressions{
			{
				Input:    "$(params.foo)",
				Operator: selection.In,
				Values:   []string{"bar"},
			},
		},
		replacements: map[string]string{
			"params.foo": "bar",
		},
		expected: WhenExpressions{
			{
				Input:    "bar",
				Operator: selection.In,
				Values:   []string{"bar"},
			},
		},
	}, {
		name: "params replacement in values",
		whenExpressions: WhenExpressions{
			{
				Input:    "bar",
				Operator: selection.In,
				Values:   []string{"$(params.foo)"},
			},
		},
		replacements: map[string]string{
			"params.foo": "bar",
		},
		expected: WhenExpressions{
			{
				Input:    "bar",
				Operator: selection.In,
				Values:   []string{"bar"},
			},
		},
	}, {
		name: "results replacement in input",
		whenExpressions: WhenExpressions{
			{
				Input:    "$(tasks.aTask.results.foo)",
				Operator: selection.In,
				Values:   []string{"bar"},
			},
		},
		replacements: map[string]string{
			"tasks.aTask.results.foo": "bar",
		},
		expected: WhenExpressions{
			{
				Input:    "bar",
				Operator: selection.In,
				Values:   []string{"bar"},
			},
		},
	}, {
		name: "results replacement in values",
		whenExpressions: WhenExpressions{
			{
				Input:    "bar",
				Operator: selection.In,
				Values:   []string{"$(tasks.aTask.results.foo)"},
			},
		},
		replacements: map[string]string{
			"tasks.aTask.results.foo": "bar",
		},
		expected: WhenExpressions{
			{
				Input:    "bar",
				Operator: selection.In,
				Values:   []string{"bar"},
			},
		},
	}, {
		name: "replacements in multiple when expressions",
		whenExpressions: WhenExpressions{
			{
				Input:    "foo",
				Operator: selection.In,
				Values:   []string{"$(tasks.aTask.results.foo)"},
			}, {
				Input:    "$(params.bar)",
				Operator: selection.In,
				Values:   []string{"bar"},
			},
		},
		replacements: map[string]string{
			"tasks.aTask.results.foo": "foo",
			"params.bar":              "bar",
		},
		expected: WhenExpressions{
			{
				Input:    "foo",
				Operator: selection.In,
				Values:   []string{"foo"},
			}, {
				Input:    "bar",
				Operator: selection.In,
				Values:   []string{"bar"},
			},
		},
	}}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.whenExpressions.ReplaceWhenExpressionsVariables(tc.replacements, nil)
			if d := cmp.Diff(tc.expected, got); d != "" {
				t.Errorf("Error evaluating When Expressions in test case %s", diff.PrintWantGot(d))
			}
		})
	}
}

func TestApplyReplacements(t *testing.T) {
	tests := []struct {
		name              string
		original          *WhenExpression
		replacements      map[string]string
		arrayReplacements map[string][]string
		expected          *WhenExpression
	}{{
		name: "replace parameters variables",
		original: &WhenExpression{
			Input:    "$(params.path)",
			Operator: selection.In,
			Values:   []string{"$(params.branch)"},
		},
		replacements: map[string]string{
			"params.path":   "readme.md",
			"params.branch": "staging",
		},
		expected: &WhenExpression{
			Input:    "readme.md",
			Operator: selection.In,
			Values:   []string{"staging"},
		},
	}, {
		name: "replace results variables",
		original: &WhenExpression{
			Input:    "$(tasks.foo.results.bar)",
			Operator: selection.In,
			Values:   []string{"$(tasks.aTask.results.aResult)"},
		},
		replacements: map[string]string{
			"tasks.foo.results.bar":       "foobar",
			"tasks.aTask.results.aResult": "barfoo",
		},
		expected: &WhenExpression{
			Input:    "foobar",
			Operator: selection.In,
			Values:   []string{"barfoo"},
		},
	}, {
		name: "replace array params",
		original: &WhenExpression{
			Input:    "$(params.path)",
			Operator: selection.In,
			Values:   []string{"$(params.branches[*])"},
		},
		replacements: map[string]string{
			"params.path": "readme.md",
		},
		arrayReplacements: map[string][]string{
			"params.branches": {"dev", "stage"},
		},
		expected: &WhenExpression{
			Input:    "readme.md",
			Operator: selection.In,
			Values:   []string{"dev", "stage"},
		},
	}, {
		name: "replace string and array params",
		original: &WhenExpression{
			Input:    "$(params.path)",
			Operator: selection.In,
			Values:   []string{"$(params.branches[*])", "$(params.files[*])"},
		},
		replacements: map[string]string{
			"params.path": "readme.md",
		},
		arrayReplacements: map[string][]string{
			"params.branches": {"dev", "stage"},
			"params.files":    {"readme.md", "test.go"},
		},
		expected: &WhenExpression{
			Input:    "readme.md",
			Operator: selection.In,
			Values:   []string{"dev", "stage", "readme.md", "test.go"},
		},
	}}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.original.applyReplacements(tc.replacements, tc.arrayReplacements)
			if d := cmp.Diff(tc.expected, &got); d != "" {
				t.Errorf("Error applying replacements for When Expressions: %s", diff.PrintWantGot(d))
			}
		})
	}
}
