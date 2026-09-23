// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build library

package bundle_test

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/mholt/archives"
	"github.com/sigstore/cosign/v3/pkg/cosign"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSignVerifyPublicContract(t *testing.T) {
	fixture := createLibraryBundle(t)
	privateKey, publicKey := libraryKeyPair(t)
	policy := bundle.VerificationPolicy{PublicKey: publicKey}

	require.NoError(t, bundle.Sign(t.Context(), bundle.SignOptions{
		Source:  fixture.ArtifactPath,
		Signing: bundle.SigningOptions{Mode: bundle.SigningModeKey, Key: privateKey},
		Config:  fixture.Config,
		TmpDir:  t.TempDir(),
	}))
	require.NoError(t, bundle.Verify(t.Context(), bundle.VerifyOptions{
		Source: fixture.ArtifactPath,
		Policy: policy,
		Config: fixture.Config,
		TmpDir: t.TempDir(),
	}))
	verifiedConfig := *fixture.Config
	verifiedConfig.SignatureVerification = &policy
	inspected, err := bundle.Inspect(t.Context(), bundle.InspectOptions{Source: fixture.ArtifactPath, Config: &verifiedConfig})
	require.NoError(t, err)
	require.NotNil(t, inspected.BundleSignature)
	assert.Equal(t, bundle.BundleSignatureStatusVerified, inspected.BundleSignature.Status)

	_, wrongPublicKey := libraryKeyPair(t)
	err = bundle.Verify(t.Context(), bundle.VerifyOptions{
		Source: fixture.ArtifactPath,
		Policy: bundle.VerificationPolicy{PublicKey: wrongPublicKey},
		Config: fixture.Config,
		TmpDir: t.TempDir(),
	})
	require.ErrorIs(t, err, bundle.ErrVerifyBundle)

	tampered := tamperLibraryArtifact(t, fixture.ArtifactPath)
	err = bundle.Verify(t.Context(), bundle.VerifyOptions{
		Source: tampered,
		Policy: policy,
		Config: fixture.Config,
		TmpDir: t.TempDir(),
	})
	require.ErrorIs(t, err, bundle.ErrVerifyBundle)
}

func TestSignVerifyOCIContract(t *testing.T) {
	fixture := createLibraryBundle(t)
	config := libraryRegistryConfig(fixture.Config)
	privateKey, publicKey := libraryKeyPair(t)
	policy := bundle.VerificationPolicy{PublicKey: publicKey}
	ref := fmt.Sprintf("%s/library/signed:v1.0.0", startLibraryRegistry(t))

	_, err := bundle.Push(t.Context(), fixture.ArtifactPath, ref, bundle.PushOptions{Config: config})
	require.NoError(t, err)
	require.NoError(t, bundle.Sign(t.Context(), bundle.SignOptions{
		Source:  ref,
		Signing: bundle.SigningOptions{Mode: bundle.SigningModeKey, Key: privateKey},
		Config:  config,
		TmpDir:  t.TempDir(),
	}))
	require.NoError(t, bundle.Verify(t.Context(), bundle.VerifyOptions{
		Source: ref,
		Policy: policy,
		Config: config,
		TmpDir: t.TempDir(),
	}))
	_, err = bundle.Pull(t.Context(), ref, t.TempDir(), bundle.PullOptions{Config: config})
	require.ErrorIs(t, err, bundle.ErrInvalidVerificationPolicy)
	pulled, err := bundle.Pull(t.Context(), ref, t.TempDir(), bundle.PullOptions{Config: config, Verification: policy})
	require.NoError(t, err)
	require.FileExists(t, pulled.OutputPath)
	require.NoError(t, bundle.Verify(t.Context(), bundle.VerifyOptions{
		Source: pulled.OutputPath,
		Policy: policy,
		Config: config,
		TmpDir: t.TempDir(),
	}))

	verifiedConfig := *config
	verifiedConfig.SignatureVerification = &policy
	inspected, err := bundle.Inspect(t.Context(), bundle.InspectOptions{Source: ref, Config: &verifiedConfig})
	require.NoError(t, err)
	assert.Equal(t, bundle.BundleSignatureStatusVerified, inspected.BundleSignature.Status)
}

