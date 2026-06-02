// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package bundle contains functions for interacting with, managing and deploying UDS packages
package bundle

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/api/v1alpha1"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
	"github.com/zarf-dev/zarf/src/pkg/signing"
)

func Test_selectPackageVerifyOpts(t *testing.T) {
	tests := []struct {
		name           string
		createBundle   bool
		createSig      bool
		wantBundlePath bool
		wantSignature  bool
	}{
		{
			name:           "bundle sig exists uses BundlePath",
			createBundle:   true,
			wantBundlePath: true,
		},
		{
			name:          "only legacy sig falls back to Signature",
			createSig:     true,
			wantSignature: true,
		},
		{
			name:          "neither file exists falls back to Signature",
			wantSignature: true,
		},
		{
			name:           "both files exist prefers BundlePath",
			createBundle:   true,
			createSig:      true,
			wantBundlePath: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			bundlePath := filepath.Join(dir, layout.Bundle)
			sigPath := filepath.Join(dir, layout.Signature)

			if tt.createBundle {
				require.NoError(t, os.WriteFile(bundlePath, []byte("{}"), 0600))
			}
			if tt.createSig {
				require.NoError(t, os.WriteFile(sigPath, []byte("sig"), 0600))
			}

			result := selectPackageVerifyOpts(signing.VerifyBlobOptions{}, sigPath, bundlePath)

			if tt.wantBundlePath {
				require.Equal(t, bundlePath, result.BundlePath)
				require.Empty(t, result.Signature)
			}
			if tt.wantSignature {
				require.Equal(t, sigPath, result.Signature)
				require.Empty(t, result.BundlePath)
			}
		})
	}
}

func Test_verifyPackageSignature(t *testing.T) {
	signed := true
	tests := []struct {
		name        string
		setupFiles  []string
		buildSigned *bool
		verifyOpts  *signing.VerifyBlobOptions
		wantErr     string
		wantNilErr  bool
	}{
		{
			name:       "unsigned package skips verification",
			wantNilErr: true,
		},
		{
			name:        "pkg.Build.Signed=true with no opts returns error",
			buildSigned: &signed,
			wantErr:     "no verification material was provided",
		},
		{
			name:       "bundle sig file present with no opts returns error",
			setupFiles: []string{layout.Bundle},
			wantErr:    "no verification material was provided",
		},
		{
			name:       "legacy sig file present with no opts returns error",
			setupFiles: []string{layout.Signature},
			wantErr:    "no verification material was provided",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, f := range tt.setupFiles {
				require.NoError(t, os.WriteFile(filepath.Join(dir, f), []byte("data"), 0600))
			}

			pkg := v1alpha1.ZarfPackage{}
			if tt.buildSigned != nil {
				pkg.Build.Signed = tt.buildSigned
			}

			err := verifyPackageSignature(dir, tt.verifyOpts, pkg)

			if tt.wantNilErr {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tt.wantErr)
			}
		})
	}
}
