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
	// CacheDir selects the shared OCI cache root. Directories are created on demand.
	CacheDir              string
	TmpDir                string
	Concurrency           int
	SkipArchitectureCheck bool
	ZarfInit              *ZarfInitOptions
}

// ZarfInitOptions holds Zarf initialization settings supplied during deployment.
type ZarfInitOptions struct {
	RegistryURL          string
	RegistryMode         string
	RegistryPort         int
	RegistrySecret       string
	RegistryPushUsername string
	RegistryPushPassword string
	RegistryPullUsername string
	RegistryPullPassword string
	GitURL               string
	GitPushUsername      string
	GitPushPassword      string
	GitPullUsername      string
	GitPullPassword      string
	InjectorImage        string
	InjectorPort         int
	AgentMutationPolicy  string
	AgentTLSCA           string
	AgentTLSCert         string
	AgentTLSKey          string
}
