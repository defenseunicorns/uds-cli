// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"

	"github.com/defenseunicorns/uds-cli/internal/redact"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

// WithHCLDiagnostics scopes sensitive HCL diagnostics to this command invocation.
func WithHCLDiagnostics(cmd *cobra.Command, streams *iostreams.IOStreams) *cobra.Command {
	run := cmd.RunE
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		parent := cmd.Context()
		ctx := parent
		if ctx == nil {
			ctx = context.Background()
		}
		original := *streams
		defer func() { *streams = original }()
		ctx, scope := redact.Bind(ctx, streams)
		cmd.SetContext(ctx)
		defer cmd.SetContext(parent)
		return scope.Error(run(cmd, args))
	}
	return cmd
}
