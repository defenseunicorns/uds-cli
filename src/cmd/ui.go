// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package cmd contains the CLI commands for UDS.
package cmd

import (
	"embed"
)

// //go:embed bin/uds-runtime-*
// var embeddedFiles embed.FS

//go:embed ui/build/*
var assets embed.FS

//go:embed ui/certs/cert.pem
var localCert []byte

//go:embed ui/certs/key.pem
var localKey []byte

// var uiOldCmd = &cobra.Command{
// 	Use:   "ui-old",
// 	Short: lang.CmdUIShort,
// 	RunE: func(cmd *cobra.Command, _ []string) error {
// 		ctx, cancel := context.WithCancel(cmd.Context())
// 		defer cancel()

// 		// Create a temporary file to hold the embedded runtime binary
// 		tmpFile, err := os.CreateTemp("", "uds-runtime-*")
// 		if err != nil {
// 			return fmt.Errorf("failed to create temp file: %v", err)
// 		}
// 		tmpFilePath := tmpFile.Name()

// 		// Set up cleanup to run on both normal exit and interrupt
// 		cleanupDone := make(chan struct{})
// 		go func() {
// 			sigChan := make(chan os.Signal, 1)
// 			signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
// 			select {
// 			case <-sigChan:
// 				cancel()
// 			case <-ctx.Done():
// 			}
// 			err := tmpFile.Close()
// 			if err != nil && !errors.Is(err, os.ErrClosed) {
// 				message.Debug("Failed to close temporary runtime bin: %v", err)
// 			}
// 			err = os.Remove(tmpFilePath)
// 			if err != nil {
// 				message.Debug("Failed to remove temporary runtime bin: %v", err)
// 			}
// 			message.Debug("Temporary runtime bin removed")
// 			close(cleanupDone)
// 		}()

// 		// Ensure cleanup happens even if the function returns early
// 		defer func() {
// 			cancel()
// 			<-cleanupDone
// 			message.Debug("Cleanup complete")
// 		}()

// 		// Get the name of the runtime binary for the current OS and architecture
// 		runtimeBinaryPath := fmt.Sprintf("bin/uds-runtime-%s-%s", runtime.GOOS, runtime.GOARCH)

// 		// Read the embedded runtime binary
// 		data, err := embeddedFiles.ReadFile(runtimeBinaryPath)
// 		if err != nil {
// 			return err
// 		}

// 		// Write the binary data to the temporary file
// 		if _, err := tmpFile.Write(data); err != nil {
// 			return fmt.Errorf("failed to write to temp file: %v", err)
// 		}
// 		if err := tmpFile.Close(); err != nil {
// 			return fmt.Errorf("failed to close temp file: %v", err)
// 		}

// 		// Make the temporary file executable
// 		if err := os.Chmod(tmpFilePath, 0700); err != nil {
// 			return fmt.Errorf("failed to make temp file executable: %v", err)
// 		}

// 		// Validate the temporary file path
// 		if !filepath.IsAbs(tmpFilePath) {
// 			return fmt.Errorf("temporary file path is not absolute: %s", tmpFilePath)
// 		}

// 		// Execute the runtime binary with context
// 		execCmd := exec.CommandContext(ctx, tmpFilePath)
// 		execCmd.Env = append(os.Environ(), "API_AUTH_DISABLED=false")
// 		execCmd.Stdout = os.Stdout
// 		execCmd.Stderr = os.Stderr

// 		if err := execCmd.Start(); err != nil {
// 			return fmt.Errorf("failed to start binary: %v", err)
// 		}

// 		// Wait for the command to finish
// 		err = execCmd.Wait()
// 		if err != nil && ctx.Err() == nil {
// 			return fmt.Errorf("binary execution failed: %v", err)
// 		}

// 		return nil
// 	},
// }

func startUI() error {
	// r, incluster, err := ui.Setup(&assets)
	// if err != nil {
	// 	return err
	// }
	// ui.Serve(r, localCert, localKey, incluster)

	return nil
}

// func init() {
// 	initViper()
// 	rootCmd.AddCommand(uiOldCmd)
// }
