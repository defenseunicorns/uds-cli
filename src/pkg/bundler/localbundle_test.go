// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package bundler defines behavior for bundling packages
package bundler

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/mholt/archives"
	"github.com/stretchr/testify/require"
)

// TestWriteTarballPriorityOrdering guards the contract that priority entries are
// written to the FRONT of the bundle tarball, in the given order, with the rest
// alphabetical. The inspect fast-path depends on this layout, and it only holds
// because mholt/archives' ArchiveAsync consumes its jobs channel serially (send
// order == tar order). If a future library version parallelizes that, this test
// fails loudly instead of the inspect speedup silently regressing.
func TestWriteTarballPriorityOrdering(t *testing.T) {
	src := t.TempDir()

	// Stage a handful of fake archive entries on disk.
	names := []string{
		"oci-layout",
		"index.json",
		filepath.Join(config.BlobsDir, "aaaa"),
		filepath.Join(config.BlobsDir, "bbbb"),
		filepath.Join(config.BlobsDir, "cccc"),
		filepath.Join(config.BlobsDir, "dddd"),
	}
	artifactPathMap := make(types.PathMap)
	for _, n := range names {
		disk := filepath.Join(src, n)
		require.NoError(t, os.MkdirAll(filepath.Dir(disk), 0o755))
		require.NoError(t, os.WriteFile(disk, []byte("x"), 0o644))
		artifactPathMap[disk] = n
	}

	// A deliberately non-alphabetical priority list to prove ordering is honored.
	priority := []string{
		"oci-layout",
		"index.json",
		filepath.Join(config.BlobsDir, "cccc"),
		filepath.Join(config.BlobsDir, "aaaa"),
	}

	out := t.TempDir()
	bundle := &types.UDSBundle{Metadata: types.UDSMetadata{Name: "test", Architecture: "amd64", Version: "0.0.1"}}
	require.NoError(t, writeTarball(bundle, artifactPathMap, priority, out))

	tarballs, err := filepath.Glob(filepath.Join(out, "*.tar.zst"))
	require.NoError(t, err)
	require.Len(t, tarballs, 1)

	// Read entries back in tar order.
	f, err := os.Open(tarballs[0])
	require.NoError(t, err)
	defer f.Close()
	var got []string
	require.NoError(t, config.BundleArchiveFormat.Extract(context.Background(), f, func(_ context.Context, file archives.FileInfo) error {
		got = append(got, file.NameInArchive)
		return nil
	}))

	// Filter to the entries we staged (ignore any incidental directory entries)
	// while preserving tar order.
	known := make(map[string]bool, len(names))
	for _, n := range names {
		known[n] = true
	}
	var ordered []string
	for _, n := range got {
		if known[n] {
			ordered = append(ordered, n)
		}
	}

	// Expected: priority entries in the given order, then the rest alphabetical.
	rest := make([]string, 0, len(names)-len(priority))
	inPriority := make(map[string]bool, len(priority))
	for _, p := range priority {
		inPriority[p] = true
	}
	for _, n := range names {
		if !inPriority[n] {
			rest = append(rest, n)
		}
	}
	sort.Strings(rest)
	want := append(append([]string{}, priority...), rest...)

	require.Equal(t, want, ordered, "priority blobs must be written to the front of the tar, in order, then the rest alphabetical")
}
