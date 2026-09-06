// Copyright 2024-2026 Defense Unicorns
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
	"github.com/spf13/pflag"
)

var (
	logLevel string

	// Default global config for the bundler
	bundleCfg = types.BundleConfig{}
)

// NewRootCommand constructs the complete Legacy command tree. It deliberately
// performs no command construction during package initialization so the mode
// bootstrap can select a different tree before Cobra is involved.
func NewRootCommand() *cobra.Command {
	message.InitializePTerm(os.Stderr)
	bundleCfg = types.BundleConfig{}
	logLevel = ""
	initViper()

	rootCmd := &cobra.Command{
		Use: "uds COMMAND",
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			return legacyPreRun(cmd)
		},
		Short:         lang.RootCmdShort,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, _ = fmt.Fprintln(cmd.ErrOrStderr())
			if err := cmd.Help(); err != nil {
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

	rootCmd.AddCommand(NewMonitorCommand())
	addBundleCommands(rootCmd)
	rootCmd.AddCommand(newDevCommand())
	rootCmd.AddCommand(newCompletionCommand(rootCmd))
	runnerCmd, zarfCmd := newVendoredCommands()
	rootCmd.AddCommand(runnerCmd, zarfCmd)
	rootCmd.AddCommand(newInternalCommand(rootCmd, zarfCmd))
	rootCmd.AddCommand(newVersionCommand())
	return rootCmd
}

// NewMonitorCommand constructs the retained Legacy monitor command.
func NewMonitorCommand() *cobra.Command {
	return monitor.NewCommand()
}

func legacyPreRun(cmd *cobra.Command) error {
	// Don't add the logo to the help command.
	if cmd.Parent() == nil {
		if err := cmd.Root().PersistentFlags().Set("no-log-file", "true"); err != nil {
			return err
		}
	}

	if err := readViperConfig(); err != nil {
		return err
	}
	if err := applyViperFlags(cmd); err != nil {
		return err
	}
	if v.ConfigFileUsed() != "" {
		overrides := captureChangedFlags(cmd)
		if err := loadViperConfig(); err != nil {
			return fmt.Errorf("failed to load uds-config: %w", err)
		}
		if err := restoreChangedFlags(overrides); err != nil {
			return err
		}
	}
	// The logs command reads an existing log and must not initialize logging.
	if cmd.Name() == "logs" {
		return nil
	}
	return cliSetup(cmd)
}

func applyViperFlags(cmd *cobra.Command) error {
	rootFlags := map[string]string{
		"log-level":                 V_LOG_LEVEL,
		"architecture":              V_ARCHITECTURE,
		"no-log-file":               V_NO_LOG_FILE,
		"no-progress":               V_NO_PROGRESS,
		"uds-cache":                 V_UDS_CACHE,
		"tmpdir":                    V_TMP_DIR,
		"insecure":                  V_INSECURE,
		"skip-signature-validation": V_SKIP_SIGNATURE_VALIDATION,
		"oci-concurrency":           V_BNDL_OCI_CONCURRENCY,
		"no-color":                  V_NO_COLOR,
	}
	for flagName, key := range rootFlags {
		if err := applyViperFlag(cmd.Root().PersistentFlags().Lookup(flagName), key); err != nil {
			return err
		}
	}

	commandFlags := map[string]map[string]string{
		"create": {
			"output":               V_BNDL_CREATE_OUTPUT,
			"signing-key":          V_BNDL_CREATE_SIGNING_KEY,
			"signing-key-password": V_BNDL_CREATE_SIGNING_KEY_PASSWORD,
		},
		"inspect": {"key": V_BNDL_INSPECT_KEY},
		"publish": {"force-upload": V_BNDL_PUBLISH_FORCE_UPLOAD},
		"pull": {
			"output": V_BNDL_PULL_OUTPUT,
			"key":    V_BNDL_PULL_KEY,
		},
	}
	for flagName, key := range commandFlags[cmd.Name()] {
		if err := applyViperFlag(cmd.Flags().Lookup(flagName), key); err != nil {
			return err
		}
	}
	return nil
}

func applyViperFlag(flag *pflag.Flag, key string) error {
	if flag == nil || flag.Changed || !v.IsSet(key) {
		return nil
	}
	if err := flag.Value.Set(fmt.Sprint(v.Get(key))); err != nil {
		return fmt.Errorf("apply %s from configuration: %w", flag.Name, err)
	}
	return nil
}

type changedFlag struct {
	flag  *pflag.Flag
	value string
	slice []string
}

func captureChangedFlags(cmd *cobra.Command) []changedFlag {
	overrides := []changedFlag{}
	cmd.Flags().Visit(func(flag *pflag.Flag) {
		override := changedFlag{flag: flag, value: flag.Value.String()}
		if value, ok := flag.Value.(pflag.SliceValue); ok {
			override.slice = append([]string(nil), value.GetSlice()...)
		}
		overrides = append(overrides, override)
	})
	return overrides
}

func restoreChangedFlags(overrides []changedFlag) error {
	for _, override := range overrides {
		if value, ok := override.flag.Value.(pflag.SliceValue); ok {
			if err := value.Replace(override.slice); err != nil {
				return fmt.Errorf("restore --%s: %w", override.flag.Name, err)
			}
			continue
		}
		value := override.value
		if override.flag.Value.Type() == "stringToString" {
			value = strings.TrimSuffix(strings.TrimPrefix(value, "["), "]")
		}
		if err := override.flag.Value.Set(value); err != nil {
			return fmt.Errorf("restore --%s: %w", override.flag.Name, err)
		}
	}
	return nil
}

// loadViperConfig reads the config file and unmarshals the relevant config into DeployOpts.Variables
func loadViperConfig() error {
	// get config file from Viper
	configFile, err := os.ReadFile(v.ConfigFileUsed())
	if err != nil {
		return err
	}

	hadSetVariables := bundleCfg.DeployOpts.SetVariables != nil
	err = unmarshalAndValidateConfig(configFile, &bundleCfg)
	if err != nil {
		return err
	}
	if hadSetVariables && bundleCfg.DeployOpts.SetVariables == nil {
		bundleCfg.DeployOpts.SetVariables = map[string]string{}
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
