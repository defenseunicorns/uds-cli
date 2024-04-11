// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package utils provides utility fns for UDS-CLI
package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/defenseunicorns/pkg/helpers"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/defenseunicorns/zarf/src/pkg/message"
	av4 "github.com/mholt/archiver/v4"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

// GracefulPanic in the event of a panic, attempt to reset the terminal using the 'reset' command.
func GracefulPanic() {
	if r := recover(); r != nil {
		fmt.Println("Recovering from panic to reset terminal before exiting")
		// todo: this approach is heavy-handed, consider alternatives using the term lib (check out what BubbleTea does)
		cmd := exec.Command("reset")
		cmd.Stdout = os.Stdout
		cmd.Stdin = os.Stdin
		_ = cmd.Run()
		panic(r)
	}
}

// IsValidTarballPath returns true if the path is a valid tarball path to a bundle tarball
func IsValidTarballPath(path string) bool {
	if helpers.InvalidPath(path) || helpers.IsDir(path) {
		return false
	}
	name := filepath.Base(path)
	if name == "" {
		return false
	}
	if !strings.HasPrefix(name, config.BundlePrefix) {
		return false
	}
	re := regexp.MustCompile(`^uds-bundle-.*-.*.tar(.zst)?$`)
	return re.MatchString(name)
}

// ConfigureLogs sets up the log file, log cache and output for the CLI
func ConfigureLogs(cmd *cobra.Command) error {
	// don't configure UDS logs for vendored cmds
	if strings.HasPrefix(cmd.Use, "zarf") || strings.HasPrefix(cmd.Use, "run") {
		return nil
	}
	writer, err := message.UseLogFile("")
	logFile := writer
	if err != nil {
		return err

	}
	tmpLogLocation := message.LogFileLocation()
	config.LogFileName = tmpLogLocation

	// Set up cache dir and cache logs file
	cacheDir := filepath.Join(config.CommonOptions.CachePath)
	if err := os.MkdirAll(cacheDir, 0o0755); err != nil { // Ensure the directory exists
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	// remove old cache logs file, and set up symlink to the new log file
	os.Remove(filepath.Join(config.CommonOptions.CachePath, config.CachedLogs))
	if err = os.Symlink(tmpLogLocation, filepath.Join(config.CommonOptions.CachePath, config.CachedLogs)); err != nil {
		return err
	}

	logWriter := io.MultiWriter(logFile)

	// use Zarf pterm output if no-tea flag is set
	// todo: as more bundle ops use BubbleTea, need to also check them alongside 'deploy'
	if !(strings.HasPrefix(cmd.Parent().Use, "uds") && strings.HasPrefix(cmd.Use, "deploy")) || config.CommonOptions.NoTea {
		message.Notef("Saving log file to %s", tmpLogLocation)
		logWriter = io.MultiWriter(os.Stderr, logFile)
		pterm.SetDefaultOutput(logWriter)
		return nil
	}

	pterm.SetDefaultOutput(logWriter)

	// disable progress bars (otherwise they will still get printed to STDERR)
	message.NoProgress = true

	message.Debugf(fmt.Sprintf("Saving log file to %s", tmpLogLocation))
	return nil
}

// ExtractJSON extracts and unmarshals a tarballed JSON file into a type
func ExtractJSON(j any) func(context.Context, av4.File) error {
	return func(_ context.Context, file av4.File) error {
		stream, err := file.Open()
		if err != nil {
			return err
		}
		defer stream.Close()

		fileBytes, err := io.ReadAll(stream)
		if err != nil {
			return err
		}
		return json.Unmarshal(fileBytes, &j)
	}
}

// ToLocalFile takes an arbitrary type, typically a struct, marshals it into JSON and stores it as a local file
func ToLocalFile(t any, filePath string) error {
	b, err := json.Marshal(t)
	if err != nil {
		return err
	}
	tFile, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer tFile.Close()
	_, err = tFile.Write(b)
	if err != nil {
		return err
	}
	return nil
}

// IsRemotePkg returns true if the Zarf package is remote
func IsRemotePkg(pkg types.Package) bool {
	return pkg.Repository != ""
}

func hasScheme(s string) bool {
	return strings.Contains(s, "://")
}

// hasDomain checks if a string contains a domain.
// It assumes the domain is at the beginning of a URL and there is no scheme (e.g., oci://).
func hasDomain(s string) bool {
	dotIndex := strings.Index(s, ".")
	firstSlashIndex := strings.Index(s, "/")

	// dot exists; dot is not first char; not preceded by any / if / exists
	return dotIndex != -1 && dotIndex != 0 && (firstSlashIndex == -1 || firstSlashIndex > dotIndex)
}

func hasPort(s string) bool {
	// look for colon and port (e.g localhost:31999)
	colonIndex := strings.Index(s, ":")
	firstSlashIndex := strings.Index(s, "/")
	endIndex := firstSlashIndex
	if firstSlashIndex == -1 {
		endIndex = len(s) - 1
	}
	if colonIndex != -1 {
		port := s[colonIndex+1 : endIndex]

		// port valid number ?
		_, err := strconv.Atoi(port)
		if err == nil {
			return true
		}
	}
	return false
}

// IsRegistryURL checks if a string is a URL
func IsRegistryURL(s string) bool {
	if hasScheme(s) || hasDomain(s) || hasPort(s) {
		return true
	}

	return false
}
