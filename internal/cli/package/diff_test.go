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
				Before: &modify.FileRecord{Path: "a.bin", Kind: "binary", Mode: 0o600, Size: 4, Digest: "sha256:original"},
				After:  &modify.FileRecord{Path: "a.bin", Kind: "binary", Mode: 0o640, Size: 8, Digest: "sha256:changed"},
			},
			{
				Before: &modify.FileRecord{Path: "b-link", Kind: "symlink", Mode: 0o777, Size: 3, Digest: "sha256:old", Target: "old"},
				After:  &modify.FileRecord{Path: "b-link", Kind: "symlink", Mode: 0o777, Size: 3, Digest: "sha256:new", Target: "new"},
			},
			{
				Before: &modify.FileRecord{Path: "d.sh", Kind: "text", Mode: 0o644, Size: 7, Digest: "sha256:same"},
				After:  &modify.FileRecord{Path: "d.sh", Kind: "text", Mode: 0o755, Size: 7, Digest: "sha256:same"},
			},
		},
	}
	var out bytes.Buffer
	require.NoError(t, printDiffReport(&out, result, false))
	report := out.String()
	for _, want := range []string{
		"modified \"a.bin\"\nold mode 100600\nnew mode 100640\nold size 4 bytes\nold sha256:original\nnew size 8 bytes\nnew sha256:changed\n",
		"modified \"b-link\"\nold target \"old\"\nnew target \"new\"\n",
		"modified \"d.sh\"\nold mode 100644\nnew mode 100755\n",
	} {
		assert.Contains(t, report, want)
	}
	assert.NotContains(t, report, "No changes.")
	assert.NotContains(t, report, "\x1b[")
	assert.NotContains(t, report, "Before:")
	assert.NotContains(t, report, "After:")
	assert.NotContains(t, report, "sha256:same")
	previous := -1
	for _, path := range []string{"a.bin", "b-link", "d.sh"} {
		index := strings.Index(report, "modified \""+path+"\"")
		require.Greater(t, index, previous, "report paths must be sorted")
		previous = index
	}
}

