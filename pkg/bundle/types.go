// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"io"

	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/hashicorp/hcl/v2"
	"github.com/zarf-dev/zarf/src/pkg/packager"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
	oras "oras.land/oras-go/v2"

	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
)

// Variables contains user-defined bundle configuration variables.
type Variables map[string]any

// GlobalOptions holds process-wide CLI options.
type GlobalOptions struct {
	LogLevel string
	Prompt   bool
}

// UDSBundleConfig is the resolved public bundle configuration.
type UDSBundleConfig struct {
	Global                *GlobalOptions
	Options               *ConfigOptions `hcl:"options,block"`
	SignatureVerification *VerificationPolicy
	Variables             Variables
	Remain                hcl.Body `hcl:",remain"`
}

// ConfigOptions holds bundle operation settings.
type ConfigOptions struct {
	LogLevel      string `hcl:"log_level,optional"`
	Architecture  string `hcl:"architecture,optional"`
	PlainHTTP     bool   `hcl:"plain_http,optional"`
	SkipTLSVerify bool   `hcl:"skip_tls_verify,optional"`
	UDSCache      string `hcl:"uds_cache,optional"`
	TmpDir        string `hcl:"tmp_dir,optional"`
	Concurrency   int    `hcl:"concurrency,optional"`
}

// SigningMode identifies the credentials used to sign a bundle.
type SigningMode string

const (
	// SigningModeKey signs with a Cosign-compatible private key or KMS reference.
	SigningModeKey SigningMode = "key"
	// SigningModeKeyless signs with an OIDC identity.
	SigningModeKeyless SigningMode = "keyless"
	// SigningModeUnsigned intentionally produces an unsigned bundle.
	SigningModeUnsigned SigningMode = "unsigned"
)

// SigningOptions configures a bundle signature operation.
type SigningOptions struct {
	Mode           SigningMode
	Key            string
	KeyPassword    string
	IdentityToken  string
	FulcioURL      string
	FulcioAuthFlow string
	OIDCIssuer     string
	OIDCClientID   string
	RekorURL       string
	TSAServerURL   string
	Overwrite      bool
}

// KeylessVerification constrains the certificate identity trusted for a keyless signature.
type KeylessVerification struct {
	CertificateIdentity         string
	CertificateIdentityRegexp   string
	CertificateOIDCIssuer       string
	CertificateOIDCIssuerRegexp string
	TrustedRoot                 string
}

// VerificationPolicy is consumer-controlled trust material for a bundle signature.
type VerificationPolicy struct {
	PublicKey string
	Keyless   *KeylessVerification
}

// SignOptions configures signing an existing bundle artifact.
type SignOptions struct {
	Source  string
	Signing SigningOptions
	Config  *UDSBundleConfig
	TmpDir  string
	Streams iostreams.IOStreams
}

// VerifyOptions configures verification of a bundle artifact.
type VerifyOptions struct {
	Source  string
	Policy  VerificationPolicy
	Config  *UDSBundleConfig
	TmpDir  string
	Streams iostreams.IOStreams
}

// Parser parses bundle definitions and configuration files.
type Parser interface {
	ParseBundleFile(ctx context.Context, filePath string) (*spec.UDSBundle, error)
	ParseBundleBytes(ctx context.Context, src []byte) (*spec.UDSBundle, error)
	ParseBundleConfig(ctx context.Context, filePath string) (*UDSBundleConfig, error)
}

// DeployPackageOptions contains options for deploying one package.
type DeployPackageOptions struct {
	Config             *UDSBundleConfig
	BundleDir          string
	PackageDeployHooks PackageDeployHooks
	ClusterDeployFn    func(ctx context.Context, pkgLayout *layout.PackageLayout, opts packager.DeployOptions) error
	Streams            iostreams.IOStreams
}

// LoadOptions carries options for package layout loading.
type LoadOptions struct {
	Streams   iostreams.IOStreams
	IsPartial bool
}

// PackageLayoutLoader loads a deployable package layout.
type PackageLayoutLoader interface {
	LoadPackageLayout(ctx context.Context, pkg *spec.Package, dstDir string, opts LoadOptions) (*layout.PackageLayout, error)
}

// Puller pulls bundle and package artifacts from OCI storage.
type Puller interface {
	PullBundle(ctx context.Context, ociReference, targetDir string, opts PullOptions) (*PullResult, error)
	PullPackage(ctx context.Context, ociReference, targetDir string, opts PullOptions) (*PullResult, error)
}

