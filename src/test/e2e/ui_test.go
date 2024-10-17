// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestUDSUI(t *testing.T) {
	t.Run("Test uds ui command", func(t *testing.T) {
		// Create a context with a timeout of 10 seconds
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Prepare the command
		cmd := exec.CommandContext(ctx, e2e.UDSBinPath, "ui", "-l", "debug")

		// Capture stdout and stderr
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		// Start the command
		err := cmd.Start()
		require.NoError(t, err, "Failed to start the command")

		// Use a channel to signal when the command is done
		done := make(chan error, 1)
		go func() {
			done <- cmd.Wait()
		}()

		// Wait for the server to start (adjust sleep time as needed)
		time.Sleep(2 * time.Second)

		// Send interrupt signal
		err = cmd.Process.Signal(os.Interrupt)
		require.NoError(t, err, "Failed to send interrupt signal")

		// Wait for either the command to finish or the context to timeout
		select {
		case <-ctx.Done():
			t.Fatal("Command did not exit after interrupt")
		case err := <-done:
			require.Error(t, err)
		}

		// Check stdout for Runtime output indicating that it's running as expected
		require.Contains(t, stdout.String(), "GET https://runtime-local.uds.dev:8443")

		// Check stderr for CLI output indicating server startup
		require.Contains(t, stderr.String(), "Starting server")
	})
}
