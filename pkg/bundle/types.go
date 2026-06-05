// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"errors"
	"io"

	"github.com/hashicorp/hcl/v2"
	"github.com/zarf-dev/zarf/src/pkg/packager"
	"github.com/zarf-dev/zarf/src/pkg/packager/filters"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
	oras "oras.land/oras-go/v2"

	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
)

// ErrPackageNotDeployed is returned by Remover.RemovePackage when the requested
// package is not present on the target. The orchestrator treats this as a skip
// rather than a failure.
var ErrPackageNotDeployed = errors.New("package not deployed")

// ErrNotImplemented is returned by interface stub methods that have no
// production implementation yet.
var ErrNotImplemented = errors.New("not yet implemented")

// BundleFileName is the name of the bundle definition file.
const BundleFileName = "bundle.uds.hcl"

// MaxConcurrency is the upper bound for parallel package deploys within a level.
// Each concurrent deploy pulls an OCI package to disk, creates a temp directory,
// and runs a Helm install against the cluster. Values above this limit risk
// exhausting disk, overwhelming the Kubernetes API server, or hitting OCI
// registry rate limits.
const MaxConcurrency = 25

// BundleDefaultsFileName is the name of the optional bundle-level defaults file.
// When present alongside bundle.uds.hcl, it is auto-discovered and applied as the
// lowest-priority variable layer. Only the variables attribute is supported.
const BundleDefaultsFileName = "defaults.uds.hcl"

// Variables is a named type for the user-defined variable map parsed from the
// variables block in defaults.uds.hcl and config.uds.hcl. Leaf values are scalars
// (string, float64, bool); intermediate nodes are nested Variables maps decoded
// from HCL object/map expressions. List, set, and tuple values are []any.
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

// Deployer is the interface for deploying packages to a target. It exposes
// both a low-level per-package primitive and a high-level bundle-level entry
// point so callers can choose the abstraction that fits their use case.
// DeployBundle iterates the bundle and delegates each package to DeployPackage
// internally.
//
// Implementations are responsible for dependency ordering, concurrency control,
// and any target-specific orchestration concerns.
//
// Implementations can include: ZarfDeployer (local), TofuDeployer, RemoteAgentDeployer.
type Deployer interface {
	// DeployPackage deploys a single package.
	// Called in topological order, dependencies are already deployed.
	DeployPackage(ctx context.Context, pkg *Package, opts DeployPackageOptions) error

	// DeployBundle deploys the bundle's packages to the target, calling
	// DeployPackage for each package internally. The implementation handles
	// dependency ordering (topological order), parallelism within levels, and
	// concurrency limits.
	DeployBundle(ctx context.Context, b *UDSBundle, opts DeployOptions) (*DeployResult, error)
}

// PackageDeployHooks provides Deployment process extensibility on a per-package basis.
// In the future, new hooks might also be provided.
type PackageDeployHooks struct {
	// PreDeploy enables customizing the options just before deploying the Package.
	// Called after the package layout is loaded and before packager.Deploy. Mutations to
	// pkgLayout.Pkg and opts take effect immediately — packager.Deploy receives the mutated values.
	// Note: the hook pair (PreDeploy+PostDeploy) is captured before PreDeploy is invoked, so
	// mutations to packageOpts.PackageDeployHooks from within PreDeploy have no effect. To install
	// per-package hooks dynamically, use BundleDeployHooks.PreDeploy instead (it runs before
	// pkgOpts is built).
	// A non-nil error aborts the deploy; packager.Deploy is never called and PostDeploy is skipped.
	// May run concurrently with PreDeploy for other packages within the same DAG level.
	PreDeploy func(ctx context.Context, pkg *Package, pkgLayout *layout.PackageLayout, opts *packager.DeployOptions, packageOpts *DeployPackageOptions) error

	// PostDeploy enables tracking what Packages have been deployed.
	// Called after a successful packager.Deploy. Not called when PreDeploy or the deploy itself errors.
	// May run concurrently with PostDeploy for other packages within the same DAG level — implementations must be concurrency-safe.
	PostDeploy func(ctx context.Context, pkg *Package) error
}

