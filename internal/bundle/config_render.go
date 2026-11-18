// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
)

// hclwrite and gohcl's number-to-string coercion use fixed-point formatting.
// Limit both exponent and precision before either can expand an evaluated
// number. 4096 binary digits allow ordinary scientific values while bounding
// the size of each rendered numeric scalar to roughly 2500 decimal digits.
const maxRenderNumberBits = 4096

// RenderBundleConfigFile evaluates a file-backed config into standalone HCL.
// Only explicitly present attributes and blocks are emitted. The result does
// not depend on the original source files or operation defaults.
func RenderBundleConfigFile(path string) ([]byte, error) {
	if path == "" {
		return nil, EmptyParameterError{Name: "path"}
	}
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%w %q: %w", ErrReadConfigFile, path, err)
	}
	file, diags := hclsyntax.ParseConfig(src, path, hcl.InitialPos)
	if diags.HasErrors() {
		return nil, fmt.Errorf("%w %q: %w", ErrParseConfig, path, diags)
	}
	body, ok := file.Body.(*hclsyntax.Body)
	if !ok {
		return nil, ErrUnexpectedHCLBody
	}
	// Use the parser's structured schema, adding the separately decoded variables
	// attribute, but reject the unknown content that its remain field permits.
	schema, _ := gohcl.ImpliedBodySchema(&UDSBundleConfig{})
	schema.Attributes = append(schema.Attributes, hcl.AttributeSchema{Name: "variables"})
	if _, diags := body.Content(schema); diags.HasErrors() {
		return nil, fmt.Errorf("%w from %q: %w", ErrDecodeConfig, path, diags)
	}
	// HCL may convert a numeric literal to a string inside an expression before
	// Value returns. Reject extreme literals even in unselected branches; without
	// this conservative check such conversions can expand before our value check.
	var excessiveLiteral *hcl.Range
	_ = hclsyntax.VisitAll(body, func(node hclsyntax.Node) hcl.Diagnostics {
		literal, ok := node.(*hclsyntax.LiteralValueExpr)
		if ok && excessiveLiteral == nil && !renderNumberWithinLimits(literal.Val) {
			r := literal.Range()
			excessiveLiteral = &r
		}
		return nil
	})
	if excessiveLiteral != nil {
		return nil, fmt.Errorf("number exceeds render limits at %s: %w", excessiveLiteral, ErrEvaluateHCLExpression)
	}

	values := make(map[int]cty.Value)
	var lastRange *hcl.Range
	var numberLimit bool
	cfg, err := decodeBundleConfig(&renderBody{Body: body, values: values, lastRange: &lastRange, numberLimit: &numberLimit}, configEvalContext(path), path)
	if err != nil {
		message := "invalid config"
		switch {
		case numberLimit:
			message = "number exceeds render limits"
		case errors.Is(err, ErrEvaluateVariables):
			message = "failed to evaluate variables or file expressions"
		case errors.Is(err, ErrConvertVariables), errors.Is(err, ErrInvalidVariables), errors.Is(err, ErrUnsupportedVariableType):
			message = "invalid variables"
		case errors.Is(err, ErrDecodeConfig):
			message = "failed to decode config"
		}
		var diags hcl.Diagnostics
		if errors.As(err, &diags) {
			for _, diag := range diags {
				if diag.Subject != nil {
					lastRange = diag.Subject
					break
				}
			}
		}
		if lastRange != nil {
			message += " at " + lastRange.String()
		}
		return nil, &renderConfigError{message: message, cause: err}
	}
	if cfg.Options != nil {
		optionLocation := func(name string) string {
			for _, block := range body.Blocks {
				if block.Type == "options" && block.Body.Attributes[name] != nil {
					return " at " + block.Body.Attributes[name].Expr.Range().String()
				}
			}
			return ""
		}
		// Zero is the unresolved default, not an invalid explicit option.
		if cfg.Options.Concurrency != 0 {
			if err := validateConcurrency(cfg.Options.Concurrency); err != nil {
				return nil, fmt.Errorf("invalid concurrency%s: %w", optionLocation("concurrency"), ErrInvalidConcurrency)
			}
		}
		// Validation errors from these functions can contain evaluated file data.
		if err := validateLogLevel(cfg.Options.LogLevel); err != nil {
			return nil, fmt.Errorf("invalid log level%s", optionLocation("log_level"))
		}
		if err := validateTmpDir(cfg.Options.TmpDir); err != nil {
			return nil, fmt.Errorf("invalid tmp_dir%s: %w", optionLocation("tmp_dir"), ErrInvalidTemporaryDirectory)
		}
	}
	out := hclwrite.NewEmptyFile()
	if err := renderConfigBody(out.Body(), body, values); err != nil {
		return nil, fmt.Errorf("rendering config %q: %w", path, err)
	}
	return out.Bytes(), nil
}

