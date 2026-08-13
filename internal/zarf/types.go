// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package zarf

import (
	"context"
	"io"
	"io/fs"
	"sync"

	"github.com/defenseunicorns/pkg/oci"
	bundleinternal "github.com/defenseunicorns/uds-cli/internal/bundle"
	udsoci "github.com/defenseunicorns/uds-cli/internal/oci"
	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/hashicorp/hcl/v2"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/zarf-dev/zarf/src/pkg/cluster"
	"github.com/zarf-dev/zarf/src/pkg/packager"
	"github.com/zarf-dev/zarf/src/pkg/packager/filters"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
	"github.com/zarf-dev/zarf/src/pkg/zoci"
)

const (
	tempDirPerm fs.FileMode = 0o700
	tmpFilePerm fs.FileMode = 0o600
)

// UDSBundle is the shared semantic bundle model.
type UDSBundle = spec.UDSBundle

// UDSBlock is the shared bundle constraints model.
type UDSBlock = spec.UDSBlock

// Metadata is the shared bundle metadata model.
type Metadata = spec.Metadata

// Package is the shared package model.
type Package = spec.Package

// PackageRef is the shared package reference model.
type PackageRef = spec.PackageRef

// SourceRange is the shared bundle source span model.
type SourceRange = spec.SourceRange

// SourcePosition is the shared bundle source position model.
type SourcePosition = spec.SourcePosition

// PackageSignatureVerification is the shared package signature policy model.
type PackageSignatureVerification = spec.PackageSignatureVerification

// KeylessSignatureVerification is the shared keyless signature policy model.
type KeylessSignatureVerification = spec.KeylessSignatureVerification

// Variables contains private deployment configuration variables.
type Variables map[string]any

// GlobalOptions contains private process-wide deployment settings.
type GlobalOptions struct {
	LogLevel string
	Prompt   bool
}

// UDSBundleConfig is the private resolved deployment configuration.
type UDSBundleConfig struct {
	Global    *GlobalOptions
	Options   *ConfigOptions `hcl:"options,block"`
	Variables Variables
	Remain    hcl.Body `hcl:",remain"`
}

// ConfigOptions contains private Zarf deployment settings.
type ConfigOptions struct {
	LogLevel      string `hcl:"log_level,optional"`
	Architecture  string `hcl:"architecture,optional"`
	PlainHTTP     bool   `hcl:"plain_http,optional"`
	SkipTLSVerify bool   `hcl:"skip_tls_verify,optional"`
	UDSCache      string `hcl:"uds_cache,optional"`
	TmpDir        string `hcl:"tmp_dir,optional"`
	Concurrency   int    `hcl:"concurrency,optional"`
}

// PackageDeployHooks provides deployment process extensibility per package.
type PackageDeployHooks struct {
	PreDeploy  func(ctx context.Context, pkg *spec.Package, pkgLayout *layout.PackageLayout, opts *packager.DeployOptions, packageOpts *DeployPackageOptions) error
	PostDeploy func(ctx context.Context, pkg *spec.Package) error
}

// BundleDeployHooks provides deployment process extensibility for a bundle.
type BundleDeployHooks struct {
	PreDeploy  func(ctx context.Context, b *spec.UDSBundle, opts *DeployOptions) error
	PostDeploy func(ctx context.Context, b *spec.UDSBundle) error
}

// DeployOptions contains private options for deploying a bundle.
type DeployOptions struct {
	Config             *UDSBundleConfig
	BundlePath         string
	Packages           []string
	BundleDeployHooks  BundleDeployHooks
	PackageDeployHooks PackageDeployHooks
	PackageDeployFn    func(ctx context.Context, pkg *spec.Package, opts DeployPackageOptions) error
}

// DeployResult represents the result of deploying a bundle.
type DeployResult struct {
	BundleName string
	Packages   int
}

