// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package zarfexec adapts embedded Zarf execution to explicit arguments.
package zarfexec

import (
	"context"
	"os"

	zarfCLI "github.com/zarf-dev/zarf/src/cmd"
)

// Execute runs Zarf with args and restores process arguments afterwards.
func Execute(ctx context.Context, args []string) error {
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
