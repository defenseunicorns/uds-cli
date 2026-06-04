// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"archive/tar"
	"bytes"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/mholt/archives"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tarEntry describes a single entry to write into a synthetic tar archive.
// Body is ignored for non-regular types.
type tarEntry struct {
	name     string
	typeflag byte
	body     []byte
	linkname string
	mode     int64
}

// writeMaliciousTarZst builds a .tar.zst at dst containing the given entries
// without any of the safety filtering that writeTarZst applies on the create
// path. It exists so tests can exercise extraction against hand-crafted
// archives that a legitimate create would never produce.
func writeMaliciousTarZst(t *testing.T, dst string, entries []tarEntry) {
	t.Helper()

	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	for _, e := range entries {
		mode := e.mode
		if mode == 0 {
			if e.typeflag == tar.TypeDir {
				mode = 0o755
			} else {
				mode = 0o644
			}
		}
		hdr := &tar.Header{
			Name:     e.name,
			Mode:     mode,
			Size:     int64(len(e.body)),
			Typeflag: e.typeflag,
			Linkname: e.linkname,
		}
		if e.typeflag == tar.TypeDir {
			hdr.Size = 0
		}
		require.NoError(t, tw.WriteHeader(hdr))
		if e.typeflag == tar.TypeReg && len(e.body) > 0 {
			_, err := tw.Write(e.body)
			require.NoError(t, err)
		}
	}
	require.NoError(t, tw.Close())

	f, err := os.Create(dst)
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	zw, err := archives.Zstd{}.OpenWriter(f)
	require.NoError(t, err)
	_, err = zw.Write(tarBuf.Bytes())
	require.NoError(t, err)
	require.NoError(t, zw.Close())
}

func TestExtractTarZst_HappyPath(t *testing.T) {
	t.Parallel()

	src := filepath.Join(t.TempDir(), "ok.tar.zst")
	writeMaliciousTarZst(t, src, []tarEntry{
		{name: "top.txt", typeflag: tar.TypeReg, body: []byte("hello")},
		{name: "sub/", typeflag: tar.TypeDir},
		{name: "sub/inner.txt", typeflag: tar.TypeReg, body: []byte("inner")},
	})

	dst := t.TempDir()
	require.NoError(t, extractTarZst(context.Background(), iostreams.IOStreams{}, src, dst))

	top, err := os.ReadFile(filepath.Join(dst, "top.txt"))
	require.NoError(t, err)
	assert.Equal(t, "hello", string(top))

	inner, err := os.ReadFile(filepath.Join(dst, "sub", "inner.txt"))
	require.NoError(t, err)
	assert.Equal(t, "inner", string(inner))
}

// TestExtractTarZst_RoundTrip confirms that archives produced by writeTarZst
// (the only legitimate create path) continue to extract cleanly under the
// hardened extractor.
func TestExtractTarZst_RoundTrip(t *testing.T) {
	t.Parallel()

	srcDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(srcDir, "nested"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "a.txt"), []byte("A"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "nested", "b.txt"), []byte("B"), 0o644))

	tarball := filepath.Join(t.TempDir(), "round.tar.zst")
	require.NoError(t, writeTarZst(context.Background(), iostreams.IOStreams{}, tarball, srcDir))

	dst := t.TempDir()
	require.NoError(t, extractTarZst(context.Background(), iostreams.IOStreams{}, tarball, dst))

	a, err := os.ReadFile(filepath.Join(dst, "a.txt"))
	require.NoError(t, err)
	assert.Equal(t, "A", string(a))

	b, err := os.ReadFile(filepath.Join(dst, "nested", "b.txt"))
	require.NoError(t, err)
	assert.Equal(t, "B", string(b))
}

