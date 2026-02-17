// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
)

func TestPullOptions_Validate(t *testing.T) {
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
			o := &PullOptions{OCIReference: tt.ociReference}
			err := o.Validate()
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestPushOptions_Validate(t *testing.T) {
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
			o := &PushOptions{OCIReference: tt.ociReference}
			err := o.Validate()
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestRemoveOptions_Validate(t *testing.T) {
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
			o := &RemoveOptions{OCIReference: tt.ociReference}
			err := o.Validate()
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestPullOptions_Complete(t *testing.T) {
	streams, _, _, _ := iostreams.NewTestIOStreams()
	o := &PullOptions{IOStreams: streams}
	cmd := NewPullCommand(streams)

	err := o.Complete(cmd, []string{"ghcr.io/test:v1"})
	require.NoError(t, err)
	assert.Equal(t, "ghcr.io/test:v1", o.OCIReference)
}

func TestPushOptions_Complete(t *testing.T) {
	streams, _, _, _ := iostreams.NewTestIOStreams()
	o := &PushOptions{IOStreams: streams}
	cmd := NewPushCommand(streams)

	err := o.Complete(cmd, []string{"ghcr.io/test:v1"})
	require.NoError(t, err)
	assert.Equal(t, "ghcr.io/test:v1", o.OCIReference)
}

func TestRemoveOptions_Complete(t *testing.T) {
	streams, _, _, _ := iostreams.NewTestIOStreams()
	o := &RemoveOptions{IOStreams: streams}
	cmd := NewRemoveCommand(streams)

	err := o.Complete(cmd, []string{"ghcr.io/test:v1"})
	require.NoError(t, err)
	assert.Equal(t, "ghcr.io/test:v1", o.OCIReference)
}
