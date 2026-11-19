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

	"github.com/defenseunicorns/uds-cli/internal/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
)

// Options selects the evaluation inputs for one bundle.
type Options struct {
	ConfigPath             string
	Variables              []string
	Architecture           string
	SkipTemplateEvaluation bool
}

// Diagnostic describes an error in a saved source file. Zero line/column values
// mean that no precise position is available.
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

	// parse --set vars
	set, err := bundle.ParseSetVariables(opts.Variables)
	if err != nil {
		return nil, fmt.Errorf("--set must contain valid key=value entries: %w", ErrInvalidInput)
	}

	root := filepath.Dir(filename)
	var diagnostics []Diagnostic
	var variables bundle.Variables
	validVariables := true

	// parse defaults
	defaultsPath := filepath.Join(root, bundle.BundleDefaultsFileName)
	defaults, err := loadDocument(defaultsPath)
	switch {
	case os.IsNotExist(err):
		// Adjacent defaults are optional.
	case err != nil:
		diagnostics = append(diagnostics, Diagnostic{Path: defaultsPath, Message: "cannot read defaults file"})
		validVariables = false
	default:
		variables, err = bundle.ParseDefaults(ctx, defaults.path)
		if err != nil {
			diagnostics = append(diagnostics, defaults.diagnostics(err, "invalid defaults variables")...)
			validVariables = false
		}
	}

	// parse config
	arch := opts.Architecture
	if opts.ConfigPath != "" {
		config, err := loadDocument(opts.ConfigPath)
		if err != nil {
			return nil, fmt.Errorf("read selected config: %w", err)
		}
		cfg, err := bundle.NewHCLParser("", iostreams.IOStreams{}).ParseBundleConfig(ctx, config.path)
		if err != nil {
			diagnostics = append(diagnostics, config.diagnostics(err, "invalid config variables or options")...)
			validVariables = false
		} else {
			variables = bundle.MergeVariables(variables, cfg.Variables)
			if arch == "" && cfg.Options != nil {
				arch = cfg.Options.Architecture
			}
		}
	}

	variables = bundle.MergeVariables(variables, set)
	if variables == nil {
		variables = make(bundle.Variables)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// bundle.uds.hcl
	doc, err := loadDocument(filename)
	if err != nil {
		return nil, fmt.Errorf("read bundle definition: %w", err)
	}
	parsed, err := bundle.NewHCLParser(arch, iostreams.IOStreams{}).ParseBundleFile(ctx, doc.path)
	if err != nil {
		diagnostics = append(diagnostics, doc.diagnostics(err, "cannot evaluate bundle definition")...)
	} else {
		diagnostics = append(diagnostics, doc.validationDiagnostics(parsed.Validate())...)
		if _, err := bundle.BuildDependencyGraph(ctx, iostreams.IOStreams{}, parsed); errors.Is(err, bundle.ErrDependencyCycle) {
			// Unknown references are already reported by domain validation.
			diagnostics = append(diagnostics, Diagnostic{Path: doc.path, Message: "package dependencies contain a cycle"})
		}
		for _, pkg := range parsed.Packages {
			for index, path := range pkg.ValuesFiles {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
				filename := path
				if !filepath.IsAbs(filename) {
					filename = filepath.Join(root, filename)
				}
				values, err := loadDocument(filename)
				if err != nil {
					d := doc.at(doc.valuesPosition(pkg.Name, index), fmt.Sprintf("package %q: cannot read referenced values file %q", pkg.Name, path))
					diagnostics = append(diagnostics, d)
					continue
				}
				if d := values.checkTemplate(variables, validVariables && !opts.SkipTemplateEvaluation); d != nil {
					d.Message = fmt.Sprintf("package %q: %s", pkg.Name, d.Message)
					diagnostics = append(diagnostics, *d)
				}
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	for i := range diagnostics {
		rel, err := filepath.Rel(root, diagnostics[i].Path)
		if err == nil {
			diagnostics[i].Path = rel
		}
		diagnostics[i].Path = filepath.ToSlash(diagnostics[i].Path)
	}
	slices.SortFunc(diagnostics, func(a, b Diagnostic) int {
		if c := cmp.Compare(a.Path, b.Path); c != 0 {
			return c
		}
		if c := cmp.Compare(a.Line, b.Line); c != 0 {
			return c
		}
		if c := cmp.Compare(a.Column, b.Column); c != 0 {
			return c
		}
		return cmp.Compare(a.Message, b.Message)
	})
	return slices.Compact(diagnostics), nil
}

func definitionPath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("bundle path must not be empty: %w", ErrInvalidInput)
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve bundle path: %w", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("access bundle path: %w", err)
	}
	if info.IsDir() {
		path = filepath.Join(path, bundle.BundleFileName)
	} else if filepath.Base(path) != bundle.BundleFileName {
		return "", fmt.Errorf("expected %s: %w", bundle.BundleFileName, ErrInvalidInput)
	}
	return path, nil
}
