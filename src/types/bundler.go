// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package types contains all the types used by UDS.
package types

// BundlerConfig is the main struct that the bundler uses to hold high-level options.
type BundlerConfig struct {
	CreateOpts  BundlerCreateOptions
	DeployOpts  BundlerDeployOptions
	PublishOpts BundlerPublishOptions
	PullOpts    BundlerPullOptions
	InspectOpts BundlerInspectOptions
	RemoveOpts  BundlerRemoveOptions
}

// BundlerCreateOptions is the options for the bundler.Create() function
type BundlerCreateOptions struct {
	SourceDirectory    string
	Output             string
	SigningKeyPath     string
	SigningKeyPassword string
	SetVariables       map[string]string
}

// BundlerDeployOptions is the options for the bundler.Deploy() function
type BundlerDeployOptions struct {
	Resume        bool
	Source        string
	Packages      []string
	PublicKeyPath string
	Variables     map[string]map[string]interface{}
}

// BundlerInspectOptions is the options for the bundler.Inspect() function
type BundlerInspectOptions struct {
	PublicKeyPath string
	Source        string
	IncludeSBOM   bool
	ExtractSBOM   bool
}

// BundlerPublishOptions is the options for the bundle.Publish() function
type BundlerPublishOptions struct {
	Source      string
	Destination string
}

// BundlerPullOptions is the options for the bundler.Pull() function
type BundlerPullOptions struct {
	OutputDirectory string
	PublicKeyPath   string
	Source          string
}

// BundlerRemoveOptions is the options for the bundler.Remove() function
type BundlerRemoveOptions struct {
	Source   string
	Packages []string
}

// BundlerCommonOptions tracks the user-defined preferences used across commands.
type BundlerCommonOptions struct {
	Confirm        bool   `json:"confirm" jsonschema:"description=Verify that Zarf should perform an action"`
	Insecure       bool   `json:"insecure" jsonschema:"description=Allow insecure connections for remote packages"`
	CachePath      string `json:"cachePath" jsonschema:"description=Path to use to cache images and git repos on package create"`
	TempDirectory  string `json:"tempDirectory" jsonschema:"description=Location Zarf should use as a staging ground when managing files and images for package creation and deployment"`
	OCIConcurrency int    `jsonschema:"description=Number of concurrent layer operations to perform when interacting with a remote package"`
}
