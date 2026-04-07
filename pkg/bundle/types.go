// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"io"

	"github.com/hashicorp/hcl/v2"
	oras "oras.land/oras-go/v2"
)

// DefaultBundleConfigFileName is the name of the optional bundle-level defaults file.
// When present alongside bundle.uds.hcl, it is auto-discovered and applied as the
// lowest-priority configuration layer. Only the variables block is supported;
// the options block is not allowed and will produce an error.
const DefaultBundleConfigFileName = "defaults.uds.hcl"

// Variables is a named type for the nested user-defined variable map parsed
// from the variables block in defaults.uds.hcl and config.uds.hcl.
// Leaf values are scalars (string, float64, bool); intermediate nodes are
// nested Variables maps decoded from HCL object expressions.
// nil means no --config was provided.
//
// Using a named type (rather than bare map[string]any) follows the same
// pattern as Zarf's value.Values and allows behaviour to be attached as
// methods — in particular Flatten(), which keeps that logic intrinsic to
// the type rather than as a scattered private helper.
type Variables map[string]any

// GlobalOptions holds process-wide CLI options that apply to all commands.
// Prompt is populated exclusively from the CLI flag, not from config.uds.hcl.
// LogLevel can be controlled by both config file and CLI flag.
// Prompt is controlled by the --prompt flag on the deploy command (see ADR-0005).
type GlobalOptions struct {
	LogLevel string
	Prompt   bool
}

// UDSBundleConfig represents the parsed content of a config.uds.hcl file.
// Global holds process-wide options populated by the CLI layer (not from HCL).
// The Options block is decoded via gohcl using HCL struct tags.
// Variables are free-form and captured via hcl:",remain" for manual extraction,
// since they have no fixed schema.
type UDSBundleConfig struct {
	Global    *GlobalOptions
	Options   *ConfigOptions `hcl:"options,block"`
	Variables Variables      // populated after decode from Remain
	Remain    hcl.Body       `hcl:",remain"` // captures variables and any other unstructured top-level attributes
}

// ConfigOptions holds bundle-component CLI options from the options block.
// Fields are defined by the Opinionated CLI Settings ADR (ADR-0006).
// All fields are optional; unset fields default to their zero values.
type ConfigOptions struct {
	LogLevel      string `hcl:"log_level,optional"`
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
	// ParseBundleBytes parses HCL bundle content from an in-memory byte slice.
	ParseBundleBytes(ctx context.Context, src []byte) (*UDSBundle, error)
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

// Deployer is the interface for deploying packages to a target.
// Implementations can include: ZarfDeployer (local), TofuDeployer, RemoteAgentDeployer.
type Deployer interface {
	// DeployPackage deploys a single package.
	// Called in topological order - dependencies are already deployed.
	DeployPackage(ctx context.Context, pkg *Package, opts DeployPackageOptions) error
}

// DeployPackageOptions contains options for deploying a single package.
type DeployPackageOptions struct {
	// Config is the merged config (options + variables); always non-nil.
	Config *UDSBundleConfig

	// BundleDir is the directory containing the bundle (for resolving relative paths)
	BundleDir string

	// Prompt enables interactive prompts (non-interactive by default per ADR 0005)
	Prompt bool
}

// DeployOptions contains options for deploying an entire bundle.
type DeployOptions struct {
	// Config is the merged config (options + variables); always non-nil.
	Config *UDSBundleConfig

	// BundlePath is the path to the bundle directory containing bundle.uds.hcl
	BundlePath string

	// Bundle is the pre-parsed bundle. When set, Deploy() skips parsing BundlePath.
	// This avoids double-parsing when the caller has already parsed the bundle.
	Bundle *UDSBundle

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
	// Config is the merged config; always non-nil.
	Config *UDSBundleConfig

	BlobDir   string
	BundleDir string
	Out       io.Writer
}

// CreateOptions holds configuration for the top-level bundle create operation.
type CreateOptions struct {
	// Config is the merged config (options only, no variables for create); always non-nil.
	Config *UDSBundleConfig

	BundleFile string

	Out io.Writer
}

// PullOptions holds configuration for pulling a bundle from an OCI registry.
type PullOptions struct {
	RegistryOptions

	// OCIReference is the source OCI registry reference (e.g. ghcr.io/org/bundle:v1).
	OCIReference string

	// OutputDir is the directory where the pulled bundle tarball will be written.
	OutputDir string

	// TmpDir is the base directory for temporary files.
	TmpDir string

	// remoteRepo overrides the remote registry source. When nil (production),
	// newRemoteRepository is used. Set in unit tests to inject an in-memory store.
	remoteRepo oras.ReadOnlyTarget
}

// InspectResult represents the output of a bundle inspect operation.
type InspectResult struct {
	Name        string           `json:"name"        yaml:"name"        text:"Name"`
	Description string           `json:"description" yaml:"description" text:"Description,omitempty"`
	Version     string           `json:"version"     yaml:"version"     text:"Version,omitempty"`
	Packages    []PackageSummary `json:"packages"    yaml:"packages"    text:"PACKAGES"`
}

// PackageSummary is a serializable summary of a package within a bundle.
// Packages are listed in DAG (deployment) order.
type PackageSummary struct {
	Name        string   `json:"name"                          yaml:"name"                          text:"Name"`
	Source      string   `json:"source"                        yaml:"source"                        text:"Source"`
	Namespace   string   `json:"namespace,omitempty"           yaml:"namespace,omitempty"           text:"Namespace,omitempty"`
	DependsOn   []string `json:"dependsOn,omitempty"           yaml:"dependsOn,omitempty"           text:"DependsOn,omitempty"`
	ValuesFiles []string `json:"valuesFiles,omitempty"         yaml:"valuesFiles,omitempty"         text:"Value Files,omitempty"`
}

// CreateResult represents the output of a bundle create operation.
type CreateResult struct {
	BundleName string `json:"bundleName" yaml:"bundleName" text:"Bundle Name"`
	OutputPath string `json:"outputPath" yaml:"outputPath" text:"Output Path"`
}

// DeployResult represents the output of a bundle deploy operation.
type DeployResult struct {
	BundleName string `json:"bundleName" yaml:"bundleName" text:"Bundle Name"`
	Packages   int    `json:"packages"   yaml:"packages"   text:"Packages"`
}

// PullResult represents the output of a bundle pull operation.
type PullResult struct {
	OCIReference string `json:"ociReference" yaml:"ociReference" text:"OCI Reference"`
	OutputPath   string `json:"outputPath"   yaml:"outputPath"   text:"Output Path"`
}

// PushResult represents the output of a bundle push operation.
type PushResult struct {
	OCIReference string `json:"ociReference" yaml:"ociReference" text:"OCI Reference"`
}

// RemoveResult represents the output of a bundle remove operation.
type RemoveResult struct {
	OCIReference string `json:"ociReference" yaml:"ociReference" text:"OCI Reference"`
}

// PushOptions holds configuration for pushing a bundle to an OCI registry.
type PushOptions struct {
	RegistryOptions

	// BundleTarball is the path to the .tar.zst bundle file.
	BundleTarball string

	// OCIReference is the target OCI registry reference (e.g. ghcr.io/org/bundle:v1).
	OCIReference string

	// TmpDir is the base directory for temporary files.
	TmpDir string

	// remoteRepo overrides the remote registry destination. When nil (production),
	// newRemoteRepository is used. Set in unit tests to inject an in-memory store.
	remoteRepo oras.Target
}
