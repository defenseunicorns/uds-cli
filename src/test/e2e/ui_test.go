// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

package test

import (
	"bytes"
	"context"
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
		cmd := exec.CommandContext(ctx, e2e.UDSBinPath, "ui")

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

		// Wait for either the command to finish or the context to timeout
		select {
		case <-ctx.Done():
			// Context timed out, kill the process
			err = cmd.Process.Kill()
			require.NoError(t, err, "Failed to kill the process")
		case err := <-done:
			// Command finished before timeout
			require.Error(t, err, "Command unexpectedly exited")
		}

		// Check the output
		output := stdout.String() + stderr.String()
		require.Contains(t, output, "Starting server", "Expected output not found")
	})
}
