// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/defenseunicorns/uds-cli/internal/legacy/cli/monitor"
	"github.com/defenseunicorns/uds-cli/pkg/legacy/config"
	"github.com/defenseunicorns/uds-cli/pkg/legacy/config/lang"
	"github.com/defenseunicorns/uds-cli/pkg/legacy/message"
	"github.com/defenseunicorns/uds-cli/pkg/legacy/types"
	goyaml "github.com/goccy/go-yaml"
	"github.com/spf13/cobra"
)

var (
	logLevel string

	// Default global config for the bundler
	bundleCfg = types.BundleConfig{}
)

// NewCommand constructs the Legacy UDS command tree.
func NewCommand() (*cobra.Command, error) {
	message.InitializePTerm(os.Stderr)
	bundleCfg = types.BundleConfig{}
	logLevel = ""
	if err := initViper(); err != nil {
		return nil, err
	}

	if v.ConfigFileUsed() != "" {
		if err := loadViperConfig(); err != nil {
			return nil, fmt.Errorf("failed to load uds-config: %w", err)
		}
	}

	rootCmd := &cobra.Command{
		Use: "uds COMMAND",
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			// Don't add the logo to the help command
			if cmd.Parent() == nil {
				config.SkipLogFile = true
			}

			// don't load typical log configs for the logs command
			if cmd.Use != "logs" {
				err := cliSetup(cmd)
				if err != nil {
					return err
				}
			}
			return nil
		},
		Short:         lang.RootCmdShort,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, _ = fmt.Fprintln(os.Stderr)
			err := cmd.Help()
			if err != nil {
				return errors.New("error calling help command")
			}
			return nil
		},
	}

	// disable default completion command
	rootCmd.CompletionOptions.DisableDefaultCmd = true

	v.SetDefault(V_LOG_LEVEL, "info")
	v.SetDefault(V_ARCHITECTURE, "")
	v.SetDefault(V_NO_LOG_FILE, false)
	v.SetDefault(V_NO_PROGRESS, false)
	v.SetDefault(V_INSECURE, false)
	v.SetDefault(V_NO_COLOR, false)
	v.SetDefault(V_BNDL_PUBLISH_FORCE_UPLOAD, false)
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
	rootCmd.PersistentFlags().BoolVar(&config.CommonOptions.SkipSignatureValidation, "skip-signature-validation", v.GetBool(V_SKIP_SIGNATURE_VALIDATION), lang.RootCmdFlagSkipSignatureValidation)
	rootCmd.PersistentFlags().IntVar(&config.CommonOptions.OCIConcurrency, "oci-concurrency", v.GetInt(V_BNDL_OCI_CONCURRENCY), lang.CmdBundleFlagConcurrency)
	rootCmd.PersistentFlags().BoolVar(&config.NoColor, "no-color", v.GetBool(V_NO_COLOR), lang.RootCmdFlagNoColor)
	rootCmd.PersistentFlags().String("cli-mode", "", "Select CLI mode")

	rootCmd.AddCommand(monitor.NewCommand())
	addUDSCommands(rootCmd)
	rootCmd.AddCommand(newDevCommand())
	rootCmd.AddCommand(newCompletionCommand(rootCmd))
	rootCmd.AddCommand(newVersionCommand())
	runnerCmd, zarfCmd := newVendoredCommands()
	rootCmd.AddCommand(runnerCmd, zarfCmd)
	rootCmd.AddCommand(newInternalCommand(rootCmd, zarfCmd))

	return rootCmd, nil
}

// loadViperConfig reads the config file and unmarshals the relevant config into DeployOpts.Variables
func loadViperConfig() error {
	// get config file from Viper
	configFile, err := os.ReadFile(v.ConfigFileUsed())
	if err != nil {
		return err
	}

	err = unmarshalAndValidateConfig(configFile, &bundleCfg)
	if err != nil {
		return err
	}

	// ensure the DeployOpts.Variables pkg vars are uppercase
	for pkgName, pkgVar := range bundleCfg.DeployOpts.Variables {
		for varName, varValue := range pkgVar {
			// delete the lowercase var and replace with uppercase
			delete(bundleCfg.DeployOpts.Variables[pkgName], varName)
			bundleCfg.DeployOpts.Variables[pkgName][strings.ToUpper(varName)] = varValue
		}
	}

	// ensure the DeployOpts.SharedVariables vars are uppercase
	for varName, varValue := range bundleCfg.DeployOpts.SharedVariables {
		// delete the lowercase var and replace with uppercase
		delete(bundleCfg.DeployOpts.SharedVariables, varName)
		bundleCfg.DeployOpts.SharedVariables[strings.ToUpper(varName)] = varValue
	}

	return nil
}

func unmarshalAndValidateConfig(configFile []byte, bundleCfg *types.BundleConfig) error {
	// read relevant config into DeployOpts.Variables
	// need to use goyaml because Viper doesn't preserve case: https://github.com/spf13/viper/issues/1014
	// unmarshalling into DeployOpts because we want to check all of the top level config keys which are currently defined in DeployOpts
	err := goyaml.UnmarshalWithOptions(configFile, &bundleCfg.DeployOpts, goyaml.Strict())
	if err != nil {
		return err
	}
	// validate config options
	for optionName := range bundleCfg.DeployOpts.Options {
		if !isValidConfigOption(optionName) {
			return fmt.Errorf("invalid config option: %s", optionName)
		}
	}
	return nil
}
