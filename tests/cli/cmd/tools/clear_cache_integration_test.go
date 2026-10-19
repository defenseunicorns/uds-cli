// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build cli

package tools_test

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/defenseunicorns/uds-cli/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClearCacheConfiguration(t *testing.T) {
	for _, source := range []string{"default", "hcl", "cli", "hcl-tilde", "cli-tilde", "empty-flag-retains-hcl", "empty-flag-uses-default"} {
		t.Run(source, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			paths := []string{filepath.Join(home, ".uds-cache"), filepath.Join(home, "hcl-cache"), filepath.Join(home, "cli-cache")}
			for _, path := range paths {
				seedCache(t, path)
			}
			args := []string{"tools", "clear-cache"}
			selected := paths[0]
			if source != "default" && source != "empty-flag-uses-default" {
				config := filepath.Join(home, "config.uds.hcl")
				hclPath := paths[1]
				if source == "hcl-tilde" {
					hclPath = "~/hcl-cache"
				}
				require.NoError(t, os.WriteFile(config, fmt.Appendf(nil, "options { uds_cache = %q }\n", hclPath), 0o600))
				args = append(args, "--config", config)
				selected = paths[1]
			}
			if source == "cli" || source == "cli-tilde" {
				cliPath := paths[2]
				if source == "cli-tilde" {
					cliPath = "~/cli-cache"
				}
				args = append(args, "--uds-cache", cliPath)
				selected = paths[2]
			}
			if source == "empty-flag-retains-hcl" || source == "empty-flag-uses-default" {
				args = append(args, "--uds-cache=")
			}
			streams, _, out, errOut := iostreams.NewTestIOStreams()
			require.NoError(t, testutil.ExecuteCLI(t.Context(), streams, args...))
			assert.Empty(t, out.String())
			assert.Empty(t, errOut.String())
			for _, path := range paths {
				blob := filepath.Join(path, "bundle-oci", "images", "blobs", "sha256", "blob")
				if path == selected {
					assert.NoFileExists(t, blob)
					assert.DirExists(t, filepath.Join(path, "bundle-oci"))
				} else {
					assert.FileExists(t, blob)
				}
				assert.FileExists(t, filepath.Join(path, "layers", "legacy"))
				assert.FileExists(t, filepath.Join(path, "images", "zarf"))
			}
		})
	}
}

func TestClearCachePrompt(t *testing.T) {
	for _, input := range []string{"yes\n", "no\n", "\n", ""} {
		t.Run(fmt.Sprintf("input=%q", input), func(t *testing.T) {
			cache := t.TempDir()
			seedCache(t, cache)
			streams, in, out, errOut := iostreams.NewTestIOStreams()
			_, err := in.WriteString(input)
			require.NoError(t, err)
			require.NoError(t, testutil.ExecuteCLI(t.Context(), streams, "tools", "clear-cache", "--uds-cache", cache, "--prompt"))
			assert.Empty(t, out.String())
			assert.Contains(t, errOut.String(), filepath.Join(cache, "bundle-oci"))
			assert.Contains(t, errOut.String(), "[y/N]")
			blob := filepath.Join(cache, "bundle-oci", "images", "blobs", "sha256", "blob")
			if input == "yes\n" {
				assert.NoFileExists(t, blob)
			} else {
				assert.FileExists(t, blob)
			}
		})
	}
}

func TestClearCacheMissingAndDiagnostics(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "missing")
	streams, _, out, errOut := iostreams.NewTestIOStreams()
	require.NoError(t, testutil.ExecuteCLI(t.Context(), streams, "tools", "clear-cache", "--uds-cache", cache, "--log-level", "debug"))
	assert.NoDirExists(t, cache)
	assert.Empty(t, out.String())
	assert.Contains(t, errOut.String(), "skipping cleanup")
	seedCache(t, cache)
	streams, _, out, errOut = iostreams.NewTestIOStreams()
	require.NoError(t, testutil.ExecuteCLI(t.Context(), streams, "tools", "clear-cache", "--uds-cache", cache, "--log-level", "debug"))
	assert.Empty(t, out.String())
	assert.Contains(t, errOut.String(), "removed Next OCI cache content")
	assert.Contains(t, errOut.String(), "Next OCI cache cleared")
}

func TestClearCacheInvalidInput(t *testing.T) {
	cache := t.TempDir()
	seedCache(t, cache)
	for _, args := range [][]string{
		{"unexpected-argument"},
		{"--config", filepath.Join(t.TempDir(), "missing.hcl")},
	} {
		streams, _, _, _ := iostreams.NewTestIOStreams()
		require.Error(t, testutil.ExecuteCLI(t.Context(), streams, append([]string{"tools", "clear-cache", "--uds-cache", cache}, args...)...))
		assert.FileExists(t, filepath.Join(cache, "bundle-oci", "images", "blobs", "sha256", "blob"))
	}
}

func TestClearCachePromptReadFailure(t *testing.T) {
	cache := t.TempDir()
	seedCache(t, cache)
	readErr := errors.New("input unavailable")
	streams := iostreams.New(failingReader{err: readErr}, io.Discard, io.Discard)
	err := testutil.ExecuteCLI(t.Context(), streams, "tools", "clear-cache", "--uds-cache", cache, "--prompt")
	require.ErrorIs(t, err, readErr)
	assert.FileExists(t, filepath.Join(cache, "bundle-oci", "images", "blobs", "sha256", "blob"))
}

type failingReader struct{ err error }

func (r failingReader) Read([]byte) (int, error) { return 0, r.err }

func seedCache(t *testing.T, cache string) {
	t.Helper()
	for _, name := range []string{"bundle-oci/images/blobs/sha256/blob", "layers/legacy", "images/zarf"} {
		path := filepath.Join(cache, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte("cached"), 0o600))
	}
}
