// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

// Variables contains user-defined bundle configuration variables.
type Variables map[string]any

// GlobalOptions holds process-wide settings retained for compatibility with
// bundle configuration consumers. Command behavior is resolved into ConfigOptions.
type GlobalOptions struct {
	LogLevel string
	Prompt   bool
}

// UDSBundleConfig is the resolved public bundle configuration.
type UDSBundleConfig struct {
	Global                *GlobalOptions
	Options               *ConfigOptions
	SignatureVerification *VerificationPolicy
	Variables             Variables
}

// ConfigOptions holds bundle operation settings.
type ConfigOptions struct {
	LogLevel      string
	Architecture  string
	PlainHTTP     bool
	SkipTLSVerify bool
	TmpDir        string
	Concurrency   int
}
