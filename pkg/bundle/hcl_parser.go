// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"slices"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

// HCLParser implements Parser for parsing HCL bundle files.
type HCLParser struct{}

// NewHCLParser creates a new HCLParser instance.
func NewHCLParser() *HCLParser {
	return &HCLParser{}
}

// Compile-time check to ensure HCLParser implements Parser.
var _ Parser = &HCLParser{}

// ParseBundleFile reads and parses an HCL bundle file with locals support.
// It uses a two-pass approach: first extracting and evaluating locals,
// then decoding the full bundle with an EvalContext containing those locals.
// The caller is responsible for validating the returned bundle.
// The context parameter is currently unused as none of the HCL parsing methods supports cancellation.
func (p *HCLParser) ParseBundleFile(_ context.Context, filePath string) (*UDSBundle, error) {
	slog.Debug("reading bundle file", "path", filePath)
	src, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("cannot read bundle file: %w", err)
	}

	hclFile, hclDiagnostics := hclsyntax.ParseConfig(src, filePath, hcl.Pos{Line: 1, Column: 1})
	if hclDiagnostics.HasErrors() {
		return nil, fmt.Errorf("failed to parse HCL: %s", hclDiagnostics.Error())
	}
	slog.Debug("HCL syntax parsed successfully")

	locals, err := extractLocals(hclFile)
	if err != nil {
		return nil, fmt.Errorf("failed to extract locals: %w", err)
	}
	slog.Debug("locals extracted", "count", len(locals))

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
		// This way we'll be reading locals in the same order as they are defined in the HCL file.
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
// It uses gohcl for standard fields and post-processes the Package.Remain
// field to extract depends_on expressions into []PackageRef.
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

	// Decode the entire bundle using gohcl - depends_on is captured in Package.Remain
	var bundle UDSBundle
	diags := gohcl.DecodeBody(hclFile.Body, ctx, &bundle)
	if diags.HasErrors() {
		return nil, fmt.Errorf("failed to decode bundle: %s", diags.Error())
	}

	// Post-process each package to extract depends_on from Remain
	for i := range bundle.Packages {
		pkg := &bundle.Packages[i]
		if pkg.Remain == nil {
			continue
		}

		refs, err := decodePackageDependsOn(pkg.Remain)
		if err != nil {
			return nil, fmt.Errorf("failed to decode depends_on for package %q: %w", pkg.Name, err)
		}
		pkg.DependsOn = refs
	}

	return &bundle, nil
}

// decodePackageDependsOn extracts depends_on from a package's Remain body as HCL traversals.
// The syntax is: depends_on = [package.core_base, package.core_logging]
// Each element must be a static traversal referencing "package.<name>".
func decodePackageDependsOn(body hcl.Body) ([]PackageRef, error) {
	attrSchema := &hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{
			{Name: "depends_on"},
		},
	}

	content, _, diags := body.PartialContent(attrSchema)
	if diags.HasErrors() {
		return nil, fmt.Errorf("failed to read depends_on: %s", diags.Error())
	}

	attr, exists := content.Attributes["depends_on"]
	if !exists {
		return nil, nil // depends_on is optional
	}

	// Get the list of expressions
	exprs, diags := hcl.ExprList(attr.Expr)
	if diags.HasErrors() {
		return nil, fmt.Errorf("depends_on must be a list of package references: %s", diags.Error())
	}

	var refs []PackageRef
	for _, expr := range exprs {
		// Each element must be a static traversal (e.g., package.core_base)
		traversal, diags := hcl.AbsTraversalForExpr(expr)
		if diags.HasErrors() {
			return nil, fmt.Errorf("depends_on element must be a package reference (e.g., package.core_base): %s", diags.Error())
		}

		// Validate the traversal structure: must be "package.<name>"
		if len(traversal) != 2 {
			return nil, fmt.Errorf("invalid package reference at %s: expected package.<name>", expr.Range())
		}

		root, ok := traversal[0].(hcl.TraverseRoot)
		if !ok || root.Name != "package" {
			return nil, fmt.Errorf("invalid package reference at %s: must start with 'package'", expr.Range())
		}

		attrStep, ok := traversal[1].(hcl.TraverseAttr)
		if !ok {
			return nil, fmt.Errorf("invalid package reference at %s: expected package.<name>", expr.Range())
		}

		refs = append(refs, PackageRef{
			Name:      attrStep.Name,
			Traversal: traversal,
		})
	}

	return refs, nil
}