// Pusher pushes bundle and package artifacts to OCI storage.
type Pusher interface {
	PushBundle(ctx context.Context, bundleDir, ociReference string, opts PushOptions) (*PushResult, error)
	PushPackage(ctx context.Context, packageDir, ociReference string, opts PushOptions) (*PushResult, error)
}

// Reconfigurer produces a derivative bundle with updated defaults.
type Reconfigurer interface {
	Reconfigure(ctx context.Context, opts ReconfigureOptions) (*ReconfigureResult, error)
}

// Deployer deploys individual packages or complete bundles.
type Deployer interface {
	DeployPackage(ctx context.Context, pkg *spec.Package, opts DeployPackageOptions) error
	DeployBundle(ctx context.Context, b *spec.UDSBundle, opts DeployOptions) (*DeployResult, error)
}

// Remover removes individual packages or complete bundles.
type Remover interface {
	RemovePackage(ctx context.Context, pkg *spec.Package, opts RemovePackageOptions) error
	RemoveBundle(ctx context.Context, b *spec.UDSBundle, packages []string, opts RemovePackageOptions) (*RemoveResult, error)
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
	PreDeploy func(ctx context.Context, pkg *spec.Package, pkgLayout *layout.PackageLayout, opts *packager.DeployOptions, packageOpts *DeployPackageOptions) error

	// PostDeploy enables tracking what Packages have been deployed.
	// Called after a successful packager.Deploy. Not called when PreDeploy or the deploy itself errors.
	// May run concurrently with PostDeploy for other packages within the same DAG level — implementations must be concurrency-safe.
	PostDeploy func(ctx context.Context, pkg *spec.Package) error
}

// BundleDeployHooks provides Deployment process extensibility at the whole-bundle scope.
// Symmetric to PackageDeployHooks, but fired exactly once per bundle deploy (not per package)
// and never concurrently. Full ordering: Bundle.PreDeploy → (Package.PreDeploy → deploy →
// Package.PostDeploy)* → Bundle.PostDeploy.
type BundleDeployHooks struct {
	// PreDeploy runs once before any package is deployed, after bundle validation.
	// It may mutate the bundle and DeployOptions (e.g. install PackageDeployHooks).
	// Mutations to opts.PackageDeployHooks are honoured: pkgOpts is built after
	// PreDeploy returns.
	// Callers must not mutate opts.Config or opts.Config.Options — those fields are validated
	// before PreDeploy but read afterward (e.g. Config.Options.Concurrency), so a mutation
	// takes effect while bypassing validation.
	// Note: opts.Source is consumed by Deploy() before the deployer is constructed and is NOT
	// re-read after PreDeploy — mutations to opts.Source have no effect.
	// Note: opts.Packages and the bundle's package set (b.Packages) are consumed for
	// package-selection validation and DAG construction before PreDeploy runs, so mutating
	// the package selection here has no effect on what is validated or deployed.
	// Note: mutations to opts.BundleDeployHooks are NOT honoured — BundleDeployHooks is captured
	// before PreDeploy is invoked, so replacing PostDeploy here has no effect.
	// A non-nil error aborts before any package is deployed.
	PreDeploy func(ctx context.Context, b *spec.UDSBundle, opts *DeployOptions) error

	// PostDeploy runs once after all packages have deployed successfully.
	// A non-nil error causes DeployBundle to return that error with the populated result
	// (packages are already deployed at this point).
	PostDeploy func(ctx context.Context, b *spec.UDSBundle) error
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
	Bundle *spec.UDSBundle

	// Source is the prepared deploy source from PrepareDeploySource. When non-nil,
	// Deploy() uses Source.Loader for the deployer and applies Source.ValuesFilesOverride.
	Source *DeploySource

	// Packages is an optional list of specific package names to deploy.
	// When empty, all packages in the bundle are deployed.
	Packages []string

	// Force bypasses the deploy safety check that blocks deploying a package whose
	// dependencies are not in the selected Packages set. Use with caution: deploying
	// out of dependency order may leave the cluster in an inconsistent state.
	Force bool

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
	PackageDeployFn func(ctx context.Context, pkg *spec.Package, opts DeployPackageOptions) error

	// Streams carries In/Out/ErrOut for the operation.
	Streams iostreams.IOStreams
}

// CreateOptions holds configuration for the top-level bundle create operation.
type CreateOptions struct {
	// Config is the merged config (options only, no variables for create); always non-nil.
	Config *UDSBundleConfig

	BundleFile string

	// Signing controls the bundle artifact signature written after creation.
	Signing SigningOptions

	// Streams carries In/Out/ErrOut for the operation.
	Streams iostreams.IOStreams
}

