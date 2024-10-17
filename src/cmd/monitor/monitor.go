// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

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
