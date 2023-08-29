// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package cmd contains the CLI commands for UDS.
package cmd

import (
	"fmt"
	"github.com/AlecAivazis/survey/v2"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/config/lang"
	"github.com/defenseunicorns/uds-cli/src/pkg/bundle"
	zarfConfig "github.com/defenseunicorns/zarf/src/config"
	"github.com/defenseunicorns/zarf/src/pkg/message"
	"github.com/defenseunicorns/zarf/src/pkg/oci"
	"github.com/defenseunicorns/zarf/src/pkg/utils/helpers"
	"os"
	"path/filepath"

	"github.com/defenseunicorns/uds-cli/src/pkg/utils"
	zarfUtils "github.com/defenseunicorns/zarf/src/pkg/utils"
	"github.com/spf13/cobra"
)

var bundleCmd = &cobra.Command{
	Use:     "bundle",
	Aliases: []string{"b"},
	Short:   lang.CmdBundleShort,
}

var bundleCreateCmd = &cobra.Command{
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

		bundleCfg.CreateOpts.SetVariables = utils.MergeVariables(v.GetStringMapString(V_BNDL_CREATE_SET), bundleCfg.CreateOpts.SetVariables)

		bndlClient := bundle.NewOrDie(&bundleCfg)
		defer bndlClient.ClearPaths()

		if err := bndlClient.Create(); err != nil {
			bndlClient.ClearPaths()
			message.Fatalf(err, "Failed to create bundle: %s", err.Error())
		}
	},
}

