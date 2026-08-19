// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
)

// BundleFileName is the name of the bundle definition file.
const BundleFileName = "bundle.uds.hcl"

type decodedBundle struct {
	UDS      decodedUDSBlock  `hcl:"uds,block"`
	Metadata decodedMetadata  `hcl:"metadata,block"`
	Packages []decodedPackage `hcl:"package,block"`
	Remain   hcl.Body         `hcl:",remain"`
}
type decodedUDSBlock struct {
	BundleAPIVersion string `hcl:"bundle_api_version"`
}
type decodedMetadata struct {
	Name        string `hcl:"name"`
	Description string `hcl:"description,optional"`
	Version     string `hcl:"version,optional"`
}
type decodedPackage struct {
	Name                  string `hcl:"name,label"`
	Source                string `hcl:"source"`
	Namespace             string `hcl:"namespace,optional"`
	DependsOn             []decodedPackageRef
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
type decodedPackageRef struct {
	Name      string
	Traversal hcl.Traversal
}

// HCLParser implements Parser for HCL bundle definitions. Its architecture is
// exposed to bundle expressions as ${sys.arch}; an empty architecture uses the
// runtime default. Diagnostics are written to streams.
type HCLParser struct {
	arch    string
	streams iostreams.IOStreams
}

// Parser defines the interface for parsing bundle definitions and
// configuration files.
type Parser interface {
	// ParseBundleFile reads and parses a bundle.uds.hcl file with locals support.
	ParseBundleFile(ctx context.Context, filePath string) (*spec.UDSBundle, error)
	// ParseBundleBytes parses in-memory bundle HCL without permitting file().
	ParseBundleBytes(ctx context.Context, src []byte) (*spec.UDSBundle, error)
	// ParseBundleConfig reads and parses a config.uds.hcl file.
	ParseBundleConfig(ctx context.Context, filePath string) (*UDSBundleConfig, error)
}

func (b *decodedBundle) toSpec() *spec.UDSBundle {
	packages := make([]spec.Package, len(b.Packages))
	for i, pkg := range b.Packages {
		dependsOn := make([]spec.PackageRef, len(pkg.DependsOn))
		for j, ref := range pkg.DependsOn {
			dependsOn[j] = spec.PackageRef{Name: ref.Name}
		}
		packages[i] = spec.Package{Name: pkg.Name, Source: pkg.Source, Namespace: pkg.Namespace, DependsOn: dependsOn, ValuesFiles: append([]string(nil), pkg.ValuesFiles...), OptionalComponents: append([]string(nil), pkg.OptionalComponents...), SignatureVerification: toSpecSignatureVerification(pkg.SignatureVerification)}
	}
	return &spec.UDSBundle{UDS: spec.UDSBlock{BundleAPIVersion: b.UDS.BundleAPIVersion}, Metadata: spec.Metadata{Name: b.Metadata.Name, Description: b.Metadata.Description, Version: b.Metadata.Version}, Packages: packages}
}

func toSpecSignatureVerification(verification *decodedPackageSignatureVerification) *spec.PackageSignatureVerification {
	if verification == nil {
		return nil
	}
	result := &spec.PackageSignatureVerification{Verify: verification.Verify, PublicKey: verification.PublicKey}
	if verification.Keyless != nil {
		k := verification.Keyless
		result.Keyless = &spec.KeylessSignatureVerification{CertificateIdentity: k.CertificateIdentity, CertificateIdentityRegexp: k.CertificateIdentityRegexp, CertificateOIDCIssuer: k.CertificateOIDCIssuer, CertificateOIDCIssuerRegexp: k.CertificateOIDCIssuerRegexp, TrustedRoot: k.TrustedRoot, InsecureIgnoreTlog: k.InsecureIgnoreTlog, InsecureIgnoreSCT: k.InsecureIgnoreSCT, UseSignedTimestamps: k.UseSignedTimestamps}
	}
	return result
}

// NewHCLParser creates a new HCLParser. arch is the effective target
// architecture exposed as ${sys.arch}; pass an empty string to use runtime.GOARCH.
// streams carries the leveled logger used for parse diagnostics.
func NewHCLParser(arch string, streams iostreams.IOStreams) *HCLParser {
	return &HCLParser{arch: arch, streams: streams}
}

// Compile-time check to ensure HCLParser implements Parser.
var _ Parser = &HCLParser{}

// ParseBundleFile reads and parses an HCL bundle file with locals support.
// ctx is accepted for cancellation/propagation; HCL parsing does not use it, and
// diagnostics are written via p.streams.
func (p *HCLParser) ParseBundleFile(ctx context.Context, filePath string) (*spec.UDSBundle, error) {
	if filePath == "" {
		return nil, errEmpty("filePath")
	}
	p.streams.Debug("reading bundle file", "path", filePath)
	src, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("cannot read bundle file: %w", err)
	}
	return p.parseBundleContent(ctx, src, filePath, filepath.Dir(filePath), true)
}

