// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present the Maru Authors

// Package cmd contains the CLI commands for maru.
package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/defenseunicorns/maru-runner/src/config/lang"
	"github.com/defenseunicorns/maru-runner/src/message"
	"github.com/defenseunicorns/maru-runner/src/types"
	"github.com/invopop/jsonschema"
	"github.com/spf13/cobra"
)

var internalCmd = &cobra.Command{
	Use:     "internal",
	Aliases: []string{"dev"},
	Hidden:  true,
	Short:   lang.CmdInternalShort,
}

var configTasksSchemaCmd = &cobra.Command{
	Use:     "config-tasks-schema",
	Aliases: []string{"c"},
	Short:   lang.CmdInternalConfigSchemaShort,
	PersistentPreRun: func(_ *cobra.Command, _ []string) {
		skipLogFile = true
	},
	Run: func(_ *cobra.Command, _ []string) {
		schema := jsonschema.Reflect(&types.TasksFile{})
		output, err := json.MarshalIndent(schema, "", "  ")
		if err != nil {
			message.Fatalf(err, "%s", lang.CmdInternalConfigSchemaErr)
		}
		fmt.Print(string(output) + "\n")
	},
}

func init() {
	rootCmd.AddCommand(internalCmd)

	internalCmd.AddCommand(configTasksSchemaCmd)
}
