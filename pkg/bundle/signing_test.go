// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/defenseunicorns/uds-cli/internal/artifact"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/mholt/archives"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/require"
)

func TestSign_RejectsTamperedBundleGraphBeforeSigning(t *testing.T) {
	source := createTestBundle(t, `uds {
  bundle_api_version = "uds.dev/v1alpha1"
}
metadata {
  name    = "tampered-sign"
  version = "1.0.0"
}
package "pkg1" {
  source = "localpkg"
}
`, "")

	workspace := t.TempDir()
	require.NoError(t, artifact.ExtractTarZst(t.Context(), iostreams.IOStreams{}, source, workspace))
	indexData, err := os.ReadFile(filepath.Join(workspace, "oci", "index.json"))
	require.NoError(t, err)
	var index ocispec.Index
	require.NoError(t, json.Unmarshal(indexData, &index))
	require.NotEmpty(t, index.Manifests)
	tamperedBlob := filepath.Join(workspace, "oci", "blobs", "sha256", strings.TrimPrefix(index.Manifests[0].Digest.String(), "sha256:"))
	require.NoError(t, os.Chmod(tamperedBlob, 0o600))
	require.NoError(t, os.WriteFile(tamperedBlob, []byte("tampered"), 0o600))
	require.NoError(t, artifact.WriteTarZst(t.Context(), iostreams.IOStreams{}, source, workspace))

	err = Sign(t.Context(), SignOptions{
		Source:  source,
		Signing: SigningOptions{Mode: SigningModeKey, Key: "unused"},
		TmpDir:  t.TempDir(),
	})
	require.ErrorContains(t, err, "verifying bundle content before signing")
}

func TestSigningOptions_PreservesDefaultOIDCClientID(t *testing.T) {
	defaultOptions := signingOptions(SigningOptions{Mode: SigningModeKeyless})
	require.Equal(t, "sigstore", defaultOptions.OIDC.ClientID)

	customOptions := signingOptions(SigningOptions{Mode: SigningModeKeyless, OIDCClientID: "custom-client"})
	require.Equal(t, "custom-client", customOptions.OIDC.ClientID)
}

func TestVerify_RejectsDuplicateSignatureEvidence(t *testing.T) {
	source := filepath.Join(t.TempDir(), "duplicate-signature.tar.zst")
	writeDuplicateSignatureArchive(t, source)

	err := Verify(t.Context(), VerifyOptions{
		Source: source,
		Policy: VerificationPolicy{PublicKey: "unused"},
		TmpDir: t.TempDir(),
	})
	require.ErrorContains(t, err, "expected exactly one bundle signature evidence entry, found 2")
}

func TestVerify_ReportsUnsignedBundle(t *testing.T) {
	source := createTestBundle(t, `uds {
  bundle_api_version = "uds.dev/v1alpha1"
}
metadata {
  name    = "unsigned"
  version = "1.0.0"
}
package "pkg1" {
  source = "localpkg"
}
`, "")

	err := Verify(t.Context(), VerifyOptions{
		Source: source,
		Policy: VerificationPolicy{PublicKey: "unused"},
		TmpDir: t.TempDir(),
	})
	require.ErrorIs(t, err, ErrBundleNotSigned)
	require.ErrorContains(t, err, "bundle is not signed")
}

func TestPush_RejectsDuplicateSignatureEvidence(t *testing.T) {
	source := filepath.Join(t.TempDir(), "duplicate-signature.tar.zst")
	writeDuplicateSignatureArchive(t, source)

	_, err := Push(t.Context(), source, "example.com/test:latest", PushOptions{Config: newTestConfig()})
	require.ErrorContains(t, err, "expected exactly one bundle signature evidence entry, found 2")
}

func writeDuplicateSignatureArchive(t *testing.T, dst string) {
	t.Helper()

	var tarData bytes.Buffer
	tw := tar.NewWriter(&tarData)
	for _, contents := range [][]byte{[]byte("first"), []byte("second")} {
		require.NoError(t, tw.WriteHeader(&tar.Header{Name: BundleSignatureFileName, Mode: 0o600, Size: int64(len(contents)), Typeflag: tar.TypeReg}))
		_, err := tw.Write(contents)
		require.NoError(t, err)
	}
	require.NoError(t, tw.Close())

	f, err := os.Create(dst)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, f.Close()) })
	zstdWriter, err := (archives.Zstd{}).OpenWriter(f)
	require.NoError(t, err)
	_, err = zstdWriter.Write(tarData.Bytes())
	require.NoError(t, err)
	require.NoError(t, zstdWriter.Close())
}
