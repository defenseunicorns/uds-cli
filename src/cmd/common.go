// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package cmd contains the CLI commands for UDS.
package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/defenseunicorns/pkg/helpers/v2"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/config/lang"
	"github.com/defenseunicorns/uds-cli/src/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	zarfConfig "github.com/zarf-dev/zarf/src/config"
	"github.com/zarf-dev/zarf/src/pkg/logger"
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
func deploy(ctx context.Context, bndlClient *bundle.Bundle) error {
	_, _, _, err := bndlClient.PreDeployValidation()
	if err != nil {
		return fmt.Errorf("failed to validate bundle: %s", err.Error())
	}

	// confirm deployment
	if ok := bndlClient.ConfirmBundleDeploy(); !ok {
		return errors.New("bundle deployment cancelled")
	}

	// deploy the bundle
	if err := bndlClient.Deploy(ctx); err != nil {
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
	ctx := cmd.Context()
	printViperConfigUsed()

	if config.NoColor {
		pterm.DisableColor()
	}

	cfg := logger.Config{
		Level: logger.Info,
		// TODO UDS will need to decide if they want to support other formats like json and if so, get that from a cli flag
		Format:      logger.FormatConsole,
		Destination: logger.DestinationDefault,
		Color:       logger.Color(!config.NoColor),
	}
	if logLevel != "" {
		lvl, err := logger.ParseLevel(logLevel)
		if err != nil {
			message.Warn(lang.RootCmdErrInvalidLogLevel)
		} else {
			cfg.Level = lvl
			// Convert string logLevel to message.LogLevel for Zarf
			zarfLogLevel := stringToMessageLogLevel(logLevel)
			message.SetLogLevel(zarfLogLevel)
		}
	}
	l, err := logger.New(cfg)
	if err != nil {
		return err
	}

	// This is using the same logger as Zarf, uds-cli could also make it's own logger
	ctx = logger.WithContext(ctx, l)
	cmd.SetContext(ctx)
	l.Debug("logger successfully initialized", "cfg", cfg)

	// configure logs for UDS after calling zarfCommon.SetupCLI
	if !config.SkipLogFile && !config.ListTasks {
		err := utils.ConfigureLogs(cmd)
		if err != nil {
			return err
		}
	}

	return nil
}

// stringToMessageLogLevel converts a string log level to message.LogLevel type
func stringToMessageLogLevel(level string) message.LogLevel {
	switch strings.ToLower(level) {
	case "warn", "warning":
		return message.WarnLevel
	case "info":
		return message.InfoLevel
	case "debug":
		return message.DebugLevel
	case "trace":
		return message.TraceLevel
	default:
		// Default to InfoLevel if not recognized
		return message.InfoLevel
	}
}
