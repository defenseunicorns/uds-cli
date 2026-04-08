// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"text/template"

	"github.com/zarf-dev/zarf/src/pkg/feature"
	"github.com/zarf-dev/zarf/src/pkg/logger"
	"github.com/zarf-dev/zarf/src/pkg/packager"
	"github.com/zarf-dev/zarf/src/pkg/packager/filters"
	"github.com/zarf-dev/zarf/src/pkg/value"
)

// enableValuesOnce ensures the Zarf "values" feature flag is enabled exactly once.
// The flag is disabled by default in Zarf; UDS requires it to pass Helm values
// via packager.DeployOptions.Values (reference: vendor/.../feature/feature.go).
// Errors from feature.Set are intentionally ignored: they indicate user features
// were already set by the embedding application, which takes precedence.
var enableValuesOnce sync.Once

// ZarfDeployer implements Deployer using the Zarf Go library.
// Reference: .ai/example-repos/uds-cli/src/pkg/bundle/deploy.go lines 38-165
type ZarfDeployer struct {
	// Out is the writer for output messages
	Out io.Writer
}

// NewZarfDeployer creates a new ZarfDeployer.
func NewZarfDeployer(out io.Writer) *ZarfDeployer {
	// Enable the Zarf "values" feature flag so packager.Deploy accepts
	// DeployOptions.Values (Helm values from values_files).
	enableValuesOnce.Do(func() {
		_ = feature.Set([]feature.Feature{{
			Name:    feature.Values,
			Enabled: true,
			Stage:   feature.Alpha,
		}})
	})
	return &ZarfDeployer{
		Out: out,
	}
}

// DeployPackage deploys a single Zarf package using the Zarf Go library.
func (d *ZarfDeployer) DeployPackage(ctx context.Context, pkg *Package, opts DeployPackageOptions) error {
	slog.Info("deploying zarf package", "name", pkg.Name, "source", pkg.Source)

	ctx = d.setupLoggerContext(ctx, opts.Config.Global.LogLevel)

	pkgTmp, err := os.MkdirTemp(opts.Config.Options.TmpDir, "zarf-pkg-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(pkgTmp); err != nil {
			slog.Warn("failed to remove temporary directory", "path", pkgTmp, "error", err)
		}
	}()

	zarfValues, setVars, err := d.prepareValuesAndVariables(ctx, pkg, opts)
	if err != nil {
		return err
	}

	slog.Info("pulling package", "source", pkg.Source)

	source := NewPackageSource(pkg.Source, *opts.Config.Options, opts.BundleDir)
	filter := BuildComponentFilter(pkg.OptionalComponents)

	pkgLayout, err := source.PullFiltered(ctx, filter, pkgTmp)
	if err != nil {
		return fmt.Errorf("failed to load package %q from %s: %w", pkg.Name, pkg.Source, err)
	}
	defer func() {
		if err := pkgLayout.Cleanup(); err != nil {
			slog.Warn("failed to clean up package layout", "name", pkg.Name, "error", err)
		}
	}()

	// IsInteractive matches Prompt - only show interactive prompts when user opted in via --prompt
	deployOpts := packager.DeployOptions{
		Values:            zarfValues, // Helm chart values from values_files
		SetVariables:      setVars,    // Zarf ###ZARF_PKG_VAR_*### passthrough
		IsInteractive:     opts.Prompt,
		NamespaceOverride: pkg.Namespace, // empty string is fine - Zarf ignores it
	}

	slog.Info("deploying zarf package to cluster", "name", pkg.Name)

	_, err = packager.Deploy(ctx, pkgLayout, deployOpts)
	if err != nil {
		return fmt.Errorf("failed to deploy package %q: %w", pkg.Name, err)
	}

	slog.Info("package deployed", "name", pkg.Name)
	return nil
}

