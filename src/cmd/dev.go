// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package cmd contains the CLI commands for UDS.
package cmd

import (
	"fmt"
	"strings"

	"github.com/defenseunicorns/pkg/helpers/v2"
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
	Use:   "deploy [BUNDLE_DIR|OCI_REF]",
	Args:  cobra.MaximumNArgs(1),
	Short: lang.CmdDevDeployShort,
	Long:  lang.CmdDevDeployLong,
	Run: func(_ *cobra.Command, args []string) {
		config.Dev = true
		config.CommonOptions.Confirm = true

		// Get bundle source
		src := ""
		if len(args) > 0 {
			src = args[0]
		}

		// Check if source is a local bundle
		isLocalBundle := isLocalBundle(src)

		// Validate flags related to creating Zarf pkgs during dev deploy
		err := validatePkgCreateFlags(isLocalBundle)
		if err != nil {
			message.Fatalf(err, "Failed to validate flags: %s", err.Error())
		}

		if isLocalBundle {
			// Populate flavor map
			err = populateFlavorMap()
			if err != nil {
				message.Fatalf(err, "Failed to populate flavor map: %s", err.Error())
			}

			// Create Bundle
			setBundleFile(args)

			bundleCfg.CreateOpts.SourceDirectory = src
		}

		configureZarf()

		// load uds-config if it exists
		if config := v.ConfigFileUsed(); config != "" {
			if err := loadViperConfig(); err != nil {
				message.Fatalf(err, "Failed to load uds-config: %s", err.Error())
			}

			bundleCfg.DeployOpts.Config = config
		}

		// pass dev deploy exclude option to create option because components are excluded during create
		bundleCfg.CreateOpts.ExcludeComponents = bundleCfg.DevDeployOpts.ExcludeComponents

		bndlClient := bundle.NewOrDie(&bundleCfg)
		defer bndlClient.ClearPaths()

		// Create dev bundle
		if isLocalBundle {
			// Check if local zarf packages need to be created
			bndlClient.CreateZarfPkgs()

			if err := bndlClient.Create(); err != nil {
				message.Fatalf(err, "Failed to create bundle: %s", err.Error())
			}
			bndlClient.SetDeploySource(src)
		} else {
			bundleCfg.DeployOpts.Source = src
		}

		// Deploy bundle
		deploy(bndlClient)
	},
}

// isLocalBundle checks if the bundle source is a local bundle
func isLocalBundle(src string) bool {
	return helpers.IsDir(src) || strings.Contains(src, ".tar.zst")
}

// validatePkgCreateFlags validates the flags for dev deploy
func validatePkgCreateFlags(isLocalBundle bool) error {
	if !isLocalBundle {
		//Throw error if trying to run with --flavor or --force-create flag with remote bundle
		if len(bundleCfg.DevDeployOpts.Flavor) > 0 || bundleCfg.DevDeployOpts.ForceCreate {
			return fmt.Errorf("cannot use --flavor or --force-create flags with remote bundle")
		}
	}
	return nil
}

// populateFlavorMap populates the flavor map based on the string input to the --flavor flag
func populateFlavorMap() error {
	if bundleCfg.DevDeployOpts.FlavorInput != "" {
		bundleCfg.DevDeployOpts.Flavor = make(map[string]string)
		flavorEntries := strings.Split(bundleCfg.DevDeployOpts.FlavorInput, ",")
		for i, entry := range flavorEntries {
			entrySplit := strings.Split(entry, "=")
			if len(entrySplit) != 2 {
				// check i==0 to check for invalid input (ex. key=value1,value2)
				if len(entrySplit) == 1 && i == 0 {
					bundleCfg.DevDeployOpts.Flavor = map[string]string{"": bundleCfg.DevDeployOpts.FlavorInput}
				} else {
					return fmt.Errorf("invalid flavor entry: %s", entry)
				}
			} else {
				bundleCfg.DevDeployOpts.Flavor[entrySplit[0]] = entrySplit[1]
			}
		}
	}
	return nil
}

func init() {
	initViper()
	rootCmd.AddCommand(devCmd)
	devCmd.AddCommand(devDeployCmd)
	devDeployCmd.Flags().StringArrayVarP(&bundleCfg.DeployOpts.Packages, "packages", "p", []string{}, lang.CmdBundleDeployFlagPackages)
	devDeployCmd.Flags().StringToStringVarP(&bundleCfg.DevDeployOpts.Ref, "ref", "r", map[string]string{}, lang.CmdBundleDeployFlagRef)
	devDeployCmd.Flags().StringVarP(&bundleCfg.DevDeployOpts.FlavorInput, "flavor", "f", "", lang.CmdBundleCreateFlagFlavor)
	devDeployCmd.Flags().BoolVar(&bundleCfg.DevDeployOpts.ForceCreate, "force-create", false, lang.CmdBundleCreateForceCreate)
	devDeployCmd.Flags().StringToStringVar(&bundleCfg.DeployOpts.SetVariables, "set", map[string]string{}, lang.CmdBundleDeployFlagSet)
	devDeployCmd.Flags().StringSliceVarP(&bundleCfg.DevDeployOpts.ExcludeComponents, "exclude-components", "e", []string{}, lang.CmdBundleFlagExcludeComponents)
}