// BundleDeployHooks provides Deployment process extensibility at the whole-bundle scope.
// Symmetric to PackageDeployHooks, but fired exactly once per bundle deploy (not per package)
// and never concurrently. Full ordering: Bundle.PreDeploy → (Package.PreDeploy → deploy →
// Package.PostDeploy)* → Bundle.PostDeploy.
type BundleDeployHooks struct {
	// PreDeploy runs once before any package is deployed, after bundle validation.
	// It may mutate the bundle and DeployOptions (e.g. install PackageDeployHooks, adjust Prompt).
	// Mutations to opts.Prompt and opts.PackageDeployHooks are honoured: pkgOpts is built after
	// PreDeploy returns.
	// Callers must not mutate opts.Config or opts.Config.Options — those fields are validated
	// before PreDeploy but read afterward (e.g. Config.Options.Concurrency), so a mutation
	// takes effect while bypassing validation.
	// Note: opts.Source is consumed by Deploy() before the deployer is constructed and is NOT
	// re-read after PreDeploy — mutations to opts.Source have no effect.
	// Note: mutations to opts.BundleDeployHooks are NOT honoured — BundleDeployHooks is captured
	// before PreDeploy is invoked, so replacing PostDeploy here has no effect.
	// A non-nil error aborts before any package is deployed.
	PreDeploy func(ctx context.Context, b *UDSBundle, opts *DeployOptions) error

	// PostDeploy runs once after all packages have deployed successfully.
	// A non-nil error causes DeployBundle to return that error with the populated result
	// (packages are already deployed at this point).
	PostDeploy func(ctx context.Context, b *UDSBundle) error
}

// DeployPackageOptions contains options for deploying a single package.
type DeployPackageOptions struct {
	// Config is the merged config (options + variables); always non-nil.
	Config *UDSBundleConfig

	// BundleDir is the directory containing the bundle (for resolving relative paths)
	BundleDir string

	// Prompt enables interactive prompts (non-interactive by default per ADR 0005)
	Prompt bool

	// PackageDeployHooks provides optional pre- and post-deploy callbacks for this package.
	// Nil func fields are replaced with no-ops by withDefaults(); every deploy traverses both call sites.
	PackageDeployHooks PackageDeployHooks

	// ClusterDeployFn performs the cluster-side deploy of the loaded package layout.
	// Nil defaults to packager.Deploy. Override it to deploy without a real cluster — this is
	// the seam that makes the full deploy pipeline (loader, hooks, layout mutation) testable.
	ClusterDeployFn func(ctx context.Context, pkgLayout *layout.PackageLayout, opts packager.DeployOptions) error

	// Streams carries In/Out/ErrOut for the operation.
	Streams iostreams.IOStreams
}

// DeploySource abstracts the differences between bundle deployment pipeline
// sources: a local source bundle directory vs an extracted .tar.zst bundle OCI
// artifact workspace. It owns any temporary resources needed to read the bundle
// and provides source-specific package loading behavior when the default source
// loader is not applicable.
type DeploySource struct {
	// BundlePath is the absolute path to the bundle definition file (bundle.uds.hcl).
	BundlePath string
	// Loader overrides how package layouts are obtained; nil means use default source package loader.
	Loader PackageLayoutLoader
	// ValuesFilesOverride maps package name to ordered values file paths to apply
	// in lieu of the values files in the bundle definition HCL. Nil for source-directory deploys.
	ValuesFilesOverride map[string][]string

	closer io.Closer
}

// Close releases any temporary resources allocated during source preparation.
func (s *DeploySource) Close() error {
	if s == nil || s.closer == nil {
		return nil
	}
	return s.closer.Close()
}

