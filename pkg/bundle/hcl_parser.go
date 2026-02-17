// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"os"
	"slices"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

// HCLParser implements BundleParser for parsing HCL bundle files.
type HCLParser struct{}

// NewHCLParser creates a new HCLParser instance.
func NewHCLParser() *HCLParser {
	return &HCLParser{}
}

// Compile-time check to ensure HCLParser implements BundleParser.
var _ BundleParser = &HCLParser{}

// ParseBundleFile reads and parses an HCL bundle file with locals support.
// It uses a two-pass approach: first extracting and evaluating locals,
// then decoding the full bundle with an EvalContext containing those locals.
// The caller is responsible for validating the returned bundle.
// The context parameter is currently unused but provided for future extensibility.
func (p *HCLParser) ParseBundleFile(_ context.Context, filePath string) (*UDSBundle, error) {
	src, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("cannot read bundle file: %w", err)
	}

	hclFile, hclDiagnostics := hclsyntax.ParseConfig(src, filePath, hcl.Pos{Line: 1, Column: 1})
	if hclDiagnostics.HasErrors() {
		return nil, fmt.Errorf("failed to parse HCL: %s", hclDiagnostics.Error())
	}

	locals, err := extractLocals(hclFile)
	if err != nil {
		return nil, fmt.Errorf("failed to extract locals: %w", err)
	}

	return decodeBundleWithLocals(hclFile, locals)
}

// extractLocals extracts and evaluates locals from the given HCL file.
// Nested objects (e.g., pkgs = { base = "core-base" }) are handled natively
// by go-cty, producing cty.ObjectVal values that support traversal like
// ${local.pkgs.base}.
func extractLocals(hclFile *hcl.File) (map[string]cty.Value, error) {
	schema := &hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{
			{Type: "locals"},
		},
	}

	content, _, diags := hclFile.Body.PartialContent(schema)
	if diags.HasErrors() {
		return nil, fmt.Errorf("failed to read locals block: %s", diags.Error())
	}

	localsMap := make(map[string]cty.Value)

	for _, block := range content.Blocks {
		if block.Type != "locals" {
			continue
		}

		attrs, diags := block.Body.JustAttributes()
		if diags.HasErrors() {
			return nil, fmt.Errorf("failed to read locals attributes: %s", diags.Error())
		}

		// Since the JustAttributes returns a map and the iteration order of maps is not
		// guaranteed, we need to sort the attributes based on their source position.
		names := slices.Collect(maps.Keys(attrs))
		slices.SortFunc(names, func(a, b string) int {
			posA := attrs[a].Expr.Range().Start
			posB := attrs[b].Expr.Range().Start
			return cmp.Or(cmp.Compare(posA.Line, posB.Line), cmp.Compare(posA.Column, posB.Column), cmp.Compare(a, b))
		})

		for _, name := range names {
			attr := attrs[name]
			localVal := cty.EmptyObjectVal
			if len(localsMap) > 0 {
				localVal = cty.ObjectVal(maps.Clone(localsMap))
			}
			ctx := &hcl.EvalContext{
				Variables: map[string]cty.Value{
					"local": localVal,
				},
			}
			val, diags := attr.Expr.Value(ctx)
			if diags.HasErrors() {
				return nil, fmt.Errorf("failed to evaluate local %q: %s", name, diags.Error())
			}
			localsMap[name] = val
		}
	}

	return localsMap, nil
}

// decodeBundleWithLocals decodes the given HCL file into a UDSBundle struct
// using an EvalContext populated with the extracted locals.
func decodeBundleWithLocals(hclFile *hcl.File, locals map[string]cty.Value) (*UDSBundle, error) {
	localVal := cty.EmptyObjectVal
	if len(locals) > 0 {
		localVal = cty.ObjectVal(locals)
	}

	ctx := &hcl.EvalContext{
		Variables: map[string]cty.Value{
			"local": localVal,
		},
	}

	var bundle UDSBundle
	diags := gohcl.DecodeBody(hclFile.Body, ctx, &bundle)
	if diags.HasErrors() {
		return nil, fmt.Errorf("failed to decode bundle: %s", diags.Error())
	}

	return &bundle, nil
}
