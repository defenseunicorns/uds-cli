// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"unicode/utf8"

	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
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

// configEvalContext returns the functions available to a file-backed HCL document.
func configEvalContext(filePath string) *hcl.EvalContext {
	return &hcl.EvalContext{
		Functions: map[string]function.Function{
			"file": newFileFunction(filepath.Dir(filePath)),
		},
	}
}

// newFileFunction returns the file() implementation for a file-backed HCL document.
func newFileFunction(baseDir string) function.Function {
	return function.New(&function.Spec{
		Params: []function.Parameter{
			{
				Name: "path",
				Type: cty.String,
			},
		},
		Type: function.StaticReturnType(cty.String),
		Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
			return readFileValue(baseDir, args[0].AsString())
		},
	})
}

func readFileValue(baseDir, path string) (cty.Value, error) {
	if !filepath.IsAbs(path) {
		path = filepath.Join(baseDir, path)
	}
	info, err := os.Stat(path)
	if err != nil {
		return cty.NilVal, fmt.Errorf("stat file %q: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return cty.NilVal, fmt.Errorf("file %q is not a regular file", path)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return cty.NilVal, fmt.Errorf("read file %q: %w", path, err)
	}
	if !utf8.Valid(contents) {
		return cty.NilVal, fmt.Errorf("file %q is not valid UTF-8", path)
	}
	return cty.StringVal(string(contents)), nil
}

// rejectFileFunctionWithoutSourcePath rejects file() for in-memory HCL. Unlike
// ParseBundleFile, ParseBundleBytes has no stable directory for relative paths.
func rejectFileFunctionWithoutSourcePath(hclFile *hcl.File, fileKind string) error {
	body, ok := hclFile.Body.(*hclsyntax.Body)
	if !ok {
		return fmt.Errorf("unexpected HCL body type")
	}

	var callRange *hcl.Range
	_ = hclsyntax.VisitAll(body, func(node hclsyntax.Node) hcl.Diagnostics {
		call, ok := node.(*hclsyntax.FunctionCallExpr)
		if ok && call.Name == "file" && callRange == nil {
			rangeCopy := call.Range()
			callRange = &rangeCopy
		}
		return nil
	})
	if callRange == nil {
		return nil
	}

	return fmt.Errorf("file() requires a file-backed bundle source; %s cannot use file() at %s", fileKind, callRange)
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
	evalContext := configEvalContext(filePath)

	// Decode structured fields (options block) via gohcl; free-form content lands in Remain
	cfg := &UDSBundleConfig{}
	diags := gohcl.DecodeBody(hclFile.Body, evalContext, cfg)
	if diags.HasErrors() {
		return nil, fmt.Errorf("failed to decode config: %s", diags.Error())
	}

	// Extract the free-form variables attribute from Remain
	if cfg.Remain != nil {
		vars, err := extractVariablesFromRemain(cfg.Remain, evalContext)
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
func extractVariablesFromRemain(body hcl.Body, evalContext *hcl.EvalContext) (Variables, error) {
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

	val, diags := attr.Expr.Value(evalContext)
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
// parser enables. A Map
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
	return parseDefaultsContent(src, path)
}

func parseDefaultsContent(src []byte, path string) (Variables, error) {
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

	val, diags := attr.Expr.Value(configEvalContext(path))
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
	return p.parseBundleContent(ctx, src, filePath, filepath.Dir(filePath), true)
}

// ParseBundleBytes parses HCL bundle content from an in-memory byte slice.
// ctx is accepted for cancellation/propagation; HCL parsing does not use it, and
// diagnostics are written via p.streams.
func (p *HCLParser) ParseBundleBytes(ctx context.Context, src []byte) (*UDSBundle, error) {
	if len(src) == 0 {
		return nil, errEmpty("src")
	}
	return p.parseBundleContent(ctx, src, "bundle.uds.hcl", "", false)
}

// parseAndMaterializeBundleFile reads a source bundle once, using those bytes
// both for runtime evaluation and the self-contained artifact representation.
func (p *HCLParser) parseAndMaterializeBundleFile(ctx context.Context, path string) (*UDSBundle, []byte, error) {
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

func materializeDefaultsFile(path string) ([]byte, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read defaults file: %w", err)
	}
	if _, err := parseDefaultsContent(src, path); err != nil {
		return nil, err
	}
	return materializeFileCallExpressions(src, path, configEvalContext(path), nil)
}

func (p *HCLParser) materializeBundleFileCalls(src []byte, filename, baseDir string) ([]byte, error) {
	funcs := map[string]function.Function{"file": newFileFunction(baseDir)}
	hclFile, diags := hclsyntax.ParseConfig(src, filename, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return nil, fmt.Errorf("failed to parse HCL: %s", diags.Error())
	}
	locals, localEvalContexts, err := p.extractLocals(hclFile, funcs)
	if err != nil {
		return nil, fmt.Errorf("failed to extract locals: %w", err)
	}
	localVal := cty.EmptyObjectVal
	if len(locals) > 0 {
		localVal = cty.ObjectVal(locals)
	}
	return materializeFileCallExpressions(src, filename, &hcl.EvalContext{Variables: map[string]cty.Value{"local": localVal, "sys": sysVars(p.arch)}, Functions: funcs}, localEvalContexts)
}

func materializeFileCallExpressions(src []byte, filename string, evalCtx *hcl.EvalContext, localEvalContexts map[int]*hcl.EvalContext) ([]byte, error) {
	hclFile, diags := hclsyntax.ParseConfig(src, filename, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return nil, fmt.Errorf("failed to parse HCL: %s", diags.Error())
	}
	body, ok := hclFile.Body.(*hclsyntax.Body)
	if !ok {
		return nil, fmt.Errorf("unexpected HCL body type")
	}
	expressions := fileCallContainingExpressions(body)
	type replacement struct {
		start, end int
		value      []byte
	}
	replacements := make([]replacement, 0, len(expressions))
	for _, expr := range expressions {
		r := expr.Range()
		exprEvalCtx := evalCtx
		if localEvalCtx, ok := localEvalContexts[r.Start.Byte]; ok {
			exprEvalCtx = localEvalCtx
		}
		value, diags := expr.Value(exprEvalCtx)
		if diags.HasErrors() {
			return nil, fmt.Errorf("failed to evaluate HCL expression containing file(): %s", diags.Error())
		}
		replacements = append(replacements, replacement{r.Start.Byte, r.End.Byte, hclwrite.TokensForValue(value).Bytes()})
	}
	sort.Slice(replacements, func(i, j int) bool { return replacements[i].start > replacements[j].start })
	out := append([]byte(nil), src...)
	for _, replacement := range replacements {
		replaced := make([]byte, 0, len(out)-replacement.end+replacement.start+len(replacement.value))
		replaced = append(replaced, out[:replacement.start]...)
		replaced = append(replaced, replacement.value...)
		out = append(replaced, out[replacement.end:]...)
	}
	return out, nil
}

// fileCallContainingExpressions returns attribute expressions that contain a file()
// call. Materializing the entire expression preserves HCL's iterator scopes and
// lazy conditional evaluation when the expression is evaluated.
func fileCallContainingExpressions(body *hclsyntax.Body) []hcl.Expression {
	var expressions []hcl.Expression
	var collect func(*hclsyntax.Body)
	collect = func(body *hclsyntax.Body) {
		for _, attr := range body.Attributes {
			if expressionContainsFileCall(attr.Expr) {
				expressions = append(expressions, attr.Expr)
			}
		}
		for _, block := range body.Blocks {
			collect(block.Body)
		}
	}
	collect(body)
	return expressions
}

func expressionContainsFileCall(expr hcl.Expression) bool {
	node, ok := expr.(hclsyntax.Node)
	if !ok {
		return false
	}
	hasFileCall := false
	_ = hclsyntax.VisitAll(node, func(node hclsyntax.Node) hcl.Diagnostics {
		call, ok := node.(*hclsyntax.FunctionCallExpr)
		if ok && call.Name == "file" {
			hasFileCall = true
		}
		return nil
	})
	return hasFileCall
}

// parseBundleContent parses HCL content using a two-pass approach: first extracting
// and evaluating locals, then decoding the full bundle with an EvalContext containing
// those locals. filename is used only for error message attribution.
func (p *HCLParser) parseBundleContent(ctx context.Context, src []byte, filename, baseDir string, allowFile bool) (*UDSBundle, error) {
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

// extractLocals extracts and evaluates locals from the given HCL file.
// Nested objects (e.g., pkgs = { base = "core-base" }) are handled natively
// by go-cty, producing cty.ObjectVal values that support traversal like
// ${local.pkgs.base}.
func (p *HCLParser) extractLocals(hclFile *hcl.File, funcs map[string]function.Function) (map[string]cty.Value, map[int]*hcl.EvalContext, error) {
	schema := &hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{
			{Type: "locals"},
		},
	}

	content, _, diags := hclFile.Body.PartialContent(schema)
	if diags.HasErrors() {
		return nil, nil, fmt.Errorf("failed to read locals block: %s", diags.Error())
	}

	localsMap := make(map[string]cty.Value)
	localEvalContexts := make(map[int]*hcl.EvalContext)

	for _, block := range content.Blocks {
		if block.Type != "locals" {
			continue
		}

		attrs, diags := block.Body.JustAttributes()
		if diags.HasErrors() {
			return nil, nil, fmt.Errorf("failed to read locals attributes: %s", diags.Error())
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
				Functions: funcs,
			}
			localEvalContexts[attr.Expr.Range().Start.Byte] = ctx
			val, diags := attr.Expr.Value(ctx)
			if diags.HasErrors() {
				return nil, nil, fmt.Errorf("failed to evaluate local %q: %s", name, diags.Error())
			}
			localsMap[name] = val
		}
	}

	return localsMap, localEvalContexts, nil
}

// decodeBundleWithLocals decodes the given HCL file into a UDSBundle struct
// using an EvalContext populated with the extracted locals.
// It uses gohcl for standard fields and post-processes the Package.Remain
// field to extract depends_on expressions into []PackageRef.
func (p *HCLParser) decodeBundleWithLocals(hclFile *hcl.File, locals map[string]cty.Value, funcs map[string]function.Function) (*UDSBundle, error) {
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
