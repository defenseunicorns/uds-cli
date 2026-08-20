// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"

	bundleinternal "github.com/defenseunicorns/uds-cli/internal/bundle"
	internalzarf "github.com/defenseunicorns/uds-cli/internal/zarf"
	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/zarf-dev/zarf/src/api/v1alpha1"
	"github.com/zarf-dev/zarf/src/pkg/packager"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"

	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeployWithSourceEnforcesDependencySafety(t *testing.T) {
	b := &spec.UDSBundle{
		UDS:      spec.UDSBlock{BundleAPIVersion: "uds.dev/v1alpha1"},
		Metadata: spec.Metadata{Name: "bundle"},
		Packages: []spec.Package{
			{Name: "core", Source: "oci://example.com/core:v1"},
			{Name: "app", Source: "oci://example.com/app:v1", DependsOn: []spec.PackageRef{{Name: "core"}}},
		},
	}

	_, err := Deploy(t.Context(), &DeploySource{Bundle: b}, DeployOptions{
		Config:   validValidationConfig(),
		Packages: []string{"app"},
	})

	require.ErrorContains(t, err, "unselected dependencies")
}

func TestPackageDeployHookReceivesBundleDirectory(t *testing.T) {
	var gotDir string
	hookErr := errors.New("stop after hook")
	_, err := Deploy(t.Context(), &DeploySource{
		BundlePath: "/tmp/bundle/bundle.uds.hcl",
		Bundle: &spec.UDSBundle{
			UDS:      spec.UDSBlock{BundleAPIVersion: "uds.dev/v1alpha1"},
			Metadata: spec.Metadata{Name: "bundle"},
			Packages: []spec.Package{{Name: "pkg", Source: "oci://example.com/pkg:v1"}},
		},
		Loader: staticPackageLayoutLoader{layout: &ZarfPackageLayout{}},
	}, DeployOptions{
		Config: validValidationConfig(),
		PackageDeployHooks: PackageDeployHooks{PreDeploy: func(_ context.Context, _ *spec.Package, _ *ZarfPackageLayout, opts *DeployPackageOptions) error {
			gotDir = opts.BundleDir
			return hookErr
		}},
	})

	require.ErrorIs(t, err, hookErr)
	assert.Equal(t, "/tmp/bundle", gotDir)
}

func TestApplyArtifactValuesFilesClearsMissingPackages(t *testing.T) {
	b := &spec.UDSBundle{Packages: []spec.Package{
		{Name: "present", ValuesFiles: []string{"stale.yaml"}},
		{Name: "missing", ValuesFiles: []string{"also-stale.yaml"}},
	}}

	err := applyArtifactValuesFiles(b, map[string][]string{
		"present": {filepath.Join("/tmp/work", "values", "present", "0.yaml")},
	}, filepath.Join("/tmp", "work"))

	require.NoError(t, err)
	assert.Equal(t, []string{filepath.Join("values", "present", "0.yaml")}, b.Packages[0].ValuesFiles)
	assert.Empty(t, b.Packages[1].ValuesFiles)
}

func TestRemoveUsesParsedBundleWithoutPath(t *testing.T) {
	_, err := Remove(t.Context(), &DeploySource{
		Bundle: &spec.UDSBundle{
			UDS:      spec.UDSBlock{BundleAPIVersion: "uds.dev/v1alpha1"},
			Metadata: spec.Metadata{},
			Packages: []spec.Package{{Name: "pkg", Source: "oci://example.com/pkg:v1"}},
		},
	}, RemoveOptions{
		Config: validValidationConfig(),
	})

	require.ErrorContains(t, err, "metadata.name")
}

func TestCountRemovalResults(t *testing.T) {
	removed, skipped := countRemovalResults([]RemovePackageResult{
		{Name: "removed", Status: RemovePackageStatusRemoved},
		{Name: "skipped", Status: RemovePackageStatusSkipped},
		{Name: "also-removed", Status: RemovePackageStatusRemoved},
	})

	assert.Equal(t, 2, removed)
	assert.Equal(t, 1, skipped)
}

