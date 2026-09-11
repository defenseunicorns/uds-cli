// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present the Maru Authors

// Package cmd contains the CLI commands for maru.
package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/defenseunicorns/maru-runner/src/config"
	"github.com/defenseunicorns/maru-runner/src/config/lang"
	"github.com/defenseunicorns/maru-runner/src/message"
	"github.com/defenseunicorns/maru-runner/src/pkg/utils"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var logLevelString string
var skipLogFile bool

var rootCmd = &cobra.Command{
	Use: "maru COMMAND",
	PersistentPreRun: func(cmd *cobra.Command, _ []string) {
		exitOnInterrupt()

		// Don't add the logo to the help command
		if cmd.Parent() == nil {
			skipLogFile = true
		}
		cliSetup()
	},
	Short: lang.RootCmdShort,
	Run: func(cmd *cobra.Command, _ []string) {
		_, _ = fmt.Fprintln(os.Stderr)
		err := cmd.Help()
		if err != nil {
			message.Fatalf(err, "error calling help command")
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
	v.SetDefault(V_NO_LOG_FILE, true)
	v.SetDefault(V_TMP_DIR, "")

	rootCmd.PersistentFlags().StringVarP(&logLevelString, "log-level", "l", v.GetString(V_LOG_LEVEL), lang.RootCmdFlagLogLevel)
	rootCmd.PersistentFlags().BoolVar(&message.NoProgress, "no-progress", v.GetBool(V_NO_PROGRESS), lang.RootCmdFlagNoProgress)
	rootCmd.PersistentFlags().BoolVar(&skipLogFile, "no-log-file", v.GetBool(V_NO_LOG_FILE), lang.RootCmdFlagSkipLogFile)
	rootCmd.PersistentFlags().StringVar(&config.TempDirectory, "tmpdir", v.GetString(V_TMP_DIR), lang.RootCmdFlagTempDir)
}

func cliSetup() {
	match := map[string]message.LogLevel{
		"warn":  message.WarnLevel,
		"info":  message.InfoLevel,
		"debug": message.DebugLevel,
		"trace": message.TraceLevel,
		"error": message.ErrorLevel,
	}

	printViperConfigUsed()

	// No log level set, so use the default
	if logLevelString != "" {
		if lvl, ok := match[logLevelString]; ok {
			message.SetLogLevel(lvl)
			message.SLog.Debug(fmt.Sprintf("Log level set to %q", logLevelString))
		} else {
			message.SLog.Warn(lang.RootCmdErrInvalidLogLevel)
		}
	}

	// Intentionally set logging to avoid problems when vendored
	if listTasks != listOff || listAllTasks != listOff {
		pterm.SetDefaultOutput(os.Stdout)
	} else if skipLogFile {
		pterm.SetDefaultOutput(os.Stderr)
	} else {
		if err := utils.UseLogFile(); err != nil {
			message.SLog.Warn(fmt.Sprintf("Unable to setup log file: %s", err.Error()))
		}
	}

	if os.Getenv("CI") == "true" {
		message.NoProgress = true
	}
}

// exitOnInterrupt catches an interrupt and exits with fatal error
func exitOnInterrupt() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		message.Fatalf(lang.ErrInterrupt, "%s", lang.ErrInterrupt.Error())
	}()
}