// RemoveResult represents the result of removing a bundle.
type RemoveResult struct {
	BundleName string
	Removed    int
	Skipped    int
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

// DeployPackageOptions contains options for deploying a single package.
type DeployPackageOptions struct {
	// Config is the merged config (options + variables); always non-nil.
	Config *UDSBundleConfig

	// BundleDir is the directory containing the bundle (for resolving relative paths)
	BundleDir string

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

// RemovePackageOptions contains options for removing a single package.
type RemovePackageOptions struct {
	// Config is the merged config (options + variables); always non-nil.
	Config *UDSBundleConfig

	// Force bypasses the removal safety check. Threaded from RemoveOptions.Force.
	Force bool
}

// LoadOptions carries options for bundle and package layout loading.
// It replaces the positional streams parameter previously on LoadPackageLayout and the
// IsPartial field previously on DirectoryPackageLayoutLoader.
type LoadOptions struct {
	// Streams carries In/Out/ErrOut and the bound logger for diagnostics.
	Streams iostreams.IOStreams

	// IsPartial controls whether the staged package is treated as partially extracted.
	// When true, checksums.txt may reference layers not present on disk.
	// Applies only to LoadPackageLayout.
	// ExtractedArtifactPackageLayoutLoader always forces IsPartial: true for
	// OCI-blob-staged packages regardless of this field.
	IsPartial bool
}

// PackageLayoutLoader abstracts how a per-package layout is obtained for deploy.
type PackageLayoutLoader interface {
	// LoadPackageLayout stages the package's contents into dstDir and returns a
	// PackageLayout ready for packager.Deploy. dstDir must already exist and
	// is owned by the caller. opts.Streams carries the bound logger for diagnostics;
	// opts.IsPartial controls partial-package semantics.
	LoadPackageLayout(ctx context.Context, pkg *Package, dstDir string, opts LoadOptions) (*layout.PackageLayout, error)
}

// PackageSource abstracts how a Zarf package is fetched, supporting both
// OCI registries and local paths. Remote implementations apply component
// filtering before downloading to avoid pulling unnecessary layers; local
// implementations may apply filtering after reading package contents.
type PackageSource interface {
	// PullFiltered pulls a Zarf package to tmpDir, applying loadOptions.Filter as part
	// of retrieval. For remote sources the filter is applied before downloading
	// layers when possible. Returns a PackageLayout ready for packager.Deploy().
	// Used by the Deploy command.
	PullFiltered(ctx context.Context, tmpDir string, loadOptions layout.PackageLayoutOptions) (*layout.PackageLayout, error)

	// IngestFiltered ingests a Zarf package into an OCI content store,
	// applying the filter during ingestion. For remote sources the filter is
	// applied before downloading layers when possible. Returns manifest
	// descriptors for the bundle's OCI index. Used by the Create command.
	IngestFiltered(ctx context.Context, filter filters.ComponentFilterStrategy, store *udsoci.Store) ([]ocispec.Descriptor, error)

	// VerifyAndIngestFiltered retrieves a package into tmpDir, verifies it with
	// loadOptions, and ingests those exact retrieved bytes into store.
	VerifyAndIngestFiltered(ctx context.Context, tmpDir string, loadOptions layout.PackageLayoutOptions, store *udsoci.Store) ([]ocispec.Descriptor, error)
}

// localSource handles Zarf packages from local directories and archives.
type localSource struct {
	path      string
	arch      string
	bundleDir string
	tmpDir    string
	streams   iostreams.IOStreams
}

// remoteSource pulls Zarf packages from OCI registries using zoci.NewRemote.
type remoteSource struct {
	ref     string
	arch    string
	opts    ConfigOptions
	streams iostreams.IOStreams
}

// resolvedLayers holds the result of connecting to a remote and resolving
// which layers to fetch based on component filtering.
type resolvedLayers struct {
	remote    *zoci.Remote
	root      *oci.Manifest
	layers    []ocispec.Descriptor
	isPartial bool
}

type layerIdentity struct {
	digest string
	title  string
}

// ExtractedArtifactPackageLayoutLoader reads package OCI blobs from an extracted bundle artifact workspace.
// LoadPackageLayout stages OCI blobs from the artifact workspace, with a fallback to directory staging
// for local-source packages not in the OCI index.
type ExtractedArtifactPackageLayoutLoader struct {
	// OCIDir is <workspace>/oci — the extracted OCI image layout root.
	OCIDir string
	// PackageManifests maps package ref.name values to complete manifest descriptors.
	PackageManifests map[string]ocispec.Descriptor
	// PackageDigests is retained for callers that only have legacy digest maps.
	// Keys are package ref.name values.
	PackageDigests map[string]string
}

// SourcePackageLayoutLoader implements PackageLayoutLoader using the standard OCI/local pull
// path via NewPackageSource. It is the default loader when ZarfDeployer.Loader is nil.
type SourcePackageLayoutLoader struct {
	configOpts ConfigOptions
	bundleDir  string
}

// ctxReader wraps an io.Reader and checks ctx on every Read so large copies observe cancellation.
type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

// orchestratedDeployer dispatches per-package deploys to either a caller-supplied
// override or the underlying ZarfDeployer. Created by ZarfDeployer.DeployBundle
// so the orchestrator always calls through a consistent Deployer.
type orchestratedDeployer struct {
	base            *ZarfDeployer
	packageDeployFn func(ctx context.Context, pkg *Package, opts DeployPackageOptions) error
}

// ZarfDeployer implements Deployer using the Zarf Go library.
// Reference: .ai/example-repos/uds-cli/src/pkg/bundle/deploy.go lines 38-165
type ZarfDeployer struct {
	// streams carries the diagnostic sink (streams.ErrOut) handed to the Zarf
	// logger and the leveled logger used for UDS-side diagnostics.
	streams iostreams.IOStreams

	// Loader, when non-nil, is used instead of NewPackageSource to obtain each
	// package's layout. Used when deploying from a pre-extracted workspace (ADR-0009).
	Loader PackageLayoutLoader
}

// deployOrchestrator schedules per-package deploys across a bundle's
// topological levels — parallelising within a level (bounded by concurrency),
// serialising across levels, and stopping gracefully on failure (already
// in-flight packages run to completion; queued packages are dropped).
//
// Its sole responsibility is orchestration. It deploys each package through the
// Deployer interface and knows nothing about how a package is actually deployed.
type deployOrchestrator struct {
	deployer    Deployer
	dag         *bundleinternal.DAG
	levels      [][]*Package
	concurrency int
	pkgOpts     DeployPackageOptions
	streams     iostreams.IOStreams
}

// errorAccumulator is a thread-safe collector for errors returned by concurrent
// package operations. It preserves every non-nil error so callers can report all
// failed packages instead of only the first goroutine failure.
type errorAccumulator struct {
	mu   sync.Mutex
	list []error
}

// packageRemover is the narrow per-package primitive that ZarfRemover.removePackages
// drives during bundle removal. ZarfRemover defaults to dispatching to its own
// RemovePackage; tests inject a mock to verify orchestration in isolation.
type packageRemover interface {
	RemovePackage(ctx context.Context, pkg *Package, opts RemovePackageOptions) error
}

// ZarfRemover implements Remover using the Zarf Go library.
// The cluster client and the set of deployed packages are lazily initialized
// on first use and reused across calls; this avoids repeated cluster
// round-trips when a bundle removes many packages.
type ZarfRemover struct {
	// streams carries the diagnostic sink (streams.ErrOut) handed to the Zarf
	// logger (see RemovePackage) and the leveled logger for UDS diagnostics.
	streams iostreams.IOStreams
	cluster *cluster.Cluster

	deployedMu     sync.Mutex
	deployed       map[string]struct{} // keyed by deployedKey(zarfMetadataName, namespaceOverride)
	deployedLoaded bool

	// pkgRemover is the per-package primitive used by removePackages.
	// Defaults to r itself in production; tests override it with a mock.
	pkgRemover packageRemover
}
