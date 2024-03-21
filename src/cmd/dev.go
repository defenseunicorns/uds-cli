// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package cmd contains the CLI commands for UDS.
package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/config/lang"
	"github.com/defenseunicorns/uds-cli/src/pkg/bundle"
	"github.com/defenseunicorns/zarf/src/pkg/message"
	"github.com/defenseunicorns/zarf/src/pkg/utils/exec"
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
	PreRun: func(_ *cobra.Command, args []string) {
		CreatePreRun(args)
	},
	Run: func(_ *cobra.Command, args []string) {
		// zarf init cluster
		if !zarfInitDeployed() {
			i, j, err := exec.Cmd("zarf", "init", "--confirm")
			fmt.Println(i)
			fmt.Println(j)
			if err != nil {
				message.Fatalf(err, "Failed to initialize zarf: %s", err.Error())
			}
		}

		// Create Bundle
		srcDir, err := os.Getwd()
		if err != nil {
			message.Fatalf(err, "error reading the current working directory")
		}
		if len(args) > 0 {
			srcDir = args[0]
		}

		config.CommonOptions.Confirm = true
		bundleCfg.CreateOpts.SourceDirectory = srcDir

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

		//TODO (in memory?) create things here
		if err := bndlClient.Create(); err != nil {
			bndlClient.ClearPaths()
			message.Fatalf(err, "Failed to create bundle: %s", err.Error())
		}

		// get bundle location and pass to deploy opts
		filename := fmt.Sprintf("%s%s-%s-%s.tar.zst", config.BundlePrefix, bndlClient.Bundle.Metadata.Name, bndlClient.Bundle.Metadata.Architecture, bndlClient.Bundle.Metadata.Version)
		bundleCfg.DeployOpts.Source = fmt.Sprintf("%s/%s", srcDir, filename)

		// TODO Deploy (from memory?)
		if err := bndlClient.Deploy(); err != nil {
			bndlClient.ClearPaths()
			message.Fatalf(err, "Failed to deploy bundle: %s", err.Error())
		}
	},
}

func init() {
	initViper()
	rootCmd.AddCommand(devCmd)
	devCmd.AddCommand(devDeployCmd)
}

func zarfInitDeployed() bool {
	args := strings.Split("tools kubectl get deployments --namespace zarf", " ")
	_, stderr, _ := exec.Cmd("zarf", args...)
	return !strings.Contains(stderr, "No resources found in zarf namespace")
}
