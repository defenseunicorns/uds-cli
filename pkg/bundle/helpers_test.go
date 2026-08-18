// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/defenseunicorns/uds-cli/internal/artifact"
	udsoci "github.com/defenseunicorns/uds-cli/internal/oci"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/mholt/archives"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/require"
	oras "oras.land/oras-go/v2"
)

const (
	tempDirPerm fs.FileMode = 0o700
	tmpFilePerm fs.FileMode = 0o600
)

func newTestConfig() *UDSBundleConfig {
	return newTestConfigWithArch(runtime.GOARCH)
}

func newTestConfigWithArch(arch string) *UDSBundleConfig {
	opts := ConfigOptions{Architecture: arch, TmpDir: os.TempDir(), Concurrency: 10}
	return &UDSBundleConfig{Options: &opts}
}

func pushTo(target oras.Target) pushHooks {
	return pushHooks{toOrasTarget: func(context.Context, string, *PushOptions) (oras.Target, error) {
		return target, nil
	}}
}

func bundleDefinitionContainsLayerTitle(t *testing.T, entries map[string][]byte, title string) bool {
	t.Helper()
	var idx ocispec.Index
	require.NoError(t, json.Unmarshal(entries["oci/index.json"], &idx))
	entry, _, err := udsoci.FindBundleDefinition(idx)
	require.NoError(t, err)
	manifestBytes := entries["oci/blobs/sha256/"+entry.Digest.Hex()]
	var manifest ocispec.Manifest
	require.NoError(t, json.Unmarshal(manifestBytes, &manifest))
	for _, layer := range manifest.Layers {
		if layer.Annotations[ocispec.AnnotationTitle] == title {
			_, ok := entries["oci/blobs/sha256/"+layer.Digest.Hex()]
			return ok
		}
	}
	return false
}

func readTarZstEntries(t *testing.T, path string) map[string][]byte {
	t.Helper()
	f, err := os.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, f.Close()) })
	archive := archives.CompressedArchive{Extraction: archives.Tar{}, Compression: archives.Zstd{}}
	entries := map[string][]byte{}
	err = archive.Extract(t.Context(), f, func(_ context.Context, info archives.FileInfo) error {
		if info.IsDir() {
			return nil
		}
		r, err := info.Open()
		if err != nil {
			return err
		}
		defer func() { _ = r.Close() }()
		entries[info.NameInArchive], err = io.ReadAll(r)
		return err
	})
	require.NoError(t, err)
	return entries
}

func writeTarZstEntries(t *testing.T, entries map[string][]byte) string {
	t.Helper()
	root := t.TempDir()
	for name, data := range entries {
		path := filepath.Join(root, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), tempDirPerm))
		require.NoError(t, os.WriteFile(path, data, tmpFilePerm))
	}
	outPath := filepath.Join(t.TempDir(), "bundle.tar.zst")
	require.NoError(t, artifact.WriteTarZst(t.Context(), iostreams.IOStreams{}, outPath, root))
	return outPath
}
