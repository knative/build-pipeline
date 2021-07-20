/*
Copyright 2019 The Tekton Authors.

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

package v1beta1_test

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"github.com/tektoncd/pipeline/test/diff"
)

func TestParamSpec_SetDefaults(t *testing.T) {
	tests := []struct {
		name            string
		before          *v1beta1.ParamSpec
		defaultsApplied *v1beta1.ParamSpec
	}{{
		name: "inferred string type",
		before: &v1beta1.ParamSpec{
			Name: "parametername",
		},
		defaultsApplied: &v1beta1.ParamSpec{
			Name: "parametername",
			Type: v1beta1.ParamTypeString,
		},
	}, {
		name: "inferred type from default value",
		before: &v1beta1.ParamSpec{
			Name:    "parametername",
			Default: v1beta1.NewArrayOrString("an", "array"),
		},
		defaultsApplied: &v1beta1.ParamSpec{
			Name:    "parametername",
			Type:    v1beta1.ParamTypeArray,
			Default: v1beta1.NewArrayOrString("an", "array"),
		},
	}, {
		name: "fully defined ParamSpec",
		before: &v1beta1.ParamSpec{
			Name:        "parametername",
			Type:        v1beta1.ParamTypeArray,
			Description: "a description",
			Default:     v1beta1.NewArrayOrString("an", "array"),
		},
		defaultsApplied: &v1beta1.ParamSpec{
			Name:        "parametername",
			Type:        v1beta1.ParamTypeArray,
			Description: "a description",
			Default:     v1beta1.NewArrayOrString("an", "array"),
		},
	}}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			tc.before.SetDefaults(ctx)
			if d := cmp.Diff(tc.before, tc.defaultsApplied); d != "" {
				t.Error(diff.PrintWantGot(d))
			}
		})
	}
}

func TestArrayOrString_ApplyReplacements(t *testing.T) {
	type args struct {
		input              *v1beta1.ArrayOrString
		stringReplacements map[string]string
		arrayReplacements  map[string][]string
	}
	tests := []struct {
		name           string
		args           args
		expectedOutput *v1beta1.ArrayOrString
	}{{
		name: "no replacements on array",
		args: args{
			input:              v1beta1.NewArrayOrString("an", "array"),
			stringReplacements: map[string]string{"some": "value", "anotherkey": "value"},
			arrayReplacements:  map[string][]string{"arraykey": {"array", "value"}, "sdfdf": {"sdf", "sdfsd"}},
		},
		expectedOutput: v1beta1.NewArrayOrString("an", "array"),
	}, {
		name: "string replacements on string",
		args: args{
			input:              v1beta1.NewArrayOrString("astring$(some) asdf $(anotherkey)"),
			stringReplacements: map[string]string{"some": "value", "anotherkey": "value"},
			arrayReplacements:  map[string][]string{"arraykey": {"array", "value"}, "sdfdf": {"asdf", "sdfsd"}},
		},
		expectedOutput: v1beta1.NewArrayOrString("astringvalue asdf value"),
	}, {
		name: "single array replacement",
		args: args{
			input:              v1beta1.NewArrayOrString("firstvalue", "$(arraykey)", "lastvalue"),
			stringReplacements: map[string]string{"some": "value", "anotherkey": "value"},
			arrayReplacements:  map[string][]string{"arraykey": {"array", "value"}, "sdfdf": {"asdf", "sdfsd"}},
		},
		expectedOutput: v1beta1.NewArrayOrString("firstvalue", "array", "value", "lastvalue"),
	}, {
		name: "multiple array replacement",
		args: args{
			input:              v1beta1.NewArrayOrString("firstvalue", "$(arraykey)", "lastvalue", "$(sdfdf)"),
			stringReplacements: map[string]string{"some": "value", "anotherkey": "value"},
			arrayReplacements:  map[string][]string{"arraykey": {"array", "value"}, "sdfdf": {"asdf", "sdfsd"}},
		},
		expectedOutput: v1beta1.NewArrayOrString("firstvalue", "array", "value", "lastvalue", "asdf", "sdfsd"),
	}, {
		name: "empty array replacement",
		args: args{
			input:              v1beta1.NewArrayOrString("firstvalue", "$(arraykey)", "lastvalue"),
			stringReplacements: map[string]string{"some": "value", "anotherkey": "value"},
			arrayReplacements:  map[string][]string{"arraykey": {}},
		},
		expectedOutput: v1beta1.NewArrayOrString("firstvalue", "lastvalue"),
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.args.input.ApplyReplacements(tt.args.stringReplacements, tt.args.arrayReplacements)
			if d := cmp.Diff(tt.expectedOutput, tt.args.input); d != "" {
				t.Errorf("ApplyReplacements() output did not match expected value %s", diff.PrintWantGot(d))
			}
		})
	}
}

type ArrayOrStringHolder struct {
	AOrS v1beta1.ArrayOrString `json:"val"`
}

func TestArrayOrString_UnmarshalJSON(t *testing.T) {
	cases := []struct {
		input  string
		result v1beta1.ArrayOrString
	}{
		{"{\"val\": \"123\"}", *v1beta1.NewArrayOrString("123")},
		{"{\"val\": \"\"}", *v1beta1.NewArrayOrString("")},
		{"{\"val\":[]}", v1beta1.ArrayOrString{Type: v1beta1.ParamTypeArray, ArrayVal: []string{}}},
		{"{\"val\":[\"oneelement\"]}", v1beta1.ArrayOrString{Type: v1beta1.ParamTypeArray, ArrayVal: []string{"oneelement"}}},
		{"{\"val\":[\"multiple\", \"elements\"]}", *v1beta1.NewArrayOrString("multiple", "elements")},
	}

	for _, c := range cases {
		var result ArrayOrStringHolder
		if err := json.Unmarshal([]byte(c.input), &result); err != nil {
			t.Errorf("Failed to unmarshal input '%v': %v", c.input, err)
		}
		if !reflect.DeepEqual(result.AOrS, c.result) {
			t.Errorf("Failed to unmarshal input '%v': expected %+v, got %+v", c.input, c.result, result)
		}
	}
}

func TestArrayOrString_MarshalJSON(t *testing.T) {
	cases := []struct {
		input  v1beta1.ArrayOrString
		result string
	}{
		{*v1beta1.NewArrayOrString("123"), "{\"val\":\"123\"}"},
		{*v1beta1.NewArrayOrString("123", "1234"), "{\"val\":[\"123\",\"1234\"]}"},
		{*v1beta1.NewArrayOrString("a", "a", "a"), "{\"val\":[\"a\",\"a\",\"a\"]}"},
	}

	for _, c := range cases {
		input := ArrayOrStringHolder{c.input}
		result, err := json.Marshal(&input)
		if err != nil {
			t.Errorf("Failed to marshal input '%v': %v", input, err)
		}
		if string(result) != c.result {
			t.Errorf("Failed to marshal input '%v': expected: %+v, got %q", input, c.result, string(result))
		}
	}
}

func TestArrayReference(t *testing.T) {
	tests := []struct {
		name, p, expectedResult string
	}{{
		name:           "valid array parameter expression with star notation returns param name",
		p:              "$(params.arrayParam[*])",
		expectedResult: "arrayParam",
	}, {
		name:           "invalid array parameter without dollar notation returns the input as is",
		p:              "params.arrayParam[*]",
		expectedResult: "params.arrayParam[*]",
	}}
	for _, tt := range tests {
		if d := cmp.Diff(tt.expectedResult, v1beta1.ArrayReference(tt.p)); d != "" {
			t.Errorf(diff.PrintWantGot(d))
		}
	}
}
