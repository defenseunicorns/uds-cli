// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2021-Present The UDS Authors

// Package cmd contains the CLI commands for UDS.
package cmd

import (
	"os"
	"strings"

	"github.com/defenseunicorns/zarf/src/cmd/tools"
	"github.com/defenseunicorns/zarf/src/config/lang"
	"github.com/defenseunicorns/zarf/src/pkg/message"
	"github.com/spf13/viper"
)

const (
	// Root config keys
	V_LOG_LEVEL    = "log_level"
	V_ARCHITECTURE = "architecture"
	V_NO_LOG_FILE  = "no_log_file"
	V_NO_PROGRESS  = "no_progress"
	V_ZARF_CACHE   = "zarf_cache"
	V_TMP_DIR      = "tmp_dir"
	V_INSECURE     = "insecure"

	// Bundle config keys
	V_BNDL_OCI_CONCURRENCY = "bundle.oci_concurrency"

	// Bundle create config keys
	V_BNDL_CREATE_OUTPUT               = "bundle.create.output"
	V_BNDL_CREATE_SIGNING_KEY          = "bundle.create.signing_key"
	V_BNDL_CREATE_SIGNING_KEY_PASSWORD = "bundle.create.signing_key_password"
	V_BNDL_CREATE_SET                  = "bundle.create.set"

	// Bundle deploy config keys
	V_BNDL_DEPLOY_PACKAGES = "bundle.deploy.packages"
	V_BNDL_DEPLOY_SET      = "bundle.deploy.set"

	// Bundle inspect config keys
	V_BNDL_INSPECT_KEY = "bundle.inspect.key"

	// Bundle remove config keys
	V_BNDL_REMOVE_PACKAGES = "bundle.remove.packages"

	// Bundle pull config keys
	V_BNDL_PULL_OUTPUT = "bundle.pull.output"
	V_BNDL_PULL_KEY    = "bundle.pull.key"
)

func initViper() {
	// Already initialized by some other command
	if v != nil {
		return
	}

	v = viper.New()

	// Skip for vendor-only commands
	if tools.CheckVendorOnlyFromArgs() {
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

	err := v.ReadInConfig()
	if err != nil {
		// Config file not found; ignore
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			message.WarnErrorf(err, lang.CmdViperErrLoadingConfigFile, err.Error())
		}
	} else {
		message.Notef(lang.CmdViperInfoUsingConfigFile, v.ConfigFileUsed())
	}
}
