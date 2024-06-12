// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package cmd contains the CLI commands for UDS.
package monitor

import (
	"github.com/defenseunicorns/uds-cli/src/config/lang"
	"github.com/spf13/cobra"
)

var namespace string

var MonitorCmd = &cobra.Command{
	Use:     "monitor",
	Aliases: []string{"mon", "m"},
	Short:   lang.CmdMonitorShort,
	Long:    lang.CmdMonitorLong,
}

func init() {
	MonitorCmd.PersistentFlags().StringVarP(&namespace, "namespace", "n", "", lang.CmdMonitorNamespaceFlag)
}
