// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundlehcl

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

	packageBlocks, _, diags := hclFile.Body.PartialContent(&hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{
			{Type: "package", LabelNames: []string{"name"}},
		},
	})
	if diags.HasErrors() {
		return nil, fmt.Errorf("failed to decode package metadata: %s", diags.Error())
	}
	for i, block := range packageBlocks.Blocks {
		if i >= len(decoded.Packages) {
			break
		}
		decoded.Packages[i].SourceRange = block.DefRange
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
