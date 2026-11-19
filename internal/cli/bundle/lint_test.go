package bundle

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type lintFailWriter struct{}

func (lintFailWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestLintWriterFailurePrecedesDiagnosticFailure(t *testing.T) {
	for _, source := range []string{
		"broken {",
		"uds { bundle_api_version = \"uds.dev/v1alpha1\" }\nmetadata { name = \"test\" }\npackage \"app\" { source = \"oci://invalid.example.test/app:1\" }\n",
	} {
		root := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(root, "bundle.uds.hcl"), []byte(source), 0o600))
		o := &LintOptions{Path: root, IOStreams: iostreams.New(nil, lintFailWriter{}, nil)}
		err := o.Run(t.Context())
		require.ErrorIs(t, err, io.ErrClosedPipe)
		assert.NotErrorIs(t, err, ErrLintFailed)
	}
}
