// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

// NewDevCommand creates the bundle development parent command.
func NewDevCommand(streams iostreams.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dev",
		Short: "Develop UDS bundles from a bundle definition",
		Long:  "Develop UDS bundles directly from a bundle definition (bundle.uds.hcl) without creating a bundle artifact",
	}

	cmd.AddCommand(NewDevDeployCommand(streams))

	return cmd
}
