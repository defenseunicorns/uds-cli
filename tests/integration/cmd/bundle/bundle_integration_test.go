// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build integration

package bundle_test

import (
	"bytes"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/cmd/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
)

func TestInspectCommand_Integration(t *testing.T) {
	streams, _, out, _ := iostreams.NewTestIOStreams()

	cmd := bundle.NewInspectCommand(streams)
	cmd.SetArgs([]string{"ghcr.io/test:v1"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !bytes.Contains(out.Bytes(), []byte("Bundle inspect command - placeholder")) {
		t.Errorf("unexpected output: %s", out.String())
	}
}

func TestCreateCommand_Integration(t *testing.T) {
	streams, _, out, _ := iostreams.NewTestIOStreams()

	cmd := bundle.NewCreateCommand(streams)
	cmd.SetArgs([]string{"test-bundle.hcl"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !bytes.Contains(out.Bytes(), []byte("Creating bundle")) {
		t.Errorf("unexpected output: %s", out.String())
	}
}

func TestDeployCommand_Integration(t *testing.T) {
	streams, _, out, _ := iostreams.NewTestIOStreams()

	cmd := bundle.NewDeployCommand(streams)
	cmd.SetArgs([]string{"ghcr.io/test:v1"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !bytes.Contains(out.Bytes(), []byte("Deploying bundle")) {
		t.Errorf("unexpected output: %s", out.String())
	}
}

func TestPullCommand_Integration(t *testing.T) {
	streams, _, out, _ := iostreams.NewTestIOStreams()

	cmd := bundle.NewPullCommand(streams)
	cmd.SetArgs([]string{"ghcr.io/test:v1"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !bytes.Contains(out.Bytes(), []byte("Pulling bundle")) {
		t.Errorf("unexpected output: %s", out.String())
	}
}

func TestPushCommand_Integration(t *testing.T) {
	streams, _, out, _ := iostreams.NewTestIOStreams()

	cmd := bundle.NewPushCommand(streams)
	cmd.SetArgs([]string{"ghcr.io/test:v1"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !bytes.Contains(out.Bytes(), []byte("Pushing bundle")) {
		t.Errorf("unexpected output: %s", out.String())
	}
}

func TestRemoveCommand_Integration(t *testing.T) {
	streams, _, out, _ := iostreams.NewTestIOStreams()

	cmd := bundle.NewRemoveCommand(streams)
	cmd.SetArgs([]string{"ghcr.io/test:v1"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !bytes.Contains(out.Bytes(), []byte("Removing bundle")) {
		t.Errorf("unexpected output: %s", out.String())
	}
}
