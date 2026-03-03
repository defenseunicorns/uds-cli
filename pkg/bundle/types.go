// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"io"

	"github.com/hashicorp/hcl/v2"
)

// Parser defines the interface for parsing bundle files.
type Parser interface {
	// ParseBundleFile reads and parses an HCL bundle file with locals support.
	ParseBundleFile(ctx context.Context, filePath string) (*UDSBundle, error)
}

// Deployer is the interface for deploying packages to a target.
// Implementations can include: ZarfDeployer (local), TofuDeployer, RemoteAgentDeployer.
type Deployer interface {
	// DeployPackage deploys a single package.
	// Called in topological order - dependencies are already deployed.
	DeployPackage(ctx context.Context, pkg *Package, opts DeployPackageOptions) error
}

// DeployPackageOptions contains options for deploying a single package.
type DeployPackageOptions struct {
	// BundleDir is the directory containing the bundle (for resolving relative paths)
	BundleDir string

	// Prompt enables interactive prompts (non-interactive by default per ADR 0005)
	Prompt bool
}

// DeployOptions contains options for deploying an entire bundle.
type DeployOptions struct {
	// BundlePath is the path to the bundle directory containing bundle.uds.hcl
	BundlePath string

	// Prompt enables interactive prompts (non-interactive by default per ADR 0005)
	Prompt bool

	// Out is the writer for output messages
	Out io.Writer
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
// Most fields are decoded via HCL annotations. The DependsOn field requires
// special handling because it uses expression syntax (depends_on = [package.core_base])
// which is captured in Remain and post-processed into []PackageRef.
//
// NOTE: There is no built-in gohcl annotation that can automatically parse HCL traversal
// expressions (like package.core_base) into Go types. The gohcl library only supports
// decoding literal values (strings, numbers, booleans, lists of literals). Traversal
// expressions are references that must be manually extracted using hcl.AbsTraversalForExpr().
// This is why we use Remain to capture unparsed content and post-process it.
type Package struct {
	Name               string       `hcl:"name,label"`
	Source             string       `hcl:"source"`
	Namespace          string       `hcl:"namespace,optional"`
	DependsOn          []PackageRef // Populated from Remain after HCL decoding
	ValueFiles         []string     `hcl:"values_files,optional"`
	OptionalComponents []string     `hcl:"optional_components,optional"`
	Remain             hcl.Body     `hcl:",remain"` // Captures depends_on for post-processing
}

// PackageRef represents a reference to another package in the bundle.
// It stores the parsed hcl.Traversal for type safety and source location tracking.
// The HCL syntax is: package.<name> (e.g., package.core_base)
//
// Design Decision: We use PackageRef (name + traversal) instead of []*Package pointers.
// Using []*Package would require resolving forward references during parsing, which is
// problematic because HCL files don't require packages to be declared in dependency order.
// For example:
//
//	package "app" {
//	  depends_on = [package.database]  // "database" not yet parsed!
//	}
//	package "database" { ... }
//
// With []*Package, we'd need two-pass parsing: first create all Package objects, then
// resolve references. This adds complexity and creates circular reference issues
// (serialization problems, fmt.Printf stack overflow, GC complexity).
//
// PackageRef keeps parsing simple (single pass) and defers reference resolution to
// the DAG building phase where all packages are available. This follows the same
// pattern used by OpenTofu/Terraform for dependency handling.
type PackageRef struct {
	// Name is the referenced package name (extracted from the traversal)
	Name string
	// Traversal is the full HCL traversal (package.<name>) with source location
	Traversal hcl.Traversal
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
