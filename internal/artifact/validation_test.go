// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package artifact

import (
	"testing"

	"github.com/defenseunicorns/uds-cli/internal/bundlehcl"
	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/stretchr/testify/require"
)

// validValidationConfig returns a baseline valid configuration for validation tests.
func validValidationConfig() *bundlehcl.UDSBundleConfig {
	return &bundlehcl.UDSBundleConfig{
		Global:  &bundlehcl.GlobalOptions{},
		Options: &bundlehcl.ConfigOptions{Concurrency: 10},
	}
}

func TestCreatePackageOptionsValidate(t *testing.T) {
	tests := []struct {
		name    string
		opts    CreatePackageOptions
		wantErr string
	}{
		{name: "nil config", wantErr: "config is required"},
		{name: "empty blob directory", opts: CreatePackageOptions{Config: validValidationConfig(), BundleDir: "/bundle"}, wantErr: "BlobDir is required"},
		{name: "empty bundle directory", opts: CreatePackageOptions{Config: validValidationConfig(), BlobDir: "/blobs"}, wantErr: "BundleDir is required"},
		{name: "valid", opts: CreatePackageOptions{Config: validValidationConfig(), BlobDir: "/blobs", BundleDir: "/bundle"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.opts.Validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tt.wantErr)
		})
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
