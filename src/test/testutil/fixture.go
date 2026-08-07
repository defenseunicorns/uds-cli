// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package testutil

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/types"
	zarfUtils "github.com/zarf-dev/zarf/src/pkg/utils"
	"sigs.k8s.io/yaml"
)

// TestDataPath returns the immutable source path for test data.
func TestDataPath(relative string) string {
	if !filepath.IsLocal(relative) {
		panic("fixture path must be relative to src/test")
	}

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		panic("unable to locate test data directory")
	}
	return filepath.Join(filepath.Dir(file), "..", relative)
}

// CopyFixture copies immutable test data to a test-private workspace.
func CopyFixture(t *testing.T, relative string) string {
	t.Helper()

	source := TestDataPath(relative)
	destination := filepath.Join(t.TempDir(), relative)
	info, err := os.Stat(source)
	if err != nil {
		t.Fatalf("stat fixture %q: %v", relative, err)
	}

	if info.IsDir() {
		if err := filepath.WalkDir(source, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			relativePath, err := filepath.Rel(source, path)
			if err != nil {
				return err
			}
			return copyFixtureEntry(path, filepath.Join(destination, relativePath), entry)
		}); err != nil {
			t.Fatalf("copy fixture %q: %v", relative, err)
		}
		return destination
	}

	if err := copyFixtureFile(source, destination, info.Mode()); err != nil {
		t.Fatalf("copy fixture %q: %v", relative, err)
	}
	return destination
}

func copyTree(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		return copyFixtureEntry(path, filepath.Join(destination, relative), entry)
	})
}

// PrepareBundleForNamespace copies a bundle and its local package sources into
// a test-private workspace, then scopes every package deployment to namespace.
func PrepareBundleForNamespace(t *testing.T, fixture, namespace string) string {
	t.Helper()

	if !filepath.IsLocal(fixture) {
		t.Fatalf("bundle fixture path must be relative to src/test: %q", fixture)
	}
	sourceBundleDir := TestDataPath(fixture)
	workspace := t.TempDir()
	bundleDir := filepath.Join(workspace, fixture)
	if err := copyTree(sourceBundleDir, bundleDir); err != nil {
		t.Fatalf("copy bundle fixture %q: %v", fixture, err)
	}

	bundlePath := filepath.Join(bundleDir, config.BundleYAML)
	var bundle types.UDSBundle
	contents, err := os.ReadFile(bundlePath)
	if err != nil {
		t.Fatalf("read copied bundle fixture %q: %v", fixture, err)
	}
	if err := yaml.Unmarshal(contents, &bundle); err != nil {
		t.Fatalf("read copied bundle fixture %q: %v", fixture, err)
	}
	for index := range bundle.Packages {
		pkg := &bundle.Packages[index]
		pkg.Namespace = namespace
		if pkg.Path == "" {
			continue
		}
		sourcePackage := filepath.Clean(filepath.Join(sourceBundleDir, pkg.Path))
		relativePackage, err := filepath.Rel(TestDataPath("."), sourcePackage)
		if err != nil || !filepath.IsLocal(relativePackage) {
			t.Fatalf("local package path %q escapes src/test", pkg.Path)
		}
		destinationPackage := filepath.Clean(filepath.Join(bundleDir, pkg.Path))
		if err := copyTree(sourcePackage, destinationPackage); err != nil {
			t.Fatalf("copy local package %q: %v", pkg.Path, err)
		}
	}
	if err := zarfUtils.WriteYaml(bundlePath, &bundle, 0o600); err != nil {
		t.Fatalf("write namespace-scoped bundle fixture %q: %v", fixture, err)
	}
	return bundleDir
}

// CreatePackageForTest creates one Zarf package archive in a test-local output
// directory and returns its path.
func CreatePackageForTest(t *testing.T, source, outputDir string) string {
	t.Helper()

	if err := os.MkdirAll(outputDir, 0o700); err != nil {
		t.Fatalf("create package output directory: %v", err)
	}
	result := RunCLI(t, CommandOptions{Dir: outputDir},
		"zarf", "package", "create", source,
		"--confirm", "--skip-sbom", "--output", outputDir,
		"--tmpdir", filepath.Join(outputDir, "tmp"),
		"--cache", filepath.Join(outputDir, "cache"),
	)
	if result.Err != nil {
		t.Fatalf("create test-local Zarf package from %q: %v\n%s", source, result.Err, result.Stderr)
	}
	archives, err := filepath.Glob(filepath.Join(outputDir, "zarf-package-*.tar.zst"))
	if err != nil {
		t.Fatalf("locate created Zarf package: %v", err)
	}
	if len(archives) != 1 {
		t.Fatalf("expected one created Zarf package in %q, found %d", outputDir, len(archives))
	}
	return archives[0]
}

func copyFixtureEntry(source, destination string, entry os.DirEntry) error {
	if entry.IsDir() {
		return os.MkdirAll(destination, 0o700)
	}
	if entry.Type()&os.ModeSymlink != 0 {
		return &os.PathError{Op: "copy fixture", Path: source, Err: os.ErrInvalid}
	}

	info, err := entry.Info()
	if err != nil {
		return err
	}
	return copyFixtureFile(source, destination, info.Mode())
}

func copyFixtureFile(source, destination string, mode os.FileMode) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()

	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return err
	}
	output, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode.Perm()|0o600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}
