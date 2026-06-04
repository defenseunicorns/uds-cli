// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"os"
	"runtime"
	"slices"

	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

// sysVars returns the cty.Value for the built-in "sys" namespace injected into
// every bundle.uds.hcl eval context. Currently exposes sys.arch, which reflects
// the bundle's effective target architecture (CLI/HCL/defaults override the
// runtime default; see config_resolver). An empty arch falls back to runtime.GOARCH.
func sysVars(arch string) cty.Value {
	if arch == "" {
		arch = runtime.GOARCH
	}
	return cty.ObjectVal(map[string]cty.Value{
		"arch": cty.StringVal(arch),
	})
}

// HCLParser implements Parser for parsing HCL bundle files.
// The arch field is exposed as ${sys.arch} during bundle HCL evaluation; an
// empty value falls back to runtime.GOARCH (see sysVars).
type HCLParser struct {
	arch    string
	streams iostreams.IOStreams
}

// NewHCLParser creates a new HCLParser. arch is the effective target
// architecture exposed as ${sys.arch}; pass an empty string to use runtime.GOARCH.
// streams carries the leveled logger used for parse diagnostics.
func NewHCLParser(arch string, streams iostreams.IOStreams) *HCLParser {
	return &HCLParser{arch: arch, streams: streams}
}

// Compile-time check to ensure HCLParser implements Parser.
var _ Parser = &HCLParser{}

// ParseBundleConfig reads and parses a config.uds.hcl file.
// It uses gohcl.DecodeBody to decode the options block via HCL struct tags on
// UDSBundleConfig, and hcl:",remain" to capture the free-form variables attribute
// which is then manually extracted and converted from cty.Value to Variables.
// The context parameter is currently unused as none of the HCL parsing methods supports cancellation.
func (p *HCLParser) ParseBundleConfig(_ context.Context, filePath string) (*UDSBundleConfig, error) {
	if filePath == "" {
		return nil, errEmpty("filePath")
	}
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

	m, ok := goVal.(Variables)
	if !ok {
		return nil, fmt.Errorf("variables must be an object, got %T", goVal)
	}

	return m, nil
}

// ctyValueToGo recursively converts a cty.Value to a Go value.
// Supported cty types: String → string, Number → float64, Bool → bool,
// Object → Variables (recursive), Tuple/List/Set → []any (recursive).
// Null and unknown values return an error at any depth. Set iteration order
// is stable per cty value but not user-controlled; prefer lists when order matters.
//
// cty.Map is intentionally not supported. HCL literal `{a=1,b=2}` always
// produces a cty.Object, never a cty.Map; reaching the Map branch would
// require functions like tomap() or typed inputs, neither of which our
// parser enables (variables are evaluated with a nil EvalContext). A Map
// surfacing here is therefore a contract violation, and we let the loud-fail
// default branch report it as an unsupported variable type.
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
		// Iterate attributes in sorted order so error messages on multi-attribute
		// failures are deterministic (Go map iteration order is randomized).
		out := make(Variables)
		names := slices.Sorted(maps.Keys(ty.AttributeTypes()))
		for _, k := range names {
			child, err := ctyValueToGo(val.GetAttr(k))
			if err != nil {
				return nil, fmt.Errorf("%q: %w", k, err)
			}
			out[k] = child
		}
		return out, nil
	case ty.IsTupleType(), ty.IsListType(), ty.IsSetType():
		out := make([]any, 0, val.LengthInt())
		it := val.ElementIterator()
		i := 0
		for it.Next() {
			_, elem := it.Element()
			child, err := ctyValueToGo(elem)
			if err != nil {
				return nil, fmt.Errorf("[%d]: %w", i, err)
			}
			out = append(out, child)
			i++
		}
		return out, nil
	default:
		// Loud-fail backstop — silent drops must not occur.
		return nil, fmt.Errorf("unsupported variable type: %s", ty.FriendlyName())
	}
}

// ParseDefaults reads a defaults file from disk and validates it.
// A valid defaults file contains at most one top-level attribute named "variables"
// and no blocks. Returns the parsed Variables, or nil if the file has no variables.
// The context parameter is currently unused as none of the HCL parsing methods supports cancellation.
func ParseDefaults(_ context.Context, path string) (Variables, error) {
	if path == "" {
		return nil, errEmpty("path")
	}
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

	m, ok := goVal.(Variables)
	if !ok {
		return nil, fmt.Errorf("variables must be an object, got %T", goVal)
	}

	return m, nil
}

// ParseBundleFile reads and parses an HCL bundle file with locals support.
// ctx is accepted for cancellation/propagation; HCL parsing does not use it, and
// diagnostics are written via p.streams.
func (p *HCLParser) ParseBundleFile(ctx context.Context, filePath string) (*UDSBundle, error) {
	if filePath == "" {
		return nil, errEmpty("filePath")
	}
	p.streams.Debug("reading bundle file", "path", filePath)
	src, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("cannot read bundle file: %w", err)
	}
	return p.parseBundleContent(ctx, src, filePath)
}

// ParseBundleBytes parses HCL bundle content from an in-memory byte slice.
// ctx is accepted for cancellation/propagation; HCL parsing does not use it, and
// diagnostics are written via p.streams.
func (p *HCLParser) ParseBundleBytes(ctx context.Context, src []byte) (*UDSBundle, error) {
	if len(src) == 0 {
		return nil, errEmpty("src")
	}
	return p.parseBundleContent(ctx, src, "bundle.uds.hcl")
}

// parseBundleContent parses HCL content using a two-pass approach: first extracting
// and evaluating locals, then decoding the full bundle with an EvalContext containing
// those locals. filename is used only for error message attribution.
func (p *HCLParser) parseBundleContent(ctx context.Context, src []byte, filename string) (*UDSBundle, error) {
	hclFile, hclDiagnostics := hclsyntax.ParseConfig(src, filename, hcl.Pos{Line: 1, Column: 1})
	if hclDiagnostics.HasErrors() {
		return nil, fmt.Errorf("failed to parse HCL: %s", hclDiagnostics.Error())
	}
	p.streams.Debug("HCL syntax parsed successfully")

	locals, err := p.extractLocals(hclFile)
	if err != nil {
		return nil, fmt.Errorf("failed to extract locals: %w", err)
	}
	p.streams.Debug("locals extracted", "count", len(locals))

	return p.decodeBundleWithLocals(hclFile, locals)
}

// extractLocals extracts and evaluates locals from the given HCL file.
// Nested objects (e.g., pkgs = { base = "core-base" }) are handled natively
// by go-cty, producing cty.ObjectVal values that support traversal like
// ${local.pkgs.base}.
func (p *HCLParser) extractLocals(hclFile *hcl.File) (map[string]cty.Value, error) {
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
					"sys":   sysVars(p.arch),
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
func (p *HCLParser) decodeBundleWithLocals(hclFile *hcl.File, locals map[string]cty.Value) (*UDSBundle, error) {
	localVal := cty.EmptyObjectVal
	if len(locals) > 0 {
		localVal = cty.ObjectVal(locals)
	}

	ctx := &hcl.EvalContext{
		Variables: map[string]cty.Value{
			"local": localVal,
			"sys":   sysVars(p.arch),
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