// PullOptions holds configuration for pulling a bundle from an OCI registry.
type PullOptions struct {
	// Config is the merged config; always non-nil in production.
	Config *UDSBundleConfig

	// Verification is the trust policy used to authenticate a pulled bundle
	// before its OCI graph is downloaded. It is required unless verification is
	// explicitly skipped.
	Verification VerificationPolicy

	// SkipSignatureVerification explicitly permits pulling an unverified bundle.
	SkipSignatureVerification bool

	// Streams carries In/Out/ErrOut for the operation.
	Streams iostreams.IOStreams

	// PullHooks provides extensibility seams for the pull path, including the
	// ToOrasTarget seam used to inject an in-memory source in tests.
	PullHooks PullHooks
}

// PrepareDeploySourceOptions configures preparation of a bundle source for deployment.
// Archive artifacts are authenticated before their contents are extracted or parsed;
// source directories are used directly.
type PrepareDeploySourceOptions struct {
	// Path is a bundle source directory, bundle.uds.hcl file, or .tar.zst artifact.
	Path string

	// Config contains resolved settings needed to verify an archive artifact.
	Config *UDSBundleConfig

	// Verification is the trust policy used to authenticate an archive artifact.
	Verification VerificationPolicy

	// SkipSignatureVerification explicitly permits deploying an unverified archive.
	SkipSignatureVerification bool

	// TmpDir is the directory under which temporary workspaces are created.
	TmpDir string

	// Streams carries operation input and output streams.
	Streams iostreams.IOStreams
}

// InspectOptions configures inspection of a built bundle.
type InspectOptions struct {
	// Source is a local .tar.zst artifact or an OCI bundle reference.
	Source string

	// Config contains resolved CLI and registry settings.
	Config *UDSBundleConfig

	// Verification is consumer-controlled trust material for the bundle signature.
	Verification VerificationPolicy

	// SkipSignatureVerification explicitly permits inspecting an unverified artifact.
	SkipSignatureVerification bool

	// Streams carries operation input and output streams.
	Streams iostreams.IOStreams
}

// InspectResult represents the output of a bundle inspect operation.
type InspectResult struct {
	Name             string                  `json:"name"                    yaml:"name"                    text:"Name"`
	Description      string                  `json:"description"             yaml:"description"             text:"Description,omitempty"`
	Version          string                  `json:"version"                 yaml:"version"                 text:"Version,omitempty"`
	ArtifactDigest   string                  `json:"artifactDigest,omitempty" yaml:"artifactDigest,omitempty" text:"Artifact Digest,omitempty"`
	ReconfiguredFrom string                  `json:"reconfiguredFrom,omitempty" yaml:"reconfiguredFrom,omitempty" text:"Reconfigured From,omitempty"`
	BundleSignature  *BundleSignatureSummary `json:"bundleSignature,omitempty" yaml:"bundleSignature,omitempty" text:"Bundle Signature,omitempty"`
	Packages         []PackageSummary        `json:"packages"                yaml:"packages"                text:"Packages"`
}

// BundleSignatureSummary reports bundle signature status.
// Package metadata is not proof of bundle integrity.
type BundleSignatureSummary struct {
	Status string `json:"status" yaml:"status" text:"Status"`
}

const (
	// BundleSignatureStatusVerified means the bundle signature matched the configured policy.
	BundleSignatureStatusVerified = "verified"
	// BundleSignatureStatusUnverified means inspection did not authenticate the bundle.
	BundleSignatureStatusUnverified = "unverified"
	// BundleSignatureStatusSkipped means the caller explicitly bypassed verification.
	BundleSignatureStatusSkipped = "skipped"
)

// PackageSummary is a serializable summary of a package within a bundle.
// Packages are listed in DAG (deployment) order.
type PackageSummary struct {
	Name        string                   `json:"name"                          yaml:"name"                          text:"Name"`
	Source      string                   `json:"source"                        yaml:"source"                        text:"Source"`
	Namespace   string                   `json:"namespace,omitempty"           yaml:"namespace,omitempty"           text:"Namespace,omitempty"`
	DependsOn   []string                 `json:"dependsOn,omitempty"           yaml:"dependsOn,omitempty"           text:"DependsOn,omitempty"`
	ValuesFiles []string                 `json:"valuesFiles,omitempty"         yaml:"valuesFiles,omitempty"         text:"Value Files,omitempty"`
	Signature   *PackageSignatureSummary `json:"signature,omitempty"           yaml:"signature,omitempty"           text:"Signature,omitempty"`
}

