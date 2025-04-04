// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

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
	"github.com/spf13/cobra"

	"github.com/zarf-dev/zarf/src/pkg/logger"
	"github.com/zarf-dev/zarf/src/pkg/message"
)

var createCmd = &cobra.Command{
	Use:     "create [DIRECTORY]",
	Aliases: []string{"c"},
	Args:    cobra.MaximumNArgs(1),
	Short:   lang.CmdBundleCreateShort,
	PreRunE: func(_ *cobra.Command, args []string) error {
		err := setBundleFile(args)
		if err != nil {
			return err
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		configureZarf()
		ctx := cmd.Context()
		// example log message
		logger.From(ctx).Info("Creating bundle...")
		srcDir, err := os.Getwd()
		if err != nil {
			return errors.New("error reading the current working directory")
		}
		if len(args) > 0 {
			srcDir = args[0]
		}
		bundleCfg.CreateOpts.SourceDirectory = srcDir

		bndlClient, err := bundle.New(&bundleCfg)
		if err != nil {
			return err
		}
		defer bndlClient.ClearPaths()

		if err := bndlClient.Create(); err != nil {
			bndlClient.ClearPaths()
			return fmt.Errorf("failed to create bundle: %s", err.Error())
		}
		return nil
	},
}

var deployCmd = &cobra.Command{
	Use:     "deploy [BUNDLE_TARBALL|OCI_REF]",
	Aliases: []string{"d"},
	Short:   lang.CmdBundleDeployShort,
	Args:    cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		var err error
		bundleCfg.DeployOpts.Source, err = chooseBundle(args)
		if err != nil {
			return err
		}
		configureZarf()

		// set DeployOptions.Config if exists
		if config := v.ConfigFileUsed(); config != "" {
			bundleCfg.DeployOpts.Config = config
		}

		// create new bundle client and deploy
		bndlClient, err := bundle.New(&bundleCfg)
		if err != nil {
			return err
		}
		defer bndlClient.ClearPaths()
		err = deploy(ctx, bndlClient)
		if err != nil {
			return err
		}
		return nil
	},
}

var inspectCmd = &cobra.Command{
	Use:     "inspect [BUNDLE_TARBALL|OCI_REF|BUNDLE_YAML_FILE]",
	Aliases: []string{"i"},
	Short:   lang.CmdBundleInspectShort,
	Args:    cobra.MaximumNArgs(1),
	PreRunE: func(cmd *cobra.Command, _ []string) error {
		if cmd.Flag("extract").Value.String() == "true" && cmd.Flag("sbom").Value.String() == "false" {
			return errors.New("cannot use 'extract' flag without 'sbom' flag")
		}
		return nil
	},
	RunE: func(_ *cobra.Command, args []string) error {
		var err error
		bundleCfg.InspectOpts.Source, err = chooseBundle(args)
		if err != nil {
			return err
		}
		configureZarf()

		bndlClient, err := bundle.New(&bundleCfg)
		if err != nil {
			return err
		}
		defer bndlClient.ClearPaths()

		if err := bndlClient.Inspect(); err != nil {
			bndlClient.ClearPaths()
			return fmt.Errorf("failed to inspect bundle: %s", err.Error())
		}
		return nil
	},
}

var removeCmd = &cobra.Command{
	Use:     "remove [BUNDLE_TARBALL|OCI_REF]",
	Aliases: []string{"r"},
	Args:    cobra.ExactArgs(1),
	Short:   lang.CmdBundleRemoveShort,
	RunE: func(_ *cobra.Command, args []string) error {
		bundleCfg.RemoveOpts.Source = args[0]
		configureZarf()

		bndlClient, err := bundle.New(&bundleCfg)
		if err != nil {
			return err
		}
		defer bndlClient.ClearPaths()

		if err := bndlClient.Remove(); err != nil {
			bndlClient.ClearPaths()
			return fmt.Errorf("failed to remove bundle: %s", err.Error())
		}
		return nil
	},
}

var publishCmd = &cobra.Command{
	Use:     "publish [BUNDLE_TARBALL] [OCI_REF]",
	Aliases: []string{"p"},
	Short:   lang.CmdPublishShort,
	Args:    cobra.ExactArgs(2),
	PreRunE: func(_ *cobra.Command, args []string) error {
		if _, err := os.Stat(args[0]); err != nil {
			return fmt.Errorf("first argument (%q) must be a valid local Bundle path: %s", args[0], err.Error())
		}

		if bundleCfg.PublishOpts.Version != "" {
			message.Warnf("the --version flag is deprecated and will be removed in a future version")
		}
		return nil
	},
	RunE: func(_ *cobra.Command, args []string) error {
		bundleCfg.PublishOpts.Source = args[0]
		bundleCfg.PublishOpts.Destination = args[1]
		configureZarf()
		bndlClient, err := bundle.New(&bundleCfg)
		if err != nil {
			return err
		}
		defer bndlClient.ClearPaths()

		if err := bndlClient.Publish(); err != nil {
			bndlClient.ClearPaths()
			return fmt.Errorf("failed to publish bundle: %s", err.Error())
		}
		return nil
	},
}

