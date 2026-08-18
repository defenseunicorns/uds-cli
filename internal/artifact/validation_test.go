// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package artifact

import (
	"testing"

	bundleinternal "github.com/defenseunicorns/uds-cli/internal/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/stretchr/testify/require"
)

// validValidationConfig returns a baseline valid configuration for validation tests.
func validValidationConfig() *bundleinternal.UDSBundleConfig {
	return &bundleinternal.UDSBundleConfig{
		Options: &bundleinternal.ConfigOptions{Concurrency: 10},
	}
}

func TestValidateBundleForCreateScopesSignaturePolicy(t *testing.T) {
	b := &spec.UDSBundle{
		UDS:      spec.UDSBlock{BundleAPIVersion: "uds.dev/v1alpha1"},
		Metadata: spec.Metadata{Name: "test"},
		Packages: []spec.Package{{Name: "pkg", Source: "oci://example.com/pkg:v1"}},
	}

	require.NoError(t, b.Validate(), "consumption-time validation should not require create-only signature policy")
	require.ErrorContains(t, ValidateBundleForCreate(b), "signature_verification block is required")
}
