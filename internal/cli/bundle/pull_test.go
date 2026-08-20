// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
)

func TestPullOptions_Run_PromptDecline(t *testing.T) {
	tempDir := t.TempDir()

	tests := []struct {
		name          string
		input         string
		wantErrOutput []string
	}{
		{
			name:  "prompt flag - user confirms no",
			input: "n\n",
			wantErrOutput: []string{
				"Pull this bundle?",
			},
		},
		{
			name:  "prompt flag - empty input treated as no",
			input: "\n",
			wantErrOutput: []string{
				"Pull this bundle?",
			},
		},
	}

	defaults := NewConfigResolver().Defaults()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			streams, in, out, errOut := iostreams.NewTestIOStreams()
			in.WriteString(tt.input)

			o := &PullOptions{
				OCIReference: "oci://example.com/bundle:v1",
				OutputDir:    tempDir,
				Prompt:       true,
				Config:       &bundle.UDSBundleConfig{Options: &defaults},
				Verification: VerifyOptions{SkipSignatureVerification: true},
				IOStreams:    streams,
			}

			err := o.Run(t.Context())
			require.NoError(t, err)
			assert.Empty(t, out.String(), "stdout should be empty when pull is cancelled")
			for _, expected := range tt.wantErrOutput {
				assert.Contains(t, errOut.String(), expected)
			}
		})
	}
}
