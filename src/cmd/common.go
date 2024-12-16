// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package cmd contains the CLI commands for UDS.
package cmd

import (
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
	zarfCLI "github.com/zarf-dev/zarf/src/cmd"
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
		if err := zarfCLI.SetupMessage(zarfCLI.MessageCfg{
			Level:       logLevel,
			SkipLogFile: config.SkipLogFile,
			NoColor:     config.NoColor,
		},
		); err != nil {
			return err
		}
	}

	// configure logs for UDS after calling zarfCommon.SetupCLI
	if !config.SkipLogFile && !config.ListTasks {
		err := utils.ConfigureLogs(cmd)
		if err != nil {
			return err
		}
	}

	return nil
}
