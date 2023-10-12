// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package utils provides utility fns for UDS-CLI
package utils

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/zarf/src/pkg/message"
	"github.com/defenseunicorns/zarf/src/pkg/utils"
	"github.com/defenseunicorns/zarf/src/pkg/utils/helpers"
	"github.com/pterm/pterm"
)

// MergeVariables merges the variables from the config file and the CLI
//
// TODO: move this to helpers.MergeAndTransformMap
func MergeVariables(left map[string]string, right map[string]string) map[string]string {
	// Ensure uppercase keys from viper and CLI --set
	leftUpper := helpers.TransformMapKeys(left, strings.ToUpper)
	rightUpper := helpers.TransformMapKeys(right, strings.ToUpper)

	// Merge the viper config file variables and provided CLI flag variables (CLI takes precedence))
	return helpers.MergeMap(leftUpper, rightUpper)
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

// UseLogFile writes output to stderr and a logFile.
func UseLogFile() {
	// LogWriter is the stream to write logs to.
	var LogWriter io.Writer = os.Stderr

	// Write logs to stderr and a buffer for logFile generation.
	var logFile *os.File

	// Prepend the log filename with a timestamp.
	ts := time.Now().Format("2006-01-02-15-04-05")

	var err error
	if logFile != nil {
		// Use the existing log file if logFile is set
		LogWriter = io.MultiWriter(os.Stderr, logFile)
		pterm.SetDefaultOutput(LogWriter)
	} else {
		// Try to create a temp log file if one hasn't been made already
		if logFile, err = os.CreateTemp("", fmt.Sprintf("uds-%s-*.log", ts)); err != nil {
			message.WarnErr(err, "Error saving a log file to a temporary directory")
		} else {
			LogWriter = io.MultiWriter(os.Stderr, logFile)
			pterm.SetDefaultOutput(LogWriter)
			msg := fmt.Sprintf("Saving log file to %s", logFile.Name())
			message.Note(msg)
		}
	}
}
