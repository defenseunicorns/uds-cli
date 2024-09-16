// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package cmd contains the CLI commands for UDS.
package cmd

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"

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

		// Create a temporary directory to hold the embedded files
		tmpDir, err := os.MkdirTemp("", "uds-runtime-*")
		if err != nil {
			return fmt.Errorf("failed to create temp directory: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		var runtimeBinaryPath string
		// Walk through the embedded files and write them to the temporary directory, eventhough we only expect one file
		// to be embedded, the name of the binary is based on the architecture and os
		err = fs.WalkDir(embeddedFiles, "bin", func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}

			data, err := embeddedFiles.ReadFile(path)
			if err != nil {
				return err
			}

			relPath, err := filepath.Rel("bin", path)
			if err != nil {
				return err
			}

			destPath := filepath.Join(tmpDir, relPath)
			if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
				return err
			}
			// Need the runtime binaries to be executable
			//nolint:gosec
			err = os.WriteFile(destPath, data, 0700)

			runtimeBinaryPath = destPath

			return err
		})

		if err != nil {
			return fmt.Errorf("failed to write embedded file: %v", err)
		}

		// Execute the runtime binary
		cmd := exec.Command(runtimeBinaryPath)
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
