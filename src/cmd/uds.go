// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package cmd contains the CLI commands for UDS.
package cmd

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/config/lang"
	"github.com/defenseunicorns/uds-cli/src/pkg/bundle"
	zarfConfig "github.com/defenseunicorns/zarf/src/config"
	"github.com/defenseunicorns/zarf/src/pkg/message"
	zarfUtils "github.com/defenseunicorns/zarf/src/pkg/utils"
	zarfTypes "github.com/defenseunicorns/zarf/src/types"
	goyaml "github.com/goccy/go-yaml"
	"github.com/spf13/cobra"
)

var createCmd = &cobra.Command{
	Use:     "create [DIRECTORY]",
	Aliases: []string{"c"},
	Args:    cobra.MaximumNArgs(1),
	Short:   lang.CmdBundleCreateShort,
	PreRun: func(cmd *cobra.Command, args []string) {
		if len(args) > 0 && !zarfUtils.IsDir(args[0]) {
			message.Fatalf(nil, "(%q) is not a valid path to a directory", args[0])
		}
		if _, err := os.Stat(config.BundleYAML); len(args) == 0 && err != nil {
			message.Fatalf(err, "%s not found in directory", config.BundleYAML)
		}
	},
	Run: func(cmd *cobra.Command, args []string) {
		srcDir, err := os.Getwd()
		if err != nil {
			message.Fatalf(err, "error reading the current working directory")
		}
		if len(args) > 0 {
			srcDir = args[0]
		}
		bundleCfg.CreateOpts.SourceDirectory = srcDir

		bndlClient := bundle.NewOrDie(&bundleCfg)
		defer bndlClient.ClearPaths()

		if err := bndlClient.Create(); err != nil {
			bndlClient.ClearPaths()
			message.Fatalf(err, "Failed to create bundle: %s", err.Error())
		}
	},
}

var deployCmd = &cobra.Command{
	Use:     "deploy [BUNDLE_TARBALL|OCI_REF]",
	Aliases: []string{"d"},
	Short:   lang.CmdBundleDeployShort,
	Args:    cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		bundleCfg.DeployOpts.Source = chooseBundle(args)
		configureZarf()

		// load uds-config if it exists
		if v.ConfigFileUsed() != "" {
			if err := loadViperConfig(); err != nil {
				message.Fatalf(err, "Failed to load uds-config: %s", err.Error())
				return
			}
		}
		bndlClient := bundle.NewOrDie(&bundleCfg)
		defer bndlClient.ClearPaths()

		if err := bndlClient.Deploy(); err != nil {
			bndlClient.ClearPaths()
			message.Fatalf(err, "Failed to deploy bundle: %s", err.Error())
		}
	},
}

var inspectCmd = &cobra.Command{
	Use:     "inspect [BUNDLE_TARBALL|OCI_REF]",
	Aliases: []string{"i"},
	Short:   lang.CmdBundleInspectShort,
	Args:    cobra.MaximumNArgs(1),
	PreRun: func(cmd *cobra.Command, args []string) {
		if cmd.Flag("extract").Value.String() == "true" && cmd.Flag("sbom").Value.String() == "false" {
			message.Fatal(nil, "cannot use 'extract' flag without 'sbom' flag")
		}
	},
	Run: func(cmd *cobra.Command, args []string) {
		bundleCfg.InspectOpts.Source = chooseBundle(args)
		configureZarf()

		bndlClient := bundle.NewOrDie(&bundleCfg)
		defer bndlClient.ClearPaths()

		if err := bndlClient.Inspect(); err != nil {
			bndlClient.ClearPaths()
			message.Fatalf(err, "Failed to inspect bundle: %s", err.Error())
		}
	},
}

var removeCmd = &cobra.Command{
	Use:     "remove [BUNDLE_TARBALL|OCI_REF]",
	Aliases: []string{"r"},
	Args:    cobra.ExactArgs(1),
	Short:   lang.CmdBundleRemoveShort,
	Run: func(cmd *cobra.Command, args []string) {
		bundleCfg.RemoveOpts.Source = args[0]
		configureZarf()

		bndlClient := bundle.NewOrDie(&bundleCfg)
		defer bndlClient.ClearPaths()

		if err := bndlClient.Remove(); err != nil {
			bndlClient.ClearPaths()
			message.Fatalf(err, "Failed to remove bundle: %s", err.Error())
		}
	},
}

var publishCmd = &cobra.Command{
	Use:     "publish [BUNDLE_TARBALL] [OCI_REF]",
	Aliases: []string{"p"},
	Short:   lang.CmdPublishShort,
	Args:    cobra.ExactArgs(2),
	PreRun: func(cmd *cobra.Command, args []string) {
		if _, err := os.Stat(args[0]); err != nil {
			message.Fatalf(err, "First argument (%q) must be a valid local Bundle path: %s", args[0], err.Error())
		}
	},
	Run: func(cmd *cobra.Command, args []string) {
		bundleCfg.PublishOpts.Source = args[0]
		bundleCfg.PublishOpts.Destination = args[1]
		configureZarf()
		bndlClient := bundle.NewOrDie(&bundleCfg)
		defer bndlClient.ClearPaths()

		if err := bndlClient.Publish(); err != nil {
			bndlClient.ClearPaths()
			message.Fatalf(err, "Failed to publish bundle: %s", err.Error())
		}
	},
}

