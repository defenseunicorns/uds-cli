// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"

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
	ValueFiles         []string `hcl:"value_files,optional"`
	OptionalComponents []string `hcl:"optional_components,optional"`
}
