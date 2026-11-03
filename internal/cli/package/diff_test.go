package packagecli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/defenseunicorns/uds-cli/internal/zarf/modify"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrintDiffReportClean(t *testing.T) {
	var out bytes.Buffer
	require.NoError(t, printDiffReport(&out, modify.DiffResult{
		PackageDigest:     "sha256:package",
		SourceHint:        "oci://registry.example/package:1.0.0",
		WorkingTreeDigest: "sha256:working",
	}))
	assert.Equal(t, "Package digest: sha256:package\nSource hint: \"oci://registry.example/package:1.0.0\"\nWorking inventory digest: sha256:working\n\nNo changes.\n", out.String())
}

func TestPrintDiffReportChanges(t *testing.T) {
	patch := "--- a/c.txt\n+++ b/c.txt\n@@ -1 +1 @@\n-before\n+after\n"
	result := modify.DiffResult{
		Changes: []modify.FileChange{
			{
				Path: "d.sh", Status: "modified",
				Before: &modify.FileRecord{Kind: "text", Mode: 0o644, Size: 7, Digest: "sha256:same"},
				After:  &modify.FileRecord{Kind: "text", Mode: 0o755, Size: 7, Digest: "sha256:same"},
			},
			{
				Path: "c.txt", Status: "modified", Diff: patch,
				Before: &modify.FileRecord{Kind: "text", Mode: 0o600, Size: 7, Digest: "sha256:before"},
				After:  &modify.FileRecord{Kind: "text", Mode: 0o600, Size: 6, Digest: "sha256:after"},
			},
			{
				Path: "b-link", Status: "modified",
				Before: &modify.FileRecord{Kind: "symlink", Mode: 0o777, Size: 3, Digest: "sha256:old", Target: "old"},
				After:  &modify.FileRecord{Kind: "symlink", Mode: 0o777, Size: 3, Digest: "sha256:new", Target: "new"},
			},
			{
				Path: "a.bin", Status: "modified",
				Before: &modify.FileRecord{Kind: "binary", Mode: 0o600, Size: 4, Digest: "sha256:original"},
				After:  &modify.FileRecord{Kind: "binary", Mode: 0o640, Size: 8, Digest: "sha256:changed"},
			},
		},
	}
	var out bytes.Buffer
	require.NoError(t, printDiffReport(&out, result))
	report := out.String()
	for _, want := range []string{
		"Before: binary mode 0600, 4 bytes, sha256:original",
		"After: binary mode 0640, 8 bytes, sha256:changed",
		"Before: symlink mode 0777, 3 bytes, sha256:old, target \"old\"",
		"After: symlink mode 0777, 3 bytes, sha256:new, target \"new\"",
		"Before: text mode 0644, 7 bytes, sha256:same",
		"After: text mode 0755, 7 bytes, sha256:same",
		patch,
	} {
		assert.Contains(t, report, want)
	}
	assert.NotContains(t, report, "No changes.")
	previous := -1
	for _, path := range []string{"a.bin", "b-link", "c.txt", "d.sh"} {
		index := strings.Index(report, "modified \""+path+"\"")
		require.Greater(t, index, previous, "report paths must be sorted")
		previous = index
	}
	assert.Equal(t, "d.sh", result.Changes[0].Path, "rendering must not reorder the supplied result")
}
