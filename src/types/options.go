// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package types contains all the types used by UDS.
package types

// BundleConfig is the main struct that the bundler uses to hold high-level options.
type BundleConfig struct {
	CreateOpts    BundleCreateOptions
	DeployOpts    BundleDeployOptions
	PublishOpts   BundlePublishOptions
	PullOpts      BundlePullOptions
	InspectOpts   BundleInspectOptions
	RemoveOpts    BundleRemoveOptions
	DevDeployOpts BundleDevDeployOptions
}

// BundleCreateOptions is the options for the bundler.Create() function
type BundleCreateOptions struct {
	SourceDirectory    string
	Output             string
	SigningKeyPath     string
	SigningKeyPassword string
	BundleFile         string
	Version            string
	Name               string
}

// BundleDeployOptions is the options for the bundler.Deploy() function
type BundleDeployOptions struct {
	Resume        bool
	Source        string
	Config        string
	Packages      []string
	PublicKeyPath string
	SetVariables  map[string]string `json:"setVariables" jsonschema:"description=Key-Value map of variable names and their corresponding values that will be used by Zarf packages in a bundle"`
	// Variables and SharedVariables are read in from uds-config.yaml
	Variables       map[string]map[string]interface{} `yaml:"variables,omitempty"`
	SharedVariables map[string]interface{}            `yaml:"shared,omitempty"`
	// PackageValues contains per-package Zarf values configuration from uds-config.yaml
	PackageValues map[string]PackageValuesConfig `yaml:"values,omitempty"`
	Retries       int                            `yaml:"retries"`
	Options       map[string]interface{}         `yaml:"options,omitempty"`
}

// PackageValuesConfig represents per-package Zarf values configuration from uds-config.yaml
type PackageValuesConfig struct {
	Files []string               `yaml:"files,omitempty" json:"files,omitempty" jsonschema:"description=Paths to values YAML files"`
	Set   map[string]interface{} `yaml:"set,omitempty" json:"set,omitempty" jsonschema:"description=Values to set using dot notation paths"`
}

// BundleInspectOptions is the options for the bundler.Inspect() function
type BundleInspectOptions struct {
	PublicKeyPath string
	Source        string
	IncludeSBOM   bool
	ExtractSBOM   bool
	ListImages    bool
	ListVariables bool
	IsYAMLFile    bool
}

// BundlePublishOptions is the options for the bundle.Publish() function
type BundlePublishOptions struct {
	Source      string
	Destination string
	Version     string
}

// BundlePullOptions is the options for the bundler.Pull() function
type BundlePullOptions struct {
	OutputDirectory string
	PublicKeyPath   string
	Source          string
}

// BundleRemoveOptions is the options for the bundler.Remove() function
type BundleRemoveOptions struct {
	Source   string
	Packages []string
}

// BundleCommonOptions tracks the user-defined preferences used across commands.
type BundleCommonOptions struct {
	Confirm                 bool   `json:"confirm" jsonschema:"description=Verify that Zarf should perform an action"`
	Insecure                bool   `json:"insecure" jsonschema:"description=Allow insecure connections for remote packages"`
	SkipSignatureValidation bool   `json:"skipSignatureValidation" jsonschema:"description=Skip signature validation for remote packages"`
	CachePath               string `json:"cachePath" jsonschema:"description=Path to use to cache images and git repos on package create"`
	TempDirectory           string `json:"tempDirectory" jsonschema:"description=Location Zarf should use as a staging ground when managing files and images for package creation and deployment"`
	OCIConcurrency          int    `jsonschema:"description=Number of concurrent layer operations to perform when interacting with a remote package"`
}

// BundleDevDeployOptions are the options for when doing a dev deploy
type BundleDevDeployOptions struct {
	FlavorInput string
	Flavor      map[string]string
	ForceCreate bool
	Ref         map[string]string
}

// PathMap is a map of either absolute paths to relative paths or relative paths to absolute paths
// used to map filenames during local bundle tarball creation
type PathMap map[string]string
