// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package modify

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiffReportsSourceChanges(t *testing.T) {
	dir := t.TempDir()
	files := map[string][]byte{
		"a.txt": []byte("original\n"), "a/z.txt": []byte("nested\n"),
		"binary-delete": {0, 1}, "binary-modify": {0, 2},
		"delete.txt": []byte("deleted text"), "empty.txt": {},
		"mode.sh": []byte("echo unchanged\n"), "newline.txt": []byte("same"),
		"to-binary": []byte("previous text\n"), "to-text": {0, 3},
		"repo/.git/config": []byte("excluded\n"), "repo/.gitignore": []byte("tracked\n"),
		"repo/.uds/notes": []byte("excluded\n"), "hidden/.config": []byte("included\n"),
	}
	for name, contents := range files {
		require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), contents, 0o600))
	}
	external := filepath.Join(t.TempDir(), "external")
	require.NoError(t, os.WriteFile(external, []byte("must not be inventoried"), 0o600))
	require.NoError(t, os.Symlink(external, filepath.Join(dir, "link")))
	require.NoError(t, os.Symlink(filepath.Dir(external), filepath.Join(dir, "directory-link")))
	require.NoError(t, captureBaseline(t.Context(), dir, "amd64", "", contentDigest([]byte("package")), "package.tar.zst"))

	root, err := os.OpenRoot(dir)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, root.Close()) })
	metadata, err := readDisassemblyMetadata(root)
	require.NoError(t, err)
	var paths []string
	for _, record := range metadata.Files {
		paths = append(paths, record.Path)
	}
	assert.True(t, slices.IsSorted(paths))
	assert.Contains(t, paths, "hidden/.config")
	assert.Contains(t, paths, "repo/.gitignore")
	assert.NotContains(t, paths, "repo/.git/config")
	assert.NotContains(t, paths, "repo/.uds/notes")
	assert.NotContains(t, paths, "directory-link/external")
	for _, name := range []string{"binary-delete", "binary-modify", "link", "directory-link"} {
		assert.NoFileExists(t, filepath.Join(dir, ".uds", "baseline", name))
	}
	require.NoError(t, filepath.WalkDir(filepath.Join(dir, ".uds"), func(name string, entry fs.DirEntry, err error) error {
		require.NoError(t, err)
		info, err := entry.Info()
		require.NoError(t, err)
		want := fs.FileMode(0o600)
		if entry.IsDir() {
			want = 0o700
		}
		assert.Equal(t, want, info.Mode().Perm(), name)
		return nil
	}))
	clean, err := Diff(t.Context(), dir)
	require.NoError(t, err)
	assert.Empty(t, clean.Changes)
	// Ignored metadata and link destinations do not change source identity.
	require.NoError(t, os.WriteFile(external, []byte("destination changed"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "repo/.git/config"), []byte("changed"), 0o600))
	ignored, err := Diff(t.Context(), dir)
	require.NoError(t, err)
	assert.Equal(t, clean, ignored)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("edited\n"), 0o600))
	require.NoError(t, os.Remove(filepath.Join(dir, "delete.txt")))
	require.NoError(t, os.Remove(filepath.Join(dir, "binary-delete")))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "binary-modify"), []byte{0, 4, 5}, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "binary-add"), []byte{0, 6}, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "new.txt"), []byte("new text\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "new-empty"), nil, 0o600))
	require.NoError(t, os.Chmod(filepath.Join(dir, "mode.sh"), 0o700|os.ModeSetuid))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "newline.txt"), []byte("same\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "to-binary"), []byte{0, 7}, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "to-text"), []byte("now text\n"), 0o600))
	require.NoError(t, os.Remove(filepath.Join(dir, "link")))
	require.NoError(t, os.Symlink("missing-target", filepath.Join(dir, "link")))
	before := snapshotTree(t, dir)
	changed, err := Diff(t.Context(), dir)
	require.NoError(t, err)
	assert.Equal(t, before, snapshotTree(t, dir), "diff must not change source or baseline")
	assert.NotEqual(t, clean.WorkingTreeDigest, changed.WorkingTreeDigest)
	repeated, err := Diff(t.Context(), dir)
	require.NoError(t, err)
	assert.Equal(t, changed, repeated)

	wantPaths := []string{"a.txt", "binary-add", "binary-delete", "binary-modify", "delete.txt", "link", "mode.sh", "new-empty", "new.txt", "newline.txt", "to-binary", "to-text"}
	require.Len(t, changed.Changes, len(wantPaths))
	for i, change := range changed.Changes {
		assert.Equal(t, wantPaths[i], change.Path)
		switch change.Path {
		case "a.txt":
			assert.Equal(t, "modified", change.Status)
			assert.Contains(t, change.Diff, "-original\n+edited\n")
		case "binary-add":
			assert.Equal(t, "added", change.Status)
			assert.Nil(t, change.Before)
			assert.Equal(t, contentDigest([]byte{0, 6}), change.After.Digest)
			assert.EqualValues(t, 2, change.After.Size)
			assert.Empty(t, change.Diff)
		case "binary-delete":
			assert.Equal(t, "deleted", change.Status)
			assert.Nil(t, change.After)
			assert.Equal(t, contentDigest([]byte{0, 1}), change.Before.Digest)
			assert.Empty(t, change.Diff)
		case "binary-modify":
			assert.Equal(t, contentDigest([]byte{0, 2}), change.Before.Digest)
			assert.Equal(t, contentDigest([]byte{0, 4, 5}), change.After.Digest)
			assert.EqualValues(t, 3, change.After.Size)
			assert.Empty(t, change.Diff)
		case "delete.txt":
			assert.Equal(t, "deleted", change.Status)
			assert.Contains(t, change.Diff, "+++ /dev/null\n")
			assert.Contains(t, change.Diff, "-deleted text\n\\ No newline at end of file\n")
		case "link":
			assert.Equal(t, external, change.Before.Target)
			assert.Equal(t, "missing-target", change.After.Target)
			assert.Empty(t, change.Diff)
		case "mode.sh":
			assert.EqualValues(t, 0o600, change.Before.Mode)
			assert.EqualValues(t, 0o4700, change.After.Mode)
			assert.Equal(t, change.Before.Digest, change.After.Digest)
			assert.Empty(t, change.Diff)
		case "new-empty":
			assert.Equal(t, "added", change.Status)
			assert.EqualValues(t, 0, change.After.Size)
		case "new.txt":
			assert.Equal(t, "added", change.Status)
			assert.Contains(t, change.Diff, "--- /dev/null\n")
			assert.Contains(t, change.Diff, "+new text\n")
		case "newline.txt":
			assert.Contains(t, change.Diff, "-same\n\\ No newline at end of file\n+same\n")
		case "to-binary":
			assert.Equal(t, "text", change.Before.Kind)
			assert.Equal(t, "binary", change.After.Kind)
			assert.Contains(t, change.Diff, "-previous text\n")
		case "to-text":
			assert.Equal(t, "binary", change.Before.Kind)
			assert.Equal(t, "text", change.After.Kind)
			assert.Contains(t, change.Diff, "+now text\n")
		}
	}
	// A readonly source and baseline remain usable: the command needs no lock,
	// temporary file, or permissions repair in either tree.
	require.NoError(t, os.Chmod(filepath.Join(dir, ".uds", "disassembly.json"), 0o400))
	require.NoError(t, os.Chmod(filepath.Join(dir, ".uds"), 0o500))
	t.Cleanup(func() { require.NoError(t, os.Chmod(filepath.Join(dir, ".uds"), 0o700)) })
	_, err = Diff(t.Context(), dir)
	require.NoError(t, err)
}

