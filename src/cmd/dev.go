// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package cmd contains the CLI commands for UDS.
package cmd

import (
	"fmt"
	"strings"

	"github.com/defenseunicorns/pkg/helpers/v2"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/config/lang"

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
	RunE: func(_ *cobra.Command, args []string) error {
		config.Dev = true
		config.CommonOptions.Confirm = true

		// Get bundle source
		src := ""
		if len(args) > 0 {
			src = args[0]
		}

		// Check if source is a local bundle
		isLocalBundle := isLocalBundle(src)

		// Validate flags
		err := validateDevDeployFlags(isLocalBundle)
		if err != nil {
			return fmt.Errorf("failed to validate flags: %s", err.Error())
		}

		if isLocalBundle {
			// Populate flavor map
			err = populateFlavorMap()
			if err != nil {
				return fmt.Errorf("failed to populate flavor map: %s", err.Error())
			}

			// Create Bundle
			err = setBundleFile(args)
			if err != nil {
				return err
			}

			bundleCfg.CreateOpts.SourceDirectory = src
		}

		configureZarf()

		// load uds-config if it exists
		if config := v.ConfigFileUsed(); config != "" {
			if err := loadViperConfig(); err != nil {
				return fmt.Errorf("failed to load uds-config: %s", err.Error())
			}

			bundleCfg.DeployOpts.Config = config
		}

		bndlClient, err := bundle.New(&bundleCfg)
		if err != nil {
			return err
		}
		defer bndlClient.ClearPaths()

		// Create dev bundle
		if isLocalBundle {
			// Check if local zarf packages need to be created
			err = bndlClient.CreateZarfPkgs()
			if err != nil {
				return err
			}

			if err := bndlClient.Create(); err != nil {
				return fmt.Errorf("failed to create bundle: %s", err.Error())
			}
			bndlClient.SetDeploySource(src)
		} else {
			bundleCfg.DeployOpts.Source = src
		}

		// Deploy bundle
		err = deploy(bndlClient)
		if err != nil {
			return err
		}
		return nil
	},
}

// NOTE: This is intended to be a temporary command. This logic will soon be baked directly into
// NOTE: the deploy command so the Terraform Provider can directly access the Zarf Package Tarballs
var extractCmd = &cobra.Command{
	Use:   "extract [BUNDLE_TARBALL|OCI_REF] [EXTRACT_DIR]",
	Short: "[alpha] Extract the Zarf Package tarballs from a Bundle",
	Args:  cobra.ExactArgs(2),
	RunE: func(_ *cobra.Command, args []string) error {
		var err error

		// load the bundle source from the CLI args
		bundleCfg.DeployOpts.Source, err = chooseBundle(args)
		if err != nil {
			return err
		}
		configureZarf()

		// create new bundle client for ex
		bndlClient, err := bundle.New(&bundleCfg)
		if err != nil {
			return err
		}

		// NOTE: I am obviously not chcking any of the outputs here, but this call is what sets
		//       bndleClient.cfg.DeployOpts.Source which we need later...
		_, _, _, err = bndlClient.PreDeployValidation()
		if err != nil {
			return err
		}

		err = bndlClient.Extract(args[1])
		return err
	},
}

// isLocalBundle checks if the bundle source is a local bundle
func isLocalBundle(src string) bool {
	return helpers.IsDir(src) || strings.Contains(src, ".tar.zst")
}

// validateDevDeployFlags validates the flags for dev deploy
func validateDevDeployFlags(isLocalBundle bool) error {
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
	devDeployCmd.Flags().StringToStringVar(&bundleCfg.DeployOpts.SetVariables, "set", nil, lang.CmdBundleDeployFlagSet)

	devCmd.AddCommand(extractCmd)
}
