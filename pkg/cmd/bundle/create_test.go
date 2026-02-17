// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
)

func TestCreateOptions_Run(t *testing.T) {
	tests := []struct {
		name       string
		bundleFile string
		wantOutput string
		wantErr    bool
	}{
		{
			name:       "valid bundle file",
			bundleFile: "test-bundle.hcl",
			wantOutput: "Creating bundle from file: test-bundle.hcl\n",
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			streams, _, out, _ := iostreams.NewTestIOStreams()

			o := &CreateOptions{
				BundleFile: tt.bundleFile,
				IOStreams:  streams,
			}

			err := o.Run()

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, tt.wantOutput, out.String())
		})
	}
}

func TestCreateOptions_Validate(t *testing.T) {
	tests := []struct {
		name       string
		bundleFile string
		wantErr    bool
	}{
		{
			name:       "empty bundle file",
			bundleFile: "",
			wantErr:    true,
		},
		{
			name:       "valid bundle file",
			bundleFile: "test.hcl",
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := &CreateOptions{BundleFile: tt.bundleFile}

			err := o.Validate()

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestCreateOptions_Complete(t *testing.T) {
	streams, _, _, _ := iostreams.NewTestIOStreams()
	o := &CreateOptions{IOStreams: streams}

	cmd := NewCreateCommand(streams)

	err := o.Complete(cmd, []string{"my-bundle.hcl"})
	require.NoError(t, err)
	assert.Equal(t, "my-bundle.hcl", o.BundleFile)
}

func TestCreateOptions_Complete_NoArgs(t *testing.T) {
	streams, _, _, _ := iostreams.NewTestIOStreams()
	o := &CreateOptions{IOStreams: streams}

	cmd := NewCreateCommand(streams)

	err := o.Complete(cmd, []string{})
	require.NoError(t, err)
	assert.Empty(t, o.BundleFile)
}
