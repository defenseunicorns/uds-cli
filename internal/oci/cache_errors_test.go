// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package oci

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsCacheStorageError(t *testing.T) {
	cacheDir := filepath.Join(t.TempDir(), "cache")
	store := filepath.Join(cacheDir, "bundle-oci", "images")
	pathError := func(path string) error { return &os.PathError{Op: "mkdir", Path: path, Err: os.ErrPermission} }
	tests := []struct {
		name     string
		err      error
		cacheDir string
		want     bool
	}{
		{name: "configured root blocked", err: pathError(cacheDir), cacheDir: cacheDir, want: true},
		{name: "store initialization", err: pathError(store), cacheDir: cacheDir, want: true},
		{name: "wrapped cache write", err: fmt.Errorf("pull: %w", pathError(filepath.Join(store, "ingest"))), cacheDir: cacheDir, want: true},
		{name: "cache rename", err: &os.LinkError{Op: "rename", Old: filepath.Join(store, "ingest", "blob"), New: filepath.Join(store, "blobs", "sha256", "blob"), Err: os.ErrPermission}, cacheDir: cacheDir, want: true},
		{name: "no error", cacheDir: cacheDir},
		{name: "disabled", err: pathError(store)},
		{name: "legacy cache", err: pathError(filepath.Join(cacheDir, "layers")), cacheDir: cacheDir},
		{name: "zarf default cache", err: pathError(filepath.Join(cacheDir, "images")), cacheDir: cacheDir},
		{name: "similar prefix", err: pathError(filepath.Join(cacheDir, "bundle-oci-other")), cacheDir: cacheDir},
		{name: "workspace", err: pathError(filepath.Join(t.TempDir(), "package")), cacheDir: cacheDir},
		{name: "unclassified", err: errors.New("invalid OCI Image Index"), cacheDir: cacheDir},
		{name: "canceled", err: errors.Join(pathError(store), context.Canceled), cacheDir: cacheDir},
		{name: "deadline", err: errors.Join(pathError(store), context.DeadlineExceeded), cacheDir: cacheDir},
		{name: "rename outside cache", err: &os.LinkError{Op: "rename", Old: filepath.Join(store, "ingest", "blob"), New: filepath.Join(t.TempDir(), "blob"), Err: os.ErrPermission}, cacheDir: cacheDir},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsCacheStorageError(tt.err, tt.cacheDir))
		})
	}
}
