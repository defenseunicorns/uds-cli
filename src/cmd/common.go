// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package cmd contains the CLI commands for UDS.
package cmd

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/defenseunicorns/pkg/helpers"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/config/lang"
	"github.com/defenseunicorns/uds-cli/src/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils"
	zarfConfig "github.com/defenseunicorns/zarf/src/config"
	"github.com/defenseunicorns/zarf/src/pkg/message"
	zarfTypes "github.com/defenseunicorns/zarf/src/types"
	"github.com/spf13/cobra"
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
)

// isValidConfigOption checks if a string is a valid config option
func isValidConfigOption(str string) bool {
	switch configOption(str) {
	case confirm, insecure, cachePath, tempDirectory, logLevelOption, architecture, noLogFile, noProgress:
		return true
	default:
		return false
	}
}

// deploy performs validation, confirmation and deployment of a bundle
func deploy(bndlClient *bundle.Bundle) {
	_, _, _, err := bndlClient.PreDeployValidation()
	if err != nil {
		message.Fatalf(err, "Failed to validate bundle: %s", err.Error())
	}
	// confirm deployment
	if ok := bndlClient.ConfirmBundleDeploy(); !ok {
		message.Fatal(nil, "bundle deployment cancelled")
	}

	// deploy the bundle
	if err := bndlClient.Deploy(); err != nil {
		bndlClient.ClearPaths()
		message.Fatalf(err, "Failed to deploy bundle: %s", err.Error())
	}
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
}

func setBundleFile(args []string) {
	pathToBundleFile := ""
	if len(args) > 0 {
		if !helpers.IsDir(args[0]) {
			message.Fatalf(nil, "(%q) is not a valid path to a directory", args[0])
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
		message.Fatalf(err, "Neither %s or %s found", config.BundleYAML, bundleYml)
	}
}

func cliSetup(cmd *cobra.Command) {
	match := map[string]message.LogLevel{
		"warn":  message.WarnLevel,
		"info":  message.InfoLevel,
		"debug": message.DebugLevel,
		"trace": message.TraceLevel,
	}

	printViperConfigUsed()

	// No log level set, so use the default
	if logLevel != "" {
		if lvl, ok := match[logLevel]; ok {
			message.SetLogLevel(lvl)
			message.Debug("Log level set to " + logLevel)
		} else {
			message.Warn(lang.RootCmdErrInvalidLogLevel)
		}
	}

	if !config.SkipLogFile && !config.ListTasks {
		err := utils.ConfigureLogs(cmd)
		if err != nil {
			message.Fatalf(err, "Error configuring logs")
		}
	}
}