// DeployOptions contains options for deploying an entire bundle.
type DeployOptions struct {
	// Config is the merged config (options + variables); always non-nil.
	Config *UDSBundleConfig

	// BundlePath is the path to the bundle definition file (bundle.uds.hcl).
	BundlePath string

	// Bundle is the pre-parsed bundle. When set, Deploy() skips parsing BundlePath.
	// This avoids double-parsing when the caller has already parsed the bundle.
	Bundle *UDSBundle

	// Source is the prepared deploy source from PrepareDeploySource. When non-nil,
	// Deploy() uses Source.Loader for the deployer and applies Source.ValuesFilesOverride.
	Source *DeploySource

	// Prompt enables interactive prompts (non-interactive by default per ADR 0005)
	Prompt bool

	// BundleDeployHooks fires once at bundle scope in DeployBundle, before and after all packages.
	// Nil func fields are replaced with no-ops; every deploy traverses both call sites.
	BundleDeployHooks BundleDeployHooks

	// PackageDeployHooks are threaded into each package's DeployPackageOptions.
	// Nil func fields are replaced with no-ops; every deploy traverses both call sites.
	PackageDeployHooks PackageDeployHooks

	// PackageDeployFn deploys a single package. Nil defaults to the deployer's DeployPackage.
	// Its signature mirrors Deployer.DeployPackage: overriding it replaces the whole per-package
	// deploy, so an override that still wants the loader + hooks should delegate to DeployPackage
	// (e.g. set opts.ClusterDeployFn, then call the deployer's DeployPackage).
	PackageDeployFn func(ctx context.Context, pkg *Package, opts DeployPackageOptions) error

	// Streams carries In/Out/ErrOut for the operation.
	Streams iostreams.IOStreams
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

// PackageLayoutLoader abstracts how a per-package layout is obtained for deploy.
type PackageLayoutLoader interface {
	// LoadPackageLayout stages the package's contents into dstDir and returns a
	// PackageLayout ready for packager.Deploy. dstDir must already exist and
	// is owned by the caller. streams carries the bound logger for diagnostics.
	LoadPackageLayout(ctx context.Context, streams iostreams.IOStreams, pkg *Package, dstDir string) (*layout.PackageLayout, error)
}

// PackageSource abstracts how a Zarf package is fetched, supporting both
// OCI registries and local paths. Remote implementations apply component
// filtering before downloading to avoid pulling unnecessary layers; local
// implementations may apply filtering after reading package contents.
type PackageSource interface {
	// PullFiltered pulls a Zarf package to tmpDir, applying the filter as part
	// of retrieval. For remote sources the filter is applied before downloading
	// layers when possible. Returns a PackageLayout ready for packager.Deploy().
	// Used by the Deploy command.
	PullFiltered(ctx context.Context, filter filters.ComponentFilterStrategy, tmpDir string) (*layout.PackageLayout, error)

	// IngestFiltered ingests a Zarf package into an OCI blob directory at blobDir,
	// applying the filter during ingestion. For remote sources the filter is
	// applied before downloading layers when possible. Returns manifest
	// descriptors for the bundle's OCI index. Used by the Create command.
	IngestFiltered(ctx context.Context, filter filters.ComponentFilterStrategy, blobDir string) ([]ociManifest, error)
}

// CreatePackageOptions holds per-package configuration during bundle creation.
type CreatePackageOptions struct {
	// Config is the merged config; always non-nil.
	Config *UDSBundleConfig

	BlobDir   string
	BundleDir string

	// Streams carries In/Out/ErrOut for the operation.
	Streams iostreams.IOStreams
}

// CreateOptions holds configuration for the top-level bundle create operation.
type CreateOptions struct {
	// Config is the merged config (options only, no variables for create); always non-nil.
	Config *UDSBundleConfig

	BundleFile string

	// Streams carries In/Out/ErrOut for the operation.
	Streams iostreams.IOStreams
}

// PullOptions holds configuration for pulling a bundle from an OCI registry.
type PullOptions struct {
	// Config is the merged config; always non-nil in production.
	Config *UDSBundleConfig

	// Streams carries In/Out/ErrOut for the operation.
	Streams iostreams.IOStreams

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

// Remover is the interface for removing packages from a target. It exposes
// both a low-level per-package primitive and a high-level bundle-level entry
// point so callers can choose the abstraction that fits their use case.
// RemoveBundle iterates the bundle and delegates each package to RemovePackage
// internally.
//
// Implementations are responsible for any cluster-state checks (e.g. whether
// the package is currently deployed). When the package is not present on the
// target, RemovePackage must return ErrPackageNotDeployed so the caller can
// count it as skipped rather than failed.
//
// Implementations can include: ZarfRemover (delegates to Zarf packager.Remove).
type Remover interface {
	// RemovePackage removes a single package from the target. It must return
	// ErrPackageNotDeployed if the package is not currently deployed.
	RemovePackage(ctx context.Context, pkg *Package, opts RemovePackageOptions) error

	// RemoveBundle removes the bundle's packages from the target, calling
	// RemovePackage for each package internally. The implementation handles
	// dependency ordering (reverse topological order), deployment-status
	// checks, and skip behavior. When packages is non-empty, only those
	// package names are removed.
	RemoveBundle(ctx context.Context, b *UDSBundle, packages []string, opts RemovePackageOptions) (*RemoveResult, error)
}

// Puller is the interface for pulling bundle artifacts from an OCI registry.
// OCIReference and targetDir are method-level parameters; config and extensibility
// belong in PullOptions. This matches the Technical Design Doc surface.
type Puller interface {
	// PullBundle pulls a bundle from the given OCI reference and writes it to targetDir.
	PullBundle(ctx context.Context, ociReference, targetDir string, opts PullOptions) (*PullResult, error)
	// PullPackage pulls a single Zarf package from the given OCI reference to targetDir.
	PullPackage(ctx context.Context, ociReference, targetDir string, opts PullOptions) (*PullResult, error)
}

// Pusher is the interface for pushing bundle artifacts to an OCI registry.
// bundleDir/packageDir and OCIReference are method-level parameters; config and
// extensibility belong in PushOptions.
type Pusher interface {
	// PushBundle pushes the OCI layout in bundleDir to the given OCI reference.
	// bundleDir must contain an oci/ subdirectory with a valid OCI layout (index.json + blobs/).
	PushBundle(ctx context.Context, bundleDir, ociReference string, opts PushOptions) (*PushResult, error)
	// PushPackage pushes a single Zarf package from packageDir to the given OCI reference.
	PushPackage(ctx context.Context, packageDir, ociReference string, opts PushOptions) (*PushResult, error)
}

// RemovePackageOptions contains options for removing a single package.
type RemovePackageOptions struct {
	// Config is the merged config (options + variables); always non-nil.
	Config *UDSBundleConfig
}

// RemoveOptions contains options for removing an entire bundle.
type RemoveOptions struct {
	// Config is the merged config; always non-nil.
	Config *UDSBundleConfig

	// BundlePath is the path to the bundle definition file (bundle.uds.hcl).
	BundlePath string

	// Bundle is the pre-parsed bundle. When set, Remove() skips parsing BundlePath.
	Bundle *UDSBundle

	// Packages is an optional list of specific package names to remove.
	// When empty, all packages in the bundle are removed.
	Packages []string

	// Streams carries In/Out/ErrOut for the operation.
	Streams iostreams.IOStreams
}

// RemoveResult represents the output of a bundle remove operation.
type RemoveResult struct {
	BundleName string `json:"bundleName" yaml:"bundleName" text:"Bundle Name"`
	Removed    int    `json:"removed"    yaml:"removed"    text:"Removed"`
	Skipped    int    `json:"skipped"    yaml:"skipped"    text:"Skipped"`
}

// Reconfigurer replaces the defaults layer in a bundle artifact,
// producing a new derivative artifact.
type Reconfigurer interface {
	Reconfigure(ctx context.Context, opts ReconfigureOptions) (*ReconfigureResult, error)
}

// ReconfigureOptions holds configuration for the bundle reconfigure operation.
type ReconfigureOptions struct {
	// Source is the local .tar.zst path or OCI reference (oci://...) to reconfigure.
	Source string

	// DefaultsFile is the path to the new defaults.uds.hcl on disk.
	DefaultsFile string

	// Suffix is appended to the output artifact name (default: "-reconfigured").
	Suffix string

	// OutputDir is the directory where the reconfigured local tarball is written.
	// Only valid for local sources; must be empty for OCI sources.
	OutputDir string

	// Options provides shared CLI configuration for the operation.
	Options ConfigOptions

	// Streams carries In/Out/ErrOut for the operation.
	Streams iostreams.IOStreams

	// remoteRepo overrides the remote registry target. When nil (production),
	// newRemoteRepository is used. Set in tests to inject a fake ORAS store.
	remoteRepo oras.Target
}

// ReconfigureResult represents the output of a bundle reconfigure operation.
type ReconfigureResult struct {
	OutputPath   string `json:"outputPath,omitempty"   yaml:"outputPath,omitempty"   text:"Output Path,omitempty"`
	OCIReference string `json:"ociReference,omitempty" yaml:"ociReference,omitempty" text:"OCI Reference,omitempty"`
}

// PushOptions holds configuration for pushing a bundle to an OCI registry.
type PushOptions struct {
	// Config is the merged config; always non-nil in production.
	Config *UDSBundleConfig

	// Streams carries In/Out/ErrOut for the operation.
	Streams iostreams.IOStreams

	// remoteRepo overrides the remote registry destination. When nil (production),
	// newRemoteRepository is used. Set in unit tests to inject an in-memory store.
	remoteRepo oras.Target
}
