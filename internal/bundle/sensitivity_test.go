// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/defenseunicorns/uds-cli/internal/redact"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSensitiveBundleRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, BundleFileName)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "key"), []byte("private-key"), 0o600))
	require.NoError(t, os.WriteFile(path, []byte(`
uds { bundle_api_version = sensitive("uds.dev/v1alpha1") }
locals { keys = true ? { key = sensitive(file("key")) } : { other = "unused" } }
metadata { name = "bundle-${local.keys.key}" }
package "base" { source = "base.tar" }
package "app" {
  source = sensitive("app.tar")
  depends_on = [package.base]
  optional_components = [sensitive("component")]
  signature_verification { public_key = local.keys.key }
}
`), 0o600))
	parser := NewHCLParser("", iostreams.IOStreams{})
	ctx, scope := redact.WithScope(t.Context())
	bundle, materialized, err := parser.ParseAndMaterializeBundleFile(ctx, path)
	require.NoError(t, err)
	require.Equal(t, "bundle-private-key", bundle.Metadata.Name)
	assert.Equal(t, "private-key", bundle.Packages[1].SignatureVerification.PublicKey)
	assert.Equal(t, "base", bundle.Packages[1].DependsOn[0].Name)
	assert.Equal(t, []string{"component"}, bundle.Packages[1].OptionalComponents)
	assert.Equal(t, "*** *** ***", scope.Replace("bundle-private-key private-key app.tar"))
	assert.Contains(t, string(materialized), `sensitive("private-key")`)
	artifactCtx, artifactScope := redact.WithScope(t.Context())
	decoded, err := parser.ParseBundleBytes(artifactCtx, materialized)
	require.NoError(t, err)
	assert.Equal(t, bundle, decoded)
	assert.Equal(t, "***", artifactScope.Replace(decoded.Metadata.Name))
}

func TestSensitiveConfigNestedBlock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.uds.hcl")
	require.NoError(t, os.WriteFile(path, []byte(`
signature_verification {
  keyless { certificate_identity = sensitive("private-id") }
}
`), 0o600))
	ctx, scope := redact.WithScope(t.Context())
	cfg, err := NewHCLParser("", iostreams.IOStreams{}).ParseBundleConfig(ctx, path)
	require.NoError(t, err)
	assert.Equal(t, "private-id", cfg.SignatureVerification.Keyless.CertificateIdentity)
	assert.Equal(t, "***", scope.Replace(cfg.SignatureVerification.Keyless.CertificateIdentity))
}

func TestSensitiveBundleDecodeError(t *testing.T) {
	_, err := NewHCLParser("", iostreams.IOStreams{}).ParseBundleBytes(t.Context(), []byte(`
uds { bundle_api_version = "uds.dev/v1alpha1" }
metadata { name = sensitive(["private-value"]) }
`))
	require.ErrorIs(t, err, ErrDecodeBundle)
	assert.NotContains(t, err.Error(), "private-value")
}

func TestUnmarkedConfigKeepsDiagnosticCause(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.uds.hcl")
	parser := NewHCLParser("", iostreams.IOStreams{})
	for _, tc := range []struct{ config, cause string }{
		{`options { concurrency = "many" }`, "Unsuitable value type"},
		{`variables = { token = file("missing.txt") }`, os.ErrNotExist.Error()},
	} {
		require.NoError(t, os.WriteFile(path, []byte(tc.config), 0o600))
		_, err := parser.ParseBundleConfig(t.Context(), path)
		require.ErrorContains(t, err, tc.cause)
	}
}

func TestConfigDiagnosticIsIndependentOfSensitivity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.uds.hcl")
	parser := NewHCLParser("", iostreams.IOStreams{})
	for _, expr := range []string{`"private-value"`, `sensitive("private-value")`} {
		require.NoError(t, os.WriteFile(path, []byte("options { concurrency = "+expr+" }"), 0o600))
		_, err := parser.ParseBundleConfig(t.Context(), path)
		var diagnostic *HCLDiagnosticError
		require.ErrorAs(t, err, &diagnostic)
		require.Positive(t, diagnostic.Line)
		require.NotContains(t, err.Error(), "private-value")
		require.ErrorContains(t, err, "Unsuitable value type at line 1, column")
	}
}