func TestDiffRejectsInvalidBaselineWithoutPartialResults(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(t *testing.T, dir string, metadata *disassemblyMetadata)
		want string
	}{
		{name: "missing text", edit: func(t *testing.T, dir string, _ *disassemblyMetadata) {
			require.NoError(t, os.Remove(filepath.Join(dir, ".uds/baseline/source.txt")))
		}, want: "invalid text baseline"},
		{name: "corrupt text same size", edit: func(t *testing.T, dir string, _ *disassemblyMetadata) {
			require.NoError(t, os.WriteFile(filepath.Join(dir, ".uds/baseline/source.txt"), []byte("broken\n"), 0o600))
		}, want: "recorded digest"},
		{name: "missing baseline directory", edit: func(t *testing.T, dir string, _ *disassemblyMetadata) {
			require.NoError(t, os.RemoveAll(filepath.Join(dir, ".uds/baseline")))
		}, want: "restore the original .uds directory"},
		{name: "text replaced by symlink", edit: func(t *testing.T, dir string, _ *disassemblyMetadata) {
			name := filepath.Join(dir, ".uds/baseline/source.txt")
			require.NoError(t, os.Remove(name))
			require.NoError(t, os.Symlink(filepath.Join(dir, "source.txt"), name))
		}, want: "recorded type"},
		{name: "metadata traversal", edit: func(_ *testing.T, _ string, m *disassemblyMetadata) { m.Files[0].Path = "../outside" }, want: "inventory path"},
		{name: "absolute metadata path", edit: func(_ *testing.T, _ string, m *disassemblyMetadata) { m.Files[0].Path = "/outside" }, want: "inventory path"},
		{name: "excluded metadata path", edit: func(_ *testing.T, _ string, m *disassemblyMetadata) { m.Files[0].Path = "repo/.git/config" }, want: "inventory path"},
		{name: "duplicate paths", edit: func(_ *testing.T, _ string, m *disassemblyMetadata) { m.Files = append(m.Files, m.Files[0]) }, want: "inventory path"},
		{name: "bad hash", edit: func(_ *testing.T, _ string, m *disassemblyMetadata) { m.Files[0].Digest = "not-a-digest" }, want: "SHA-256 digest"},
		{name: "negative size", edit: func(_ *testing.T, _ string, m *disassemblyMetadata) { m.Files[0].Size = -1 }, want: "size"},
		{name: "invalid kind", edit: func(_ *testing.T, _ string, m *disassemblyMetadata) { m.Files[0].Kind = "unknown" }, want: "file kind"},
		{name: "missing inventory", edit: func(_ *testing.T, _ string, m *disassemblyMetadata) { m.Files = nil }, want: "file inventory is required"},
		{name: "old format", edit: func(_ *testing.T, _ string, m *disassemblyMetadata) { m.FormatVersion = "v1alpha1" }, want: "use a matching CLI version"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "source.txt"), []byte("source\n"), 0o600))
			require.NoError(t, captureBaseline(t.Context(), dir, "amd64", "", contentDigest([]byte("package")), "package.tar.zst"))
			root, err := os.OpenRoot(dir)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, root.Close()) })
			metadata, err := readDisassemblyMetadata(root)
			require.NoError(t, err)
			tc.edit(t, dir, &metadata)
			contents, err := json.Marshal(metadata)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(dir, ".uds/disassembly.json"), contents, 0o600))
			before := snapshotTree(t, dir)
			result, err := Diff(t.Context(), dir)
			require.ErrorContains(t, err, tc.want)
			assert.Equal(t, DiffResult{}, result)
			assert.Equal(t, before, snapshotTree(t, dir))
		})
	}
}

