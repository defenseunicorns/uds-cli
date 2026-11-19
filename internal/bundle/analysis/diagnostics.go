package analysis

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	path, err := filepath.Abs(path)
	if err != nil {
		return document{}, err
	}
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
	if diags, ok := errors.AsType[hcl.Diagnostics](err); ok {
		result := make([]Diagnostic, 0, len(diags))
		for _, item := range diags {
			if item.Severity != hcl.DiagError {
				continue
			}
			var pos *hcl.Pos
			if item.Subject != nil {
				pos = &item.Subject.Start
			}
			// HCL details can contain evaluated values. The summary and source
			// location identify the problem without printing config/file contents.
			result = append(result, d.at(pos, item.Summary))
		}
		return result
	}
	var duplicate *bundle.DuplicateLocalError
	var undefined *bundle.UndefinedLocalDependencyError
	switch {
	case errors.As(err, &duplicate):
		return []Diagnostic{d.at(&duplicate.Duplicate.Start, fmt.Sprintf("duplicate local %q", duplicate.Name))}
	case errors.As(err, &undefined):
		return []Diagnostic{d.at(&undefined.Position.Start, fmt.Sprintf("undefined local %q", undefined.Name))}
	case errors.Is(err, bundle.ErrInvalidVariables):
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
	message := err.Error()
	var version *spec.UnsupportedBundleAPIVersionError
	var component *spec.DuplicateOptionalComponentError
	switch {
	case errors.As(err, &version):
		message = "unsupported uds.bundle_api_version; expected uds.dev/v1alpha1"
	case errors.As(err, &component):
		message = fmt.Sprintf("package %q: duplicate optional component", component.Package)
	}
	return []Diagnostic{d.at(nil, message)}
}

func (d document) valuesPosition(packageName string, index int) *hcl.Pos {
	file, _ := hclsyntax.ParseConfig(d.text, d.path, hcl.InitialPos)
	if file == nil {
		return nil
	}
	body, ok := file.Body.(*hclsyntax.Body)
	if !ok {
		return nil
	}
	for _, block := range body.Blocks {
		if block.Type != "package" || len(block.Labels) != 1 || block.Labels[0] != packageName {
			continue
		}
		attr := block.Body.Attributes["values_files"]
		if attr == nil {
			return nil
		}
		r := attr.Expr.Range()
		expressions, diags := hcl.ExprList(attr.Expr)
		if !diags.HasErrors() && index < len(expressions) {
			r = expressions[index].Range()
		}
		return &r.Start
	}
	return nil
}
