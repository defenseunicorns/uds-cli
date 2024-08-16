// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package cmd contains the CLI commands for UDS.
package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alecthomas/jsonschema"
	runnerCLI "github.com/defenseunicorns/maru-runner/src/cmd"
	"github.com/defenseunicorns/uds-cli/src/config/lang"
	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
	"github.com/zarf-dev/zarf/src/pkg/message"
)

var internalCmd = &cobra.Command{
	Use:    "internal",
	Hidden: true,
	Short:  lang.CmdInternalShort,
}

var configUDSSchemaCmd = &cobra.Command{
	Use:     "config-uds-schema",
	Aliases: []string{"c"},
	Short:   lang.CmdInternalConfigSchemaShort,
	RunE: func(_ *cobra.Command, _ []string) error {
		schema := jsonschema.Reflect(&types.UDSBundle{})
		output, err := json.MarshalIndent(schema, "", "  ")
		if err != nil {
			return fmt.Errorf(lang.CmdInternalConfigSchemaErr)
		}
		fmt.Print(string(output) + "\n")

		return nil
	},
}

var configTasksSchemaCmd = &cobra.Command{
	Use:     "config-tasks-schema",
	Aliases: []string{"c"},
	Short:   lang.CmdInternalConfigSchemaShort,
	Run: func(_ *cobra.Command, _ []string) {
		runnerCLI.RootCmd().SetArgs([]string{"internal", "config-tasks-schema"})
		runnerCLI.Execute()
	},
}

var genCLIDocs = &cobra.Command{
	Use:   "gen-cli-docs",
	Short: lang.CmdInternalGenerateCliDocsShort,
	RunE: func(_ *cobra.Command, _ []string) error {
		// Don't include the datestamp in the output
		rootCmd.DisableAutoGenTag = true

		rootCmd.RemoveCommand(zarfCmd)
		rootCmd.RemoveCommand(scanCmd)

		// Set the default value for the uds-cache flag (otherwise this defaults to the user's home directory)
		rootCmd.Flag("uds-cache").DefValue = "~/.uds-cache"

		// remove existing docs but ignore the _index.md
		glob, err := filepath.Glob("./docs/command-reference/uds*.md")
		if err != nil {
			return err
		}
		for _, f := range glob {
			err := os.Remove(f)
			if err != nil {
				return err
			}
		}

		var prependTitle = func(s string) string {
			fmt.Println(s)
			name := filepath.Base(s)

			// strip .md extension
			name = name[:len(name)-3]

			// replace _ with space
			title := strings.Replace(name, "_", " ", -1)

			return fmt.Sprintf(`---
title: %s
description: UDS CLI command reference for <code>%s</code>.
type: docs
---
`, title, title)
		}

		var linkHandler = func(link string) string {
			return "/cli/command-reference/" + link[:len(link)-3] + "/"
		}

		err = doc.GenMarkdownTreeCustom(rootCmd, "./docs/command-reference", prependTitle, linkHandler)
		if err != nil {
			return err
		}

		message.Success(lang.CmdInternalGenerateCliDocsSuccess)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(internalCmd)

	internalCmd.AddCommand(genCLIDocs)
	internalCmd.AddCommand(configUDSSchemaCmd)
	internalCmd.AddCommand(configTasksSchemaCmd)
}