// ParseBundleBytes parses HCL bundle content from an in-memory byte slice.
// ctx is accepted for cancellation/propagation; HCL parsing does not use it, and
// diagnostics are written via p.streams.
func (p *HCLParser) ParseBundleBytes(ctx context.Context, src []byte) (*spec.UDSBundle, error) {
	if len(src) == 0 {
		return nil, errEmpty("src")
	}
	return p.parseBundleContent(ctx, src, "bundle.uds.hcl", "", false)
}

// ParseAndMaterializeBundleFile reads a source bundle once, using those bytes
// both for runtime evaluation and the self-contained artifact representation.
func (p *HCLParser) ParseAndMaterializeBundleFile(ctx context.Context, path string) (*spec.UDSBundle, []byte, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot read bundle file: %w", err)
	}
	bundle, err := p.parseBundleContent(ctx, src, path, filepath.Dir(path), true)
	if err != nil {
		return nil, nil, err
	}
	materialized, err := p.materializeBundleFileCalls(src, path, filepath.Dir(path))
	if err != nil {
		return nil, nil, err
	}
	return bundle, materialized, nil
}

// parseBundleContent parses HCL content using a two-pass approach: first extracting
// and evaluating locals, then decoding the full bundle with an EvalContext containing
// those locals. filename is used only for error message attribution.
func (p *HCLParser) parseBundleContent(ctx context.Context, src []byte, filename, baseDir string, allowFile bool) (*spec.UDSBundle, error) {
	hclFile, hclDiagnostics := hclsyntax.ParseConfig(src, filename, hcl.Pos{Line: 1, Column: 1})
	if hclDiagnostics.HasErrors() {
		return nil, fmt.Errorf("failed to parse HCL: %s", hclDiagnostics.Error())
	}
	if !allowFile {
		if err := rejectFileFunctionWithoutSourcePath(hclFile, "bundle.uds.hcl"); err != nil {
			return nil, err
		}
	}
	p.streams.Debug("HCL syntax parsed successfully")

	funcs := map[string]function.Function(nil)
	if allowFile {
		funcs = map[string]function.Function{"file": newFileFunction(baseDir)}
	}
	locals, _, err := p.extractLocals(hclFile, funcs)
	if err != nil {
		return nil, err
	}
	p.streams.Debug("locals extracted", "count", len(locals))

	return p.decodeBundleWithLocals(hclFile, locals, funcs)
}

// decodeBundleWithLocals decodes the given HCL file into a UDSBundle struct
// using an EvalContext populated with the extracted locals.
// It uses gohcl for standard fields and post-processes the Package.Remain
// field to extract depends_on expressions into []PackageRef.
func (p *HCLParser) decodeBundleWithLocals(hclFile *hcl.File, locals map[string]cty.Value, funcs map[string]function.Function) (*spec.UDSBundle, error) {
	localVal := cty.EmptyObjectVal
	if len(locals) > 0 {
		localVal = cty.ObjectVal(locals)
	}

	ctx := &hcl.EvalContext{
		Variables: map[string]cty.Value{
			"local": localVal,
			"sys":   sysVars(p.arch),
		},
		Functions: funcs,
	}

	// Decode the entire bundle using gohcl - depends_on is captured in Package.Remain
	var decoded decodedBundle
	diags := gohcl.DecodeBody(hclFile.Body, ctx, &decoded)
	if diags.HasErrors() {
		return nil, fmt.Errorf("failed to decode bundle: %s", diags.Error())
	}

	// Post-process each package to extract depends_on from Remain
	for i := range decoded.Packages {
		pkg := &decoded.Packages[i]
		if pkg.Remain == nil {
			continue
		}

		refs, err := decodePackageDependsOn(pkg.Remain)
		if err != nil {
			return nil, fmt.Errorf("failed to decode depends_on for package %q: %w", pkg.Name, err)
		}
		pkg.DependsOn = refs
	}

	return decoded.toSpec(), nil
}
