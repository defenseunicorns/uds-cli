// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package testutil provides isolated helpers for CLI integration tests.
package testutil

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// CommandOptions configures an isolated UDS CLI child process.
type CommandOptions struct {
	Dir string
	Env map[string]string
}

// CommandResult contains the output and error from a UDS CLI child process.
type CommandResult struct {
	Stdout string
	Stderr string
	Err    error
}

// CLIPath returns the CLI binary configured for integration tests.
func CLIPath(t *testing.T) string {
	t.Helper()

	path := os.Getenv("UDS_CLI_PATH")
	if path == "" {
		panic("UDS_CLI_PATH must be configured for CLI integration tests")
	}

	return path
}

// RunCLI runs the configured CLI in an isolated child process.
func RunCLI(t *testing.T, opts CommandOptions, args ...string) CommandResult {
	t.Helper()

	command := exec.CommandContext(t.Context(), CLIPath(t), args...)
	command.Dir = opts.Dir
	command.Env = childEnv(opts.Env)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr

	err := command.Run()
	result := CommandResult{Stdout: stdout.String(), Stderr: stderr.String(), Err: err}
	if result.Err != nil {
		t.Log(commandDiagnostics(args, result))
	}

	return result
}

func childEnv(overrides map[string]string) []string {
	environment := os.Environ()
	for key, value := range overrides {
		prefix := key + "="
		for index := 0; index < len(environment); index++ {
			if strings.HasPrefix(environment[index], prefix) {
				environment = append(environment[:index], environment[index+1:]...)
				index--
			}
		}
		environment = append(environment, prefix+value)
	}
	return environment
}
