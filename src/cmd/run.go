// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

package cmd

import (
	"os"
	"strings"

	"github.com/defenseunicorns/zarf/src/pkg/message"
	"github.com/defenseunicorns/zarf/src/pkg/utils"
	"github.com/spf13/cobra"

	"github.com/defenseunicorns/zarf/src/cmd/common"
	"github.com/defenseunicorns/zarf/src/pkg/utils/helpers"

	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/config/lang"
	"github.com/defenseunicorns/uds-cli/src/pkg/runner"
	"github.com/defenseunicorns/uds-cli/src/types"
)

// runCmd represents the run command
var runCmd = &cobra.Command{
	Use:   "run [ TASK NAME ]",
	Short: "run a task",
	Long:  `run a task from an tasks file`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		var tasksFile types.TasksFile

		if _, err := os.Stat(config.TaskFileLocation); os.IsNotExist(err) {
			message.Fatalf(err, "%s not found", config.TaskFileLocation)
		}

		// Ensure uppercase keys from viper
		v := common.GetViper()
		config.SetVariables = helpers.TransformAndMergeMap(
			v.GetStringMapString(common.VPkgCreateSet), config.SetVariables, strings.ToUpper)

		err := utils.ReadYaml(config.TaskFileLocation, &tasksFile)
		if err != nil {
			message.Fatalf(err, "Cannot unmarshal %s", config.TaskFileLocation)
		}

		taskName := args[0]
		if err := runner.Run(tasksFile, taskName, config.SetVariables); err != nil {
			message.Fatalf(err, "Failed to run action: %s", err)
		}
	},
}

func init() {
	initViper()
	rootCmd.AddCommand(runCmd)
	runFlags := runCmd.Flags()
	runFlags.StringVarP(&config.TaskFileLocation, "file", "f", config.TasksYAML, lang.CmdRunFlag)
	runFlags.StringToStringVar(&config.SetVariables, "set", v.GetStringMapString(common.VPkgCreateSet), lang.CmdRunSetVarFlag)
}
