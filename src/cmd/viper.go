// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2021-Present The UDS Authors

// Package cmd contains the CLI commands for UDS.
package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/defenseunicorns/uds-cli/src/config/lang"
	"github.com/defenseunicorns/zarf/src/cmd/common"
	"github.com/defenseunicorns/zarf/src/pkg/message"
	"github.com/spf13/viper"
)

const (
	// Root config keys
	V_LOG_LEVEL    = "log_level"
	V_ARCHITECTURE = "architecture"
	V_NO_LOG_FILE  = "no_log_file"
	V_NO_PROGRESS  = "no_progress"
	V_UDS_CACHE    = "uds_cache"
	V_TMP_DIR      = "tmp_dir"
	V_INSECURE     = "insecure"

	// Bundle config keys
	V_BNDL_OCI_CONCURRENCY = "bundle.oci_concurrency"

	// Bundle create config keys
	V_BNDL_CREATE_OUTPUT               = "bundle.create.output"
	V_BNDL_CREATE_SIGNING_KEY          = "bundle.create.signing-key"
	V_BNDL_CREATE_SIGNING_KEY_PASSWORD = "bundle.create.signing-key-password"
	V_BNDL_CREATE_SET                  = "bundle.create.set"

	// Bundle deploy config keys
	V_BNDL_DEPLOY_ZARF_PACKAGES = "bundle.deploy.zarf-packages"

	// Bundle inspect config keys
	V_BNDL_INSPECT_KEY = "bundle.inspect.key"

	// Bundle remove config keys
	V_BNDL_REMOVE_PACKAGES = "bundle.remove.packages"

	// Bundle pull config keys
	V_BNDL_PULL_OUTPUT = "bundle.pull.output"
	V_BNDL_PULL_KEY    = "bundle.pull.key"
)

var (
	// Viper instance used by the cmd package
	v *viper.Viper

	// holds any error from reading in Viper config
	vConfigError error
)

func initViper() {
	// Already initialized by some other command
	if v != nil {
		return
	}

	v = viper.New()

	// Skip for vendor-only commands
	if common.CheckVendorOnlyFromArgs() {
		return
	}

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

	// E.g. UDS_LOG_LEVEL=debug
	v.SetEnvPrefix("uds")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	vConfigError = v.ReadInConfig()
	if vConfigError != nil {
		// Config file not found; ignore
		if _, ok := vConfigError.(viper.ConfigFileNotFoundError); !ok {
			message.WarnErr(vConfigError, fmt.Sprintf("%s - %s", lang.CmdViperErrLoadingConfigFile, vConfigError.Error()))
		}
	}
}

func printViperConfigUsed() {
	// Optional, so ignore file not found errors
	if vConfigError != nil {
		// Config file not found; ignore
		if _, ok := vConfigError.(viper.ConfigFileNotFoundError); !ok {
			message.WarnErr(vConfigError, fmt.Sprintf("%s - %s", lang.CmdViperErrLoadingConfigFile, vConfigError.Error()))
		}
	} else {
		message.Notef(lang.CmdViperInfoUsingConfigFile, v.ConfigFileUsed())
	}
}
