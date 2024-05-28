// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package cmd contains the CLI commands for UDS.
package cmd

import (
	"github.com/defenseunicorns/pkg/helpers"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/config/lang"
	"github.com/defenseunicorns/zarf/src/pkg/message"

	"github.com/defenseunicorns/uds-cli/src/pkg/bundle"
	"github.com/spf13/cobra"
)

var devCmd = &cobra.Command{
	Use:   "dev",
	Short: lang.CmdDevShort,
}

var devDeployCmd = &cobra.Command{
	Use:   "deploy",
	Args:  cobra.MaximumNArgs(1),
	Short: lang.CmdDevDeployShort,
	Long:  lang.CmdDevDeployLong,
	Run: func(_ *cobra.Command, args []string) {
		config.Dev = true

		// Get bundle source
		src := ""
		if len(args) > 0 {
			src = args[0]
		}

		// Check if source is a local bundle
		localBundle := helpers.IsDir(src)

		if localBundle {
			// Create Bundle
			setBundleFile(args)

			config.CommonOptions.Confirm = true
			bundleCfg.CreateOpts.SourceDirectory = src
		}

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

		// Create dev bundle
		if localBundle {
			// Check if local zarf packages need to be created
			bndlClient.CreateZarfPkgs()

			if err := bndlClient.Create(); err != nil {
				message.Fatalf(err, "Failed to create bundle: %s", err.Error())
			}
		}

		// Set dev source
		if localBundle {
			bndlClient.SetDeploySource(src)
		} else {
			bundleCfg.DeployOpts.Source = src
		}

		// Deploy bundle
		deploy(bndlClient)
	},
}

func init() {
	initViper()
	rootCmd.AddCommand(devCmd)
	devCmd.AddCommand(devDeployCmd)
	devDeployCmd.Flags().StringArrayVarP(&bundleCfg.DeployOpts.Packages, "packages", "p", []string{}, lang.CmdBundleDeployFlagPackages)
	devDeployCmd.Flags().BoolVarP(&config.CommonOptions.Confirm, "confirm", "c", false, lang.CmdBundleDeployFlagConfirm)
	devDeployCmd.Flags().StringToStringVar(&bundleCfg.DeployOpts.SetVariables, "set", nil, lang.CmdBundleDeployFlagSet)
}
