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

// ParseBundleConfig reads and parses a config.uds.hcl file.
// It uses gohcl.DecodeBody to decode the options block via HCL struct tags on
// UDSBundleConfig, and hcl:",remain" to capture the free-form variables attribute
// which is then manually extracted and converted from cty.Value to map[string]any.
// The context parameter is currently unused as none of the HCL parsing methods supports cancellation.
func (p *HCLParser) ParseBundleConfig(_ context.Context, filePath string) (*UDSBundleConfig, error) {
	src, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("cannot read config file: %w", err)
	}

	hclFile, hclDiagnostics := hclsyntax.ParseConfig(src, filePath, hcl.Pos{Line: 1, Column: 1})
	if hclDiagnostics.HasErrors() {
		return nil, fmt.Errorf("failed to parse config HCL: %s", hclDiagnostics.Error())
	}

	// Decode structured fields (options block) via gohcl; free-form content lands in Remain
	cfg := &UDSBundleConfig{}
	diags := gohcl.DecodeBody(hclFile.Body, nil, cfg)
	if diags.HasErrors() {
		return nil, fmt.Errorf("failed to decode config: %s", diags.Error())
	}

	// Extract the free-form variables attribute from Remain
	if cfg.Remain != nil {
		vars, err := extractVariablesFromRemain(cfg.Remain)
		if err != nil {
			return nil, err
		}
		cfg.Variables = vars
	}

	return cfg, nil
}

// extractVariablesFromRemain extracts the optional "variables" attribute from the
// remaining HCL body. Variables are free-form (arbitrary nesting of scalars and objects),
// so they can't be decoded via struct tags and must be manually converted from cty.Value.
func extractVariablesFromRemain(body hcl.Body) (Variables, error) {
	schema := &hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{
			{Name: "variables", Required: false},
		},
	}

	content, _, diags := body.PartialContent(schema)
	if diags.HasErrors() {
		return nil, fmt.Errorf("failed to read variables: %s", diags.Error())
	}

	attr, ok := content.Attributes["variables"]
	if !ok {
		return nil, nil
	}

	val, diags := attr.Expr.Value(nil)
	if diags.HasErrors() {
		return nil, fmt.Errorf("failed to evaluate variables: %s", diags.Error())
	}

	goVal, err := ctyValueToGo(val)
	if err != nil {
		return nil, fmt.Errorf("failed to convert variables: %w", err)
	}

	m, ok := goVal.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("variables must be an object, got %T", goVal)
	}

	return Variables(m), nil
}

// ctyValueToGo recursively converts a cty.Value to a Go value.
// Supported cty types: String → string, Number → float64, Bool → bool,
// Object → map[string]any (recursive). Null and unknown values return an error.
func ctyValueToGo(val cty.Value) (any, error) {
	if val.IsNull() {
		return nil, fmt.Errorf("null values are not supported in config")
	}
	if !val.IsKnown() {
		return nil, fmt.Errorf("unknown values are not supported in config")
	}

	ty := val.Type()
	switch {
	case ty == cty.String:
		return val.AsString(), nil
	case ty == cty.Number:
		f, _ := val.AsBigFloat().Float64()
		return f, nil
	case ty == cty.Bool:
		return val.True(), nil
	case ty.IsObjectType():
		m := make(map[string]any)
		for k := range ty.AttributeTypes() {
			child, err := ctyValueToGo(val.GetAttr(k))
			if err != nil {
				return nil, fmt.Errorf("variable %q: %w", k, err)
			}
			m[k] = child
		}
		return m, nil
	default:
		return nil, fmt.Errorf("unsupported variable type: %s", ty.FriendlyName())
	}
}

// ParseDefaults reads a defaults file from disk and validates it.
// A valid defaults file contains at most one top-level attribute named "variables"
// and no blocks. Returns the parsed Variables, or nil if the file has no variables.
// The context parameter is currently unused as none of the HCL parsing methods supports cancellation.
func ParseDefaults(_ context.Context, path string) (Variables, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read defaults file: %w", err)
	}

	hclFile, diags := hclsyntax.ParseConfig(src, path, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return nil, fmt.Errorf("failed to parse defaults HCL: %s", diags.Error())
	}

	// JustAttributes rejects any blocks (e.g. options {}).
	attrs, diags := hclFile.Body.JustAttributes()
	if diags.HasErrors() {
		return nil, fmt.Errorf("defaults file must not contain blocks: %s", diags.Error())
	}

	// Only "variables" is allowed at the top level.
	for name := range attrs {
		if name != "variables" {
			return nil, fmt.Errorf("defaults file contains unexpected attribute %q; only \"variables\" is allowed", name)
		}
	}

	attr, ok := attrs["variables"]
	if !ok {
		return nil, nil
	}

	val, diags := attr.Expr.Value(nil)
	if diags.HasErrors() {
		return nil, fmt.Errorf("failed to evaluate variables: %s", diags.Error())
	}

	goVal, err := ctyValueToGo(val)
	if err != nil {
		return nil, fmt.Errorf("failed to convert variables: %w", err)
	}

	m, ok := goVal.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("variables must be an object, got %T", goVal)
	}

	return Variables(m), nil
}

// ParseBundleFile reads and parses an HCL bundle file with locals support.
// The context parameter is currently unused as none of the HCL parsing methods supports cancellation.
func (p *HCLParser) ParseBundleFile(_ context.Context, filePath string) (*UDSBundle, error) {
	slog.Debug("reading bundle file", "path", filePath)
	src, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("cannot read bundle file: %w", err)
	}
	return parseBundleContent(src, filePath)
}

// ParseBundleBytes parses HCL bundle content from an in-memory byte slice.
// The context parameter is currently unused as none of the HCL parsing methods supports cancellation.
func (p *HCLParser) ParseBundleBytes(_ context.Context, src []byte) (*UDSBundle, error) {
	return parseBundleContent(src, "bundle.uds.hcl")
}

// parseBundleContent parses HCL content using a two-pass approach: first extracting
// and evaluating locals, then decoding the full bundle with an EvalContext containing
// those locals. filename is used only for error message attribution.
func parseBundleContent(src []byte, filename string) (*UDSBundle, error) {
	hclFile, hclDiagnostics := hclsyntax.ParseConfig(src, filename, hcl.Pos{Line: 1, Column: 1})
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
