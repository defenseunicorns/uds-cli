// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
)

func TestPushOptions_Run_PromptDecline(t *testing.T) {
	tempDir := t.TempDir()
	tarball := filepath.Join(tempDir, "bundle.tar.zst")
	require.NoError(t, os.WriteFile(tarball, []byte("fake"), 0o644))

	tests := []struct {
		name          string
		input         string
		wantErrOutput []string
	}{
		{
			name:  "prompt flag - user confirms no",
			input: "n\n",
			wantErrOutput: []string{
				"Push this bundle?",
			},
		},
		{
			name:  "prompt flag - empty input treated as no",
			input: "\n",
			wantErrOutput: []string{
				"Push this bundle?",
			},
		},
	}

	defaults := NewConfigResolver().Defaults()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			streams, in, out, errOut := iostreams.NewTestIOStreams()
			in.WriteString(tt.input)

			o := &PushOptions{
				Tarball:      tarball,
				OCIReference: "oci://example.com/bundle:v1",
				Config:       &bundle.UDSBundleConfig{Global: &bundle.GlobalOptions{Prompt: true}, Options: &defaults},
				IOStreams:    streams,
			}

			err := o.Run(context.Background())
			require.NoError(t, err)
			assert.Empty(t, out.String(), "stdout should be empty when push is cancelled")
			for _, expected := range tt.wantErrOutput {
				assert.Contains(t, errOut.String(), expected)
			}
		})
	}
}

func TestPushOptions_NoninteractivePrompt(t *testing.T) {
	global := &bundle.GlobalOptions{}
	assert.False(t, global.Prompt, "Prompt should default to false (non-interactive)")
}
