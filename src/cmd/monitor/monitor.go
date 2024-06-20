// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package monitor contains the CLI commands for UDS monitor.
package monitor

import (
	"github.com/defenseunicorns/uds-cli/src/config/lang"
	"github.com/spf13/cobra"
)

var namespace string

var Cmd = &cobra.Command{
	Use:     "monitor",
	Aliases: []string{"mon", "m"},
	Short:   lang.CmdMonitorShort,
	Long:    lang.CmdMonitorLong,
}

func init() {
	Cmd.PersistentFlags().StringVarP(&namespace, "namespace", "n", "", lang.CmdMonitorNamespaceFlag)
}
