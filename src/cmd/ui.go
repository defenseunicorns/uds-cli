// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package cmd contains the CLI commands for UDS.
package cmd

import (
	"embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/defenseunicorns/uds-cli/src/config/lang"
	"github.com/spf13/cobra"
)

//go:embed bin/uds-runtime-*
var embeddedFiles embed.FS

var uiCmd = &cobra.Command{
	Use:     "ui",
	Aliases: []string{"u"},
	Short:   lang.CmdUIShort,
	RunE: func(_ *cobra.Command, _ []string) error {

		// Create a temporary file to hold the embedded runtime binary
		tmpFile, err := os.CreateTemp("", "uds-runtime-*")
		if err != nil {
			return fmt.Errorf("failed to create temp file: %v", err)
		}
		defer os.Remove(tmpFile.Name())

		// Get the name of the runtime binary for the current OS and architecture
		var runtimeBinaryPath = fmt.Sprintf("bin/uds-runtime-%s-%s", runtime.GOOS, runtime.GOARCH)

		// Read the embedded runtime binary
		data, err := embeddedFiles.ReadFile(runtimeBinaryPath)
		if err != nil {
			return err
		}

		// Write the binary data to the temporary file
		if _, err := tmpFile.Write(data); err != nil {
			return fmt.Errorf("failed to write to temp file: %v", err)
		}
		if err := tmpFile.Close(); err != nil {
			return fmt.Errorf("failed to close temp file: %v", err)
		}

		// Make the temporary file executable
		if err := os.Chmod(tmpFile.Name(), 0700); err != nil {
			return fmt.Errorf("failed to make temp file executable: %v", err)
		}

		// Validate the temporary file path
		tmpFilePath := tmpFile.Name()
		if !filepath.IsAbs(tmpFilePath) {
			return fmt.Errorf("temporary file path is not absolute: %s", tmpFilePath)
		}

		// Execute the runtime binary
		cmd := exec.Command(tmpFilePath)
		cmd.Env = append(os.Environ(), "API_AUTH_DISABLED=false")

		// Set the command's standard output and error to the current process's output and error
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		// Run the command
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to run binary: %v", err)
		}

		return nil
	},
}

func init() {
	initViper()
	rootCmd.AddCommand(uiCmd)
}
