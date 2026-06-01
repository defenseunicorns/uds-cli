// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package config provides shared configuration resolution logic used across
// all CLI command groups (bundle, tofu, etc.).
package config

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/logger"
)

// ResolveGlobalOptions builds GlobalOptions from resolved values and re-initializes
// the logger if the effective level (from HCL or config) differs from the flag default.
//
// This function is intentionally command-group agnostic: both "uds bundle"
// and future groups (e.g., "uds tofu") can call it after their own
// component-specific option resolution.
func ResolveGlobalOptions(prompt bool, flagLogLevel, effectiveLogLevel string) (*bundle.GlobalOptions, error) {
	global := &bundle.GlobalOptions{
		LogLevel: effectiveLogLevel,
		Prompt:   prompt,
	}

	// Re-initialize logger if the effective level (from HCL or defaults)
	// differs from the --log-level flag value. The root PersistentPreRunE
	// already set the logger from the flag, but a config file may specify a
	// different level that the CLI didn't override.
	if effectiveLogLevel != flagLogLevel {
		level, err := logger.ParseLevel(effectiveLogLevel)
		if err != nil {
			return nil, fmt.Errorf("invalid log level %q: %w", effectiveLogLevel, err)
		}
		slog.SetDefault(logger.New(os.Stderr, level))
	}

	return global, nil
}
