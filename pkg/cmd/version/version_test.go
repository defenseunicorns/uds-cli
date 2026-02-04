// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package version

import (
	"strings"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/defenseunicorns/uds-cli/pkg/version"
)

func TestVersionOptions_Run(t *testing.T) {
	// Save original values
	origVersion := version.Version
	origGitCommit := version.GitCommit
	origBuildDate := version.BuildDate

	// Set test values
	version.Version = "test-version"
	version.GitCommit = "abc123"
	version.BuildDate = "2026-02-03"

	// Restore original values after test
	defer func() {
		version.Version = origVersion
		version.GitCommit = origGitCommit
		version.BuildDate = origBuildDate
	}()

	streams, _, out, _ := iostreams.NewTestIOStreams()

	o := &Options{
		IOStreams: streams,
	}

	err := o.Run()
	if err != nil {
		t.Errorf("Run() error = %v, want nil", err)
	}

	output := out.String()
	if !strings.Contains(output, "test-version") {
		t.Errorf("Run() output missing version, got: %s", output)
	}
	if !strings.Contains(output, "abc123") {
		t.Errorf("Run() output missing git commit, got: %s", output)
	}
	if !strings.Contains(output, "2026-02-03") {
		t.Errorf("Run() output missing build date, got: %s", output)
	}
}

func TestVersionOptions_Complete(t *testing.T) {
	streams, _, _, _ := iostreams.NewTestIOStreams()
	o := NewOptions(streams)

	cmd := NewVersionCommand(streams)
	err := o.Complete(cmd, []string{})
	if err != nil {
		t.Errorf("Complete() error = %v, want nil", err)
	}
}

func TestVersionOptions_Validate(t *testing.T) {
	streams, _, _, _ := iostreams.NewTestIOStreams()
	o := NewOptions(streams)

	err := o.Validate()
	if err != nil {
		t.Errorf("Validate() error = %v, want nil", err)
	}
}
