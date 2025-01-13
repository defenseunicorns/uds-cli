// Copyright 2025 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package featureflags

import (
	"os"
	"strings"
)

// FeatureFlagMap stores the state of feature flags.
var FeatureFlagMap = map[string]bool{}

// Initialize initializes the feature flags from an environment variable.
func Initialize() {
	// Check the FEATURE_FLAG environment variable.
	// example: FEATURE_FLAG=tofu,encrypt-oci
	envFeatures := os.Getenv("FEATURE_FLAG")
	if envFeatures != "" {
		for _, feature := range strings.Split(envFeatures, ",") {
			FeatureFlagMap[feature] = true
		}
	}
}

// EnableFeature enables a feature flag programmatically.
func EnableFeature(feature string) {
	FeatureFlagMap[feature] = true
}

// IsEnabled checks if a feature flag is enabled.
func IsEnabled(feature string) bool {
	return FeatureFlagMap[feature]
}
