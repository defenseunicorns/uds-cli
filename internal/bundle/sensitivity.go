// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"bytes"
	"fmt"

	"github.com/defenseunicorns/uds-cli/internal/redact"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"
)

// A malformed expression may not have a complete AST. Recognize the call in tokens
// so its argument never falls back to an unmarked literal or an HCL diagnostic.
func hasSensitiveCallTokens(src []byte) bool {
	if !bytes.Contains(src, []byte("sensitive")) {
		return false
	}
	tokens, _ := hclsyntax.LexExpression(src, "", hcl.InitialPos)
	pending := false
	for _, token := range tokens {
		if token.Type == hclsyntax.TokenComment || token.Type == hclsyntax.TokenNewline {
			continue
		}
		if pending && token.Type == hclsyntax.TokenOParen {
			return true
		}
		pending = token.Type == hclsyntax.TokenIdent && string(token.Bytes) == "sensitive"
	}
	return false
}

// recordSensitive registers scalar leaves of a marked value, including derived expressions.
func recordSensitive(scope *redact.Scope, value cty.Value) {
	if scope == nil {
		return
	}
	value, _ = value.UnmarkDeep()
	var values []string
	for _, leaf := range cty.DeepValues(value) {
		if leaf.IsKnown() && !leaf.IsNull() && leaf.Type().IsPrimitiveType() {
			if text, err := convert.Convert(leaf, cty.String); err == nil {
				values = append(values, text.AsString())
			}
		}
	}
	scope.Add(values...)
}

// decodeStructured adapts only attributes consumed by gohcl. Free-form variables
// and static dependency traversals in a remain body keep their original expressions.
func decodeStructured(file *hcl.File, ctx *hcl.EvalContext, target any, scope *redact.Scope, onMark func(block, name string)) hcl.Diagnostics {
	return gohcl.DecodeBody(decodingBody{Body: file.Body, scope: scope, onMark: onMark}, ctx, target)
}

type decodingBody struct {
	hcl.Body
	block  string
	scope  *redact.Scope
	onMark func(block, name string)
}

func (b decodingBody) Content(schema *hcl.BodySchema) (*hcl.BodyContent, hcl.Diagnostics) {
	content, diags := b.Body.Content(schema)
	b.prepare(content)
	return content, diags
}

func (b decodingBody) PartialContent(schema *hcl.BodySchema) (*hcl.BodyContent, hcl.Body, hcl.Diagnostics) {
	content, remain, diags := b.Body.PartialContent(schema)
	b.prepare(content)
	return content, remain, diags
}

func (b decodingBody) JustAttributes() (hcl.Attributes, hcl.Diagnostics) {
	attrs, diags := b.Body.JustAttributes()
	b.prepareAttributes(attrs)
	return attrs, diags
}

func (b decodingBody) prepare(content *hcl.BodyContent) {
	if content == nil {
		return
	}
	b.prepareAttributes(content.Attributes)
	for _, block := range content.Blocks {
		block.Body = decodingBody{block.Body, block.Type, b.scope, b.onMark}
	}
}

func (b decodingBody) prepareAttributes(attrs hcl.Attributes) {
	for _, attr := range attrs {
		attr.Expr = decodedExpression{attr.Expr, func(value cty.Value) {
			if b.scope != nil {
				_ = cty.Walk(value, func(_ cty.Path, part cty.Value) (bool, error) {
					if !part.HasMark(sensitiveMark) {
						return true, nil
					}
					recordSensitive(b.scope, part)
					return false, nil
				})
			}
			if b.onMark != nil {
				b.onMark(b.block, attr.Name)
			}
		}}
	}
}

type decodedExpression struct {
	hcl.Expression
	onSensitive func(cty.Value)
}

func (e decodedExpression) Value(ctx *hcl.EvalContext) (cty.Value, hcl.Diagnostics) {
	value, diags := e.Expression.Value(ctx)
	if diags.HasErrors() || !value.HasMarkDeep(sensitiveMark) {
		return value, diags
	}
	e.onSensitive(value)
	value, _ = value.UnmarkDeep()
	return value, diags
}

// SensitiveValues returns the scalar values of marked variables after all overrides.
func (variables Variables) SensitiveValues(names map[string]bool) []string {
	var secrets []string
	var collect func(any)
	collect = func(value any) {
		switch v := value.(type) {
		case Variables:
			for _, child := range v {
				collect(child)
			}
		case []any:
			for _, child := range v {
				collect(child)
			}
		case nil:
		default:
			// An empty search pattern would match every position in a message.
			if text := fmt.Sprint(v); text != "" {
				secrets = append(secrets, text)
			}
		}
	}
	for name, sensitive := range names {
		if sensitive {
			collect(variables[name])
		}
	}
	return secrets
}
