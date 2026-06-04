// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package config provides shared configuration resolution logic used across
// all CLI command groups (bundle, tofu, etc.).
package config

import (
	"fmt"

	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/logger"
)

// ResolveGlobalOptions builds GlobalOptions from resolved values, validating the
// log level here so library entrypoints can rely on it without re-checking.
//
// The library builds its own logger from the operation's IOStreams; this function
// does not touch the process-global slog default.
//
// This function is intentionally command-group agnostic: both "uds bundle" and
// future groups (e.g., "uds tofu") can call it after their own component-specific
// option resolution.
func ResolveGlobalOptions(prompt bool, logLevel string) (*bundle.GlobalOptions, error) {
	if _, err := logger.ParseLevel(logLevel); err != nil {
		return nil, fmt.Errorf("invalid log level %q: %w", logLevel, err)
	}
	return &bundle.GlobalOptions{
		LogLevel: logLevel,
		Prompt:   prompt,
	}, nil
}
