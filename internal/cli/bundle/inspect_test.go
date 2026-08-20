// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/defenseunicorns/uds-cli/internal/printer"
	bundlepkg "github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
)

func testInspectConfig() *bundlepkg.UDSBundleConfig {
	defaults := NewConfigResolver().Defaults()
	return &bundlepkg.UDSBundleConfig{
		Options: &defaults,
	}
}

func createTestArtifact(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	pkgDir := filepath.Join(root, "pkg")
	require.NoError(t, os.MkdirAll(pkgDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "zarf.yaml"), []byte("kind: ZarfPackageConfig\nmetadata:\n  name: test\n  version: 1.0.0\n  aggregateChecksum: e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855\ncomponents: []\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "checksums.txt"), nil, 0o644))

	bundleFile := filepath.Join(root, bundleFileName)
	require.NoError(t, os.WriteFile(bundleFile, []byte(`uds {
  bundle_api_version = "uds.dev/v1alpha1"
}
metadata {
  name    = "inspect-test"
  version = "1.0.0"
}
package "pkg" {
  source = "pkg"
  signature_verification { verify = false }
}
`), 0o644))

	config := testInspectConfig()
	config.Options.TmpDir = t.TempDir()
	result, err := bundlepkg.Create(t.Context(), bundleFile, bundlepkg.CreateOptions{
		Config:  config,
		Signing: bundlepkg.SigningOptions{Mode: bundlepkg.SigningModeUnsigned},
		Streams: iostreams.IOStreams{},
	})
	require.NoError(t, err)
	return result.OutputPath
}

func TestInspectOptions_Complete(t *testing.T) {
	artifact := "bundle.tar.zst"
	tests := []struct {
		name           string
		args           []string
		wantBundlePath string
	}{
		{
			name:           "with artifact path",
			args:           []string{artifact},
			wantBundlePath: artifact,
		},
		{
			name:           "without args requires a source",
			args:           []string{},
			wantBundlePath: "",
		},
		{
			name:           "with OCI reference",
			args:           []string{"oci://ghcr.io/example/bundle:v1"},
			wantBundlePath: "oci://ghcr.io/example/bundle:v1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			streams, _, _, _ := iostreams.NewTestIOStreams()
			o := NewInspectOptions(streams)

			cmd := &cobra.Command{}
			err := o.Complete(cmd, tt.args)
			require.NoError(t, err)
			assert.Equal(t, tt.wantBundlePath, o.BundlePath)
			assert.NotNil(t, o.Config)
		})
	}
}

func TestInspectOptions_Validate(t *testing.T) {
	tempDir := t.TempDir()
	validArtifact := filepath.Join(tempDir, "bundle.tar.zst")
	require.NoError(t, os.WriteFile(validArtifact, []byte("test"), 0o644))
	artifactDir := filepath.Join(tempDir, "artifact.tar.zst")
	require.NoError(t, os.Mkdir(artifactDir, 0o755))

	tests := []struct {
		name       string
		bundlePath string
		wantErr    string
	}{
		{
			name:       "local artifact",
			bundlePath: validArtifact,
		},
		{
			name:       "OCI reference with scheme",
			bundlePath: "oci://ghcr.io/test/bundle:v1",
		},
		{
			name:       "OCI reference without scheme",
			bundlePath: "ghcr.io/test/bundle:v1",
		},
		{
			name:       "OCI reference with tar suffix",
			bundlePath: "oci://ghcr.io/test/bundle:v1.tar.zst",
		},
		{
			name:       "localhost OCI reference",
			bundlePath: "localhost:5000/test/bundle:v1",
		},
		{
			name:       "invalid OCI reference",
			bundlePath: "oci://",
			wantErr:    "parsing OCI reference",
		},
		{
			name:       "empty path",
			bundlePath: "",
			wantErr:    "source is required",
		},
		{
			name:       "source HCL file",
			bundlePath: filepath.Join(tempDir, "bundle.uds.hcl"),
			wantErr:    "source must be a .tar.zst bundle artifact or OCI reference",
		},
		{
			name:       "source directory",
			bundlePath: tempDir,
			wantErr:    "source must be a .tar.zst bundle artifact or OCI reference",
		},
		{
			name:       "missing artifact",
			bundlePath: filepath.Join(tempDir, "missing.tar.zst"),
			wantErr:    "bundle artifact not found",
		},
		{
			name:       "artifact directory",
			bundlePath: artifactDir,
			wantErr:    "bundle artifact path is a directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := &InspectOptions{
				BundlePath: tt.bundlePath,
				Config:     testInspectConfig(),
			}
			err := o.Validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestInspectOptions_CompleteDoesNotReadSiblingDefaults(t *testing.T) {
	root := t.TempDir()
	artifact := filepath.Join(root, "bundle.tar.zst")
	require.NoError(t, os.WriteFile(artifact, []byte("artifact"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "defaults.uds.hcl"), []byte("not valid HCL {{{"), 0o644))

	streams, _, _, _ := iostreams.NewTestIOStreams()
	o := NewInspectOptions(streams)
	cmd := &cobra.Command{}

	require.NoError(t, o.Complete(cmd, []string{artifact}))
}

func TestInspectOptions_Run(t *testing.T) {
	artifact := createTestArtifact(t)
	streams, _, out, errOut := iostreams.NewTestIOStreams()
	textPrinter, err := printer.NewPrinter(printer.FormatText)
	require.NoError(t, err)

	o := &InspectOptions{
		BundlePath: artifact,
		Config:     testInspectConfig(),
		Printer:    textPrinter,
		IOStreams:  streams,
	}

	require.NoError(t, o.Run(t.Context()))
	assert.Contains(t, out.String(), "inspect-test")
	assert.Contains(t, out.String(), "pkg")
	assert.Contains(t, out.String(), "not_checked")
	assert.NotContains(t, out.String(), "package verification policy and signing metadata do not establish bundle integrity")
	assert.Contains(t, errOut.String(), "bundle signature verification was not performed")
}

func TestInspectOptions_Run_SkipSignatureVerificationWarns(t *testing.T) {
	artifact := createTestArtifact(t)
	streams, _, out, errOut := iostreams.NewTestIOStreams()
	textPrinter, err := printer.NewPrinter(printer.FormatText)
	require.NoError(t, err)

	o := &InspectOptions{
		BundlePath: artifact,
		Config:     testInspectConfig(),
		Printer:    textPrinter,
		Verification: VerifyOptions{
			SkipSignatureVerification: true,
		},
		IOStreams: streams,
	}

	require.NoError(t, o.Run(t.Context()))
	assert.Contains(t, out.String(), "skipped")
	assert.Contains(t, errOut.String(), "signature verification was skipped")
}

func TestInspectOptions_Run_JSONOutput(t *testing.T) {
	artifact := createTestArtifact(t)
	streams, _, out, _ := iostreams.NewTestIOStreams()
	jsonPrinter, err := printer.NewPrinter(printer.FormatJSON)
	require.NoError(t, err)

	o := &InspectOptions{
		BundlePath: artifact,
		Config:     testInspectConfig(),
		Printer:    jsonPrinter,
		IOStreams:  streams,
	}
	require.NoError(t, o.Run(t.Context()))

	var result inspectResult
	require.NoError(t, json.Unmarshal(out.Bytes(), &result))
	assert.Equal(t, "inspect-test", result.Name)
	assert.NotEmpty(t, result.ArtifactDigest)
	require.NotNil(t, result.BundleSignature)
	assert.Equal(t, "not_checked", result.BundleSignature.Status)
	assert.NotContains(t, out.String(), "warning")
	assert.Len(t, result.Packages, 1)
}

func TestInspectOptions_Run_YAMLOutput(t *testing.T) {
	artifact := createTestArtifact(t)
	streams, _, out, _ := iostreams.NewTestIOStreams()
	yamlPrinter, err := printer.NewPrinter(printer.FormatYAML)
	require.NoError(t, err)

	o := &InspectOptions{
		BundlePath: artifact,
		Config:     testInspectConfig(),
		Printer:    yamlPrinter,
		IOStreams:  streams,
	}
	require.NoError(t, o.Run(t.Context()))

	var result inspectResult
	require.NoError(t, yaml.Unmarshal(out.Bytes(), &result))
	assert.Equal(t, "inspect-test", result.Name)
	assert.NotEmpty(t, result.ArtifactDigest)
	require.NotNil(t, result.BundleSignature)
	assert.Equal(t, "not_checked", result.BundleSignature.Status)
	assert.NotContains(t, out.String(), "warning")
	assert.Len(t, result.Packages, 1)
}

func TestInspectResult_OmitsEmptyDescriptionAndVersion(t *testing.T) {
	data, err := json.Marshal(inspectResult{Name: "bundle"})
	require.NoError(t, err)
	assert.JSONEq(t, `{"name":"bundle","packages":null}`, string(data))
}
