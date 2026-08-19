// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package monitor contains the CLI commands for UDS monitor.
package monitor

import (
	"github.com/defenseunicorns/uds-cli/pkg/legacy/config/lang"
	"github.com/spf13/cobra"
)

// NewCommand constructs the Legacy monitor command without package import side effects.
func NewCommand() *cobra.Command {
	var namespace string
	cmd := &cobra.Command{
		Use:     "monitor",
		Aliases: []string{"mon", "m"},
		Short:   lang.CmdMonitorShort,
		Long:    lang.CmdMonitorLong,
	}
	cmd.PersistentFlags().StringVarP(&namespace, "namespace", "n", "", lang.CmdMonitorNamespaceFlag)
	cmd.AddCommand(newPeprCommand(&namespace))
	return cmd
}

// NewPeprCommand constructs the Legacy Pepr monitor command.
func NewPeprCommand() *cobra.Command {
	var namespace string
	return newPeprCommand(&namespace)
}
