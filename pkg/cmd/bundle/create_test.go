// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"testing"

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

			if (err != nil) != tt.wantErr {
				t.Errorf("Run() error = %v, wantErr %v", err, tt.wantErr)
			}

			if got := out.String(); got != tt.wantOutput {
				t.Errorf("Run() output = %q, want %q", got, tt.wantOutput)
			}
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

			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCreateOptions_Complete(t *testing.T) {
	streams, _, _, _ := iostreams.NewTestIOStreams()
	o := &CreateOptions{IOStreams: streams}

	cmd := NewCreateCommand(streams)

	err := o.Complete(cmd, []string{"my-bundle.hcl"})
	if err != nil {
		t.Errorf("Complete() error = %v", err)
	}

	if o.BundleFile != "my-bundle.hcl" {
		t.Errorf("Complete() BundleFile = %q, want %q", o.BundleFile, "my-bundle.hcl")
	}
}

func TestCreateOptions_Complete_NoArgs(t *testing.T) {
	streams, _, _, _ := iostreams.NewTestIOStreams()
	o := &CreateOptions{IOStreams: streams}

	cmd := NewCreateCommand(streams)

	err := o.Complete(cmd, []string{})
	if err != nil {
		t.Errorf("Complete() error = %v", err)
	}

	if o.BundleFile != "" {
		t.Errorf("Complete() BundleFile = %q, want empty string", o.BundleFile)
	}
}
