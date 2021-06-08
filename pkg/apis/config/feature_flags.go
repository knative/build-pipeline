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

package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

const (
	StableAPIFields                         = "stable"
	AlphaAPIFields                          = "alpha"
	disableHomeEnvOverwriteKey              = "disable-home-env-overwrite"
	disableWorkingDirOverwriteKey           = "disable-working-directory-overwrite"
	disableAffinityAssistantKey             = "disable-affinity-assistant"
	disableCredsInitKey                     = "disable-creds-init"
	runningInEnvWithInjectedSidecarsKey     = "running-in-environment-with-injected-sidecars"
	requireGitSSHSecretKnownHostsKey        = "require-git-ssh-secret-known-hosts" // nolint: gosec
	enableTektonOCIBundles                  = "enable-tekton-oci-bundles"
	enableCustomTasks                       = "enable-custom-tasks"
	enableAPIFields                         = "enable-api-fields"
	scopeWhenExpressionsToTask              = "scope-when-expressions-to-task"
	DefaultDisableHomeEnvOverwrite          = true
	DefaultDisableWorkingDirOverwrite       = true
	DefaultDisableAffinityAssistant         = false
	DefaultDisableCredsInit                 = false
	DefaultRunningInEnvWithInjectedSidecars = true
	DefaultRequireGitSSHSecretKnownHosts    = false
	DefaultEnableTektonOciBundles           = false
	DefaultEnableCustomTasks                = false
	DefaultScopeWhenExpressionsToTask       = false
	DefaultEnableAPIFields                  = StableAPIFields
)

// FeatureFlags holds the features configurations
// +k8s:deepcopy-gen=true
type FeatureFlags struct {
	DisableHomeEnvOverwrite          bool
	DisableWorkingDirOverwrite       bool
	DisableAffinityAssistant         bool
	DisableCredsInit                 bool
	RunningInEnvWithInjectedSidecars bool
	RequireGitSSHSecretKnownHosts    bool
	EnableTektonOCIBundles           bool
	EnableCustomTasks                bool
	ScopeWhenExpressionsToTask       bool
	EnableAPIFields                  string
}

// GetFeatureFlagsConfigName returns the name of the configmap containing all
// feature flags.
func GetFeatureFlagsConfigName() string {
	if e := os.Getenv("CONFIG_FEATURE_FLAGS_NAME"); e != "" {
		return e
	}
	return "feature-flags"
}

// NewFeatureFlagsFromMap returns a Config given a map corresponding to a ConfigMap
func NewFeatureFlagsFromMap(cfgMap map[string]string) (*FeatureFlags, error) {
	setFeature := func(key string, defaultValue bool, feature *bool) error {
		if cfg, ok := cfgMap[key]; ok {
			value, err := strconv.ParseBool(cfg)
			if err != nil {
				return fmt.Errorf("failed parsing feature flags config %q: %v", cfg, err)
			}
			*feature = value
			return nil
		}
		*feature = defaultValue
		return nil
	}

	tc := FeatureFlags{}
	if err := setFeature(disableHomeEnvOverwriteKey, DefaultDisableHomeEnvOverwrite, &tc.DisableHomeEnvOverwrite); err != nil {
		return nil, err
	}
	if err := setFeature(disableWorkingDirOverwriteKey, DefaultDisableWorkingDirOverwrite, &tc.DisableWorkingDirOverwrite); err != nil {
		return nil, err
	}
	if err := setFeature(disableAffinityAssistantKey, DefaultDisableAffinityAssistant, &tc.DisableAffinityAssistant); err != nil {
		return nil, err
	}
	if err := setFeature(disableCredsInitKey, DefaultDisableCredsInit, &tc.DisableCredsInit); err != nil {
		return nil, err
	}
	if err := setFeature(runningInEnvWithInjectedSidecarsKey, DefaultRunningInEnvWithInjectedSidecars, &tc.RunningInEnvWithInjectedSidecars); err != nil {
		return nil, err
	}
	if err := setFeature(requireGitSSHSecretKnownHostsKey, DefaultRequireGitSSHSecretKnownHosts, &tc.RequireGitSSHSecretKnownHosts); err != nil {
		return nil, err
	}
	if err := setFeature(scopeWhenExpressionsToTask, DefaultScopeWhenExpressionsToTask, &tc.ScopeWhenExpressionsToTask); err != nil {
		return nil, err
	}
	if err := setEnabledAPIFields(cfgMap, DefaultEnableAPIFields, &tc.EnableAPIFields); err != nil {
		return nil, err
	}

	// Given that they are alpha features, Tekton Bundles and Custom Tasks should be switched on if
	// enable-api-fields is "alpha". If enable-api-fields is not "alpha" then fall back to the value of
	// each feature's individual flag.
	//
	// Note: the user cannot enable "alpha" while disabling bundles or custom tasks - that would
	// defeat the purpose of having a single shared gate for all alpha features.
	if tc.EnableAPIFields == AlphaAPIFields {
		tc.EnableTektonOCIBundles = true
		tc.EnableCustomTasks = true
	} else {
		if err := setFeature(enableTektonOCIBundles, DefaultEnableTektonOciBundles, &tc.EnableTektonOCIBundles); err != nil {
			return nil, err
		}
		if err := setFeature(enableCustomTasks, DefaultEnableCustomTasks, &tc.EnableCustomTasks); err != nil {
			return nil, err
		}
	}
	return &tc, nil
}

// setEnabledAPIFields sets the "enable-api-fields" flag based on the content of a given map.
// If the feature gate is invalid or missing then an error is returned.
func setEnabledAPIFields(cfgMap map[string]string, defaultValue string, feature *string) error {
	value := defaultValue
	if cfg, ok := cfgMap[enableAPIFields]; ok {
		value = strings.ToLower(cfg)
	}
	switch value {
	case AlphaAPIFields, StableAPIFields:
		*feature = value
	default:
		return fmt.Errorf("invalid value for feature flag %q: %q", enableAPIFields, value)
	}
	return nil
}

// NewFeatureFlagsFromConfigMap returns a Config for the given configmap
func NewFeatureFlagsFromConfigMap(config *corev1.ConfigMap) (*FeatureFlags, error) {
	return NewFeatureFlagsFromMap(config.Data)
}
