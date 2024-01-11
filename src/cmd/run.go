// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/config/lang"
	"github.com/defenseunicorns/uds-cli/src/pkg/runner"
	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/defenseunicorns/zarf/src/cmd/common"
	"github.com/defenseunicorns/zarf/src/pkg/message"
	"github.com/defenseunicorns/zarf/src/pkg/utils"
	"github.com/defenseunicorns/zarf/src/pkg/utils/helpers"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

// runCmd represents the run command
var runCmd = &cobra.Command{
	Use:   "run [ TASK NAME ]",
	Short: "run a task",
	Long:  `run a task from an tasks file`,
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 && !config.ListTasks {
			return fmt.Errorf("accepts 1 arg(s), received 0")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		var tasksFile types.TasksFile

		if _, err := os.Stat(config.TaskFileLocation); os.IsNotExist(err) {
			message.Fatalf(err, "%s not found", config.TaskFileLocation)
		}

		// Ensure uppercase keys from viper
		v := common.GetViper()
		config.SetRunnerVariables = helpers.TransformAndMergeMap(
			v.GetStringMapString(common.VPkgCreateSet), config.SetRunnerVariables, strings.ToUpper)

		err := utils.ReadYaml(config.TaskFileLocation, &tasksFile)
		if err != nil {
			message.Fatalf(err, "Cannot unmarshal %s", config.TaskFileLocation)
		}

		if config.ListTasks {
			rows := [][]string{
				{"Name", "Description"},
			}
			for _, task := range tasksFile.Tasks {
				rows = append(rows, []string{task.Name, task.Description})
			}
			pterm.DefaultTable.WithHasHeader().WithData(rows).Render()
			os.Exit(0)
		}

		taskName := args[0]
		if err := runner.Run(tasksFile, taskName, config.SetRunnerVariables); err != nil {
			message.Fatalf(err, "Failed to run action: %s", err)
		}
	},
}

func init() {
	initViper()
	rootCmd.AddCommand(runCmd)
	runFlags := runCmd.Flags()
	runFlags.StringVarP(&config.TaskFileLocation, "file", "f", config.TasksYAML, lang.CmdRunFlag)
	runFlags.BoolVar(&config.ListTasks, "list", false, lang.CmdRunList)
	runFlags.StringToStringVar(&config.SetRunnerVariables, "set", v.GetStringMapString(common.VPkgCreateSet), lang.CmdRunSetVarFlag)
}