func TestPublicPackageHookConvertsLayoutMutations(t *testing.T) {
	hooks := toZarfPackageHooks(PackageDeployHooks{PreDeploy: func(_ context.Context, _ *spec.Package, pkgLayout *ZarfPackageLayout, _ *DeployPackageOptions) error {
		pkgLayout.Pkg.Components[0].Images = nil
		pkgLayout.Pkg.Components[0].ImageArchives = nil
		pkgLayout.SetDeployedDigest("sha256:registry")
		return nil
	}})
	zarfLayout := &layout.PackageLayout{Pkg: v1alpha1.ZarfPackage{Components: []v1alpha1.ZarfComponent{{
		Name:          "main",
		Images:        []string{"example/image:v1"},
		ImageArchives: []v1alpha1.ImageArchive{{Path: "images.tar", Images: []string{"example/image:v1"}}},
	}}}}
	internalOpts := toZarfDeployPackageOptions(DeployPackageOptions{Config: validValidationConfig(), BundleDir: t.TempDir()})

	require.NoError(t, hooks.PreDeploy(t.Context(), &spec.Package{}, zarfLayout, &packager.DeployOptions{}, &internalOpts))
	assert.Empty(t, zarfLayout.Pkg.Components[0].Images)
	assert.Empty(t, zarfLayout.Pkg.Components[0].ImageArchives)
	assert.Equal(t, "sha256:registry", zarfLayout.Digest())
}

func TestPrepareDeploySourceFindsAdjacentDefaults(t *testing.T) {
	dir := t.TempDir()
	defaultsPath := filepath.Join(dir, "defaults.uds.hcl")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bundle.uds.hcl"), []byte(""), 0o600))
	require.NoError(t, os.WriteFile(defaultsPath, []byte("variables = {}"), 0o600))

	source, err := PrepareDeploySource(t.Context(), iostreams.IOStreams{}, dir, t.TempDir(), "")
	require.NoError(t, err)
	require.NoError(t, source.Close())
	assert.Equal(t, defaultsPath, source.DefaultsPath)
}

func TestAdjacentDefaultsPathPropagatesStatErrors(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "not-a-directory")
	require.NoError(t, os.WriteFile(parent, nil, 0o600))

	defaultsPath, err := bundleinternal.AdjacentDefaultsPath(parent)

	require.Error(t, err)
	assert.Empty(t, defaultsPath)
}

func TestPublicPackageHookPreservesPartialLayout(t *testing.T) {
	var partial bool
	hooks := toZarfPackageHooks(PackageDeployHooks{PreDeploy: func(_ context.Context, _ *spec.Package, pkgLayout *ZarfPackageLayout, _ *DeployPackageOptions) error {
		partial = pkgLayout.IsPartial
		return nil
	}})
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "zarf.yaml"), []byte("metadata:\n  name: test\n  version: 0.0.1\n  aggregateChecksum: e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855\ncomponents: []\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "checksums.txt"), nil, 0o600))
	layout, err := layout.LoadFromDir(t.Context(), dir, layout.PackageLayoutOptions{IsPartial: true, VerificationStrategy: layout.VerifyNever})
	require.NoError(t, err)

	internalOpts := toZarfDeployPackageOptions(DeployPackageOptions{Config: validValidationConfig(), BundleDir: dir})
	internalOpts.IsPartial = true
	require.NoError(t, hooks.PreDeploy(t.Context(), &spec.Package{}, layout, &packager.DeployOptions{}, &internalOpts))
	assert.True(t, partial)
}

func TestApplyPublicPackageLayoutPreservesPrivateComponentFields(t *testing.T) {
	manifests := []v1alpha1.ZarfManifest{{Name: "manifest.yaml"}}
	dst := &layout.PackageLayout{Pkg: v1alpha1.ZarfPackage{Components: []v1alpha1.ZarfComponent{{
		Name:      "main",
		Manifests: manifests,
		Charts:    []v1alpha1.ZarfChart{{Name: "chart"}},
	}}}}
	src := fromZarfPackageLayout(dst)

	require.NoError(t, applyPublicPackageLayout(dst, src))

	assert.Equal(t, manifests, dst.Pkg.Components[0].Manifests)
	assert.Equal(t, "chart", dst.Pkg.Components[0].Charts[0].Name)
}

