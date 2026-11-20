// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package analysis checks saved Next bundle sources without building or deploying.
package analysis

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/defenseunicorns/uds-cli/internal/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

// Options selects the evaluation inputs for one bundle.
type Options struct {
	ConfigPath             string
	Variables              []string
	Architecture           string
	SkipTemplateEvaluation bool
}

// Diagnostic describes an error in a saved source file. Path is the path used
// to read the file. Zero line/column values mean no exact position is available.
type Diagnostic struct {
	Path    string
	Line    int
	Column  int
	Message string
}

// Lint checks one bundle definition, accepting its directory or the definition
// path. Source errors are diagnostics; invocation and cancellation errors are
// returned separately. It never prints results or writes files.
func Lint(ctx context.Context, path string, opts Options) ([]Diagnostic, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	filename, err := definitionPath(path)
	if err != nil {
		return nil, err
	}

	var set bundle.Variables
	if len(opts.Variables) > 0 {
		set, err = bundle.ParseSetVariables(opts.Variables)
		if err != nil {
			return nil, fmt.Errorf("--set must contain valid key=value entries: %w", ErrInvalidInput)
		}
	}

	root := filepath.Dir(filename)
	var diagnostics []Diagnostic
	var defaultVariables bundle.Variables
	defaultsPath := filepath.Join(root, bundle.BundleDefaultsFileName)
	defaults, err := loadDocument(defaultsPath)
	switch {
	case errors.Is(err, os.ErrNotExist):
		// Adjacent defaults are optional.
	case err != nil:
		diagnostics = append(diagnostics, Diagnostic{Path: defaultsPath, Message: "cannot read defaults file"})
	default:
		defaultVariables, err = bundle.ParseDefaultsFileContent(defaults.path, defaults.text)
		if err != nil {
			diagnostics = append(diagnostics, defaults.diagnostics(err, "invalid defaults variables")...)
		}
	}

	arch := opts.Architecture
	architectureKnown := true
	var configVariables bundle.Variables
	if opts.ConfigPath != "" {
		config, err := loadDocument(opts.ConfigPath)
		if err != nil {
			return nil, fmt.Errorf("read selected config: %w", err)
		}
		cfg, err := bundle.ParseBundleConfigFileContent(config.path, config.text)
		architectureKnown = arch != "" || cfg != nil
		if arch == "" && cfg != nil && cfg.Options != nil {
			arch = cfg.Options.Architecture
		}
		if err != nil {
			diagnostics = append(diagnostics, config.diagnostics(err, "invalid config variables or options")...)
		} else {
			configVariables = cfg.Variables
		}
	}

	// Source deploy applies --set to the selected config before merging in
	// bundle defaults. Deep merges with scalar overrides depend on that order.
	variables := bundle.MergeVariables(defaultVariables, bundle.MergeVariables(configVariables, set))
	// Deployment treats values files as plain YAML when no variables are
	// configured. Invalid variable inputs still allow syntax checks.
	variableErrors := len(diagnostics) > 0
	checkTemplates := variables != nil || variableErrors
	executeTemplates := !variableErrors && !opts.SkipTemplateEvaluation
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	doc, err := loadDocument(filename)
	if err != nil {
		return nil, fmt.Errorf("read bundle definition: %w", err)
	}
	if !architectureKnown {
		// A broken config may select a different architecture. Report bundle
		// syntax errors without evaluating sys.arch against the host.
		_, diags := hclsyntax.ParseConfig(doc.text, doc.path, hcl.InitialPos)
		if diags.HasErrors() {
			diagnostics = append(diagnostics, doc.diagnostics(diags, "cannot parse bundle definition")...)
		}
	} else if parsed, err := bundle.NewHCLParser(arch, iostreams.IOStreams{}).ParseBundleContent(ctx, doc.text, doc.path, root, true); err != nil {
		diagnostics = append(diagnostics, doc.diagnostics(err, "cannot evaluate bundle definition")...)
	} else {
		diagnostics = append(diagnostics, doc.validationDiagnostics(parsed.Validate())...)
		if _, err := bundle.BuildDependencyGraph(ctx, iostreams.IOStreams{}, parsed); errors.Is(err, bundle.ErrDependencyCycle) {
			// Unknown references are already reported by domain validation.
			diagnostics = append(diagnostics, Diagnostic{Path: doc.path, Message: "package dependencies contain a cycle"})
		}
		valuesExpressions := sync.OnceValue(doc.valuesExpressions)
		for pkgIndex, pkg := range parsed.Packages {
			for index, path := range pkg.ValuesFiles {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
				values, err := loadDocument(bundle.ResolveValuesFilePath(root, path))
				if err != nil {
					d := doc.at(valuesPosition(valuesExpressions(), pkgIndex, index), fmt.Sprintf("package %q: cannot read referenced values file %q", pkg.Name, path))
					diagnostics = append(diagnostics, d)
					continue
				}
				if !checkTemplates {
					continue
				}
				if d := values.checkTemplate(variables, executeTemplates); d != nil {
					d.Message = fmt.Sprintf("package %q: %s", pkg.Name, d.Message)
					diagnostics = append(diagnostics, *d)
				}
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	slices.SortFunc(diagnostics, func(a, b Diagnostic) int {
		return cmp.Or(
			cmp.Compare(a.Path, b.Path),
			cmp.Compare(a.Line, b.Line),
			cmp.Compare(a.Column, b.Column),
			cmp.Compare(a.Message, b.Message),
		)
	})
	return slices.Compact(diagnostics), nil
}

func definitionPath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("bundle path must not be empty: %w", ErrInvalidInput)
	}
	// Validate the supplied source before using the same selection and
	// normalization that source deployment uses.
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("access bundle path: %w", err)
	}
	if !info.IsDir() && filepath.Base(path) != bundle.BundleFileName {
		return "", fmt.Errorf("expected %s: %w", bundle.BundleFileName, ErrInvalidInput)
	}
	filename, err := filepath.Abs(bundle.ResolveBundlePath(path))
	if err != nil {
		return "", fmt.Errorf("resolve bundle path: %w", err)
	}
	return filename, nil
}
