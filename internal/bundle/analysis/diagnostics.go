// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package analysis

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/defenseunicorns/uds-cli/internal/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

type document struct {
	path string
	text []byte
}

func loadDocument(path string) (document, error) {
	doc := document{path: path}
	info, err := os.Stat(path)
	if err != nil {
		return doc, err
	}
	if !info.Mode().IsRegular() {
		return doc, fmt.Errorf("%q is not a regular file: %w", path, ErrInvalidInput)
	}
	doc.text, err = os.ReadFile(path)
	return doc, err
}

func (d document) at(pos *hcl.Pos, message string) Diagnostic {
	result := Diagnostic{Path: d.path, Message: message}
	if pos != nil {
		offset := max(0, min(pos.Byte, len(d.text)))
		prefix := string(d.text[:offset])
		start := strings.LastIndexByte(prefix, '\n') + 1
		result.Line = strings.Count(prefix, "\n") + 1
		result.Column = utf8.RuneCountInString(prefix[start:]) + 1
	}
	return result
}

func (d document) diagnostics(err error, fallback string) []Diagnostic {
	diags, hasDiagnostics := errors.AsType[hcl.Diagnostics](err)
	if !hasDiagnostics && (errors.Is(err, bundle.ErrParseConfig) || errors.Is(err, bundle.ErrParseDefaults)) {
		// Recover syntax locations from the saved source when the parser
		// returns a sanitized error without structured diagnostics.
		_, diags = hclsyntax.ParseConfig(d.text, d.path, hcl.InitialPos)
	}
	if diags.HasErrors() {
		var prefix string
		switch {
		case errors.Is(err, bundle.ErrInvalidPackageDependencies):
			prefix = "depends_on must be a list of package references: "
		case errors.Is(err, bundle.ErrInvalidPackageReference):
			prefix = "depends_on entries must use package.<name>: "
		}
		result := make([]Diagnostic, 0, len(diags))
		for _, item := range diags {
			if item.Severity != hcl.DiagError {
				continue
			}
			var pos *hcl.Pos
			if item.Subject != nil {
				pos = &item.Subject.Start
			}
			message := item.Summary
			// Customize individual HCL messages known to exclude evaluated values.
			switch item.Summary {
			case "Missing required argument":
				if item.Detail != "" {
					message = item.Detail
				}
			}
			result = append(result, d.at(pos, prefix+message))
		}
		return result
	}
	var duplicate *bundle.DuplicateLocalError
	var undefined *bundle.UndefinedLocalDependencyError
	var cycle *bundle.CyclicLocalDependencyError
	var reference *bundle.InvalidPackageReferenceError
	switch {
	case errors.As(err, &duplicate):
		return []Diagnostic{d.at(&duplicate.Duplicate.Start, fmt.Sprintf("duplicate local %q", duplicate.Name))}
	case errors.As(err, &undefined):
		return []Diagnostic{d.at(&undefined.Position.Start, fmt.Sprintf("undefined local %q", undefined.Name))}
	case errors.As(err, &cycle):
		return []Diagnostic{d.at(nil, cycle.Error())}
	case errors.As(err, &reference):
		return []Diagnostic{d.at(&reference.Range.Start, "invalid depends_on reference: "+reference.Reason)}
	case errors.Is(err, bundle.ErrEvaluateVariables):
		fallback = "cannot evaluate variables"
	case errors.Is(err, bundle.ErrInvalidVariables), errors.Is(err, bundle.ErrConvertVariables):
		fallback = "variables must be an object containing supported non-null values"
	case errors.Is(err, bundle.ErrInvalidDefaults):
		fallback = "defaults may contain only a variables attribute"
	}
	return []Diagnostic{d.at(nil, fallback)}
}

func (d document) validationDiagnostics(err error) []Diagnostic {
	if err == nil {
		return nil
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		var result []Diagnostic
		for _, child := range joined.Unwrap() {
			result = append(result, d.validationDiagnostics(child)...)
		}
		return result
	}
	var message string
	var version *spec.UnsupportedBundleAPIVersionError
	var component *spec.DuplicateOptionalComponentError
	switch {
	case errors.As(err, &version):
		message = "unsupported uds.bundle_api_version; expected " + version.Expected
	case errors.As(err, &component):
		message = fmt.Sprintf("package %q: duplicate optional component", component.Package)
	default:
		message = err.Error()
	}
	return []Diagnostic{d.at(nil, message)}
}

func (d document) valuesExpressions() []hcl.Expression {
	file, _ := hclsyntax.ParseConfig(d.text, d.path, hcl.InitialPos)
	if file == nil {
		return nil
	}
	body, ok := file.Body.(*hclsyntax.Body)
	if !ok {
		return nil
	}
	var expressions []hcl.Expression
	for _, block := range body.Blocks {
		if block.Type != "package" {
			continue
		}
		var expr hcl.Expression
		if attr := block.Body.Attributes["values_files"]; attr != nil {
			expr = attr.Expr
		}
		expressions = append(expressions, expr)
	}
	return expressions
}

func valuesPosition(expressions []hcl.Expression, pkgIndex, fileIndex int) *hcl.Pos {
	if pkgIndex >= len(expressions) || expressions[pkgIndex] == nil {
		return nil
	}
	expr := expressions[pkgIndex]
	if entries, diags := hcl.ExprList(expr); !diags.HasErrors() && fileIndex < len(entries) {
		expr = entries[fileIndex]
	}
	r := expr.Range()
	return &r.Start
}