func TestPushPreservesBundleSignature(t *testing.T) {
	fixture := createLibraryBundle(t)
	privateKey, publicKey := libraryKeyPair(t)
	policy := bundle.VerificationPolicy{PublicKey: publicKey}
	require.NoError(t, bundle.Sign(t.Context(), bundle.SignOptions{
		Source:  fixture.ArtifactPath,
		Signing: bundle.SigningOptions{Mode: bundle.SigningModeKey, Key: privateKey},
		Config:  fixture.Config,
		TmpDir:  t.TempDir(),
	}))
	config := libraryRegistryConfig(fixture.Config)
	ref := fmt.Sprintf("%s/library/preserved-signature:v1.0.0", startLibraryRegistry(t))
	pushed, err := bundle.Push(t.Context(), fixture.ArtifactPath, ref, bundle.PushOptions{Config: config})
	require.NoError(t, err)
	require.NoError(t, bundle.Verify(t.Context(), bundle.VerifyOptions{
		Source: pushed.OCIReference,
		Policy: policy,
		Config: config,
		TmpDir: t.TempDir(),
	}))
	pulled, err := bundle.Pull(t.Context(), pushed.OCIReference, t.TempDir(), bundle.PullOptions{Config: config, Verification: policy})
	require.NoError(t, err)
	require.NoError(t, bundle.Verify(t.Context(), bundle.VerifyOptions{
		Source: pulled.OutputPath,
		Policy: policy,
		Config: config,
		TmpDir: t.TempDir(),
	}))
}

func libraryKeyPair(t *testing.T) (privateKey, publicKey string) {
	t.Helper()
	keys, err := cosign.GenerateKeyPair(nil)
	require.NoError(t, err)
	privateKey = filepath.Join(t.TempDir(), "cosign.key")
	require.NoError(t, os.WriteFile(privateKey, keys.PrivateBytes, 0o600))
	return privateKey, string(keys.PublicBytes)
}

func tamperLibraryArtifact(t *testing.T, artifactPath string) string {
	t.Helper()
	return rewriteLibraryArtifact(t, artifactPath, func(root string) {
		artifactRoot, err := os.OpenRoot(root)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, artifactRoot.Close()) })
		index, err := artifactRoot.ReadFile("oci/index.json")
		require.NoError(t, err)
		require.NoError(t, artifactRoot.WriteFile("oci/index.json", append(index, '\n'), 0o600))
	})
}

func rewriteLibraryArtifact(t *testing.T, artifactPath string, rewrite func(root string)) string {
	t.Helper()
	root := t.TempDir()
	input, err := os.Open(artifactPath)
	require.NoError(t, err)
	archive := archives.CompressedArchive{Extraction: archives.Tar{}, Compression: archives.Zstd{}}
	require.NoError(t, archive.Extract(t.Context(), input, func(_ context.Context, info archives.FileInfo) error {
		if info.IsDir() {
			return os.MkdirAll(filepath.Join(root, filepath.FromSlash(info.NameInArchive)), 0o700)
		}
		path := filepath.Join(root, filepath.FromSlash(info.NameInArchive))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return err
		}
		reader, err := info.Open()
		if err != nil {
			return err
		}
		defer func() { require.NoError(t, reader.Close()) }()
		contents, err := io.ReadAll(reader)
		if err != nil {
			return err
		}
		return os.WriteFile(path, contents, 0o600)
	}))
	require.NoError(t, input.Close())
	rewrite(root)

	tampered := filepath.Join(t.TempDir(), "tampered.tar.zst")
	files, err := archives.FilesFromDisk(t.Context(), nil, map[string]string{root + string(filepath.Separator): ""})
	require.NoError(t, err)
	output, err := os.Create(tampered)
	require.NoError(t, err)
	archive = archives.CompressedArchive{Archival: archives.Tar{}, Compression: archives.Zstd{}}
	require.NoError(t, archive.Archive(t.Context(), output, files))
	require.NoError(t, output.Close())
	return tampered
}
