// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"strings"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
)

func TestInspectOptions_Run(t *testing.T) {
	tests := []struct {
		name      string
		bundleRef string
		wantOut   []string
	}{
		{
			name:      "with bundle reference",
			bundleRef: "ghcr.io/test/bundle:v1",
			wantOut:   []string{"Bundle inspect command - placeholder", "Would inspect: ghcr.io/test/bundle:v1"},
		},
		{
			name:      "without bundle reference",
			bundleRef: "",
			wantOut:   []string{"Bundle inspect command - placeholder", "No bundle reference provided"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			streams, _, out, _ := iostreams.NewTestIOStreams()
			o := &InspectOptions{
				BundleRef: tt.bundleRef,
				IOStreams: streams,
			}

			err := o.Run()
			if err != nil {
				t.Errorf("Run() error = %v, want nil", err)
			}

			output := out.String()
			for _, want := range tt.wantOut {
				if !strings.Contains(output, want) {
					t.Errorf("Run() output missing %q, got: %s", want, output)
				}
			}
		})
	}
}

func TestInspectOptions_Validate(t *testing.T) {
	streams, _, _, _ := iostreams.NewTestIOStreams()
	o := NewInspectOptions(streams)

	err := o.Validate()
	if err != nil {
		t.Errorf("Validate() error = %v, want nil", err)
	}
}

func TestInspectOptions_Complete(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		wantBundleRef string
	}{
		{
			name:          "with bundle reference",
			args:          []string{"ghcr.io/test/bundle:v1"},
			wantBundleRef: "ghcr.io/test/bundle:v1",
		},
		{
			name:          "without args",
			args:          []string{},
			wantBundleRef: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			streams, _, _, _ := iostreams.NewTestIOStreams()
			o := &InspectOptions{IOStreams: streams}

			cmd := NewInspectCommand(streams)

			err := o.Complete(cmd, tt.args)
			if err != nil {
				t.Errorf("Complete() error = %v", err)
			}

			if o.BundleRef != tt.wantBundleRef {
				t.Errorf("Complete() BundleRef = %q, want %q", o.BundleRef, tt.wantBundleRef)
			}
		})
	}
}
