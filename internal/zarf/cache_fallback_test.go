// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package zarf

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	bundleinternal "github.com/defenseunicorns/uds-cli/internal/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemoteSourceCacheFallback(t *testing.T) {
	for _, retryFails := range []bool{false, true} {
		t.Run(fmt.Sprintf("retry fails=%t", retryFails), func(t *testing.T) {
			cacheDir := t.TempDir()
			cacheErr := &os.PathError{Op: "mkdir", Path: filepath.Join(cacheDir, "bundle-oci", "images"), Err: os.ErrPermission}
			retryErr := errors.New("registry unavailable")
			streams, _, out, errOut := iostreams.NewTestIOStreams()
			streams = streams.WithLogger(slog.New(slog.NewTextHandler(streams.ErrOut(), nil)), nil)
			source := &remoteSource{opts: bundleinternal.ConfigOptions{CacheDir: cacheDir}, streams: streams}
			root := ocispec.Descriptor{Digest: digest.FromString("pinned package")}
			attempts := 0
			err := source.withCacheFallback(t.Context(), func(attempt *remoteSource) error {
				attempts++
				if attempts == 1 {
					assert.Equal(t, cacheDir, attempt.opts.CacheDir)
					attempt.resolvedRoot = &root
					return cacheErr
				}
				assert.Empty(t, attempt.opts.CacheDir)
				assert.Equal(t, &root, attempt.resolvedRoot)
				if retryFails {
					return retryErr
				}
				return nil
			})
			assert.Equal(t, 2, attempts)
			assert.Equal(t, cacheDir, source.opts.CacheDir, "fallback must not mutate configured caching")
			assert.Empty(t, out.String())
			assert.Contains(t, errOut.String(), "retrying retrieval once without caching")
			if retryFails {
				require.ErrorIs(t, err, cacheErr)
				require.ErrorIs(t, err, retryErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestRemoteSourceDoesNotRetryOtherFailures(t *testing.T) {
	cacheDir := t.TempDir()
	cacheErr := &os.PathError{Op: "open", Path: filepath.Join(cacheDir, "bundle-oci", "images"), Err: os.ErrPermission}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	tests := []struct {
		name     string
		err      error
		cacheDir string
		ctx      context.Context
	}{
		{name: "success", cacheDir: cacheDir, ctx: t.Context()},
		{name: "registry failure", err: errors.New("registry unavailable"), cacheDir: cacheDir, ctx: t.Context()},
		{name: "package verification failure", err: fmt.Errorf("%w: %w", ErrLoadPackage, cacheErr), cacheDir: cacheDir, ctx: t.Context()},
		{name: "workspace inside cache", err: fmt.Errorf("%w: %w", ErrCreatePackageWorkspace, cacheErr), cacheDir: cacheDir, ctx: t.Context()},
		{name: "workspace failure", err: &os.PathError{Op: "open", Path: filepath.Join(t.TempDir(), "package"), Err: os.ErrPermission}, cacheDir: cacheDir, ctx: t.Context()},
		{name: "cache disabled", err: cacheErr, ctx: t.Context()},
		{name: "canceled operation", err: cacheErr, cacheDir: cacheDir, ctx: canceled},
		{name: "wrapped cancellation", err: errors.Join(cacheErr, context.Canceled), cacheDir: cacheDir, ctx: t.Context()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := &remoteSource{opts: bundleinternal.ConfigOptions{CacheDir: tt.cacheDir}}
			attempts := 0
			err := source.withCacheFallback(tt.ctx, func(*remoteSource) error {
				attempts++
				return tt.err
			})
			assert.Equal(t, 1, attempts)
			assert.Equal(t, tt.err, err)
		})
	}
}