func TestDiffTextPatches(t *testing.T) {
	const header = "--- \"a/source.txt\"\n+++ \"b/source.txt\"\n"
	for _, tc := range []struct {
		name, before, after, want string
		added, deleted            bool
	}{
		{name: "modified", before: "before\n", after: "after\n", want: header + "@@ -1 +1 @@\n-before\n+after\n"},
		{name: "missing final newlines", before: "before", after: "after", want: header + "@@ -1 +1 @@\n-before\n\\ No newline at end of file\n+after\n\\ No newline at end of file\n"},
		{name: "remove final newline", before: "same\n", after: "same", want: header + "@@ -1 +1 @@\n-same\n+same\n\\ No newline at end of file\n"},
		{name: "CRLF", before: "before\r\n", after: "after\r\n", want: header + "@@ -1 +1 @@\n-before\r\n+after\r\n"},
		{name: "Unicode", before: "é☀\n", after: "é🌙\n", want: header + "@@ -1 +1 @@\n-é☀\n+é🌙\n"},
		{name: "blank context line", before: "one\n\nthree\n", after: "one\n\nfour\n", want: header + "@@ -1,3 +1,3 @@\n one\n \n-three\n+four\n"},
		{name: "context without final newline", before: "before\nsame", after: "after\nsame", want: header + "@@ -1,2 +1,2 @@\n-before\n+after\n same\n\\ No newline at end of file\n"},
		{name: "added", added: true, after: "added\n", want: "--- /dev/null\n+++ \"b/source.txt\"\n@@ -0,0 +1 @@\n+added\n"},
		{name: "deleted", deleted: true, before: "deleted\n", want: "--- \"a/source.txt\"\n+++ /dev/null\n@@ -1 +0,0 @@\n-deleted\n"},
		{name: "added without final newline", added: true, after: "added", want: "--- /dev/null\n+++ \"b/source.txt\"\n@@ -0,0 +1 @@\n+added\n\\ No newline at end of file\n"},
		{name: "empty added file", added: true},
		{name: "empty deleted file", deleted: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			name := filepath.Join(dir, "source.txt")
			if !tc.added {
				require.NoError(t, os.WriteFile(name, []byte(tc.before), 0o600))
			}
			require.NoError(t, captureBaseline(t.Context(), dir, "amd64", "", contentDigest([]byte("package")), "package.tar.zst"))
			if tc.deleted {
				require.NoError(t, os.Remove(name))
			} else {
				require.NoError(t, os.WriteFile(name, []byte(tc.after), 0o600))
			}
			result, err := Diff(t.Context(), dir)
			require.NoError(t, err)
			require.Len(t, result.Changes, 1)
			assert.Equal(t, tc.want, result.Changes[0].Diff)
		})
	}

	t.Run("quoted path", func(t *testing.T) {
		dir := t.TempDir()
		name := "sp ace\t\"é\n.txt"
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("before\n"), 0o600))
		require.NoError(t, captureBaseline(t.Context(), dir, "amd64", "", contentDigest([]byte("package")), "package.tar.zst"))
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("after\n"), 0o600))
		result, err := Diff(t.Context(), dir)
		require.NoError(t, err)
		require.Len(t, result.Changes, 1)
		assert.Equal(t, "--- \"a/sp ace\\t\\\"é\\n.txt\"\n+++ \"b/sp ace\\t\\\"é\\n.txt\"\n@@ -1 +1 @@\n-before\n+after\n", result.Changes[0].Diff)
	})
}