// PackageSignatureSummary reports package metadata and the verification result
// recorded during bundle creation. Inspect does not perform cryptographic verification.
type PackageSignatureSummary struct {
	Signed       string `json:"signed"       yaml:"signed"       text:"Signed"`
	Verification string `json:"verification" yaml:"verification" text:"Verification Posture"`
}

const (
	// PackageSigningStatusSigned means package metadata marks it signed.
	PackageSigningStatusSigned = "signed"
	// PackageSigningStatusUnsigned means package metadata marks it unsigned.
	PackageSigningStatusUnsigned = "unsigned"
	// PackageSigningStatusUnknown means signing metadata was unavailable.
	PackageSigningStatusUnknown = "unknown"

	// PackageVerificationStatusVerified means a persisted result records successful verification.
	PackageVerificationStatusVerified = "verified"
	// PackageVerificationStatusSkipped means verification was disabled.
	PackageVerificationStatusSkipped = "skipped"
	// PackageVerificationStatusUnknown means no posture was recorded.
	PackageVerificationStatusUnknown = "unknown"
)

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

// PushHooks provides Push process extensibility (CLI-185 / Tech Design Doc).
type PushHooks struct {
	// ToOrasTarget resolves a plain OCI reference into the ORAS target the bytes are
	// copied to. Nil defaults to newRemoteRepository (a live registry repository).
	// The return type is the oras.Target interface rather than a concrete
	// *registry.Repository so consumers can substitute any target — a live registry,
	// an in-memory store, a filesystem layout, or a cross-mounting shim. A real
	// registry repository satisfies oras.Target, so the default path is unaffected.
	// This is also the seam unit tests use to inject an in-memory store.
	ToOrasTarget func(ctx context.Context, ociReference string, opts *PushOptions) (oras.Target, error)
	// ModifyOrasSettings tweaks copy options just before oras.Copy. Nil = no-op.
	// It is not called when a bundle push is already fully published and no copy is required.
	ModifyOrasSettings func(ctx context.Context, copyOptions *oras.CopyOptions) error
}

// PullHooks mirrors PushHooks for the pull path. ToOrasTarget returns a plain
// OCI reference resolved to an ORAS target; oras.Copy only reads from it.
type PullHooks struct {
	ToOrasTarget       func(ctx context.Context, ociReference string, opts *PullOptions) (oras.Target, error)
	ModifyOrasSettings func(ctx context.Context, copyOptions *oras.CopyOptions) error
}

// RemoveOptions contains options for removing an entire bundle.
type RemoveOptions struct {
	// Config is the merged config; always non-nil.
	Config *UDSBundleConfig

	// BundlePath is the path to the bundle definition file (bundle.uds.hcl).
	BundlePath string

	// Bundle is the pre-parsed bundle. When set, Remove() skips parsing BundlePath.
	Bundle *spec.UDSBundle

	// Packages is an optional list of specific package names to remove.
	// When empty, all packages in the bundle are removed.
	Packages []string

	// Force bypasses the removal safety check that blocks removing a package that
	// other bundle packages depend on. Threaded to RemovePackageOptions.Force.
	Force bool

	// Streams carries In/Out/ErrOut for the operation.
	Streams iostreams.IOStreams
}

// RemovePackageOptions contains options for removing one package.
type RemovePackageOptions struct {
	Config *UDSBundleConfig
	Force  bool
}

// RemoveResult represents the output of a bundle remove operation.
type RemoveResult struct {
	BundleName string `json:"bundleName" yaml:"bundleName" text:"Bundle Name"`
	Removed    int    `json:"removed"    yaml:"removed"    text:"Removed"`
	Skipped    int    `json:"skipped"    yaml:"skipped"    text:"Skipped"`
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

	// Config provides the resolved registry configuration for OCI verification
	// and signing.
	Config *UDSBundleConfig

	// Signing controls the signature of the reconfigured output artifact.
	Signing SigningOptions

	// Verification is consumer-owned trust material for the input artifact.
	Verification VerificationPolicy

	// SkipSignatureVerification explicitly permits reconfiguring an unverified input.
	SkipSignatureVerification bool

	// Streams carries In/Out/ErrOut for the operation.
	Streams iostreams.IOStreams
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

	// PushHooks provides extensibility seams for the push path, including the
	// ToOrasTarget seam used to inject an in-memory destination in tests.
	PushHooks PushHooks
}