var pullCmd = &cobra.Command{
	Use:     "pull [OCI_REF]",
	Aliases: []string{"p"},
	Short:   lang.CmdBundlePullShort,
	Args:    cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		bundleCfg.PullOpts.Source = args[0]
		configureZarf()
		bndlClient := bundle.NewOrDie(&bundleCfg)
		defer bndlClient.ClearPaths()

		if err := bndlClient.Pull(); err != nil {
			bndlClient.ClearPaths()
			message.Fatalf(err, "Failed to pull bundle: %s", err.Error())
		}
	},
}

// loadViperConfig reads the config file and unmarshals the relevant config into DeployOpts.Variables
func loadViperConfig() error {
	// get config file from Viper
	configFile, err := os.ReadFile(v.ConfigFileUsed())
	if err != nil {
		return err
	}

	// read relevant config into DeployOpts.Variables
	// need to use goyaml because Viper doesn't preserve case: https://github.com/spf13/viper/issues/1014
	err = goyaml.Unmarshal(configFile, &bundleCfg.DeployOpts)
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

func init() {
	initViper()

	// create cmd flags
	rootCmd.AddCommand(createCmd)
	createCmd.Flags().BoolVarP(&config.CommonOptions.Confirm, "confirm", "c", false, lang.CmdBundleRemoveFlagConfirm)
	createCmd.Flags().StringVarP(&bundleCfg.CreateOpts.Output, "output", "o", v.GetString(V_BNDL_CREATE_OUTPUT), lang.CmdBundleCreateFlagOutput)
	createCmd.Flags().StringVarP(&bundleCfg.CreateOpts.SigningKeyPath, "signing-key", "k", v.GetString(V_BNDL_CREATE_SIGNING_KEY), lang.CmdBundleCreateFlagSigningKey)
	createCmd.Flags().StringVarP(&bundleCfg.CreateOpts.SigningKeyPassword, "signing-key-password", "p", v.GetString(V_BNDL_CREATE_SIGNING_KEY_PASSWORD), lang.CmdBundleCreateFlagSigningKeyPassword)

	// deploy cmd flags
	rootCmd.AddCommand(deployCmd)
	deployCmd.Flags().BoolVarP(&config.CommonOptions.Confirm, "confirm", "c", false, lang.CmdBundleDeployFlagConfirm)
	deployCmd.Flags().StringArrayVarP(&bundleCfg.DeployOpts.Packages, "packages", "p", []string{}, lang.CmdBundleDeployFlagPackages)
	deployCmd.Flags().BoolVarP(&bundleCfg.DeployOpts.Resume, "resume", "r", false, lang.CmdBundleDeployFlagResume)

	// inspect cmd flags
	rootCmd.AddCommand(inspectCmd)
	inspectCmd.Flags().BoolVarP(&bundleCfg.InspectOpts.IncludeSBOM, "sbom", "s", false, lang.CmdPackageInspectFlagSBOM)
	inspectCmd.Flags().BoolVarP(&bundleCfg.InspectOpts.ExtractSBOM, "extract", "e", false, lang.CmdPackageInspectFlagExtractSBOM)
	inspectCmd.Flags().StringVarP(&bundleCfg.InspectOpts.PublicKeyPath, "key", "k", v.GetString(V_BNDL_INSPECT_KEY), lang.CmdBundleInspectFlagKey)

	// remove cmd flags
	rootCmd.AddCommand(removeCmd)
	// confirm does not use the Viper config
	removeCmd.Flags().BoolVarP(&config.CommonOptions.Confirm, "confirm", "c", false, lang.CmdBundleRemoveFlagConfirm)
	_ = removeCmd.MarkFlagRequired("confirm")
	removeCmd.Flags().StringArrayVarP(&bundleCfg.RemoveOpts.Packages, "packages", "p", []string{}, lang.CmdBundleRemoveFlagPackages)

	// publish cmd flags
	rootCmd.AddCommand(publishCmd)

	// pull cmd flags
	rootCmd.AddCommand(pullCmd)
	pullCmd.Flags().StringVarP(&bundleCfg.PullOpts.OutputDirectory, "output", "o", v.GetString(V_BNDL_PULL_OUTPUT), lang.CmdBundlePullFlagOutput)
	pullCmd.Flags().StringVarP(&bundleCfg.PullOpts.PublicKeyPath, "key", "k", v.GetString(V_BNDL_PULL_KEY), lang.CmdBundlePullFlagKey)
}

// configureZarf copies configs from UDS-CLI to Zarf
func configureZarf() {
	zarfConfig.CommonOptions = zarfTypes.ZarfCommonOptions{
		Insecure:       config.CommonOptions.Insecure,
		TempDirectory:  config.CommonOptions.TempDirectory,
		OCIConcurrency: config.CommonOptions.OCIConcurrency,
		Confirm:        config.CommonOptions.Confirm,
		// todo: decouple Zarf cache?
		CachePath: config.CommonOptions.CachePath,
	}
}

// chooseBundle provides a file picker when users don't specify a file
func chooseBundle(args []string) string {
	if len(args) > 0 {
		return args[0]
	}
	var path string
	prompt := &survey.Input{
		Message: lang.CmdPackageChoose,
		Suggest: func(toComplete string) []string {
			files, _ := filepath.Glob(config.BundlePrefix + toComplete + "*.tar")
			gzFiles, _ := filepath.Glob(config.BundlePrefix + toComplete + "*.tar.zst")
			partialFiles, _ := filepath.Glob(config.BundlePrefix + toComplete + "*.part000")

			files = append(files, gzFiles...)
			files = append(files, partialFiles...)
			return files
		},
	}

	if err := survey.AskOne(prompt, &path, survey.WithValidator(survey.Required)); err != nil {
		message.Fatalf(nil, lang.CmdPackageChooseErr, err.Error())
	}

	return path
}