var pullCmd = &cobra.Command{
	Use:     "pull [OCI_REF]",
	Aliases: []string{"p"},
	Short:   lang.CmdBundlePullShort,
	Args:    cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		bundleCfg.PullOpts.Source = args[0]
		configureZarf()
		bndlClient, err := bundle.New(&bundleCfg)
		if err != nil {
			return err
		}
		defer bndlClient.ClearPaths()

		if err := bndlClient.Pull(); err != nil {
			bndlClient.ClearPaths()
			return fmt.Errorf("failed to pull bundle: %s", err.Error())
		}
		return nil
	},
}

var logsCmd = &cobra.Command{
	Use:     "logs",
	Aliases: []string{"l"},
	Short:   lang.CmdBundleLogsShort,
	RunE: func(_ *cobra.Command, _ []string) error {
		logFilePath := filepath.Join(config.CommonOptions.CachePath, config.CachedLogs)

		// Open the cached log file
		logfile, err := os.Open(logFilePath)
		if err != nil {
			var pathError *os.PathError
			if errors.As(err, &pathError) {
				return fmt.Errorf("no cached logs found at %s", logFilePath)
			}
			return fmt.Errorf("error opening log file: %w", err)
		}
		defer logfile.Close()

		// Copy the contents of the log file to stdout
		if _, err := io.Copy(os.Stdout, logfile); err != nil {
			// Handle the error if the contents can't be read or written to stdout
			return fmt.Errorf("error reading or printing log file: %w", err)
		}
		return nil
	},
}

func init() {
	initViper()

	// create cmd flags
	rootCmd.AddCommand(createCmd)
	createCmd.Flags().BoolVarP(&config.CommonOptions.Confirm, "confirm", "c", false, lang.CmdBundleCreateFlagConfirm)
	createCmd.Flags().StringVarP(&bundleCfg.CreateOpts.Output, "output", "o", v.GetString(V_BNDL_CREATE_OUTPUT), lang.CmdBundleCreateFlagOutput)
	createCmd.Flags().StringVarP(&bundleCfg.CreateOpts.SigningKeyPath, "signing-key", "k", v.GetString(V_BNDL_CREATE_SIGNING_KEY), lang.CmdBundleCreateFlagSigningKey)
	createCmd.Flags().StringVarP(&bundleCfg.CreateOpts.SigningKeyPassword, "signing-key-password", "p", v.GetString(V_BNDL_CREATE_SIGNING_KEY_PASSWORD), lang.CmdBundleCreateFlagSigningKeyPassword)
	createCmd.Flags().StringVarP(&bundleCfg.CreateOpts.Version, "version", "v", "", lang.CmdBundleCreateFlagVersion)
	createCmd.Flags().StringVarP(&bundleCfg.CreateOpts.Name, "name", "n", "", lang.CmdBundleCreateFlagName)

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
	inspectCmd.Flags().BoolVarP(&bundleCfg.InspectOpts.ListImages, "list-images", "i", false, lang.CmdBundleInspectFlagFindImages)
	inspectCmd.Flags().BoolVarP(&bundleCfg.InspectOpts.ListVariables, "list-variables", "v", false, lang.CmdBundleInspectFlagListVariables)

	// remove cmd flags
	rootCmd.AddCommand(removeCmd)
	// confirm does not use the Viper config
	removeCmd.Flags().BoolVarP(&config.CommonOptions.Confirm, "confirm", "c", false, lang.CmdBundleRemoveFlagConfirm)
	_ = removeCmd.MarkFlagRequired("confirm")
	removeCmd.Flags().StringArrayVarP(&bundleCfg.RemoveOpts.Packages, "packages", "p", []string{}, lang.CmdBundleRemoveFlagPackages)

	// publish cmd flags
	rootCmd.AddCommand(publishCmd)
	publishCmd.Flags().StringVarP(&bundleCfg.PublishOpts.Version, "version", "v", "", lang.CmdPublishVersionFlag)

	// pull cmd flags
	rootCmd.AddCommand(pullCmd)
	pullCmd.Flags().StringVarP(&bundleCfg.PullOpts.OutputDirectory, "output", "o", v.GetString(V_BNDL_PULL_OUTPUT), lang.CmdBundlePullFlagOutput)
	pullCmd.Flags().StringVarP(&bundleCfg.PullOpts.PublicKeyPath, "key", "k", v.GetString(V_BNDL_PULL_KEY), lang.CmdBundlePullFlagKey)

	// logs cmd
	rootCmd.AddCommand(logsCmd)
}

// chooseBundle provides a file picker when users don't specify a file
func chooseBundle(args []string) (string, error) {
	if len(args) > 0 {
		return args[0], nil
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
		return "", fmt.Errorf(lang.CmdPackageChooseErr, err.Error())
	}

	return path, nil
}
