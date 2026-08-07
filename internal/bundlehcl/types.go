// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundlehcl

import (
	"context"

	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/hashicorp/hcl/v2"
)

const (
	// BundleFileName is the name of the bundle definition file.
	BundleFileName = "bundle.uds.hcl"

	// BundleDefaultsFileName is the name of the optional bundle-level defaults file.
	// When present alongside bundle.uds.hcl, it is auto-discovered and applied as the
	// lowest-priority variable layer. Only the variables attribute is supported.
	BundleDefaultsFileName = "defaults.uds.hcl"

	// MaxConcurrency is the upper bound for parallel package deploys within a level.
	// Each concurrent deploy pulls an OCI package to disk, creates a temp directory,
	// and runs a Helm install against the cluster. Values above this limit risk
	// exhausting disk, overwhelming the Kubernetes API server, or hitting OCI
	// registry rate limits.
	MaxConcurrency = 25
)

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
// Prompt is controlled by the --prompt flag (see ADR-0005).
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
	ParseBundleFile(ctx context.Context, filePath string) (*spec.UDSBundle, error)
	// ParseBundleBytes parses HCL bundle content from an in-memory byte slice.
	ParseBundleBytes(ctx context.Context, src []byte) (*spec.UDSBundle, error)
	// ParseBundleConfig reads and parses a config.uds.hcl file.
	ParseBundleConfig(ctx context.Context, filePath string) (*UDSBundleConfig, error)
}

// decodedBundle captures the top-level HCL blocks before conversion to the public specification.
type decodedBundle struct {
	UDS      decodedUDSBlock  `hcl:"uds,block"`
	Metadata decodedMetadata  `hcl:"metadata,block"`
	Packages []decodedPackage `hcl:"package,block"`
	Remain   hcl.Body         `hcl:",remain"`
}

// decodedUDSBlock captures bundle constraints from the UDS HCL block.
type decodedUDSBlock struct {
	BundleAPIVersion string `hcl:"bundle_api_version"`
}

// decodedMetadata captures bundle metadata from HCL.
type decodedMetadata struct {
	Name        string `hcl:"name"`
	Description string `hcl:"description,optional"`
	Version     string `hcl:"version,optional"`
}

// decodedPackage captures a package block and its unresolved dependency expressions.
type decodedPackage struct {
	Name                  string `hcl:"name,label"`
	SourceRange           hcl.Range
	Source                string                               `hcl:"source"`
	Namespace             string                               `hcl:"namespace,optional"`
	DependsOn             []decodedPackageRef                  // Populated from Remain after HCL decoding.
	ValuesFiles           []string                             `hcl:"values_files,optional"`
	OptionalComponents    []string                             `hcl:"optional_components,optional"`
	SignatureVerification *decodedPackageSignatureVerification `hcl:"signature_verification,block"`
	Remain                hcl.Body                             `hcl:",remain"`
}

type decodedPackageSignatureVerification struct {
	Verify    *bool                                `hcl:"verify,optional"`
	PublicKey string                               `hcl:"public_key,optional"`
	Keyless   *decodedKeylessSignatureVerification `hcl:"keyless,block"`
}

type decodedKeylessSignatureVerification struct {
	CertificateIdentity         string `hcl:"certificate_identity,optional"`
	CertificateIdentityRegexp   string `hcl:"certificate_identity_regexp,optional"`
	CertificateOIDCIssuer       string `hcl:"certificate_oidc_issuer,optional"`
	CertificateOIDCIssuerRegexp string `hcl:"certificate_oidc_issuer_regexp,optional"`
	TrustedRoot                 string `hcl:"trusted_root,optional"`
	InsecureIgnoreTlog          bool   `hcl:"insecure_ignore_tlog,optional"`
	InsecureIgnoreSCT           bool   `hcl:"insecure_ignore_sct,optional"`
	UseSignedTimestamps         bool   `hcl:"use_signed_timestamps,optional"`
}

// decodedPackageRef retains a package dependency name and its source traversal.
type decodedPackageRef struct {
	Name      string
	Traversal hcl.Traversal
}

// toSpec converts the decoded HCL representation to the canonical bundle model.
func (b *decodedBundle) toSpec() *spec.UDSBundle {
	packages := make([]spec.Package, len(b.Packages))
	for i, pkg := range b.Packages {
		dependsOn := make([]spec.PackageRef, len(pkg.DependsOn))
		for j, ref := range pkg.DependsOn {
			dependsOn[j] = spec.PackageRef{Name: ref.Name}
		}
		packages[i] = spec.Package{
			Name:                  pkg.Name,
			SourceRange:           fromHCLRange(pkg.SourceRange),
			Source:                pkg.Source,
			Namespace:             pkg.Namespace,
			DependsOn:             dependsOn,
			ValuesFiles:           append([]string(nil), pkg.ValuesFiles...),
			OptionalComponents:    append([]string(nil), pkg.OptionalComponents...),
			SignatureVerification: toSpecSignatureVerification(pkg.SignatureVerification),
		}
	}

	return &spec.UDSBundle{
		UDS: spec.UDSBlock{BundleAPIVersion: b.UDS.BundleAPIVersion},
		Metadata: spec.Metadata{
			Name:        b.Metadata.Name,
			Description: b.Metadata.Description,
			Version:     b.Metadata.Version,
		},
		Packages: packages,
	}
}

func toSpecSignatureVerification(verification *decodedPackageSignatureVerification) *spec.PackageSignatureVerification {
	if verification == nil {
		return nil
	}
	result := &spec.PackageSignatureVerification{Verify: verification.Verify, PublicKey: verification.PublicKey}
	if verification.Keyless != nil {
		keyless := verification.Keyless
		result.Keyless = &spec.KeylessSignatureVerification{
			CertificateIdentity: keyless.CertificateIdentity, CertificateIdentityRegexp: keyless.CertificateIdentityRegexp,
			CertificateOIDCIssuer: keyless.CertificateOIDCIssuer, CertificateOIDCIssuerRegexp: keyless.CertificateOIDCIssuerRegexp,
			TrustedRoot: keyless.TrustedRoot, InsecureIgnoreTlog: keyless.InsecureIgnoreTlog,
			InsecureIgnoreSCT: keyless.InsecureIgnoreSCT, UseSignedTimestamps: keyless.UseSignedTimestamps,
		}
	}
	return result
}

// HCLParser implements Parser for parsing HCL bundle files.
// The arch field is exposed as ${sys.arch} during bundle HCL evaluation; an
// empty value falls back to runtime.GOARCH (see sysVars).
type HCLParser struct {
	arch    string
	streams iostreams.IOStreams
}

// PackageTraversal wraps a package with an hcl.Traversal for structured
// reference handling. The bundle HCL syntax uses expression-based dependencies
// (depends_on = [package.core_base]), which are parsed into hcl.Traversal values
// for type safety and source location tracking.
// Inspired by OpenTofu's dependency handling:
// https://github.com/opentofu/opentofu/blob/d088667ed7e5bd611ce8d922add3f5ec313beae1/internal/configs/depends_on.go#L13
type PackageTraversal struct {
	Package   *spec.Package
	Traversal hcl.Traversal
}

// DAG represents a directed acyclic graph of package dependencies using hcl.Traversal.
// Edges point from a package to its dependencies (i.e., the things it depends on).
type DAG struct {
	packages map[string]*PackageTraversal
	edges    map[string][]hcl.Traversal // package name -> dependency traversals
}