// prepareValuesAndVariables resolves, templates, and parses values_files for a package,
// and flattens config variables for Zarf ###ZARF_PKG_VAR_*### substitution.
// Temporary files created during templating are cleaned up before this method returns,
// since value.ParseFiles reads them into memory before returning.
func (d *ZarfDeployer) prepareValuesAndVariables(ctx context.Context, pkg *Package, opts DeployPackageOptions) (zarfValues value.Values, setVars map[string]string, err error) {
	var configVars Variables
	if opts.Config != nil {
		configVars = opts.Config.Variables
	}

	if len(pkg.ValuesFiles) > 0 {
		// 1. Resolve relative paths against the bundle directory
		resolved := resolveValuesFiles(pkg.ValuesFiles, opts.BundleDir)

		// 2. Template {{ .vars.* }} placeholders using config variables
		filesToParse, err := templateValuesFiles(ctx, resolved, configVars, opts.Config.Options.TmpDir)
		// Temp files are fully consumed by ParseFiles below; clean up on return.
		if configVars != nil {
			defer cleanupTempFiles(filesToParse)
		}
		if err != nil {
			return nil, nil, fmt.Errorf("failed to template values files for package %q: %w", pkg.Name, err)
		}

		// 3. Parse YAML files with Zarf's value package
		zarfValues, err = value.ParseFiles(ctx, filesToParse, value.ParseFilesOptions{})
		if err != nil {
			return nil, nil, fmt.Errorf("failed to parse values files for package %q: %w", pkg.Name, err)
		}
		_, _ = fmt.Fprintf(d.Out, "  Loaded %d values file(s)\n", len(filesToParse))
	}

	// Flatten top-level scalar variables for Zarf ###ZARF_PKG_VAR_*### substitution
	setVars = configVars.Flatten()
	return zarfValues, setVars, nil
}

// resolveValuesFiles resolves values file paths relative to bundleDir.
// Absolute paths are returned unchanged. nil input returns nil.
func resolveValuesFiles(files []string, bundleDir string) []string {
	if files == nil {
		return nil
	}
	resolved := make([]string, len(files))
	for i, f := range files {
		if filepath.IsAbs(f) {
			resolved[i] = filepath.Clean(f)
		} else {
			resolved[i] = filepath.Join(bundleDir, f)
		}
	}
	return resolved
}

// templateValuesFiles renders {{ .vars.* }} templates in values files using vars
// from config.uds.hcl. Returns paths to temporary files with rendered content.
// The caller is responsible for calling cleanupTempFiles on the returned paths.
//
// Template context: { "vars": vars }
// Access: {{ .vars.domain }}, {{ .vars.logging.vectorEnabled }}, etc.
//
// Missing keys produce a clear error (template.Option("missingkey=error")).
// If vars is nil, the original file paths are returned unchanged (no temp copies created).
func templateValuesFiles(_ context.Context, files []string, vars Variables, tmpDir string) ([]string, error) {
	if vars == nil {
		return files, nil
	}

	data := map[string]any{"vars": map[string]any(vars)}
	result := make([]string, 0, len(files))

	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			return result, fmt.Errorf("failed to read values file %q: %w", f, err)
		}

		tmpl, err := template.New(filepath.Base(f)).Option("missingkey=error").Parse(string(src))
		if err != nil {
			return result, fmt.Errorf("failed to parse template in values file %q: %w", f, err)
		}

		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, data); err != nil {
			return result, fmt.Errorf("failed to render values file %q: %w", f, err)
		}

		tmp, err := os.CreateTemp(tmpDir, "uds-values-*.yaml")
		if err != nil {
			return result, fmt.Errorf("failed to create temp file for values: %w", err)
		}
		if _, err := tmp.Write(buf.Bytes()); err != nil {
			_ = tmp.Close()
			return result, fmt.Errorf("failed to write temp values file: %w", err)
		}
		if err := tmp.Close(); err != nil {
			return result, fmt.Errorf("failed to close temp values file: %w", err)
		}
		result = append(result, tmp.Name())
	}
	return result, nil
}

// cleanupTempFiles removes temporary files created by templateValuesFiles.
// Removal errors are logged but not returned, consistent with existing cleanup patterns.
func cleanupTempFiles(files []string) {
	for _, f := range files {
		if err := os.Remove(f); err != nil && !os.IsNotExist(err) {
			slog.Warn("failed to remove temp file", "path", f, "error", err)
		}
	}
}

// setupLoggerContext creates a context with a Zarf logger configured at the given log level.
// logLevel is already validated by the config resolver; ParseLevel will not fail here.
func (d *ZarfDeployer) setupLoggerContext(ctx context.Context, logLevel string) context.Context {
	level, _ := logger.ParseLevel(logLevel)

	cfg := logger.Config{
		Level:       level,
		Format:      logger.FormatConsole,
		Destination: logger.Destination(d.Out),
		Color:       true,
	}
	l, err := logger.New(cfg)
	if err != nil {
		// Fall back to default logger if config fails
		l = logger.Default()
	}
	return logger.WithContext(ctx, l)
}

// BuildComponentFilter creates a component filter strategy from optional component names.
// When optionalComponents is empty, only Required and Default Zarf components are included.
// When optionalComponents lists component names, those are explicitly included alongside
// Required components. Use the "-name" prefix to explicitly exclude a component.
func BuildComponentFilter(optionalComponents []string) filters.ComponentFilterStrategy {
	return filters.Combine(
		filters.ForDeploy(strings.Join(optionalComponents, ","), false),
	)
}