func TestDiffRejectsMetadataEscapesAndMalformedJSON(t *testing.T) {
	for _, contents := range []string{"{bad", `{"formatVersion":"v1alpha2","unknown":true}`, `{} {}`} {
		t.Run(contents, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.Mkdir(filepath.Join(dir, ".uds"), 0o700))
			require.NoError(t, os.WriteFile(filepath.Join(dir, ".uds/disassembly.json"), []byte(contents), 0o600))
			result, err := Diff(t.Context(), dir)
			require.ErrorContains(t, err, "decoding disassembly metadata")
			assert.Equal(t, DiffResult{}, result)
		})
	}
	dir := t.TempDir()
	require.NoError(t, os.Symlink(t.TempDir(), filepath.Join(dir, ".uds")))
	_, err := Diff(t.Context(), dir)
	require.ErrorContains(t, err, "reading disassembly metadata")
	_, err = Diff(t.Context(), "")
	require.ErrorContains(t, err, "source directory is required")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	dir = t.TempDir()
	require.NoError(t, captureBaseline(t.Context(), dir, "amd64", "", contentDigest([]byte("package")), "package.tar.zst"))
	_, err = Diff(ctx, dir)
	require.ErrorIs(t, err, context.Canceled)
}

func TestTextClassificationAcrossChunks(t *testing.T) {
	for _, text := range []string{"", "ascii\n", "café—🦅🇺🇸", "a\x00b", "a\xff", "a\xe2\x82", strings.Repeat("a", 32767) + "🦅", strings.Repeat("a", 40000) + "\x00"} {
		for _, size := range []int{1, 2, 3, 7, 32768} {
			var classifier textClassifier
			for chunk := range slices.Chunk([]byte(text), size) {
				classifier.add(chunk)
			}
			wantBinary := !utf8.ValidString(text) || strings.ContainsRune(text, 0)
			assert.Equal(t, wantBinary, classifier.binary || len(classifier.tail) > 0, "length %d chunk size %d", len(text), size)
		}
	}
}

func TestCaptureBaselineRejectsUnrepresentableLinkTargets(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Symlink("non-utf8-\xff", filepath.Join(dir, "link")))
	err := captureBaseline(t.Context(), dir, "amd64", "", contentDigest([]byte("package")), "package.tar.zst")
	require.ErrorContains(t, err, "symbolic link target is not valid UTF-8")
	assert.NoDirExists(t, filepath.Join(dir, ".uds"))
}

func TestSanitizedSourceHint(t *testing.T) {
	for _, tc := range []struct{ source, want string }{
		{"/private/directory/package.tar.zst", "package.tar.zst"},
		{"oci://user:password@registry.example/private/team/package:v1?token=secret#fragment", "oci://registry.example/package:v1"},
		{"https://user:password@server.example/private/package.tar.zst?token=secret#fragment", "https://server.example/package.tar.zst"},
		{"file:///private/directory/package.tar.zst", "file:///package.tar.zst"},
		{"https://example.com/private/package%20name.tar.zst?token=secret", "https://example.com/package%20name.tar.zst"},
	} {
		assert.Equal(t, tc.want, sanitizedSourceHint(tc.source))
	}
}

type fileSnapshot struct {
	Mode     fs.FileMode
	Modified time.Time
	Contents string
}

func snapshotTree(t *testing.T, dir string) map[string]fileSnapshot {
	t.Helper()
	root, err := os.OpenRoot(dir)
	require.NoError(t, err)
	defer func() { require.NoError(t, root.Close()) }()
	result := make(map[string]fileSnapshot)
	require.NoError(t, fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, err error) error {
		require.NoError(t, err)
		info, err := entry.Info()
		require.NoError(t, err)
		snapshot := fileSnapshot{Mode: info.Mode(), Modified: info.ModTime()}
		if info.Mode()&os.ModeSymlink != 0 {
			snapshot.Contents, err = root.Readlink(name)
		} else if info.Mode().IsRegular() {
			var contents []byte
			contents, err = root.ReadFile(name)
			snapshot.Contents = string(contents)
		}
		require.NoError(t, err)
		result[name] = snapshot
		return nil
	}))
	return result
}
