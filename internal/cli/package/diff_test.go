// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package packagecli

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/defenseunicorns/uds-cli/internal/zarf/modify"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrintDiffReportClean(t *testing.T) {
	var out bytes.Buffer
	require.NoError(t, printDiffReport(&out, modify.DiffResult{
		PackageDigest: "sha256:package",
		SourceHint:    "oci://registry.example/package:1.0.0",
	}, false))
	assert.Equal(t, "Package source: \"oci://registry.example/package:1.0.0\"\nPackage digest: sha256:package\n\nNo changes.\n", out.String())
}

func TestPrintDiffReportChanges(t *testing.T) {
	result := modify.DiffResult{
		Changes: []modify.FileChange{
			{
				Before: &modify.FileRecord{Path: "a.bin", Kind: "binary", Size: 4, Digest: "sha256:original"},
				After:  &modify.FileRecord{Path: "a.bin", Kind: "binary", Size: 8, Digest: "sha256:changed"},
			},
			{
				Before: &modify.FileRecord{Path: "b-link", Kind: "symlink", Size: 3, Digest: "sha256:old", Target: "old"},
				After:  &modify.FileRecord{Path: "b-link", Kind: "symlink", Size: 3, Digest: "sha256:new", Target: "new"},
			},
		},
	}
	var out bytes.Buffer
	require.NoError(t, printDiffReport(&out, result, false))
	assert.Equal(t, "Package source: \"\"\nPackage digest: \n\n"+
		"modified \"a.bin\"\nold size 4 bytes\nold sha256:original\nnew size 8 bytes\nnew sha256:changed\n\n"+
		"modified \"b-link\"\nold target \"old\"\nnew target \"new\"\n", out.String())
}

