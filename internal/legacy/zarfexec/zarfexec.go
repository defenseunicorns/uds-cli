// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package zarfexec adapts the embedded Zarf command to explicit arguments.
package zarfexec

import (
	"context"
	"os"

	zarfCLI "github.com/zarf-dev/zarf/src/cmd"
)

// Execute runs Zarf with args and restores the process arguments afterward.
func Execute(ctx context.Context, args []string) error {
	originalOutput := zarfCLI.OutputWriter
	zarfCLI.OutputWriter = os.Stderr
	defer func() {
		zarfCLI.OutputWriter = originalOutput
	}()
	return execute(ctx, args, zarfCLI.Execute)
}

func execute(ctx context.Context, args []string, run func(context.Context) error) error {
	originalArgs := os.Args
	os.Args = append([]string{"zarf"}, args...)
	defer func() {
		os.Args = originalArgs
	}()
	return run(ctx)
}