var bundleDeployCmd = &cobra.Command{
	Use:     "deploy [BUNDLE_TARBALL|OCI_REF]",
	Aliases: []string{"d"},
	Short:   lang.CmdBundleDeployShort,
	Args:    cobra.MaximumNArgs(1),
	PreRun:  firstArgIsEitherOCIorTarball,
	Run: func(cmd *cobra.Command, args []string) {
		bundleCfg.DeployOpts.Source = choosePackage(args)
		configureZarf()

		// read config file and unmarshal
		if v.ConfigFileUsed() != "" {
			err := v.ReadInConfig()
			if err != nil {
				message.Fatalf(err, "Failed to read config: %s", err.Error())
				return
			}
			err = v.UnmarshalKey(V_BNDL_DEPLOY_ZARF_PACKAGES, &bundleCfg.DeployOpts.ZarfPackageVariables)
			if err != nil {
				message.Fatalf(err, "Failed to unmarshal config: %s", err.Error())
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

var bundleInspectCmd = &cobra.Command{
	Use:     "inspect [BUNDLE_TARBALL|OCI_REF]",
	Aliases: []string{"i"},
	Short:   lang.CmdBundleInspectShort,
	Args:    cobra.MaximumNArgs(1),
	PreRun:  firstArgIsEitherOCIorTarball,
	Run: func(cmd *cobra.Command, args []string) {
		bundleCfg.InspectOpts.Source = choosePackage(args)
		configureZarf()

		bndlClient := bundle.NewOrDie(&bundleCfg)
		defer bndlClient.ClearPaths()

		if err := bndlClient.Inspect(); err != nil {
			bndlClient.ClearPaths()
			message.Fatalf(err, "Failed to inspect bundle: %s", err.Error())
		}
	},
}

var bundleRemoveCmd = &cobra.Command{
	Use:     "remove [BUNDLE_TARBALL|OCI_REF]",
	Aliases: []string{"r"},
	Args:    cobra.ExactArgs(1),
	Short:   lang.CmdBundleRemoveShort,
	PreRun:  firstArgIsEitherOCIorTarball,
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

var bundlePullCmd = &cobra.Command{
	Use:     "pull [OCI_REF]",
	Aliases: []string{"p"},
	Short:   lang.CmdBundlePullShort,
	Args:    cobra.ExactArgs(1),
	PreRun: func(cmd *cobra.Command, args []string) {
		if err := oci.ValidateReference(args[0]); err != nil {
			message.Fatalf(err, "First agument (%q) must be a valid OCI URL: %s", args[0], err.Error())
		}
	},
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

func firstArgIsEitherOCIorTarball(_ *cobra.Command, args []string) {
	if len(args) == 0 {
		return
	}
	var errString string
	var err error
	if utils.IsValidTarballPath(args[0]) {
		return
	}
	if !helpers.IsOCIURL(args[0]) && !utils.IsValidTarballPath(args[0]) {
		errString = fmt.Sprintf("First argument (%q) must either be a valid OCI URL or a valid path to a bundle tarball", args[0])
	} else {
		err = oci.ValidateReference(args[0])
	}
	if errString != "" {
		message.Fatalf(err, "Failed to validate first argument: %s", errString)
	}
}

func init() {
	initViper()

	rootCmd.AddCommand(bundleCmd)
	v.SetDefault(V_BNDL_OCI_CONCURRENCY, 3)
	bundleCmd.PersistentFlags().IntVar(&config.CommonOptions.OCIConcurrency, "oci-concurrency", v.GetInt(V_BNDL_OCI_CONCURRENCY), lang.CmdBundleFlagConcurrency)

	bundleCmd.AddCommand(bundleCreateCmd)
	bundleCreateCmd.Flags().BoolVarP(&config.CommonOptions.Confirm, "confirm", "c", false, lang.CmdBundleRemoveFlagConfirm)
	bundleCreateCmd.Flags().StringVarP(&bundleCfg.CreateOpts.Output, "output", "o", v.GetString(V_BNDL_CREATE_OUTPUT), lang.CmdBundleCreateFlagOutput)
	bundleCreateCmd.Flags().StringVarP(&bundleCfg.CreateOpts.SigningKeyPath, "signing-key", "k", v.GetString(V_BNDL_CREATE_SIGNING_KEY), lang.CmdBundleCreateFlagSigningKey)
	bundleCreateCmd.Flags().StringVarP(&bundleCfg.CreateOpts.SigningKeyPassword, "signing-key-password", "p", v.GetString(V_BNDL_CREATE_SIGNING_KEY_PASSWORD), lang.CmdBundleCreateFlagSigningKeyPassword)
	bundleCreateCmd.Flags().StringToStringVarP(&bundleCfg.CreateOpts.SetVariables, "set", "s", v.GetStringMapString(V_BNDL_CREATE_SET), lang.CmdBundleCreateFlagSet)

	bundleCmd.AddCommand(bundleDeployCmd)
	// todo: add "set" flag on deploy for high-level bundle configs?
	bundleDeployCmd.Flags().BoolVarP(&config.CommonOptions.Confirm, "confirm", "c", false, lang.CmdBundleDeployFlagConfirm)

	bundleCmd.AddCommand(bundleInspectCmd)
	bundleInspectCmd.Flags().StringVarP(&bundleCfg.InspectOpts.PublicKeyPath, "key", "k", v.GetString(V_BNDL_INSPECT_KEY), lang.CmdBundleInspectFlagKey)

	bundleCmd.AddCommand(bundleRemoveCmd)
	// confirm does not use the Viper config
	bundleRemoveCmd.Flags().BoolVarP(&config.CommonOptions.Confirm, "confirm", "c", false, lang.CmdBundleRemoveFlagConfirm)
	_ = bundleRemoveCmd.MarkFlagRequired("confirm")

	bundleCmd.AddCommand(bundlePullCmd)
	bundlePullCmd.Flags().StringVarP(&bundleCfg.PullOpts.OutputDirectory, "output", "o", v.GetString(V_BNDL_PULL_OUTPUT), lang.CmdBundlePullFlagOutput)
	bundlePullCmd.Flags().StringVarP(&bundleCfg.PullOpts.PublicKeyPath, "key", "k", v.GetString(V_BNDL_PULL_KEY), lang.CmdBundlePullFlagKey)
}

// configureZarf copies configs from UDS-CLI to Zarf
// todo: make a PR to Zarf to get rid of this: https://github.com/defenseunicorns/uds-cli/pull/26#discussion_r1307945876
func configureZarf() {
	zarfConfig.CommonOptions.Insecure = config.CommonOptions.Insecure
	zarfConfig.CommonOptions.TempDirectory = config.CommonOptions.TempDirectory
	zarfConfig.CommonOptions.OCIConcurrency = config.CommonOptions.OCIConcurrency
	zarfConfig.CommonOptions.Confirm = config.CommonOptions.Confirm
	zarfConfig.CommonOptions.CachePath = config.CommonOptions.CachePath
}

// choosePackage provides a file picker when users don't specify a file
func choosePackage(args []string) string {
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