// TestExtractTarZst_RejectsMaliciousEntries covers malicious entries that
// zarfarchive.Decompress must reject outright. If zarf's validation changes,
// these assertions fail and force a conscious re-evaluation.
func TestExtractTarZst_RejectsMaliciousEntries(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		entries []tarEntry
		// errSubstr is matched against the returned error. Empty means "any
		// error is acceptable" — used for cases where the exact message is an
		// implementation detail of os.Root.
		errSubstr string
		// assertEscape, if set, is a path relative to the test dst parent that
		// must not exist after extraction, confirming no escape occurred.
		assertEscape string
	}{
		{
			name: "relative_parent_traversal",
			entries: []tarEntry{
				{name: "../escape.txt", typeflag: tar.TypeReg, body: []byte("x")},
			},
			assertEscape: "escape.txt",
		},
		{
			name: "nested_parent_traversal",
			entries: []tarEntry{
				{name: "foo/../../escape.txt", typeflag: tar.TypeReg, body: []byte("x")},
			},
			assertEscape: "escape.txt",
		},
		{
			name: "symlink_escape",
			entries: []tarEntry{
				{name: "link", typeflag: tar.TypeSymlink, linkname: "../../etc/passwd"},
			},
			errSubstr: "escapes root directory",
		},
		{
			name: "symlink_absolute_target",
			entries: []tarEntry{
				{name: "link", typeflag: tar.TypeSymlink, linkname: "/etc/passwd"},
			},
			errSubstr: "absolute or rooted",
		},
		{
			name: "hardlink_escape",
			entries: []tarEntry{
				{name: "real.txt", typeflag: tar.TypeReg, body: []byte("x")},
				{name: "link", typeflag: tar.TypeLink, linkname: "../outside"},
			},
		},
		{
			name: "backslash_in_name",
			entries: []tarEntry{
				{name: `dir\sub.txt`, typeflag: tar.TypeReg, body: []byte("x")},
			},
			errSubstr: "backslash",
		},
		{
			name: "colon_in_name",
			entries: []tarEntry{
				{name: "file:stream.txt", typeflag: tar.TypeReg, body: []byte("x")},
			},
			errSubstr: "':'",
		},
		{
			name: "trailing_dot",
			entries: []tarEntry{
				{name: "file.txt.", typeflag: tar.TypeReg, body: []byte("x")},
			},
			errSubstr: "trailing dots or spaces",
		},
		{
			name: "trailing_space",
			entries: []tarEntry{
				{name: "file.txt ", typeflag: tar.TypeReg, body: []byte("x")},
			},
			errSubstr: "trailing dots or spaces",
		},
		{
			name: "windows_reserved_name",
			entries: []tarEntry{
				{name: "CON", typeflag: tar.TypeReg, body: []byte("x")},
			},
			errSubstr: "reserved device name",
		},
		{
			name: "windows_reserved_name_with_extension",
			entries: []tarEntry{
				{name: "NUL.txt", typeflag: tar.TypeReg, body: []byte("x")},
			},
			errSubstr: "reserved device name",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			parent := t.TempDir()
			dst := filepath.Join(parent, "extract")
			require.NoError(t, os.MkdirAll(dst, tempDirPerm))

			src := filepath.Join(t.TempDir(), "bad.tar.zst")
			writeMaliciousTarZst(t, src, tc.entries)

			err := extractTarZst(context.Background(), iostreams.IOStreams{}, src, dst)

			require.Error(t, err, "expected error for %s", tc.name)
			if tc.errSubstr != "" {
				assert.Contains(t, err.Error(), tc.errSubstr)
			}

			if tc.assertEscape != "" {
				_, statErr := os.Stat(filepath.Join(parent, tc.assertEscape))
				assert.True(t, os.IsNotExist(statErr),
					"malicious entry escaped extraction dir: %s", filepath.Join(parent, tc.assertEscape))
			}
			assertNoEscape(t, parent, dst)
		})
	}
}

// TestExtractTarZst_ContainsMaliciousEntries covers entries that zarf does
// not reject but still neutralizes — extraction may succeed, but nothing
// must be written outside dst. The "no escape" invariant is the contract.
func TestExtractTarZst_ContainsMaliciousEntries(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		entries []tarEntry
	}{
		{
			// zarf's stripHandler normalizes a leading "/" to a relative
			// path ("abs/path.txt") via path.Join, so the write lands
			// inside root rather than erroring.
			name: "absolute_path",
			entries: []tarEntry{
				{name: "/abs/path.txt", typeflag: tar.TypeReg, body: []byte("x")},
			},
		},
		{
			// mholt/archives ignores non-regular types; zarf's
			// validateEntryName passes and writeFile is skipped by the
			// IsDir/link branches, so no error is returned.
			name: "char_device_entry",
			entries: []tarEntry{
				{name: "dev", typeflag: tar.TypeChar},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			parent := t.TempDir()
			dst := filepath.Join(parent, "extract")
			require.NoError(t, os.MkdirAll(dst, tempDirPerm))

			src := filepath.Join(t.TempDir(), "bad.tar.zst")
			writeMaliciousTarZst(t, src, tc.entries)

			_ = extractTarZst(context.Background(), iostreams.IOStreams{}, src, dst)

			assertNoEscape(t, parent, dst)
		})
	}
}

// assertNoEscape walks the parent of dst and verifies no file was created
// outside dst itself. This catches any escape regardless of the specific
// entry used in the archive.
func assertNoEscape(t *testing.T, parent, dst string) {
	t.Helper()

	dstAbs, err := filepath.Abs(dst)
	require.NoError(t, err)

	err = filepath.WalkDir(parent, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		if abs == parent || abs == dstAbs {
			return nil
		}
		// Anything under dst is fine.
		rel, err := filepath.Rel(dstAbs, abs)
		if err == nil && rel != ".." && !startsWithDotDot(rel) {
			return nil
		}
		// Sibling of dst under parent — that's the escape surface.
		t.Errorf("unexpected file outside extraction dir: %s", abs)
		return nil
	})
	require.NoError(t, err)
}

func startsWithDotDot(rel string) bool {
	if rel == ".." {
		return true
	}
	sep := string(filepath.Separator)
	if len(rel) >= 3 && rel[:3] == ".."+sep {
		return true
	}
	// On Windows, filepath.Rel may use backslash; guard cross-platform.
	if runtime.GOOS == "windows" && len(rel) >= 3 && rel[:3] == `..\` {
		return true
	}
	return false
}