func TestPrintDiffReportMetadata(t *testing.T) {
	for _, tc := range []struct {
		name, status  string
		before, after *modify.FileRecord
		want          string
	}{
		{
			name: "binary same size", status: "modified",
			before: &modify.FileRecord{Kind: "binary", Mode: 0o600, Size: 4, Digest: "sha256:old"},
			after:  &modify.FileRecord{Kind: "binary", Mode: 0o600, Size: 4, Digest: "sha256:new"},
			want:   "old sha256:old\nnew sha256:new\n",
		},
		{
			name: "binary mode only", status: "modified",
			before: &modify.FileRecord{Kind: "binary", Mode: 0o600, Size: 4, Digest: "sha256:same"},
			after:  &modify.FileRecord{Kind: "binary", Mode: 0o700, Size: 4, Digest: "sha256:same"},
			want:   "old mode 100600\nnew mode 100700\n",
		},
		{
			name: "binary size only", status: "modified",
			before: &modify.FileRecord{Kind: "binary", Mode: 0o600, Size: 4, Digest: "sha256:same"},
			after:  &modify.FileRecord{Kind: "binary", Mode: 0o600, Size: 8, Digest: "sha256:same"},
			want:   "old size 4 bytes\nnew size 8 bytes\n",
		},
		{
			name: "added binary", status: "added",
			after: &modify.FileRecord{Kind: "binary", Mode: 0o600, Size: 4, Digest: "sha256:new"},
			want:  "new file mode 100600\nsize 4 bytes\nsha256:new\n",
		},
		{
			name: "deleted binary", status: "deleted",
			before: &modify.FileRecord{Kind: "binary", Mode: 0o600, Size: 4, Digest: "sha256:old"},
			want:   "deleted file mode 100600\nsize 4 bytes\nsha256:old\n",
		},
		{
			name: "added empty text", status: "added",
			after: &modify.FileRecord{Kind: "text", Mode: 0o600, Digest: "sha256:empty"},
			want:  "new file mode 100600\n",
		},
		{
			name: "deleted empty text", status: "deleted",
			before: &modify.FileRecord{Kind: "text", Mode: 0o600, Digest: "sha256:empty"},
			want:   "deleted file mode 100600\n",
		},
		{
			name: "text to binary", status: "modified",
			before: &modify.FileRecord{Kind: "text", Mode: 0o600, Size: 4, Digest: "sha256:old"},
			after:  &modify.FileRecord{Kind: "binary", Mode: 0o600, Size: 4, Digest: "sha256:new"},
			want:   "old content text\nnew content binary\nnew size 4 bytes\nnew sha256:new\n",
		},
		{
			name: "binary to text", status: "modified",
			before: &modify.FileRecord{Kind: "binary", Mode: 0o600, Size: 4, Digest: "sha256:old"},
			after:  &modify.FileRecord{Kind: "text", Mode: 0o600, Size: 4, Digest: "sha256:new"},
			want:   "old content binary\nnew content text\nold size 4 bytes\nold sha256:old\n",
		},
		{
			name: "added symlink", status: "added",
			after: &modify.FileRecord{Kind: "symlink", Mode: 0o777, Size: 4, Digest: "sha256:new", Target: "a\n\"b"},
			want:  "new file mode 120777\ntarget \"a\\n\\\"b\"\n",
		},
		{
			name: "deleted symlink", status: "deleted",
			before: &modify.FileRecord{Kind: "symlink", Mode: 0o777, Size: 3, Digest: "sha256:old", Target: "old"},
			want:   "deleted file mode 120777\ntarget \"old\"\n",
		},
		{
			name: "symlink mode only", status: "modified",
			before: &modify.FileRecord{Kind: "symlink", Mode: 0o777, Target: "same"},
			after:  &modify.FileRecord{Kind: "symlink", Mode: 0o700, Target: "same"},
			want:   "old mode 120777\nnew mode 120700\n",
		},
		{
			name: "text mode only", status: "modified",
			before: &modify.FileRecord{Kind: "text", Mode: 0o600, Digest: "sha256:same"},
			after:  &modify.FileRecord{Kind: "text", Mode: 0o700, Digest: "sha256:same"},
			want:   "old mode 100600\nnew mode 100700\n",
		},
		{
			name: "text to symlink same permissions", status: "modified",
			before: &modify.FileRecord{Kind: "text", Mode: 0o700, Size: 4, Digest: "sha256:old"},
			after:  &modify.FileRecord{Kind: "symlink", Mode: 0o700, Target: "new"},
			want:   "old mode 100700\nnew mode 120700\nnew target \"new\"\n",
		},
		{
			name: "symlink to text same permissions", status: "modified",
			before: &modify.FileRecord{Kind: "symlink", Mode: 0o700, Target: "old"},
			after:  &modify.FileRecord{Kind: "text", Mode: 0o700, Size: 4, Digest: "sha256:new"},
			want:   "old mode 120700\nnew mode 100700\nold target \"old\"\n",
		},
		{
			name: "binary to symlink", status: "modified",
			before: &modify.FileRecord{Kind: "binary", Mode: 0o600, Size: 4, Digest: "sha256:old"},
			after:  &modify.FileRecord{Kind: "symlink", Mode: 0o777, Target: "new"},
			want:   "old mode 100600\nnew mode 120777\nold size 4 bytes\nold sha256:old\nnew target \"new\"\n",
		},
		{
			name: "symlink to binary", status: "modified",
			before: &modify.FileRecord{Kind: "symlink", Mode: 0o777, Target: "old"},
			after:  &modify.FileRecord{Kind: "binary", Mode: 0o600, Size: 4, Digest: "sha256:new"},
			want:   "old mode 120777\nnew mode 100600\nold target \"old\"\nnew size 4 bytes\nnew sha256:new\n",
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
		name, kind, metadata  string
		beforeMode, afterMode uint32
	}{
		{name: "binary", kind: "binary", beforeMode: 0o600, afterMode: 0o600, metadata: "sha256:same\n"},
		{name: "binary with mode change", kind: "binary", beforeMode: 0o600, afterMode: 0o700, metadata: "old mode 100600\nnew mode 100700\nsha256:same\n"},
		{name: "text", kind: "text", beforeMode: 0o600, afterMode: 0o600},
		{name: "text with mode change", kind: "text", beforeMode: 0o600, afterMode: 0o700, metadata: "old mode 100600\nnew mode 100700\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := modify.FileRecord{Path: "old\n\"name", Kind: tc.kind, Mode: tc.beforeMode, Size: 4, Digest: "sha256:same"}
			after := modify.FileRecord{Path: "new name", Kind: tc.kind, Mode: tc.afterMode, Size: 4, Digest: "sha256:same"}
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
			assert.Equal(t, tc.beforeMode, before.Mode, "rendering must preserve original permissions")
			assert.Equal(t, tc.afterMode, after.Mode, "rendering must preserve current permissions")
		})
	}
}

func TestPrintDiffReportWriteError(t *testing.T) {
	reader, writer := io.Pipe()
	require.NoError(t, reader.Close())
	t.Cleanup(func() { require.NoError(t, writer.Close()) })
	require.ErrorIs(t, printDiffReport(writer, modify.DiffResult{}, false), io.ErrClosedPipe)
}