func TestApplyPublicPackageLayoutPreservesFieldsWhenComponentsAreFiltered(t *testing.T) {
	dst := &layout.PackageLayout{Pkg: v1alpha1.ZarfPackage{Components: []v1alpha1.ZarfComponent{
		{
			Name:      "optional",
			Manifests: []v1alpha1.ZarfManifest{{Name: "optional.yaml"}},
		},
		{
			Name:  "required",
			Files: []v1alpha1.ZarfFile{{Target: "/opt/required"}},
		},
	}}}
	filtered := fromZarfPackageLayout(dst)
	filtered.Pkg.Components = filtered.Pkg.Components[1:]

	require.NoError(t, applyPublicPackageLayout(dst, filtered))

	require.Len(t, dst.Pkg.Components, 1)
	assert.Equal(t, "required", dst.Pkg.Components[0].Name)
	assert.Equal(t, "/opt/required", dst.Pkg.Components[0].Files[0].Target)
}

func TestApplyPublicPackageLayoutPreservesFieldsWhenComponentRenamed(t *testing.T) {
	dst := &layout.PackageLayout{Pkg: v1alpha1.ZarfPackage{Components: []v1alpha1.ZarfComponent{{
		Name:      "original",
		Manifests: []v1alpha1.ZarfManifest{{Name: "manifest.yaml"}},
	}}}}
	src := fromZarfPackageLayout(dst)
	src.Pkg.Components[0].Name = "renamed"

	require.NoError(t, applyPublicPackageLayout(dst, src))

	assert.Equal(t, "renamed", dst.Pkg.Components[0].Name)
	assert.Equal(t, "manifest.yaml", dst.Pkg.Components[0].Manifests[0].Name)
}

func TestApplyPublicPackageLayoutMatchesExternallyConstructedComponentByName(t *testing.T) {
	dst := &layout.PackageLayout{Pkg: v1alpha1.ZarfPackage{Components: []v1alpha1.ZarfComponent{{
		Name:  "main",
		Files: []v1alpha1.ZarfFile{{Target: "/opt/main"}},
	}}}}
	src := &ZarfPackageLayout{Pkg: ZarfPackage{Components: []ZarfPackageComponent{{Name: "main"}}}}

	require.NoError(t, applyPublicPackageLayout(dst, src))

	assert.Equal(t, "/opt/main", dst.Pkg.Components[0].Files[0].Target)
}

func TestApplyPublicPackageLayoutRejectsUnidentifiedComponentMutation(t *testing.T) {
	dst := &layout.PackageLayout{Pkg: v1alpha1.ZarfPackage{Components: []v1alpha1.ZarfComponent{{Name: "original"}}}}
	src := &ZarfPackageLayout{Pkg: ZarfPackage{Components: []ZarfPackageComponent{{Name: "new"}}}}

	err := applyPublicPackageLayout(dst, src)

	require.ErrorContains(t, err, "cannot be reconciled after public layout mutation")
}

func TestApplyPublicPackageLayoutRejectsDuplicateComponentNames(t *testing.T) {
	dst := &layout.PackageLayout{Pkg: v1alpha1.ZarfPackage{Components: []v1alpha1.ZarfComponent{
		{Name: "first"},
		{Name: "second"},
	}}}
	src := fromZarfPackageLayout(dst)
	src.Pkg.Components[0].Name = "duplicate"
	src.Pkg.Components[1].Name = "duplicate"

	err := applyPublicPackageLayout(dst, src)

	assert.ErrorContains(t, err, `component name "duplicate" appears more than once`)
}

func TestBundlePreDeployCopiesConfigMutations(t *testing.T) {
	config := &UDSBundleConfig{Options: &ConfigOptions{LogLevel: "info", Concurrency: 2}}
	internal := toZarfDeployOptions(DeployOptions{
		Config: config,
		BundleDeployHooks: BundleDeployHooks{
			PreDeploy: func(_ context.Context, _ *spec.UDSBundle, opts *DeployOptions) error {
				opts.Config.Options.LogLevel = "debug"
				opts.Config.Options.Concurrency = 4
				return nil
			},
		},
	}, nil)

	require.NoError(t, internal.BundleDeployHooks.PreDeploy(t.Context(), &spec.UDSBundle{}, &internal))
	assert.Equal(t, "debug", internal.Config.Options.LogLevel)
	assert.Equal(t, 4, internal.Config.Options.Concurrency)
}

