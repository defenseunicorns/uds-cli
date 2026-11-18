// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present the Maru Authors

package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/defenseunicorns/maru-runner/src/config"
	"github.com/defenseunicorns/maru-runner/src/config/lang"
	"github.com/defenseunicorns/maru-runner/src/message"
	"github.com/spf13/viper"
)

// Root config keys
const (
	V_LOG_LEVEL    = "options.log_level"
	V_ARCHITECTURE = "options.architecture"
	V_NO_PROGRESS  = "options.no_progress"
	V_NO_LOG_FILE  = "options.no_log_file"
	V_TMP_DIR      = "options.tmp_dir"
	V_AUTH         = "options.auth"
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

	// Specify an alternate config file
	cfgFile := os.Getenv("MARU_CONFIG")

	// Don't forget to read config either from cfgFile or from home directory!
	if cfgFile != "" {
		// Use config file from the flag.
		v.SetConfigFile(cfgFile)
	} else {
		// Search config paths (order matters!)
		v.AddConfigPath(".")
		v.AddConfigPath("$HOME/.maru")
		// todo: make configurable
		v.SetConfigName("maru-config")
	}

	// we replace 'OPTIONS.' because in a maru-config.yaml, the key is options.<opt>, but in the environment, it's MARU_<OPT>
	// e.g. MARU_LOG_LEVEL=debug
	v.SetEnvPrefix(config.EnvPrefix)
	v.SetEnvKeyReplacer(strings.NewReplacer("OPTIONS.", ""))
	v.AutomaticEnv()

	vConfigError = v.ReadInConfig()
	if vConfigError != nil {
		// Config file not found; ignore
		if _, ok := vConfigError.(viper.ConfigFileNotFoundError); !ok {
			message.SLog.Debug(vConfigError.Error())
			message.SLog.Warn(fmt.Sprintf("%s - %s", lang.CmdViperErrLoadingConfigFile, vConfigError.Error()))
		}
	}
}

func printViperConfigUsed() {
	// Optional, so ignore file not found errors
	if vConfigError != nil {
		// Config file not found; ignore
		if _, ok := vConfigError.(viper.ConfigFileNotFoundError); !ok {
			message.SLog.Debug(vConfigError.Error())
			message.SLog.Warn(fmt.Sprintf("%s - %s", lang.CmdViperErrLoadingConfigFile, vConfigError.Error()))
		}
	} else {
		message.SLog.Info(fmt.Sprintf(lang.CmdViperInfoUsingConfigFile, v.ConfigFileUsed()))
	}
}
