// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"io"

	"github.com/hashicorp/hcl/v2"
)

// BundleParser defines the interface for parsing bundle files.
type BundleParser interface {
	// ParseBundleFile reads and parses an HCL bundle file with locals support.
	ParseBundleFile(ctx context.Context, filePath string) (*UDSBundle, error)
}

// UDSBundle represents a parsed HCL bundle definition.
// The locals block is not represented here; it is resolved during parsing
// and substituted via EvalContext before this struct is populated.
type UDSBundle struct {
	UDS      UDSBlock  `hcl:"uds,block"`
	Metadata Metadata  `hcl:"metadata,block"`
	Packages []Package `hcl:"package,block"`
	Remain   hcl.Body  `hcl:",remain"`
}

// UDSBlock contains tooling and schema constraints.
type UDSBlock struct {
	BundleAPIVersion string `hcl:"bundle_api_version"`
}

// Metadata holds bundle-level identity and descriptive fields.
type Metadata struct {
	Name        string `hcl:"name"`
	Description string `hcl:"description,optional"`
	Version     string `hcl:"version,optional"`
}

// Package represents a Zarf package entry in the bundle.
type Package struct {
	Name               string   `hcl:"name,label"`
	Source             string   `hcl:"source"`
	Namespace          string   `hcl:"namespace,optional"`
	DependsOn          []string `hcl:"depends_on,optional"`
	ValueFiles         []string `hcl:"values_files,optional"`
	OptionalComponents []string `hcl:"optional_components,optional"`
}

// Creator is implemented by types that can create UDS bundle artifacts.
// It handles per-package ingestion and output naming, allowing library
// consumers to substitute or mock the creation logic independently.
type Creator interface {
	// CreatePackage ingests a single package into the OCI layout at opts.BlobDir.
	CreatePackage(ctx context.Context, pkg *Package, opts CreatePackageOptions) error
	// BundleName returns the output filename for the bundle artifact.
	BundleName(b *UDSBundle) string
}

// CreateOptions holds configuration for the top-level bundle create operation.
type CreateOptions struct {
	BundleFile string
	// Arch is the target CPU architecture (e.g. "amd64", "arm64").
	// Defaults to runtime.GOARCH when empty.
	Arch string
	Out  io.Writer
}

// CreatePackageOptions holds per-package configuration during bundle creation.
type CreatePackageOptions struct {
	BlobDir   string
	BundleDir string
	// Arch is the target CPU architecture forwarded from CreateOptions.
	Arch string
	Out  io.Writer
}
