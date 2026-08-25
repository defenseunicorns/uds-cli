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
	"github.com/zarf-dev/zarf/src/api"
	"github.com/zarf-dev/zarf/src/api/v1alpha1"
	"github.com/zarf-dev/zarf/src/api/v1beta1"
	"github.com/zarf-dev/zarf/src/pkg/packager"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"

	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newPackageLayout(pkg v1alpha1.ZarfPackage) *layout.PackageLayout {
	return &layout.PackageLayout{PackageDefinition: api.NewPackageDefinitionFromV1alpha1(pkg)}
}

func newV1beta1PackageLayout() *layout.PackageLayout {
	return &layout.PackageLayout{PackageDefinition: api.NewPackageDefinitionFromV1beta1(v1beta1.Package{
		APIVersion: v1beta1.APIVersion,
		Kind:       v1beta1.ZarfPackageConfig,
		Metadata:   v1beta1.PackageMetadata{Name: "beta-package"},
		Components: []v1beta1.Component{{
			Name: "main",
			ComponentSpec: v1beta1.ComponentSpec{
				Images: []v1beta1.Image{{Name: "example/image:v1", Source: "daemon"}},
				Import: v1beta1.ComponentImport{
					Local:  []v1beta1.ComponentImportLocal{{Path: "one.yaml"}, {Path: "two.yaml"}},
					Remote: []v1beta1.ComponentImportRemote{{URL: "oci://example.com/component"}},
				},
				Service: v1beta1.ServiceRegistry,
			},
		}},
	})}
}

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
		pkgLayout.PackageDefinition.RemoveImages()
		pkgLayout.SetDeployedDigest("sha256:registry")
		return nil
	}})
	zarfLayout := newPackageLayout(v1alpha1.ZarfPackage{Components: []v1alpha1.ZarfComponent{{
		Name:          "main",
		Images:        []string{"example/image:v1"},
		ImageArchives: []v1alpha1.ImageArchive{{Path: "images.tar", Images: []string{"example/image:v1"}}},
	}}})
	internalOpts := toZarfDeployPackageOptions(DeployPackageOptions{Config: validValidationConfig(), BundleDir: t.TempDir()})

	require.NoError(t, hooks.PreDeploy(t.Context(), &spec.Package{}, zarfLayout, &packager.DeployOptions{}, &internalOpts))
	assert.Empty(t, zarfLayout.AsV1alpha1().Components[0].Images)
	assert.Empty(t, zarfLayout.AsV1alpha1().Components[0].ImageArchives)
	assert.Equal(t, "sha256:registry", zarfLayout.Digest())
}

func TestPublicPackageHookPreservesV1beta1Fields(t *testing.T) {
	hooks := toZarfPackageHooks(PackageDeployHooks{PreDeploy: func(_ context.Context, _ *spec.Package, pkgLayout *ZarfPackageLayout, _ *DeployPackageOptions) error {
		pkg := pkgLayout.PackageDefinition.AsV1beta1()
		pkg.Components[0].Images[0].Name = "updated/image:v1"
		pkgLayout.PackageDefinition = api.NewPackageDefinitionFromV1beta1(pkg)
		return nil
	}})
	zarfLayout := newV1beta1PackageLayout()
	internalOpts := toZarfDeployPackageOptions(DeployPackageOptions{Config: validValidationConfig(), BundleDir: t.TempDir()})

	require.NoError(t, hooks.PreDeploy(t.Context(), &spec.Package{}, zarfLayout, &packager.DeployOptions{}, &internalOpts))

	component := zarfLayout.AsV1beta1().Components[0]
	assert.Equal(t, "updated/image:v1", component.Images[0].Name)
	assert.Equal(t, "daemon", component.Images[0].Source)
	assert.Len(t, component.Import.Local, 2)
	assert.Len(t, component.Import.Remote, 1)
	assert.Equal(t, v1beta1.ServiceRegistry, component.Service)
}

