// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWriteRenderedConfigWithoutHardLinks(t *testing.T) {
	t.Parallel()
	for _, unsupported := range []error{errors.ErrUnsupported, syscall.EOPNOTSUPP, syscall.ENOSYS} {
		t.Run(unsupported.Error(), func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			input := filepath.Join(dir, "input")
			output := filepath.Join(dir, "output")
			require.NoError(t, os.WriteFile(input, []byte("input"), 0o600))

			removed := false
			err := writeRenderedConfigWithOperations(input, output, []byte("rendered"), false, func(_, _ string) error {
				return unsupported
			}, func(path string) error {
				_, err := os.Lstat(output)
				require.ErrorIs(t, err, os.ErrNotExist)
				removed = true
				return os.Remove(path)
			})
			require.NoError(t, err)
			require.True(t, removed)
			content, err := os.ReadFile(output)
			require.NoError(t, err)
			require.Equal(t, "rendered", string(content))
			info, err := os.Stat(output)
			require.NoError(t, err)
			require.Equal(t, os.FileMode(0), info.Mode().Perm()&0o077)
		})
	}
}

func TestWriteRenderedConfigExclusiveCreationRace(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	input := filepath.Join(dir, "input")
	output := filepath.Join(dir, "output")
	require.NoError(t, os.WriteFile(input, []byte("input"), 0o600))

	err := writeRenderedConfigWithLink(input, output, []byte("rendered"), false, func(_, _ string) error {
		require.NoError(t, os.WriteFile(output, []byte("other"), 0o600))
		return errors.ErrUnsupported
	})
	require.ErrorIs(t, err, os.ErrExist)
	content, err := os.ReadFile(output)
	require.NoError(t, err)
	require.Equal(t, "other", string(content))
}

func TestWriteRenderedConfigLinkFailures(t *testing.T) {
	t.Parallel()
	for _, linkErr := range []error{syscall.EIO, syscall.ENOSPC, syscall.EPERM, syscall.EXDEV, os.ErrPermission, os.ErrExist} {
		t.Run(linkErr.Error(), func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			input := filepath.Join(dir, "input")
			output := filepath.Join(dir, "output")
			require.NoError(t, os.WriteFile(input, []byte("input"), 0o600))
			err := writeRenderedConfigWithLink(input, output, []byte("rendered"), false, func(_, _ string) error {
				return linkErr
			})
			require.ErrorIs(t, err, linkErr)
			_, err = os.Lstat(output)
			require.ErrorIs(t, err, os.ErrNotExist)
			matches, err := filepath.Glob(filepath.Join(dir, ".uds-config-render-*"))
			require.NoError(t, err)
			require.Empty(t, matches)
		})
	}
}

func TestIsUnsupportedLinkSyscallErrors(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		err         error
		unsupported bool
	}{
		{syscall.ENOSYS, true},
		{syscall.ENOTSUP, true},
		{syscall.EOPNOTSUPP, true},
		{syscall.EIO, false},
		{syscall.ENOSPC, false},
		{syscall.EPERM, false},
		{syscall.EXDEV, false},
	} {
		t.Run(tc.err.Error(), func(t *testing.T) {
			err := &os.LinkError{Op: "link", Old: "source", New: "destination", Err: tc.err}
			require.Equal(t, tc.unsupported, isUnsupportedLink(err))
		})
	}
}

func TestIsUnsupportedLinkRealFailure(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	err := os.Link(filepath.Join(dir, "missing"), filepath.Join(dir, "output"))
	require.ErrorIs(t, err, os.ErrNotExist)
	var linkErr *os.LinkError
	require.ErrorAs(t, err, &linkErr)
	require.False(t, isUnsupportedLink(err))
}

func TestIsUnsupportedLinkRealCrossDeviceFailure(t *testing.T) {
	t.Parallel()
	err := os.Link(os.DevNull, filepath.Join(t.TempDir(), "output"))
	if !errors.Is(err, syscall.EXDEV) {
		t.Skipf("cross-device link unavailable: %v", err)
	}
	require.False(t, isUnsupportedLink(err))
}

