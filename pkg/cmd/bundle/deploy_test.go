// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"testing"

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

			if (err != nil) != tt.wantErr {
				t.Errorf("Run() error = %v, wantErr %v", err, tt.wantErr)
			}

			if got := out.String(); got != tt.wantOutput {
				t.Errorf("Run() output = %q, want %q", got, tt.wantOutput)
			}
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

			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDeployOptions_Complete(t *testing.T) {
	streams, _, _, _ := iostreams.NewTestIOStreams()
	o := &DeployOptions{IOStreams: streams}

	cmd := NewDeployCommand(streams)

	err := o.Complete(cmd, []string{"ghcr.io/test:v1"})
	if err != nil {
		t.Errorf("Complete() error = %v", err)
	}

	if o.OCIReference != "ghcr.io/test:v1" {
		t.Errorf("Complete() OCIReference = %q, want %q", o.OCIReference, "ghcr.io/test:v1")
	}
}
