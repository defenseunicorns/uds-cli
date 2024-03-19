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
	"strings"

	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/defenseunicorns/zarf/src/pkg/message"
	"github.com/defenseunicorns/zarf/src/pkg/utils"
	av4 "github.com/mholt/archiver/v4"
	"github.com/pterm/pterm"
)

var (
	CacheLogFile *os.File
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
	if utils.InvalidPath(path) || utils.IsDir(path) {
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
func ConfigureLogs() error {
	writer, err := message.UseLogFile("")
	logFile := writer
	if err != nil {
		return err

	}
	location := message.LogFileLocation()
	config.LogFileName = location

	// empty cache logs file
	os.Remove(filepath.Join(config.CommonOptions.CachePath, config.CachedLogs))

	// Set up cache dir and cache logs file
	cacheDir := filepath.Join(config.CommonOptions.CachePath)
	if err := os.MkdirAll(cacheDir, 0o0755); err != nil { // Ensure the directory exists
		return fmt.Errorf("failed to create cache directory: %w", err)
	}
	CacheLogFile, err = os.OpenFile(filepath.Join(config.CommonOptions.CachePath, config.CachedLogs), os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	logWriter := io.MultiWriter(logFile, CacheLogFile)

	// use Zarf pterm output if no-tea flag is set
	if !config.TeaEnabled || config.CommonOptions.NoTea {
		message.Notef("Saving log file to %s", location)
		logWriter = io.MultiWriter(os.Stderr, CacheLogFile, logFile)
		pterm.SetDefaultOutput(logWriter)
		return nil
	}

	// set pterm output to only go to this logfile
	pterm.SetDefaultOutput(logWriter)

	// disable progress bars (otherwise they will still get printed to STDERR)
	message.NoProgress = true

	message.Debugf(fmt.Sprintf("Saving log file to %s", location))
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