func TestPrintDiffReportMetadata(t *testing.T) {
	for _, tc := range []struct {
		name, status  string
		before, after *modify.FileRecord
		want          string
	}{
		{
			name: "binary same size", status: "modified",
			before: &modify.FileRecord{Kind: "binary", Size: 4, Digest: "sha256:old"},
			after:  &modify.FileRecord{Kind: "binary", Size: 4, Digest: "sha256:new"},
			want:   "old sha256:old\nnew sha256:new\n",
		},
		{
			name: "binary size only", status: "modified",
			before: &modify.FileRecord{Kind: "binary", Size: 4, Digest: "sha256:same"},
			after:  &modify.FileRecord{Kind: "binary", Size: 8, Digest: "sha256:same"},
			want:   "old size 4 bytes\nnew size 8 bytes\n",
		},
		{
			name: "added binary", status: "added",
			after: &modify.FileRecord{Kind: "binary", Size: 4, Digest: "sha256:new"},
			want:  "size 4 bytes\nsha256:new\n",
		},
		{
			name: "deleted binary", status: "deleted",
			before: &modify.FileRecord{Kind: "binary", Size: 4, Digest: "sha256:old"},
			want:   "size 4 bytes\nsha256:old\n",
		},
		{
			name: "added empty text", status: "added",
			after: &modify.FileRecord{Kind: "text", Digest: "sha256:empty"},
		},
		{
			name: "deleted empty text", status: "deleted",
			before: &modify.FileRecord{Kind: "text", Digest: "sha256:empty"},
		},
		{
			name: "text to binary", status: "modified",
			before: &modify.FileRecord{Kind: "text", Size: 4, Digest: "sha256:old"},
			after:  &modify.FileRecord{Kind: "binary", Size: 4, Digest: "sha256:new"},
			want:   "old type text\nnew type binary\nnew size 4 bytes\nnew sha256:new\n",
		},
		{
			name: "binary to text", status: "modified",
			before: &modify.FileRecord{Kind: "binary", Size: 4, Digest: "sha256:old"},
			after:  &modify.FileRecord{Kind: "text", Size: 4, Digest: "sha256:new"},
			want:   "old type binary\nnew type text\nold size 4 bytes\nold sha256:old\n",
		},
		{
			name: "added symlink", status: "added",
			after: &modify.FileRecord{Kind: "symlink", Size: 4, Digest: "sha256:new", Target: "a\n\"b"},
			want:  "target \"a\\n\\\"b\"\n",
		},
		{
			name: "deleted symlink", status: "deleted",
			before: &modify.FileRecord{Kind: "symlink", Size: 3, Digest: "sha256:old", Target: "old"},
			want:   "target \"old\"\n",
		},
		{
			name: "text to symlink", status: "modified",
			before: &modify.FileRecord{Kind: "text", Size: 4, Digest: "sha256:old"},
			after:  &modify.FileRecord{Kind: "symlink", Target: "new"},
			want:   "old type text\nnew type symlink\nnew target \"new\"\n",
		},
		{
			name: "symlink to text", status: "modified",
			before: &modify.FileRecord{Kind: "symlink", Target: "old"},
			after:  &modify.FileRecord{Kind: "text", Size: 4, Digest: "sha256:new"},
			want:   "old type symlink\nnew type text\nold target \"old\"\n",
		},
		{
			name: "binary to symlink", status: "modified",
			before: &modify.FileRecord{Kind: "binary", Size: 4, Digest: "sha256:old"},
			after:  &modify.FileRecord{Kind: "symlink", Target: "new"},
			want:   "old type binary\nnew type symlink\nold size 4 bytes\nold sha256:old\nnew target \"new\"\n",
		},
		{
			name: "symlink to binary", status: "modified",
			before: &modify.FileRecord{Kind: "symlink", Target: "old"},
			after:  &modify.FileRecord{Kind: "binary", Size: 4, Digest: "sha256:new"},
			want:   "old type symlink\nnew type binary\nold target \"old\"\nnew size 4 bytes\nnew sha256:new\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.before != nil {
				tc.before.Path = "source"
			}
			if tc.after != nil {
				tc.after.Path = "source"
			}
			result := modify.DiffResult{Changes: []modify.FileChange{{Before: tc.before, After: tc.after}}}
			for _, color := range []bool{false, true} {
				var out bytes.Buffer
				require.NoError(t, printDiffReport(&out, result, color))
				want := tc.status + " \"source\"\n" + tc.want
				if color {
					want = "\x1b[1m" + strings.ReplaceAll(strings.TrimSuffix(want, "\n"), "\n", "\x1b[m\n\x1b[1m") + "\x1b[m\n"
				}
				assert.Equal(t, "Package source: \"\"\nPackage digest: \n\n"+want, out.String(), "color=%t", color)
			}
		})
	}
}

func TestPrintDiffReportRenames(t *testing.T) {
	for _, tc := range []struct {
		kind, metadata string
	}{
		{kind: "binary", metadata: "sha256:same\n"},
		{kind: "text"},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			before := modify.FileRecord{Path: "old\n\"name", Kind: tc.kind, Size: 4, Digest: "sha256:same"}
			after := modify.FileRecord{Path: "new name", Kind: tc.kind, Size: 4, Digest: "sha256:same"}
			result := modify.DiffResult{Changes: []modify.FileChange{{Before: &before, After: &after}}}
			for _, color := range []bool{false, true} {
				var out bytes.Buffer
				require.NoError(t, printDiffReport(&out, result, color))
				want := "renamed \"old\\n\\\"name\" -> \"new name\"\n" + tc.metadata
				if color {
					want = "\x1b[1m" + strings.ReplaceAll(strings.TrimSuffix(want, "\n"), "\n", "\x1b[m\n\x1b[1m") + "\x1b[m\n"
				}
				assert.Equal(t, "Package source: \"\"\nPackage digest: \n\n"+want, out.String(), "color=%t", color)
			}
		})
	}
}

func TestPrintDiffReportWriteError(t *testing.T) {
	reader, writer := io.Pipe()
	require.NoError(t, reader.Close())
	t.Cleanup(func() { require.NoError(t, writer.Close()) })
	require.ErrorIs(t, printDiffReport(writer, modify.DiffResult{}, false), io.ErrClosedPipe)
}
