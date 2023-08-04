// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package types contains all the types used by UDS.
package types

// BundlerConfig is the main struct that the bundler uses to hold high-level options.
type BundlerConfig struct {
	CreateOpts  BundlerCreateOptions
	DeployOpts  BundlerDeployOptions
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
	Source        string
	PublicKeyPath string
	SetVariables  map[string]string
}

// BundlerInspectOptions is the options for the bundler.Inspect() function
type BundlerInspectOptions struct {
	PublicKeyPath string
	Source        string
}

// BundlerPullOptions is the options for the bundler.Pull() function
type BundlerPullOptions struct {
	OutputDirectory string
	PublicKeyPath   string
	Source          string
}

// BundlerRemoveOptions is the options for the bundler.Remove() function
type BundlerRemoveOptions struct {
	Source string
}
