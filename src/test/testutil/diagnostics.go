// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package testutil

import (
	"fmt"
	"strings"
)

func commandDiagnostics(args []string, result CommandResult) string {
	return fmt.Sprintf("uds %s failed: %v\nstdout:\n%s\nstderr:\n%s", strings.Join(args, " "), result.Err, result.Stdout, result.Stderr)
}
