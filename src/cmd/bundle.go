// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package cmd contains the CLI commands for UDS.
package cmd

import (
	"fmt"
	"github.com/defenseunicorns/zarf/src/pkg/utils/helpers"

	"github.com/defenseunicorns/uds-cli/src/config/lang"
	"github.com/defenseunicorns/uds-cli/src/pkg/bundler"
	"github.com/defenseunicorns/zarf/src/config"
	"github.com/defenseunicorns/zarf/src/pkg/message"
	"github.com/defenseunicorns/zarf/src/pkg/oci"

	"github.com/defenseunicorns/zarf/src/pkg/utils"
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
	Args:    cobra.ExactArgs(1),
	Short:   lang.CmdBundleCreateShort,
	PreRun: func(cmd *cobra.Command, args []string) {
		if !utils.IsDir(args[0]) {
			message.Fatalf(nil, "first argument (%q) must be a valid path to a directory", args[0])
		}
	},
	Run: func(cmd *cobra.Command, args []string) {
		bundleCfg.CreateOpts.SourceDirectory = args[0]

		bundleCfg.CreateOpts.SetVariables = bundler.MergeVariables(v.GetStringMapString(V_BNDL_CREATE_SET), bundleCfg.CreateOpts.SetVariables)

		bndlClient := bundler.NewOrDie(&bundleCfg)
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
	Args:    cobra.ExactArgs(1),
	PreRun:  firstArgIsEitherOCIorTarball,
	Run: func(cmd *cobra.Command, args []string) {
		bundleCfg.DeployOpts.Source = args[0]

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

		bndlClient := bundler.NewOrDie(&bundleCfg)
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
	Args:    cobra.ExactArgs(1),
	PreRun:  firstArgIsEitherOCIorTarball,
	Run: func(cmd *cobra.Command, args []string) {
		bundleCfg.InspectOpts.Source = args[0]

		bndlClient := bundler.NewOrDie(&bundleCfg)
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

		bndlClient := bundler.NewOrDie(&bundleCfg)
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

		bndlClient := bundler.NewOrDie(&bundleCfg)
		defer bndlClient.ClearPaths()

		if err := bndlClient.Pull(); err != nil {
			bndlClient.ClearPaths()
			message.Fatalf(err, "Failed to pull bundle: %s", err.Error())
		}
	},
}

func firstArgIsEitherOCIorTarball(_ *cobra.Command, args []string) {
	var errString string
	var err error
	if bundler.IsValidTarballPath(args[0]) {
		return
	}
	if !helpers.IsOCIURL(args[0]) && !bundler.IsValidTarballPath(args[0]) {
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
