// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
)

func TestDeployOptions_Run(t *testing.T) {
	tests := []struct {
		name         string
		ociReference string
		wantOutput   string
		wantErr      bool
	}{
		{
			name:         "valid OCI reference",
			ociReference: "ghcr.io/defenseunicorns/test-bundle:v1.0.0",
			wantOutput:   "Deploying bundle to Kubernetes cluster: ghcr.io/defenseunicorns/test-bundle:v1.0.0\n",
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			streams, _, out, _ := iostreams.NewTestIOStreams()

			o := &DeployOptions{
				OCIReference: tt.ociReference,
				IOStreams:    streams,
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

func TestDeployOptions_Validate(t *testing.T) {
	tests := []struct {
		name         string
		ociReference string
		wantErr      bool
	}{
		{
			name:         "empty OCI reference",
			ociReference: "",
			wantErr:      true,
		},
		{
			name:         "valid OCI reference",
			ociReference: "ghcr.io/test:v1",
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := &DeployOptions{OCIReference: tt.ociReference}

			err := o.Validate()

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestDeployOptions_Complete(t *testing.T) {
	streams, _, _, _ := iostreams.NewTestIOStreams()
	o := &DeployOptions{IOStreams: streams}

	cmd := NewDeployCommand(streams)

	err := o.Complete(cmd, []string{"ghcr.io/test:v1"})
	require.NoError(t, err)
	assert.Equal(t, "ghcr.io/test:v1", o.OCIReference)
}
