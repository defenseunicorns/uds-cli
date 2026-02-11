// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build integration

package version_test

import (
	"bytes"
	"testing"

	cmdversion "github.com/defenseunicorns/uds-cli/pkg/cmd/version"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
)

func TestVersionCommand_Integration(t *testing.T) {
	streams, _, out, _ := iostreams.NewTestIOStreams()

	cmd := cmdversion.NewVersionCommand(streams)
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !bytes.Contains(out.Bytes(), []byte("uds version")) {
		t.Errorf("unexpected output: %s", out.String())
	}
}
