// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package cmd contains the CLI commands for UDS.
package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/AlecAivazis/survey/v2"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/config/lang"
	"github.com/defenseunicorns/uds-cli/src/pkg/bundle"
	"github.com/defenseunicorns/zarf/src/pkg/message"
	"github.com/spf13/cobra"
)

var createCmd = &cobra.Command{
	Use:     "create [DIRECTORY]",
	Aliases: []string{"c"},
	Args:    cobra.MaximumNArgs(1),
	Short:   lang.CmdBundleCreateShort,
	PreRun: func(_ *cobra.Command, args []string) {
		setBundleFile(args)
	},
	Run: func(_ *cobra.Command, args []string) {
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
	Run: func(_ *cobra.Command, args []string) {
		bundleCfg.DeployOpts.Source = chooseBundle(args)
		configureZarf()

		// create new bundle client and deploy
		bndlClient := bundle.NewOrDie(&bundleCfg)
		defer bndlClient.ClearPaths()
		deploy(bndlClient)
	},
}

var inspectCmd = &cobra.Command{
	Use:     "inspect [BUNDLE_TARBALL|OCI_REF]",
	Aliases: []string{"i"},
	Short:   lang.CmdBundleInspectShort,
	Args:    cobra.MaximumNArgs(1),
	PreRun: func(cmd *cobra.Command, _ []string) {
		if cmd.Flag("extract").Value.String() == "true" && cmd.Flag("sbom").Value.String() == "false" {
			message.Fatal(nil, "cannot use 'extract' flag without 'sbom' flag")
		}
	},
	Run: func(_ *cobra.Command, args []string) {
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
	Run: func(_ *cobra.Command, args []string) {
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
	PreRun: func(_ *cobra.Command, args []string) {
		if _, err := os.Stat(args[0]); err != nil {
			message.Fatalf(err, "First argument (%q) must be a valid local Bundle path: %s", args[0], err.Error())
		}
	},
	Run: func(_ *cobra.Command, args []string) {
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
	Run: func(_ *cobra.Command, args []string) {
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

var logsCmd = &cobra.Command{
	Use:     "logs",
	Aliases: []string{"l"},
	Short:   "Display log file contents",
	Run: func(_ *cobra.Command, _ []string) {
		logFilePath := filepath.Join(config.CommonOptions.CachePath, config.CachedLogs)

		// Open the cached log file
		logfile, err := os.Open(logFilePath)
		if err != nil {
			var pathError *os.PathError
			if errors.As(err, &pathError) {
				msg := fmt.Sprintf("No cached logs found at %s", logFilePath)
				message.Fatalf(nil, msg)
			}
			message.Fatalf("Error opening log file: %s\n", err.Error())
		}
		defer logfile.Close()

		// Copy the contents of the log file to stdout
		if _, err := io.Copy(os.Stdout, logfile); err != nil {
			// Handle the error if the contents can't be read or written to stdout
			message.Fatalf(err, "Error reading or printing log file: %v\n", err.Error())
		}
	},
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
	deployCmd.Flags().StringToStringVar(&bundleCfg.DeployOpts.SetVariables, "set", nil, lang.CmdBundleDeployFlagSet)
	deployCmd.Flags().BoolVarP(&config.CommonOptions.Confirm, "confirm", "c", false, lang.CmdBundleDeployFlagConfirm)
	deployCmd.Flags().StringArrayVarP(&bundleCfg.DeployOpts.Packages, "packages", "p", []string{}, lang.CmdBundleDeployFlagPackages)
	deployCmd.Flags().BoolVarP(&bundleCfg.DeployOpts.Resume, "resume", "r", false, lang.CmdBundleDeployFlagResume)
	deployCmd.Flags().IntVar(&bundleCfg.DeployOpts.Retries, "retries", 3, lang.CmdBundleDeployFlagRetries)

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

	// logs cmd
	rootCmd.AddCommand(logsCmd)
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
