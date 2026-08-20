// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"cmp"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"unicode/utf8"

	"github.com/hashicorp/hcl/v2"
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

// readFileValue reads a file for evaluation by the HCL file function.
func readFileValue(baseDir, path string) (cty.Value, error) {
	if !filepath.IsAbs(path) {
		path = filepath.Join(baseDir, path)
	}
	info, err := os.Stat(path)
	if err != nil {
		return cty.NilVal, fmt.Errorf("stat file %q: %w: %w", path, ErrReadFile, err)
	}
	if !info.Mode().IsRegular() {
		return cty.NilVal, fmt.Errorf("file %q is not a regular file: %w", path, ErrInvalidFile)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return cty.NilVal, fmt.Errorf("read file %q: %w: %w", path, ErrReadFile, err)
	}
	if !utf8.Valid(contents) {
		return cty.NilVal, fmt.Errorf("file %q is not valid UTF-8: %w", path, ErrInvalidFile)
	}
	return cty.StringVal(string(contents)), nil
}

// rejectFileFunctionWithoutSourcePath rejects file() for in-memory HCL. Unlike
// ParseBundleFile, ParseBundleBytes has no stable directory for relative paths.
func rejectFileFunctionWithoutSourcePath(hclFile *hcl.File, fileKind string) error {
	body, ok := hclFile.Body.(*hclsyntax.Body)
	if !ok {
		return ErrUnexpectedHCLBody
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

	return fmt.Errorf("file() requires a file-backed bundle source; %s cannot use file() at %s: %w", fileKind, callRange, ErrFileFunctionUnavailable)
}

// materializeDefaultsFile resolves file function calls in a defaults file.
func materializeDefaultsFile(path string) ([]byte, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%w %q: %w", ErrReadDefaultsFile, path, err)
	}
	if _, err := parseDefaultsContent(src, path); err != nil {
		return nil, err
	}
	return materializeFileCallExpressions(src, path, configEvalContext(path), nil)
}

// MaterializeDefaultsFile resolves file() calls in a defaults file.
func MaterializeDefaultsFile(path string) ([]byte, error) {
	return materializeDefaultsFile(path)
}

// materializeBundleFileCalls resolves file function calls in bundle HCL source.
func (p *HCLParser) materializeBundleFileCalls(src []byte, filename, baseDir string) ([]byte, error) {
	funcs := map[string]function.Function{"file": newFileFunction(baseDir)}
	hclFile, diags := hclsyntax.ParseConfig(src, filename, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return nil, fmt.Errorf("%w %q: %w", ErrParseHCL, filename, diags)
	}
	locals, localEvalContexts, err := p.extractLocals(hclFile, funcs)
	if err != nil {
		return nil, fmt.Errorf("%w from %q: %w", ErrExtractLocals, filename, err)
	}
	localVal := cty.EmptyObjectVal
	if len(locals) > 0 {
		localVal = cty.ObjectVal(locals)
	}
	return materializeFileCallExpressions(src, filename, &hcl.EvalContext{Variables: map[string]cty.Value{"local": localVal, "sys": sysVars(p.arch)}, Functions: funcs}, localEvalContexts)
}

