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

package config_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/tektoncd/pipeline/pkg/apis/config"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/pod"
	test "github.com/tektoncd/pipeline/pkg/reconciler/testing"
	"github.com/tektoncd/pipeline/test/diff"
)

func TestNewDefaultsFromConfigMap(t *testing.T) {
	type testCase struct {
		expectedConfig *config.Defaults
		expectedError  bool
		fileName       string
	}

	testCases := []testCase{
		{
			expectedConfig: &config.Defaults{
				DefaultShortTimeoutSecondsValue: 30,
				DefaultTimeoutMinutes:           50,
				DefaultServiceAccount:           "tekton",
				DefaultManagedByLabelValue:      "something-else",
			},
			fileName: config.GetDefaultsConfigName(),
		},
		{
			expectedConfig: &config.Defaults{
				DefaultShortTimeoutSecondsValue: 30,
				DefaultTimeoutMinutes:           50,
				DefaultServiceAccount:           "tekton",
				DefaultManagedByLabelValue:      config.DefaultManagedByLabelValue,
				DefaultPodTemplate: &pod.Template{
					NodeSelector: map[string]string{
						"label": "value",
					},
				},
			},
			fileName: "config-defaults-with-pod-template",
		},
		// the github.com/ghodss/yaml package in the vendor directory does not support UnmarshalStrict
		// update it, switch to UnmarshalStrict in defaults.go, then uncomment these tests
		// {
		// 	expectedError: true,
		// 	fileName:      "config-defaults-timeout-err",
		// },
		// {
		// 	expectedError: true,
		// 	fileName:      "config-defaults-pod-template-err",
		// },
	}

	for _, tc := range testCases {
		if tc.expectedError {
			verifyConfigFileWithExpectedError(t, tc.fileName)
		} else {
			verifyConfigFileWithExpectedConfig(t, tc.fileName, tc.expectedConfig)
		}
	}
}

func TestNewDefaultsFromEmptyConfigMap(t *testing.T) {
	DefaultsConfigEmptyName := "config-defaults-empty"
	expectedConfig := &config.Defaults{
		DefaultShortTimeoutSecondsValue: 30,
		DefaultTimeoutMinutes:           60,
		DefaultManagedByLabelValue:      "tekton-pipelines",
		DefaultServiceAccount:           "default",
	}
	verifyConfigFileWithExpectedConfig(t, DefaultsConfigEmptyName, expectedConfig)
}

func TestEquals(t *testing.T) {
	testCases := []struct {
		name     string
		left     *config.Defaults
		right    *config.Defaults
		expected bool
	}{
		{
			name:     "left and right nil",
			left:     nil,
			right:    nil,
			expected: true,
		},
		{
			name:     "left nil",
			left:     nil,
			right:    &config.Defaults{},
			expected: false,
		},
		{
			name:     "right nil",
			left:     &config.Defaults{},
			right:    nil,
			expected: false,
		},
		{
			name:     "right and right default",
			left:     &config.Defaults{},
			right:    &config.Defaults{},
			expected: true,
		},
		{
			name: "different default short timeout",
			left: &config.Defaults{
				DefaultShortTimeoutSecondsValue: 10,
			},
			right: &config.Defaults{
				DefaultShortTimeoutSecondsValue: 20,
			},
			expected: false,
		},
		{
			name: "different default timeout",
			left: &config.Defaults{
				DefaultTimeoutMinutes: 10,
			},
			right: &config.Defaults{
				DefaultTimeoutMinutes: 20,
			},
			expected: false,
		},
		{
			name: "same default timeout",
			left: &config.Defaults{
				DefaultTimeoutMinutes: 20,
			},
			right: &config.Defaults{
				DefaultTimeoutMinutes: 20,
			},
			expected: true,
		},
		{
			name: "same default short timeout",
			left: &config.Defaults{
				DefaultShortTimeoutSecondsValue: 20,
			},
			right: &config.Defaults{
				DefaultShortTimeoutSecondsValue: 20,
			},
			expected: true,
		},
		{
			name: "different default pod template",
			left: &config.Defaults{
				DefaultPodTemplate: &pod.Template{
					NodeSelector: map[string]string{
						"label": "value",
					},
				},
			},
			right: &config.Defaults{
				DefaultPodTemplate: &pod.Template{
					NodeSelector: map[string]string{
						"label2": "value",
					},
				},
			},
			expected: false,
		},
		{
			name: "same default pod template",
			left: &config.Defaults{
				DefaultPodTemplate: &pod.Template{
					NodeSelector: map[string]string{
						"label": "value",
					},
				},
			},
			right: &config.Defaults{
				DefaultPodTemplate: &pod.Template{
					NodeSelector: map[string]string{
						"label": "value",
					},
				},
			},
			expected: true,
		},
		{
			name: "different default workspace",
			left: &config.Defaults{
				DefaultTaskRunWorkspaceBinding: "emptyDir: {}",
			},
			right: &config.Defaults{
				DefaultTaskRunWorkspaceBinding: "source",
			},
			expected: false,
		},
		{
			name: "same default workspace",
			left: &config.Defaults{
				DefaultTaskRunWorkspaceBinding: "emptyDir: {}",
			},
			right: &config.Defaults{
				DefaultTaskRunWorkspaceBinding: "emptyDir: {}",
			},
			expected: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual := tc.left.Equals(tc.right)
			if actual != tc.expected {
				t.Errorf("Comparison failed expected: %t, actual: %t", tc.expected, actual)
			}
		})
	}
}

func verifyConfigFileWithExpectedConfig(t *testing.T, fileName string, expectedConfig *config.Defaults) {
	t.Helper()
	cm := test.ConfigMapFromTestFile(t, fileName)
	if Defaults, err := config.NewDefaultsFromConfigMap(cm); err == nil {
		if d := cmp.Diff(Defaults, expectedConfig); d != "" {
			t.Errorf("Diff:\n%s", diff.PrintWantGot(d))
		}
	} else {
		t.Errorf("NewDefaultsFromConfigMap(actual) = %v", err)
	}
}

func verifyConfigFileWithExpectedError(t *testing.T, fileName string) {
	cm := test.ConfigMapFromTestFile(t, fileName)
	if _, err := config.NewDefaultsFromConfigMap(cm); err == nil {
		t.Errorf("NewDefaultsFromConfigMap(actual) was expected to return an error")
	}
}
