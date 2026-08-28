// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package disassemble converts packaged artifacts into recreatable local source.
package disassemble

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/defenseunicorns/pkg/helpers/v2"
	"github.com/zarf-dev/zarf/src/api/v1alpha1"
	"github.com/zarf-dev/zarf/src/pkg/packager/filters"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
)

// Disassemble converts one packaged artifact into local source. Package inputs
// are supported today; the source-shaped API leaves room for bundle inputs.
func Disassemble(ctx context.Context, opts Options) (*Result, error) {
	if strings.TrimSpace(opts.Source) == "" {
		return nil, errors.New("source is required")
	}
	if strings.TrimSpace(opts.OutputDir) == "" {
		return nil, errors.New("output directory is required")
	}

	finalDir, err := filepath.Abs(opts.OutputDir)
	if err != nil {
		return nil, fmt.Errorf("resolving output directory: %w", err)
	}
	if err := validateOutputDir(finalDir); err != nil {
		return nil, err
	}

	tmpRoot, err := os.MkdirTemp(opts.TmpDir, "uds-dev-disassemble-*")
	if err != nil {
		return nil, fmt.Errorf("creating temporary directory: %w", err)
	}
	defer warnRemoveAll(opts.Warn, "temporary directory", tmpRoot)()

	if opts.Architecture == "" {
		opts.Architecture = runtime.GOARCH
	}
	pkgLayout, err := loadPackageSource(ctx, tmpRoot, opts, layout.PackageLayoutOptions{
		Filter:               filters.Empty(),
		VerificationStrategy: opts.VerificationStrategy,
	})
	if err != nil {
		return nil, fmt.Errorf("loading source package: %w", err)
	}
	defer func() {
		if err := pkgLayout.Cleanup(); err != nil {
			warn(opts.Warn, "failed to remove package layout", "path", pkgLayout.DirPath(), "error", err)
		}
	}()

	// Zarf's generic definition provides one alpha working view for asset localization;
	// localizedDefinition maps those changes back onto the package's native API version.
	pkg := pkgLayout.AsV1alpha1()
	if pkg.Build.Differential {
		return nil, errors.New("differential Zarf packages do not contain complete recreatable source")
	}
	if pkg.Metadata.Architecture == v1alpha1.SkeletonArch {
		return nil, errors.New("skeleton Zarf packages do not contain complete recreatable source")
	}
	pkg.Build = v1alpha1.ZarfBuildData{Migrations: pkg.Build.Migrations}
	normalizeMetadata(&pkg.Metadata)
	for idx := range pkg.Components {
		pkg.Components[idx].Only.Flavor = ""
	}

	stageDir, err := createOutputStage(finalDir)
	if err != nil {
		return nil, err
	}
	defer func() {
		if stageDir != "" {
			warnRemoveAll(opts.Warn, "output staging directory", stageDir)()
		}
	}()

	if err := copyPackageLevelAssets(ctx, pkgLayout, stageDir, &pkg); err != nil {
		return nil, err
	}
	for i := range pkg.Components {
		componentTmpRoot := filepath.Join(tmpRoot, "disassemble-components", pkg.Components[i].Name)
		if err := os.MkdirAll(componentTmpRoot, helpers.ReadWriteExecuteUser); err != nil {
			return nil, fmt.Errorf("creating component temporary directory: %w", err)
		}
		if err := localizeComponent(ctx, pkgLayout, stageDir, finalDir, componentTmpRoot, &pkg.Components[i]); err != nil {
			return nil, err
		}
	}

	definition, err := localizedDefinition(pkgLayout.PackageDefinition, pkg)
	if err != nil {
		return nil, err
	}
	if err := writeSourceDefinition(filepath.Join(stageDir, layout.ZarfYAML), definition); err != nil {
		return nil, fmt.Errorf("writing zarf.yaml: %w", err)
	}
	if err := publishOutput(stageDir, finalDir); err != nil {
		return nil, err
	}
	stageDir = ""

	return &Result{Source: opts.Source, OutputDir: opts.OutputDir}, nil
}

func warnRemoveAll(warnFn func(string, ...any), kind, path string) func() {
	return func() {
		if err := os.RemoveAll(path); err != nil {
			warn(warnFn, "failed to remove "+kind, "path", path, "error", err)
		}
	}
}

func warn(warnFn func(string, ...any), msg string, args ...any) {
	if warnFn != nil {
		warnFn(msg, args...)
	}
}

func localizeComponent(ctx context.Context, pkgLayout *layout.PackageLayout, outputDir, finalDir, tmpRoot string, component *v1alpha1.ZarfComponent) error {
	componentOutDir := filepath.Join(outputDir, component.Name)
	if err := os.MkdirAll(componentOutDir, helpers.ReadWriteExecuteUser); err != nil {
		return fmt.Errorf("creating component output directory: %w", err)
	}
	if len(component.Charts) > 0 {
		if err := localizeCharts(ctx, pkgLayout, componentOutDir, tmpRoot, component); err != nil {
			return err
		}
	}
	if len(component.Manifests) > 0 {
		if err := localizeManifests(ctx, pkgLayout, componentOutDir, tmpRoot, component); err != nil {
			return err
		}
	}
	if len(component.Files) > 0 {
		if err := localizeFiles(ctx, pkgLayout, componentOutDir, tmpRoot, component); err != nil {
			return err
		}
	}
	if len(component.Repos) > 0 {
		if err := localizeRepos(ctx, pkgLayout, componentOutDir, finalDir, tmpRoot, component); err != nil {
			return err
		}
	}
	if len(component.DataInjections) > 0 {
		if err := localizeDataInjections(ctx, pkgLayout, componentOutDir, tmpRoot, component); err != nil {
			return err
		}
	}
	if err := localizeImages(ctx, pkgLayout, outputDir, component); err != nil {
		return err
	}

	component.Actions.OnCreate = v1alpha1.ZarfComponentActionSet{}
	return nil
}

func createOutputStage(finalDir string) (string, error) {
	if err := validateOutputDir(finalDir); err != nil {
		return "", err
	}
	parent := filepath.Dir(finalDir)
	if err := os.MkdirAll(parent, helpers.ReadWriteExecuteUser); err != nil {
		return "", fmt.Errorf("creating output parent directory: %w", err)
	}
	stageDir, err := os.MkdirTemp(parent, "."+filepath.Base(finalDir)+"-*")
	if err != nil {
		return "", fmt.Errorf("creating output staging directory: %w", err)
	}
	return stageDir, nil
}

func publishOutput(stageDir, finalDir string) error {
	if err := validateOutputDir(finalDir); err != nil {
		return err
	}
	if _, err := os.Stat(finalDir); err == nil {
		if err := os.Remove(finalDir); err != nil {
			return fmt.Errorf("replacing empty output directory: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("checking output directory: %w", err)
	}
	if err := os.Rename(stageDir, finalDir); err != nil {
		return fmt.Errorf("publishing output directory: %w", err)
	}
	return nil
}