func TestPackagePreDeployRejectsNilConfig(t *testing.T) {
	internalOpts := toZarfDeployPackageOptions(DeployPackageOptions{Config: validValidationConfig(), BundleDir: t.TempDir()})
	hooks := toZarfPackageHooks(PackageDeployHooks{
		PreDeploy: func(_ context.Context, _ *spec.Package, _ *ZarfPackageLayout, opts *DeployPackageOptions) error {
			opts.Config = nil
			return nil
		},
	})
	internalLayout := &layout.PackageLayout{}

	err := hooks.PreDeploy(t.Context(), &spec.Package{}, internalLayout, &packager.DeployOptions{}, &internalOpts)

	assert.Error(t, err)
}

func TestPublicPreDeployReceivesStagedDirectory(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "zarf.yaml"), []byte("metadata:\n  name: test\n  version: 0.0.1\n  aggregateChecksum: e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855\ncomponents: []\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "checksums.txt"), nil, 0o600))
	internalOpts := toZarfDeployPackageOptions(DeployPackageOptions{Config: validValidationConfig(), BundleDir: dir})
	internalOpts.PackageDeployHooks = toZarfPackageHooks(PackageDeployHooks{
		PreDeploy: func(_ context.Context, _ *spec.Package, pkgLayout *ZarfPackageLayout, _ *DeployPackageOptions) error {
			assert.Equal(t, dir, pkgLayout.Directory)
			return nil
		},
	})
	internalLayout, err := layout.LoadFromDir(t.Context(), dir, layout.PackageLayoutOptions{
		IsPartial:            true,
		VerificationStrategy: layout.VerifyNever,
	})
	require.NoError(t, err)

	err = internalOpts.PackageDeployHooks.PreDeploy(
		t.Context(), &spec.Package{}, internalLayout, &packager.DeployOptions{}, &internalOpts,
	)
	require.NoError(t, err)
}

func TestPublicPreDeployPreservesRename(t *testing.T) {
	internalOpts := toZarfDeployPackageOptions(DeployPackageOptions{Config: validValidationConfig(), BundleDir: t.TempDir()})
	internalOpts.PackageDeployHooks = toZarfPackageHooks(PackageDeployHooks{
		PreDeploy: func(_ context.Context, _ *spec.Package, pkgLayout *ZarfPackageLayout, _ *DeployPackageOptions) error {
			pkgLayout.Pkg.Components[0].Name = "renamed"
			return nil
		},
	})
	internalLayout := &layout.PackageLayout{Pkg: v1alpha1.ZarfPackage{Components: []v1alpha1.ZarfComponent{
		{Name: "original", Manifests: []v1alpha1.ZarfManifest{{Name: "manifest.yaml"}}},
	}}}

	err := internalOpts.PackageDeployHooks.PreDeploy(
		t.Context(), &spec.Package{}, internalLayout, &packager.DeployOptions{}, &internalOpts,
	)
	require.NoError(t, err)
	assert.Equal(t, "renamed", internalLayout.Pkg.Components[0].Name)
	assert.Equal(t, "manifest.yaml", internalLayout.Pkg.Components[0].Manifests[0].Name)
}

func TestPackageLayoutLoaderAdapterPreservesPartialLayout(t *testing.T) {
	dir := t.TempDir()
	missingFileDigest := sha256.Sum256([]byte("optional component"))
	checksums := []byte(hex.EncodeToString(missingFileDigest[:]) + " components/optional.tar\n")
	checksum := sha256.Sum256(checksums)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "zarf.yaml"), []byte("metadata:\n  name: test\n  version: 0.0.1\n  aggregateChecksum: "+hex.EncodeToString(checksum[:])+"\ncomponents: []\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "checksums.txt"), checksums, 0o600))

	loader := packageLayoutLoaderAdapter{loader: staticPackageLayoutLoader{layout: &ZarfPackageLayout{Directory: dir, IsPartial: true}}}
	_, _, err := loader.LoadPackageLayout(t.Context(), &spec.Package{Name: "test"}, dir, internalzarf.LoadOptions{})
	require.NoError(t, err)
}

