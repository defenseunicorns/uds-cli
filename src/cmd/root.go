// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/config/lang"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils"
	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/defenseunicorns/zarf/src/cmd/common"
	"github.com/defenseunicorns/zarf/src/pkg/message"
	zarfUtils "github.com/defenseunicorns/zarf/src/pkg/utils"
	"github.com/defenseunicorns/zarf/src/pkg/utils/exec"
	"github.com/spf13/cobra"
)

var (
	logLevel string

	// Default global config for the bundler
	bundleCfg = types.BundleConfig{}
)

var rootCmd = &cobra.Command{
	Use: "uds COMMAND",
	PersistentPreRun: func(cmd *cobra.Command, _ []string) {
		// Skip for vendor-only commands
		if common.CheckVendorOnlyFromPath(cmd) {
			return
		}

		exec.ExitOnInterrupt()

		// Don't add the logo to the help command
		if cmd.Parent() == nil {
			config.SkipLogFile = true
		}
		cliSetup()
	},
	Short: lang.RootCmdShort,
	Run: func(cmd *cobra.Command, _ []string) {
		_, _ = fmt.Fprintln(os.Stderr)
		err := cmd.Help()
		if err != nil {
			message.Fatal(err, "error calling help command")
		}
	},
}

// Execute is the entrypoint for the CLI.
func Execute() {
	cobra.CheckErr(rootCmd.Execute())
}

// RootCmd returns the root command.
func RootCmd() *cobra.Command {
	return rootCmd
}

func init() {

	initViper()

	v.SetDefault(V_LOG_LEVEL, "info")
	v.SetDefault(V_ARCHITECTURE, "")
	v.SetDefault(V_NO_LOG_FILE, false)
	v.SetDefault(V_NO_PROGRESS, false)
	v.SetDefault(V_INSECURE, false)
	v.SetDefault(V_TMP_DIR, "")
	v.SetDefault(V_BNDL_OCI_CONCURRENCY, 3)

	homeDir, _ := os.UserHomeDir()
	v.SetDefault(V_UDS_CACHE, filepath.Join(homeDir, config.UDSCache))

	rootCmd.PersistentFlags().StringVarP(&logLevel, "log-level", "l", v.GetString(V_LOG_LEVEL), lang.RootCmdFlagLogLevel)
	rootCmd.PersistentFlags().StringVarP(&config.CLIArch, "architecture", "a", v.GetString(V_ARCHITECTURE), lang.RootCmdFlagArch)
	rootCmd.PersistentFlags().BoolVar(&config.SkipLogFile, "no-log-file", v.GetBool(V_NO_LOG_FILE), lang.RootCmdFlagSkipLogFile)
	rootCmd.PersistentFlags().BoolVar(&message.NoProgress, "no-progress", v.GetBool(V_NO_PROGRESS), lang.RootCmdFlagNoProgress)
	rootCmd.PersistentFlags().StringVar(&config.CommonOptions.CachePath, "uds-cache", v.GetString(V_UDS_CACHE), lang.RootCmdFlagCachePath)
	rootCmd.PersistentFlags().StringVar(&config.CommonOptions.TempDirectory, "tmpdir", v.GetString(V_TMP_DIR), lang.RootCmdFlagTempDir)
	rootCmd.PersistentFlags().BoolVar(&config.CommonOptions.Insecure, "insecure", v.GetBool(V_INSECURE), lang.RootCmdFlagInsecure)
	rootCmd.PersistentFlags().IntVar(&config.CommonOptions.OCIConcurrency, "oci-concurrency", v.GetInt(V_BNDL_OCI_CONCURRENCY), lang.CmdBundleFlagConcurrency)
}

func cliSetup() {
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

	// Disable progress bars for CI envs
	if os.Getenv("CI") == "true" {
		message.Debug("CI environment detected, disabling progress bars")
		message.NoProgress = true
	}

	if !config.SkipLogFile && !config.ListTasks {
		utils.UseLogFile()
	}
}

func CreatePreRun(args []string) {
	pathToBundleFile := ""
	if len(args) > 0 {
		if !zarfUtils.IsDir(args[0]) {
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