func TestWriteRenderedConfigReportsStagingCleanupFailure(t *testing.T) {
	t.Parallel()
	for _, unsupported := range []bool{false, true} {
		t.Run(map[bool]string{false: "published", true: "fallback"}[unsupported], func(t *testing.T) {
			dir := t.TempDir()
			input := filepath.Join(dir, "input")
			output := filepath.Join(dir, "output")
			require.NoError(t, os.WriteFile(input, []byte("input"), 0o600))
			cleanupErr := errors.New("cleanup failed")
			err := writeRenderedConfigWithOperations(input, output, []byte("rendered"), false, func(old, new string) error {
				if unsupported {
					return errors.ErrUnsupported
				}
				return os.Link(old, new)
			}, func(string) error { return cleanupErr })
			require.ErrorIs(t, err, cleanupErr)
			_, err = os.Stat(output)
			if unsupported {
				require.ErrorIs(t, err, os.ErrNotExist)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestWriteRenderedConfigReportsCleanupAndPublishFailures(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	input := filepath.Join(dir, "input")
	require.NoError(t, os.WriteFile(input, []byte("input"), 0o600))
	cleanupErr := errors.New("cleanup failed")
	err := writeRenderedConfigWithOperations(input, filepath.Join(dir, "output"), []byte("rendered"), false,
		func(_, _ string) error { return syscall.EIO },
		func(string) error { return cleanupErr })
	require.ErrorIs(t, err, syscall.EIO)
	require.ErrorIs(t, err, cleanupErr)
}

func TestWriteRenderedConfigExclusiveCleansUpWriteFailure(t *testing.T) {
	t.Parallel()
	output := filepath.Join(t.TempDir(), "output")
	writeErr := errors.New("write failed")
	err := writeRenderedConfigExclusiveWithWrite(output, []byte("rendered"), func(f *os.File, content []byte) (int, error) {
		n, err := f.Write(content[:1])
		require.NoError(t, err)
		return n, writeErr
	})
	require.ErrorIs(t, err, writeErr)
	_, err = os.Lstat(output)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestWriteRenderedConfigProtectsInput(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	input := filepath.Join(dir, "input")
	require.NoError(t, os.WriteFile(input, []byte("input"), 0o600))
	hardlink := filepath.Join(dir, "hardlink")
	outputs := []string{input}
	if err := os.Link(input, hardlink); err == nil {
		outputs = append(outputs, hardlink)
	}
	symlink := filepath.Join(dir, "symlink")
	if err := os.Symlink(input, symlink); err == nil {
		outputs = append(outputs, symlink)
	}

	for _, output := range outputs {
		t.Run(filepath.Base(output), func(t *testing.T) {
			require.Error(t, writeRenderedConfig(input, output, []byte("rendered"), true))
			content, err := os.ReadFile(input)
			require.NoError(t, err)
			require.Equal(t, "input", string(content))
		})
	}
}

func TestWriteRenderedConfigExistingOutput(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	input := filepath.Join(dir, "input")
	output := filepath.Join(dir, "output")
	require.NoError(t, os.WriteFile(input, []byte("input"), 0o600))
	require.NoError(t, os.WriteFile(output, []byte("original"), 0o644)) //nolint:gosec // Verify replacement tightens permissions on an existing file.

	require.Error(t, writeRenderedConfig(input, output, []byte("rendered"), false))
	content, err := os.ReadFile(output)
	require.NoError(t, err)
	require.Equal(t, "original", string(content))
	require.NoError(t, writeRenderedConfig(input, output, []byte("rendered"), true))
	content, err = os.ReadFile(output)
	require.NoError(t, err)
	require.Equal(t, "rendered", string(content))
	info, err := os.Stat(output)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0), info.Mode().Perm()&0o077)
}