func TestPackageLayoutLoaderAdapterRejectsCallerOwnedDirectory(t *testing.T) {
	stagingDir := t.TempDir()
	ownedDir := t.TempDir()
	loader := packageLayoutLoaderAdapter{loader: staticPackageLayoutLoader{layout: &ZarfPackageLayout{Directory: ownedDir}}}

	_, _, err := loader.LoadPackageLayout(t.Context(), &spec.Package{Name: "test"}, stagingDir, internalzarf.LoadOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be the supplied staging directory")
	assert.DirExists(t, ownedDir)
}

type staticPackageLayoutLoader struct{ layout *ZarfPackageLayout }

func (l staticPackageLayoutLoader) LoadPackageLayout(context.Context, *spec.Package, string, ZarfPackageLayoutLoadOptions) (*ZarfPackageLayout, error) {
	return l.layout, nil
}

func TestZarfRemoverRemoveBundleValidatesRemovalSafety(t *testing.T) {
	b := &spec.UDSBundle{
		UDS:      spec.UDSBlock{BundleAPIVersion: "uds.dev/v1alpha1"},
		Metadata: spec.Metadata{Name: "example"},
		Packages: []spec.Package{
			{Name: "core", Source: "oci://example/core:1"},
			{Name: "app", Source: "oci://example/app:1", DependsOn: []spec.PackageRef{{Name: "core"}}},
		},
	}

	result, err := newZarfRemover(iostreams.IOStreams{}).removeBundle(
		t.Context(), b, []string{"core"}, removePackageOptions{Config: newTestConfig()},
	)
	require.Error(t, err)
	assert.Nil(t, result)

	var dependencyErr *DependencyViolationError
	require.ErrorAs(t, err, &dependencyErr)
	assert.Equal(t, map[string][]string{"core": {"app"}}, dependencyErr.Violations)
}

func TestPublicAdaptersValidateOptionsBeforeOperationLogic(t *testing.T) {
	b := &spec.UDSBundle{
		Packages: []spec.Package{
			{Name: "core", Source: "oci://example/core:1"},
			{Name: "app", Source: "oci://example/app:1", DependsOn: []spec.PackageRef{{Name: "core"}}},
		},
	}

	tests := []struct {
		name string
		run  func() error
	}{
		{name: "pull bundle", run: func() error {
			_, err := Pull(t.Context(), "invalid", "", PullOptions{SkipSignatureVerification: true})
			return err
		}},
		{name: "push bundle", run: func() error {
			_, err := Push(t.Context(), "", "invalid", PushOptions{})
			return err
		}},
		{name: "deploy package", run: func() error {
			_, err := Deploy(t.Context(), &DeploySource{Bundle: b}, DeployOptions{})
			return err
		}},
		{name: "deploy bundle", run: func() error {
			_, err := Deploy(t.Context(), &DeploySource{Bundle: b}, DeployOptions{Packages: []string{"app"}})
			return err
		}},
		{name: "remove bundle", run: func() error {
			_, err := newZarfRemover(iostreams.IOStreams{}).removeBundle(
				t.Context(), b, []string{"core"}, removePackageOptions{},
			)
			return err
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.ErrorContains(t, tt.run(), "config is required")
		})
	}
}

func TestExtractedArtifactPackageLayoutLoaderPackageStagingRoot(t *testing.T) {
	tests := []struct {
		name   string
		ociDir string
		want   string
	}{
		{name: "empty OCI directory", want: ""},
		{name: "OCI directory", ociDir: "/workspace/oci", want: "/workspace"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loader := &extractedArtifactPackageLayoutLoader{loader: &internalzarf.ExtractedArtifactPackageLayoutLoader{OCIDir: tt.ociDir}}
			assert.Equal(t, tt.want, loader.PackageStagingRoot(t.Context()))
		})
	}
}
