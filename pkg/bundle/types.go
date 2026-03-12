// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"io"
	"runtime"

	"github.com/hashicorp/hcl/v2"
)

// Variables is a named type for the nested user-defined variable map parsed
// from the variables block in config.uds.hcl.
// Leaf values are scalars (string, float64, bool); intermediate nodes are
// nested Variables maps decoded from HCL object expressions.
// nil means no --config was provided.
//
// Using a named type (rather than bare map[string]any) follows the same
// pattern as Zarf's value.Values and allows behaviour to be attached as
// methods — in particular Flatten(), which keeps that logic intrinsic to
// the type rather than as a scattered private helper.
type Variables map[string]any

// UDSBundleConfig represents the parsed content of a config.uds.hcl file.
// The Options block is decoded via gohcl using HCL struct tags.
// Variables are free-form and captured via hcl:",remain" for manual extraction,
// since they have no fixed schema.
type UDSBundleConfig struct {
	Options   *ConfigOptions `hcl:"options,block"`
	Variables Variables      // populated after decode from Remain
	Remain    hcl.Body       `hcl:",remain"` // captures variables and any other unstructured top-level attributes
}

// ConfigOptions holds deploy-time CLI options from the options block.
// Fields are defined by the Opinionated CLI Settings ADR (uds-bundle-options.md).
// All fields are optional; unset fields default to their zero values.
type ConfigOptions struct {
	// Global
	LogLevel string `hcl:"log_level,optional"`

	// Bundle component
	Architecture  string `hcl:"architecture,optional"`
	PlainHTTP     bool   `hcl:"plain_http,optional"`
	SkipTLSVerify bool   `hcl:"skip_tls_verify,optional"`
	UDSCache      string `hcl:"uds_cache,optional"`
	TmpDir        string `hcl:"tmp_dir,optional"`
	Concurrency   int    `hcl:"concurrency,optional"`
}

// Parser defines the interface for parsing bundle files.
type Parser interface {
	// ParseBundleFile reads and parses a bundle.uds.hcl file with locals support.
	ParseBundleFile(ctx context.Context, filePath string) (*UDSBundle, error)
	// ParseBundleConfig reads and parses a config.uds.hcl file.
	ParseBundleConfig(ctx context.Context, filePath string) (*UDSBundleConfig, error)
}

// RegistryOptions holds shared OCI registry settings used across create, deploy,
// pull, and push operations.
type RegistryOptions struct {
	// Arch is the target CPU architecture (e.g. "amd64", "arm64").
	Arch string

	// PlainHTTP allows registry communication over plain HTTP.
	PlainHTTP bool

	// SkipTLSVerify skips TLS certificate verification for registries.
	SkipTLSVerify bool

	// Concurrency controls the degree of parallelism for concurrent operations.
	Concurrency int
}

// DefaultRegistryOptions returns RegistryOptions with default architecture and concurrency.
func DefaultRegistryOptions() RegistryOptions {
	return RegistryOptions{
		Arch:        runtime.GOARCH,
		Concurrency: 10,
	}
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
	RegistryOptions

	// BundleDir is the directory containing the bundle (for resolving relative paths)
	BundleDir string

	// Prompt enables interactive prompts (non-interactive by default per ADR 0005)
	Prompt bool

	// Config is the optional parsed config.uds.hcl; nil when no --config was provided.
	Config *UDSBundleConfig
}

// DeployOptions contains options for deploying an entire bundle.
type DeployOptions struct {
	RegistryOptions

	// BundlePath is the path to the bundle directory containing bundle.uds.hcl
	BundlePath string

	// Bundle is the pre-parsed bundle. When set, Deploy() skips parsing BundlePath.
	// This avoids double-parsing when the caller has already parsed the bundle.
	Bundle *UDSBundle

	// ConfigPath is the optional path to config.uds.hcl.
	// Empty string means no config was provided.
	ConfigPath string

	// Prompt enables interactive prompts (non-interactive by default per ADR 0005)
	Prompt bool

	// TmpDir is the base directory for temporary files.
	TmpDir string

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
	ValuesFiles        []string     `hcl:"values_files,optional"`
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

// CreatePackageOptions holds per-package configuration during bundle creation.
type CreatePackageOptions struct {
	RegistryOptions

	BlobDir   string
	BundleDir string
	Out       io.Writer
}

// CreateOptions holds configuration for the top-level bundle create operation.
type CreateOptions struct {
	RegistryOptions

	BundleFile string

	// TmpDir is the base directory for temporary files.
	TmpDir string

	Out io.Writer
}
