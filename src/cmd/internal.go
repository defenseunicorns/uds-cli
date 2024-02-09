// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package cmd contains the CLI commands for UDS.
package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/alecthomas/jsonschema"
	"github.com/defenseunicorns/uds-cli/src/config/lang"
	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/defenseunicorns/zarf/src/pkg/message"
	"github.com/spf13/cobra"
)

var internalCmd = &cobra.Command{
	Use:     "internal",
	Aliases: []string{"dev"},
	Hidden:  true,
	Short:   lang.CmdInternalShort,
}

var configUDSSchemaCmd = &cobra.Command{
	Use:     "config-uds-schema",
	Aliases: []string{"c"},
	Short:   lang.CmdInternalConfigSchemaShort,
	Run: func(_ *cobra.Command, _ []string) {
		schema := jsonschema.Reflect(&types.UDSBundle{})
		output, err := json.MarshalIndent(schema, "", "  ")
		if err != nil {
			message.Fatal(err, lang.CmdInternalConfigSchemaErr)
		}
		fmt.Print(string(output) + "\n")
	},
}

var configTasksSchemaCmd = &cobra.Command{
	Use:     "config-tasks-schema",
	Aliases: []string{"c"},
	Short:   lang.CmdInternalConfigSchemaShort,
	Run: func(_ *cobra.Command, _ []string) {
		schema := jsonschema.Reflect(&types.TasksFile{})
		output, err := json.MarshalIndent(schema, "", "  ")
		if err != nil {
			message.Fatal(err, lang.CmdInternalConfigSchemaErr)
		}
		fmt.Print(string(output) + "\n")
	},
}

func init() {
	rootCmd.AddCommand(internalCmd)

	internalCmd.AddCommand(configUDSSchemaCmd)
	internalCmd.AddCommand(configTasksSchemaCmd)
}