// materializeFileCallExpressions replaces file call expressions in an HCL body.
func materializeFileCallExpressions(src []byte, filename string, evalCtx *hcl.EvalContext, localEvalContexts map[int]*hcl.EvalContext) ([]byte, error) {
	hclFile, diags := hclsyntax.ParseConfig(src, filename, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return nil, fmt.Errorf("%w %q: %w", ErrParseHCL, filename, diags)
	}
	body, ok := hclFile.Body.(*hclsyntax.Body)
	if !ok {
		return nil, ErrUnexpectedHCLBody
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
			return nil, fmt.Errorf("evaluating HCL expression containing file(): %w: %w", ErrEvaluateHCLExpression, diags)
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

// expressionContainsFileCall reports whether an expression invokes the file function.
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

// localDependencies returns the local.<name> references used by an expression
func localDependencies(expr hcl.Expression) []string {
	var deps []string
	seen := map[string]struct{}{}

	for _, traversal := range expr.Variables() {
		if len(traversal) < 2 {
			continue
		}

		root, ok := traversal[0].(hcl.TraverseRoot)
		if !ok || root.Name != "local" {
			continue
		}

		var dep string

		switch step := traversal[1].(type) {
		case hcl.TraverseAttr:
			dep = step.Name
		case hcl.TraverseIndex:
			if step.Key.Type() != cty.String {
				continue
			}
			dep = step.Key.AsString()
		default:
			continue
		}

		if _, exists := seen[dep]; exists {
			continue
		}

		seen[dep] = struct{}{}
		deps = append(deps, dep)
	}

	return deps
}

// topoSortLocals orders local names so each local is evaluated after the locals it references
func topoSortLocals(sourceOrder []string, exprs map[string]hcl.Expression, deps map[string][]string) ([]string, error) {
	const (
		unvisited = iota
		visiting
		visited
	)

	state := make(map[string]int)
	stack := []string{}
	stackIndex := map[string]int{}
	order := make([]string, 0, len(exprs))

	var visit func(string) error
	visit = func(name string) error {
		switch state[name] {
		case visiting:
			start := stackIndex[name]
			cycle := append([]string(nil), stack[start:]...)
			cycle = append(cycle, name)
			return &CyclicLocalDependencyError{
				Cycle: cycle,
			}
		case visited:
			return nil
		}

		state[name] = visiting
		stackIndex[name] = len(stack)
		stack = append(stack, name)

		for _, dep := range deps[name] {
			if _, ok := exprs[dep]; !ok {
				return &UndefinedLocalDependencyError{
					Name:     dep,
					Position: exprs[name].Range(),
				}
			}

			if err := visit(dep); err != nil {
				return err
			}
		}

		stack = stack[:len(stack)-1]
		delete(stackIndex, name)

		state[name] = visited
		order = append(order, name)

		return nil
	}

	for _, name := range sourceOrder {
		if err := visit(name); err != nil {
			return nil, err
		}
	}

	return order, nil
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
		return nil, nil, &LocalsBlockError{Diags: diags}
	}

	exprs := make(map[string]hcl.Expression)
	positions := make(map[string]int)
	deps := make(map[string][]string)
	sourceOrder := make([]string, 0)

	for _, block := range content.Blocks {
		if block.Type != "locals" {
			continue
		}

		attrs, diags := block.Body.JustAttributes()
		if diags.HasErrors() {
			return nil, nil, &LocalsAttributeError{Diags: diags}
		}

		// JustAttributes returns a map, so sort by source position before building sourceOrder
		names := slices.Collect(maps.Keys(attrs))
		slices.SortFunc(names, func(a, b string) int {
			posA := attrs[a].Expr.Range().Start
			posB := attrs[b].Expr.Range().Start

			return cmp.Or(
				cmp.Compare(posA.Byte, posB.Byte),
				cmp.Compare(a, b),
			)
		})

		for _, name := range names {
			attr := attrs[name]

			// catch already defined variables
			if existing, exists := exprs[name]; exists {
				return nil, nil, &DuplicateLocalError{name, existing.Range(), attr.Expr.Range()}
			}

			exprs[name] = attr.Expr
			positions[name] = attr.Expr.Range().Start.Byte
			deps[name] = localDependencies(attr.Expr)
			sourceOrder = append(sourceOrder, name)
		}
	}

	order, err := topoSortLocals(sourceOrder, exprs, deps)
	if err != nil {
		return nil, nil, err
	}

	locals := make(map[string]cty.Value)
	contexts := make(map[int]*hcl.EvalContext)

	// build the local namespace
	for _, name := range order {
		localVal := cty.EmptyObjectVal
		if len(locals) > 0 {
			localVal = cty.ObjectVal(maps.Clone(locals))
		}

		ctx := &hcl.EvalContext{
			Variables: map[string]cty.Value{
				"local": localVal,
				"sys":   sysVars(p.arch),
			},
			Functions: funcs,
		}

		contexts[positions[name]] = ctx

		val, diags := exprs[name].Value(ctx)
		if diags.HasErrors() {
			return nil, nil, &LocalEvaluationError{Name: name, Diags: diags}
		}

		locals[name] = val
	}

	return locals, contexts, nil
}

// decodePackageDependsOn extracts depends_on from a package's Remain body as HCL traversals.
// The syntax is: depends_on = [package.core_base, package.core_logging]
// Each element must be a static traversal referencing "package.<name>".
func decodePackageDependsOn(body hcl.Body) ([]decodedPackageRef, error) {
	attrSchema := &hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{
			{Name: "depends_on"},
		},
	}

	content, _, diags := body.PartialContent(attrSchema)
	if diags.HasErrors() {
		return nil, fmt.Errorf("%w from depends_on attribute: %w", ErrReadPackageDependencies, diags)
	}

	attr, exists := content.Attributes["depends_on"]
	if !exists {
		return nil, nil // depends_on is optional
	}

	// Get the list of expressions
	exprs, diags := hcl.ExprList(attr.Expr)
	if diags.HasErrors() {
		return nil, fmt.Errorf("depends_on must be a list of package references: %w: %w", ErrInvalidPackageDependencies, diags)
	}

	var refs []decodedPackageRef
	for _, expr := range exprs {
		// Each element must be a static traversal (e.g., package.core_base)
		traversal, diags := hcl.AbsTraversalForExpr(expr)
		if diags.HasErrors() {
			return nil, fmt.Errorf("depends_on element must be a package reference (e.g., package.core_base): %w: %w", ErrInvalidPackageReference, diags)
		}

		// Validate the traversal structure: must be "package.<name>"
		if len(traversal) != 2 {
			return nil, fmt.Errorf("invalid package reference at %s: expected package.<name>: %w", expr.Range(), ErrInvalidPackageReference)
		}

		root, ok := traversal[0].(hcl.TraverseRoot)
		if !ok || root.Name != "package" {
			return nil, fmt.Errorf("invalid package reference at %s: must start with 'package': %w", expr.Range(), ErrInvalidPackageReference)
		}

		attrStep, ok := traversal[1].(hcl.TraverseAttr)
		if !ok {
			return nil, fmt.Errorf("invalid package reference at %s: expected package.<name>: %w", expr.Range(), ErrInvalidPackageReference)
		}

		refs = append(refs, decodedPackageRef{
			Name:      attrStep.Name,
			Traversal: traversal,
		})
	}

	return refs, nil
}