func TestPublicPackageLayoutLoaderPreservesV1beta1Fields(t *testing.T) {
	publicLayout := fromZarfPackageLayout(newV1beta1PackageLayout())
	converted, err := toZarfPackageLayoutForDeploy(publicLayout)
	require.NoError(t, err)

	component := converted.AsV1beta1().Components[0]
	assert.Equal(t, "daemon", component.Images[0].Source)
	assert.Len(t, component.Import.Local, 2)
	assert.Len(t, component.Import.Remote, 1)
	assert.Equal(t, v1beta1.ServiceRegistry, component.Service)
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

func TestPublicPackageHookPreservesPartialMetadata(t *testing.T) {
	var partial bool
	hooks := toZarfPackageHooks(PackageDeployHooks{PreDeploy: func(_ context.Context, _ *spec.Package, _ *ZarfPackageLayout, opts *DeployPackageOptions) error {
		partial = opts.IsPartial
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

func TestApplyPublicPackageLayoutUsesHookDefinition(t *testing.T) {
	manifests := []v1alpha1.ZarfManifest{{Name: "manifest.yaml"}}
	dst := newPackageLayout(v1alpha1.ZarfPackage{Components: []v1alpha1.ZarfComponent{{
		Name:      "main",
		Manifests: manifests,
		Charts:    []v1alpha1.ZarfChart{{Name: "chart"}},
	}}})
	src := fromZarfPackageLayout(dst)
	pkg := src.PackageDefinition.AsV1alpha1()
	pkg.Components = pkg.Components[:0]
	src.PackageDefinition = api.NewPackageDefinitionFromV1alpha1(pkg)

	require.NoError(t, applyPublicPackageLayout(dst, src))

	assert.Empty(t, dst.AsV1alpha1().Components)
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
			assert.Equal(t, dir, pkgLayout.dirPath)
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
			pkg := pkgLayout.PackageDefinition.AsV1alpha1()
			pkg.Components[0].Name = "renamed"
			pkgLayout.PackageDefinition = api.NewPackageDefinitionFromV1alpha1(pkg)
			return nil
		},
	})
	internalLayout := newPackageLayout(v1alpha1.ZarfPackage{Components: []v1alpha1.ZarfComponent{
		{Name: "original", Manifests: []v1alpha1.ZarfManifest{{Name: "manifest.yaml"}}},
	}})

	err := internalOpts.PackageDeployHooks.PreDeploy(
		t.Context(), &spec.Package{}, internalLayout, &packager.DeployOptions{}, &internalOpts,
	)
	require.NoError(t, err)
	assert.Equal(t, "renamed", internalLayout.AsV1alpha1().Components[0].Name)
	assert.Equal(t, "manifest.yaml", internalLayout.AsV1alpha1().Components[0].Manifests[0].Name)
}

func TestPackageLayoutLoaderAdapterPreservesStagedPrivateDeploymentData(t *testing.T) {
	dir := t.TempDir()
	const zarfYAML = `kind: ZarfPackageConfig
metadata:
  name: test
  version: 0.0.1
  aggregateChecksum: e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
components:
  - name: main
    required: true
    charts:
      - name: chart
        version: 1.0.0
        url: https://example.com/charts
    manifests:
      - name: manifests
        files:
          - manifest.yaml
    files:
      - source: file.txt
        target: /tmp/file.txt
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "zarf.yaml"), []byte(zarfYAML), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "checksums.txt"), nil, 0o600))

	loader := packageLayoutLoaderAdapter{loader: staticPackageLayoutLoader{
		layout: &ZarfPackageLayout{PackageDefinition: api.NewPackageDefinitionFromV1alpha1(v1alpha1.ZarfPackage{Components: []v1alpha1.ZarfComponent{{
			Name:      "main",
			Charts:    []v1alpha1.ZarfChart{{Name: "chart", Version: "1.0.0", URL: "https://example.com/charts"}},
			Manifests: []v1alpha1.ZarfManifest{{Name: "manifests", Files: []string{"manifest.yaml"}}},
			Files:     []v1alpha1.ZarfFile{{Source: "file.txt", Target: "/tmp/file.txt"}},
		}}})},
	}}
	result, err := loader.LoadPackageLayout(t.Context(), &spec.Package{Name: "test"}, dir, internalzarf.LoadOptions{IsPartial: true})
	require.NoError(t, err)
	components := result.Layout.AsV1alpha1().Components
	require.Len(t, components, 1)
	component := components[0]
	assert.Equal(t, "chart", component.Charts[0].Name)
	assert.Equal(t, "manifests", component.Manifests[0].Name)
	assert.Equal(t, "/tmp/file.txt", component.Files[0].Target)
}

func TestPackageLayoutLoaderAdapterPreservesPartialLayout(t *testing.T) {
	dir := t.TempDir()
	missingFileDigest := sha256.Sum256([]byte("optional component"))
	checksums := []byte(hex.EncodeToString(missingFileDigest[:]) + " components/optional.tar\n")
	checksum := sha256.Sum256(checksums)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "zarf.yaml"), []byte("metadata:\n  name: test\n  version: 0.0.1\n  aggregateChecksum: "+hex.EncodeToString(checksum[:])+"\ncomponents: []\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "checksums.txt"), checksums, 0o600))

	loader := packageLayoutLoaderAdapter{loader: staticPackageLayoutLoader{
		layout:    &ZarfPackageLayout{dirPath: dir},
		isPartial: true,
	}}
	result, err := loader.LoadPackageLayout(t.Context(), &spec.Package{Name: "test"}, dir, internalzarf.LoadOptions{})
	require.NoError(t, err)
	assert.True(t, result.IsPartial)
}

func TestPackageLayoutLoaderAdapterRequiresStagedPackage(t *testing.T) {
	loader := packageLayoutLoaderAdapter{loader: staticPackageLayoutLoader{
		layout:      &ZarfPackageLayout{},
		skipStaging: true,
	}}

	_, err := loader.LoadPackageLayout(t.Context(), &spec.Package{Name: "test"}, t.TempDir(), internalzarf.LoadOptions{})
	require.ErrorContains(t, err, "loading package layout staged")
}

func TestPackageLayoutLoaderAdapterUsesSuppliedStagingDirectory(t *testing.T) {
	stagingDir := t.TempDir()
	ownedDir := t.TempDir()
	loader := packageLayoutLoaderAdapter{loader: staticPackageLayoutLoader{layout: &ZarfPackageLayout{dirPath: ownedDir}}}

	result, err := loader.LoadPackageLayout(t.Context(), &spec.Package{Name: "test"}, stagingDir, internalzarf.LoadOptions{})
	require.NoError(t, err)
	assert.Equal(t, stagingDir, result.Layout.DirPath())
	assert.DirExists(t, ownedDir)
}

type staticPackageLayoutLoader struct {
	layout      *ZarfPackageLayout
	isPartial   bool
	skipStaging bool
}

func (l staticPackageLayoutLoader) LoadPackageLayout(_ context.Context, _ *spec.Package, dstDir string, _ ZarfPackageLayoutLoadOptions) (*ZarfPackageLayoutLoadResult, error) {
	zarfYAML := filepath.Join(dstDir, "zarf.yaml")
	_, err := os.Stat(zarfYAML)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if errors.Is(err, os.ErrNotExist) && !l.skipStaging {
		const contents = "kind: ZarfPackageConfig\nmetadata:\n  name: test\n  version: 0.0.1\n  aggregateChecksum: e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855\ncomponents: []\n"
		if err := os.WriteFile(zarfYAML, []byte(contents), 0o600); err != nil {
			return nil, err
		}
		if err := os.WriteFile(filepath.Join(dstDir, "checksums.txt"), nil, 0o600); err != nil {
			return nil, err
		}
	}
	return &ZarfPackageLayoutLoadResult{Layout: *l.layout, IsPartial: l.isPartial}, nil
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
