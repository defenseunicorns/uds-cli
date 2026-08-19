// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package cmd contains the CLI commands for UDS.
package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/defenseunicorns/uds-cli/pkg/legacy/config/lang"
	"github.com/defenseunicorns/uds-cli/pkg/legacy/message"
	"github.com/spf13/viper"
)

const (
	// Root config keys
	V_LOG_LEVEL                 = "options.log_level"
	V_ARCHITECTURE              = "options.architecture"
	V_NO_LOG_FILE               = "options.no_log_file"
	V_NO_PROGRESS               = "options.no_progress"
	V_UDS_CACHE                 = "options.uds_cache"
	V_TMP_DIR                   = "options.tmp_dir"
	V_INSECURE                  = "options.insecure"
	V_NO_COLOR                  = "options.no_color"
	V_BNDL_OCI_CONCURRENCY      = "options.oci_concurrency"
	V_SKIP_SIGNATURE_VALIDATION = "options.skip_signature_validation"

	// Bundle create config keys
	V_BNDL_CREATE_OUTPUT               = "create.output"
	V_BNDL_CREATE_SIGNING_KEY          = "create.signing-key"
	V_BNDL_CREATE_SIGNING_KEY_PASSWORD = "create.signing-key-password"

	// Bundle inspect config keys
	V_BNDL_INSPECT_KEY = "bundle.inspect.key"

	// Bundle pull config keys
	V_BNDL_PULL_OUTPUT = "bundle.pull.output"
	V_BNDL_PULL_KEY    = "bundle.pull.key"

	// Bundle publish config keys
	V_BNDL_PUBLISH_FORCE_UPLOAD = "options.publish_force_upload"
)

var (
	// Viper instance used by the cmd package
	v *viper.Viper

	// holds any error from reading in Viper config
	vConfigError error
)

func initViper() {
	v = viper.New()
	vConfigError = nil

	// Specify an alternate config file
	cfgFile := os.Getenv("UDS_CONFIG")

	// Don't forget to read config either from cfgFile or from home directory!
	if cfgFile != "" {
		// Use config file from the flag.
		v.SetConfigFile(cfgFile)
	} else {
		// Search config paths (order matters!)
		v.AddConfigPath(".")
		v.AddConfigPath("$HOME/.uds")
		v.SetConfigName("uds-config")
	}

	// we replace 'OPTIONS.' because in a uds-config.yaml, the key is options.<opt>, but in the environment, it's UDS_<OPT>
	// e.g. UDS_LOG_LEVEL=debug
	v.SetEnvPrefix("uds")
	v.SetEnvKeyReplacer(strings.NewReplacer("OPTIONS.", ""))
	v.AutomaticEnv()
}

func readViperConfig() error {
	vConfigError = v.ReadInConfig()
	if vConfigError != nil {
		// Config file not found; ignore
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(vConfigError, &notFound) {
			return fmt.Errorf("%s: %w", lang.CmdViperErrLoadingConfigFile, vConfigError)
		}
	}
	return nil
}

func printViperConfigUsed() {
	// Optional, so ignore file not found errors
	if vConfigError != nil {
		// Config file not found; ignore
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(vConfigError, &notFound) {
			message.WarnErr(vConfigError, fmt.Sprintf("%s - %s", lang.CmdViperErrLoadingConfigFile, vConfigError.Error()))
		}
	} else {
		message.Notef(lang.CmdViperInfoUsingConfigFile, v.ConfigFileUsed())
	}
}