// renderBody records each expression's evaluated value while the normal parser
// decodes it. It retains the syntax tree for presence and output ordering.
type renderBody struct {
	hcl.Body
	values      map[int]cty.Value
	lastRange   **hcl.Range
	numberLimit *bool
}

func (b *renderBody) Content(schema *hcl.BodySchema) (*hcl.BodyContent, hcl.Diagnostics) {
	content, diags := b.Body.Content(schema)
	b.wrap(content)
	return content, diags
}

func (b *renderBody) PartialContent(schema *hcl.BodySchema) (*hcl.BodyContent, hcl.Body, hcl.Diagnostics) {
	content, remain, diags := b.Body.PartialContent(schema)
	b.wrap(content)
	return content, &renderBody{Body: remain, values: b.values, lastRange: b.lastRange, numberLimit: b.numberLimit}, diags
}

func (b *renderBody) JustAttributes() (hcl.Attributes, hcl.Diagnostics) {
	attrs, diags := b.Body.JustAttributes()
	b.wrapAttributes(attrs)
	return attrs, diags
}

func (b *renderBody) wrap(content *hcl.BodyContent) {
	if content == nil {
		return
	}
	b.wrapAttributes(content.Attributes)
	for _, block := range content.Blocks {
		block.Body = &renderBody{Body: block.Body, values: b.values, lastRange: b.lastRange, numberLimit: b.numberLimit}
	}
}

func (b *renderBody) wrapAttributes(attrs hcl.Attributes) {
	for _, attr := range attrs {
		attr.Expr = &renderExpression{Expression: attr.Expr, values: b.values, lastRange: b.lastRange, numberLimit: b.numberLimit}
	}
}

type renderExpression struct {
	hcl.Expression
	values      map[int]cty.Value
	lastRange   **hcl.Range
	numberLimit *bool
}

func (e *renderExpression) Value(ctx *hcl.EvalContext) (cty.Value, hcl.Diagnostics) {
	if value, ok := e.values[e.Range().Start.Byte]; ok {
		return value, nil
	}
	value, diags := e.Expression.Value(ctx)
	if e.lastRange != nil {
		r := e.Range()
		*e.lastRange = &r
	}
	if !diags.HasErrors() {
		if !renderNumberWithinLimits(value) {
			if e.numberLimit != nil {
				*e.numberLimit = true
			}
			r := e.Range()
			return cty.DynamicVal, hcl.Diagnostics{&hcl.Diagnostic{Severity: hcl.DiagError, Summary: "number exceeds render limits", Subject: &r}}
		}
		e.values[e.Range().Start.Byte] = value
	}
	return value, diags
}

// Check every nested number before decoding can coerce it or output can expand it.
func renderNumberWithinLimits(value cty.Value) bool {
	if !value.IsKnown() || value.IsNull() {
		return true
	}
	ty := value.Type()
	if ty == cty.Number {
		n := value.AsBigFloat()
		exp := n.MantExp(nil)
		return !n.IsInf() && n.Prec() <= maxRenderNumberBits && exp >= -maxRenderNumberBits && exp <= maxRenderNumberBits
	}
	if ty.IsObjectType() || ty.IsTupleType() || ty.IsListType() || ty.IsSetType() || ty.IsMapType() {
		it := value.ElementIterator()
		for it.Next() {
			_, child := it.Element()
			if !renderNumberWithinLimits(child) {
				return false
			}
		}
	}
	return true
}

func renderConfigBody(out *hclwrite.Body, body *hclsyntax.Body, values map[int]cty.Value) error {
	for _, name := range slices.Sorted(maps.Keys(body.Attributes)) {
		attr := body.Attributes[name]
		value, ok := values[attr.Expr.Range().Start.Byte]
		if !ok || !value.IsWhollyKnown() {
			return fmt.Errorf("evaluating %q: unknown values cannot be rendered: %w", name, ErrEvaluateHCLExpression)
		}
		out.SetAttributeValue(name, value)
	}
	for _, block := range body.Blocks {
		child := out.AppendNewBlock(block.Type, block.Labels)
		if err := renderConfigBody(child.Body(), block.Body, values); err != nil {
			return err
		}
	}
	return nil
}
