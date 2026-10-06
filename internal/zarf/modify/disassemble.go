// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package modify converts Zarf packages into recreatable local source and back.
package modify

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/defenseunicorns/pkg/helpers/v2"
	internalzarf "github.com/defenseunicorns/uds-cli/internal/zarf"
	"github.com/zarf-dev/zarf/src/api"
	"github.com/zarf-dev/zarf/src/api/v1alpha1"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
)

const componentsDir = "components"

// Disassemble converts one Zarf package into recreatable local source.
func Disassemble(ctx context.Context, opts Options) (string, error) {
	if err := opts.validate(); err != nil {
		return "", err
	}
	warnFn := opts.Warn
	type warning struct {
		message string
		args    []any
	}
	var warnings []warning
	opts.Warn = func(message string, args ...any) {
		warnings = append(warnings, warning{message: message, args: args})
	}
	outputDir, err := internalzarf.WithTempDir(opts.TmpDir, func() (string, error) {
		return disassemble(ctx, opts)
	})
	for _, warning := range warnings {
		warn(warnFn, warning.message, warning.args...)
	}
	return outputDir, err
}

func disassemble(ctx context.Context, opts Options) (string, error) {
	finalDir, err := filepath.Abs(opts.OutputDir)
	if err != nil {
		return "", fmt.Errorf("resolving output directory: %w", err)
	}
	if err := validateOutputDir(finalDir); err != nil {
		return "", err
	}

	tmpRoot, err := os.MkdirTemp(opts.TmpDir, "uds-dev-disassemble-*")
	if err != nil {
		return "", fmt.Errorf("creating temporary directory: %w", err)
	}
	defer removeAllWithWarning(opts.Warn, "temporary directory", tmpRoot)

	pkgLayout, err := loadPackageSource(ctx, opts)
	if err != nil {
		return "", fmt.Errorf("loading source package: %w", err)
	}
	defer func() {
		if err := pkgLayout.Cleanup(); err != nil {
			warn(opts.Warn, "failed to remove package layout", "path", pkgLayout.DirPath(), "error", err)
		}
	}()

	pkg := pkgLayout.Definition()
	if pkg.Build.Differential {
		return "", errors.New("differential Zarf packages do not contain complete recreatable source")
	}
	if pkg.Metadata.Architecture == v1alpha1.SkeletonArch {
		return "", errors.New("skeleton Zarf packages do not contain complete recreatable source")
	}
	buildArchitecture := pkg.Build.Architecture
	buildFlavor := pkg.Build.Flavor
	if strings.TrimSpace(buildArchitecture) == "" {
		return "", errors.New("complete Zarf package build architecture is required")
	}
	pkg.Build = api.BuildData{}
	normalizeMetadata(&pkg.Metadata)
	pkg.Metadata.Architecture = buildArchitecture

	stageDir, err := createOutputStage(finalDir)
	if err != nil {
		return "", err
	}
	defer removeAllWithWarning(opts.Warn, "output staging directory", stageDir)

	if err := localizePackageLevelAssets(ctx, pkgLayout, stageDir, tmpRoot, &pkg); err != nil {
		return "", err
	}
	for i := range pkg.Components {
		componentTmpRoot := filepath.Join(tmpRoot, "disassemble-components", pkg.Components[i].Name)
		if err := os.MkdirAll(componentTmpRoot, helpers.ReadWriteExecuteUser); err != nil {
			return "", fmt.Errorf("creating component temporary directory: %w", err)
		}
		if err := localizeComponent(ctx, pkgLayout, stageDir, finalDir, componentTmpRoot, &pkg.Components[i]); err != nil {
			return "", err
		}
	}

	definitionPath := filepath.Join(stageDir, layout.ZarfYAML)
	if err := writeSourceDefinition(definitionPath, pkg); err != nil {
		return "", fmt.Errorf("writing zarf.yaml: %w", err)
	}
	if err := writeDisassemblyMetadata(stageDir, buildArchitecture, buildFlavor); err != nil {
		return "", err
	}
	if err := publishOutput(stageDir, finalDir); err != nil {
		return "", err
	}
	warn(opts.Warn, "use disassembled source only as a last resort; port all edits to the upstream source as soon as possible")

	return opts.OutputDir, nil
}

func removeAllWithWarning(warnFn func(string, ...any), kind, path string) {
	if err := os.RemoveAll(path); err != nil {
		warn(warnFn, "failed to remove "+kind, "path", path, "error", err)
	}
}

func warn(warnFn func(string, ...any), msg string, args ...any) {
	if warnFn != nil {
		warnFn(msg, args...)
	}
}

func localizeComponent(ctx context.Context, pkgLayout *layout.PackageLayout, outputDir, finalDir, tmpRoot string, component *api.Component) error {
	componentOutDir := filepath.Join(outputDir, componentsDir, component.Name)
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
	if len(component.Repositories) > 0 {
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

	component.Actions.OnCreate = api.ActionSet{}
	return nil
}

func componentSourcePath(componentName, rel string) string {
	return filepath.ToSlash(filepath.Join(componentsDir, componentName, rel))
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
