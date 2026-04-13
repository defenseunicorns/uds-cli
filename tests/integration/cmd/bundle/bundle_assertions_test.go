// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build integration

package bundle_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// assertBundleTarballsEqual asserts that two bundle tarballs contain the same
// OCI content: identical blob digest sets and identical index.json bytes.
func assertBundleTarballsEqual(t *testing.T, originalPath, otherPath string) {
	t.Helper()
	originalPaths, originalSmall := readBundleEntries(t, originalPath)
	otherPaths, otherSmall := readBundleEntries(t, otherPath)

	assert.Equal(t, blobDigests(originalPaths), blobDigests(otherPaths),
		"tarballs should contain the same blob digests")
	assert.Equal(t, originalSmall["oci/index.json"], otherSmall["oci/index.json"],
		"tarballs should have identical index.json content")
}

// blobDigests returns the set of sha256 hex digests present under oci/blobs/sha256/
// in a bundle tarball, identified by their archive paths.
func blobDigests(allPaths map[string]bool) map[string]bool {
	const prefix = "oci/blobs/sha256/"
	digests := make(map[string]bool)
	for p := range allPaths {
		if strings.HasPrefix(p, prefix) && p != prefix {
			digests[strings.TrimPrefix(p, prefix)] = true
		}
	}
	return digests
}
