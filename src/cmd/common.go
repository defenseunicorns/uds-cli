// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package cmd contains the CLI commands for UDS.
package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/defenseunicorns/pkg/helpers/v2"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/config/lang"
	"github.com/defenseunicorns/uds-cli/src/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	zarfConfig "github.com/zarf-dev/zarf/src/config"
	"github.com/zarf-dev/zarf/src/pkg/message"
	zarfTypes "github.com/zarf-dev/zarf/src/types"
)

type configOption string

// Valid values for options in uds_config.yaml
const (
	confirm        configOption = "confirm"
	insecure       configOption = "insecure"
	cachePath      configOption = "uds_cache"
	tempDirectory  configOption = "tmp_dir"
	logLevelOption configOption = "log_level"
	architecture   configOption = "architecture"
	noLogFile      configOption = "no_log_file"
	noProgress     configOption = "no_progress"
	noColor        configOption = "no_color"
	ociConcurrency configOption = "oci_concurrency"
)

// isValidConfigOption checks if a string is a valid config option
func isValidConfigOption(str string) bool {
	switch configOption(str) {
	case confirm, insecure, cachePath, tempDirectory, logLevelOption, architecture, noLogFile, noProgress, noColor, ociConcurrency:
		return true
	default:
		return false
	}
}

// deploy performs validation, confirmation and deployment of a bundle
func deploy(bndlClient *bundle.Bundle) error {
	_, _, _, err := bndlClient.PreDeployValidation()
	if err != nil {
		return fmt.Errorf("failed to validate bundle: %s", err.Error())
	}

	// confirm deployment
	if ok := bndlClient.ConfirmBundleDeploy(); !ok {
		return fmt.Errorf("bundle deployment cancelled")
	}

	// deploy the bundle
	if err := bndlClient.Deploy(); err != nil {
		bndlClient.ClearPaths()
		return fmt.Errorf("failed to deploy bundle: %s", err.Error())
	}

	return nil
}

// configureZarf copies configs from UDS-CLI to Zarf
func configureZarf() {
	zarfConfig.CommonOptions = zarfTypes.ZarfCommonOptions{
		Insecure:       config.CommonOptions.Insecure,
		TempDirectory:  config.CommonOptions.TempDirectory,
		OCIConcurrency: config.CommonOptions.OCIConcurrency,
		Confirm:        config.CommonOptions.Confirm,
		CachePath:      config.CommonOptions.CachePath, // use uds-cache instead of zarf-cache
	}

	// Zarf split it's "insecure" in to two flags, PlainHTTP and
	// InsecureSkipTLSVerify, with Insecure is converted to both being set to true.
	// UDS does not currently expose those flags and effectively shares the
	// --insecure flag with zarf, so when we set the common options we need to
	// set those additional flags here as well.
	// See https://github.com/zarf-dev/zarf/pull/2936 for more information.
	if config.CommonOptions.Insecure {
		zarfConfig.CommonOptions.PlainHTTP = true
		zarfConfig.CommonOptions.InsecureSkipTLSVerify = true
	}
}

func setBundleFile(args []string) error {
	pathToBundleFile := ""
	if len(args) > 0 {
		if !helpers.IsDir(args[0]) {
			return fmt.Errorf("(%q) is not a valid path to a directory", args[0])
		}
		pathToBundleFile = filepath.Join(args[0])
	}
	// Handle .yaml or .yml
	bundleYml := strings.Replace(config.BundleYAML, ".yaml", ".yml", 1)
	if _, err := os.Stat(filepath.Join(pathToBundleFile, config.BundleYAML)); err == nil {
		bundleCfg.CreateOpts.BundleFile = config.BundleYAML
	} else if _, err = os.Stat(filepath.Join(pathToBundleFile, bundleYml)); err == nil {
		bundleCfg.CreateOpts.BundleFile = bundleYml
	} else {
		return fmt.Errorf("neither %s or %s found", config.BundleYAML, bundleYml)
	}
	return nil
}

func cliSetup(cmd *cobra.Command) error {
	match := map[string]message.LogLevel{
		"warn":  message.WarnLevel,
		"info":  message.InfoLevel,
		"debug": message.DebugLevel,
		"trace": message.TraceLevel,
	}

	printViperConfigUsed()

	if config.NoColor {
		pterm.DisableColor()
	}

	// No log level set, so use the default
	if logLevel != "" {
		if lvl, ok := match[logLevel]; ok {
			message.SetLogLevel(lvl)
			message.Debug("Log level set to " + logLevel)
		} else {
			message.Warn(lang.RootCmdErrInvalidLogLevel)
		}
	}

	// don't configure Zarf CLI directly if we're calling vendored Zarf
	if !strings.HasPrefix(cmd.Use, "zarf") {
		if err := setupCLI(logLevel, config.SkipLogFile, config.NoColor); err != nil {
			return err
		}
	}

	// configure logs for UDS after calling zarfCommon.SetupCLI
	if !config.SkipLogFile && !config.ListTasks {
		err := utils.ConfigureLogs(cmd)
		if err != nil {
			return fmt.Errorf(err.Error())
		}
	}

	return nil
}

// setupCLI sets up the CLI logging. This was lifted from Zarf's Common lib as
// of v0.42.0 before it's removal. See:
// https://github.com/zarf-dev/zarf/blob/f60a70a0546026b578ea2781efe5c3a9bfac0fa7/src/cmd/common/setup.go
func setupCLI(logLevel string, skipLogFile, noColor bool) error {
	if noColor {
		message.DisableColor()
	}

	printViperConfigUsed()

	if logLevel != "" {
		match := map[string]message.LogLevel{
			"warn":  message.WarnLevel,
			"info":  message.InfoLevel,
			"debug": message.DebugLevel,
			"trace": message.TraceLevel,
		}
		lvl, ok := match[logLevel]
		if !ok {
			return errors.New("invalid log level, valid options are warn, info, debug, and trace")
		}
		message.SetLogLevel(lvl)
		message.Debug("Log level set to " + logLevel)
	}

	// Disable progress bars for CI envs
	if os.Getenv("CI") == "true" {
		message.Debug("CI environment detected, disabling progress bars")
		message.NoProgress = true
	}

	if !skipLogFile {
		ts := time.Now().Format("2006-01-02-15-04-05")
		f, err := os.CreateTemp("", fmt.Sprintf("zarf-%s-*.log", ts))
		if err != nil {
			return fmt.Errorf("could not create a log file in a the temporary directory: %w", err)
		}
		logFile, err := message.UseLogFile(f)
		if err != nil {
			return fmt.Errorf("could not save a log file to the temporary directory: %w", err)
		}
		pterm.SetDefaultOutput(io.MultiWriter(os.Stderr, logFile))
		message.Notef("Saving log file to %s", f.Name())
	}
	return nil
}
