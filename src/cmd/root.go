// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/config/lang"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils"
	"github.com/defenseunicorns/uds-cli/src/types"
	zarfCLI "github.com/defenseunicorns/zarf/src/cmd"
	"github.com/defenseunicorns/zarf/src/cmd/common"
	zarfConfig "github.com/defenseunicorns/zarf/src/config"
	"github.com/defenseunicorns/zarf/src/pkg/message"
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
	// grab Zarf version to make Zarf library checks happy
	if buildInfo, ok := debug.ReadBuildInfo(); ok {
		for _, dep := range buildInfo.Deps {
			if dep.Path == "github.com/defenseunicorns/zarf" {
				zarfConfig.CLIVersion = strings.Split(dep.Version, "v")[1]
			}
		}
	}

	// only vendor zarf if specifically invoked
	if len(os.Args) > 1 && (os.Args[1] == "zarf" || os.Args[1] == "z") {
		zarfCmd := &cobra.Command{
			Use:     "zarf COMMAND",
			Aliases: []string{"z"},
			Run: func(_ *cobra.Command, _ []string) {
				os.Args = os.Args[1:] // grab 'zarf' and onward from the CLI args
				zarfCLI.Execute()
			},
			DisableFlagParsing: true,
		}
		rootCmd.AddCommand(zarfCmd)

		// disable UDS log file for Zarf commands bc Zarf has its own log file
		config.SkipLogFile = true
		return
	}

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

	if !config.SkipLogFile && !config.ListTasks && !config.ListAllTasks {
		utils.UseLogFile()
	}
}
