// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build library

package bundle_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	oras "oras.land/oras-go/v2"
	"oras.land/oras-go/v2/registry/remote"
)

type recordingReadOnlyTarget struct {
	oras.ReadOnlyTarget
	fetched []ocispec.Descriptor
}

func (t *recordingReadOnlyTarget) Fetch(ctx context.Context, desc ocispec.Descriptor) (io.ReadCloser, error) {
	t.fetched = append(t.fetched, desc)
	return t.ReadOnlyTarget.Fetch(ctx, desc)
}

func TestPrepareDeploySourceFromReadOnlyTarget(t *testing.T) {
	fixture := createLibraryBundle(t)
	registryHost := startLibraryRegistry(t)
	ref := fmt.Sprintf("%s/library/fixture:v1.0.0", registryHost)
	_, err := bundle.Push(t.Context(), fixture.ArtifactPath, ref, bundle.PushOptions{Config: libraryRegistryConfig(fixture.Config)})
	require.NoError(t, err)

	repository, err := remote.NewRepository(registryHost + "/library/fixture")
	require.NoError(t, err)
	repository.PlainHTTP = true
	target := &recordingReadOnlyTarget{ReadOnlyTarget: repository}
	loader := &staticLoaderImpl{}
	workspace := t.TempDir()
	source, err := bundle.PrepareDeploySourceFromTarget(t.Context(), iostreams.IOStreams{}, bundle.TargetDeploySourceOptions{
		Target: target, Reference: "v1.0.0", Loader: loader, TmpDir: workspace, Architecture: runtime.GOARCH,
	})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, source.Close()) })
	assert.Same(t, loader, source.Loader)
	assert.True(t, filepath.IsAbs(source.BundlePath))
	assert.FileExists(t, source.BundlePath)
	require.NotNil(t, source.Bundle)
	assert.Equal(t, "library-fixture", source.Bundle.Metadata.Name)
	assert.FileExists(t, source.DefaultsPath)
	defaults, err := os.ReadFile(source.DefaultsPath)
	require.NoError(t, err)
	assert.Contains(t, string(defaults), `fixture = "default"`)
	require.Len(t, source.Bundle.Packages, 2)
	require.Len(t, source.Bundle.Packages[1].ValuesFiles, 1)
	valuesPath := filepath.Join(filepath.Dir(source.BundlePath), source.Bundle.Packages[1].ValuesFiles[0])
	values, err := os.ReadFile(valuesPath)
	require.NoError(t, err)
	assert.Equal(t, "replicas: 1\n", string(values))
	archiveSource, err := bundle.PrepareDeploySource(t.Context(), iostreams.IOStreams{}, fixture.ArtifactPath, t.TempDir(), runtime.GOARCH)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, archiveSource.Close()) })
	assert.Equal(t, archiveSource.Bundle, source.Bundle)
	archiveDefaults, err := os.ReadFile(archiveSource.DefaultsPath)
	require.NoError(t, err)
	assert.Equal(t, archiveDefaults, defaults)
	archiveValuesPath := filepath.Join(filepath.Dir(archiveSource.BundlePath), archiveSource.Bundle.Packages[1].ValuesFiles[0])
	archiveValues, err := os.ReadFile(archiveValuesPath)
	require.NoError(t, err)
	assert.Equal(t, archiveValues, values)

	definitionManifestFetches := 0
	hclFetches := 0
	for _, desc := range target.fetched {
		assert.NotEqual(t, "application/vnd.defenseunicorns.zarf.layer.v1", desc.MediaType)
		assert.Empty(t, desc.Annotations["uds.dev/package.name"])
		if desc.ArtifactType == "application/vnd.defenseunicorns.uds.bundle.definition.v1" {
			definitionManifestFetches++
		}
		if desc.MediaType == "application/vnd.defenseunicorns.uds.bundle.hcl.v1" && desc.Annotations[ocispec.AnnotationTitle] == "bundle.uds.hcl" {
			hclFetches++
		}
	}
	assert.Equal(t, 1, definitionManifestFetches, "definition manifest should be fetched once")
	assert.Equal(t, 1, hclFetches, "bundle HCL should be fetched once")
	otherArchitecture := "amd64"
	if runtime.GOARCH == "amd64" {
		otherArchitecture = "arm64"
	}
	failureWorkspace := t.TempDir()
	_, err = bundle.PrepareDeploySourceFromTarget(t.Context(), iostreams.IOStreams{}, bundle.TargetDeploySourceOptions{
		Target: target, Reference: "v1.0.0", Loader: loader, TmpDir: failureWorkspace, Architecture: otherArchitecture,
	})
	require.ErrorIs(t, err, bundle.ErrPrepareDeploySource)
	assert.ErrorContains(t, err, "no bundle for architecture")
	failureEntries, err := os.ReadDir(failureWorkspace)
	require.NoError(t, err)
	assert.Empty(t, failureEntries)

	stopped := errors.New("stop before package deployment")
	_, err = bundle.Deploy(t.Context(), source, bundle.DeployOptions{
		Config: fixture.Config,
		BundleDeployHooks: bundle.BundleDeployHooks{PreDeploy: func(_ context.Context, _ *spec.UDSBundle, opts *bundle.DeployOptions) error {
			assert.Equal(t, "default", opts.Config.Variables["fixture"])
			return stopped
		}},
	})
	require.ErrorIs(t, err, stopped)

	_, err = bundle.Deploy(t.Context(), source, bundle.DeployOptions{Config: fixture.Config, Resume: true})
	require.ErrorIs(t, err, bundle.ErrResumeSourceNotPrepared)
	loader.err = errors.New("package loader called")
	_, err = bundle.Deploy(t.Context(), source, bundle.DeployOptions{Config: fixture.Config})
	require.ErrorIs(t, err, loader.err)
	partialLayoutReached := errors.New("partial layout reached pre-deploy")
	source.Loader = &contractLoader{load: func(ctx context.Context, pkg *spec.Package, dst string, opts bundle.ZarfPackageLayoutLoadOptions) (*bundle.ZarfPackageLayoutLoadResult, error) {
		return archiveSource.Loader.LoadPackageLayout(ctx, pkg, dst, opts)
	}}
	_, err = bundle.Deploy(t.Context(), source, bundle.DeployOptions{
		Config:   fixture.Config,
		Packages: []string{source.Bundle.Packages[0].Name},
		PackageDeployHooks: bundle.PackageDeployHooks{PreDeploy: func(_ context.Context, _ *spec.Package, layout *bundle.ZarfPackageLayout, opts *bundle.DeployPackageOptions) error {
			assert.True(t, opts.IsPartial)
			assert.Equal(t, "base", layout.PackageDefinition.Metadata.Name)
			return partialLayoutReached
		}},
	})
	require.ErrorIs(t, err, partialLayoutReached)
	source.Loader = nil
	_, err = bundle.Deploy(t.Context(), source, bundle.DeployOptions{Config: fixture.Config})
	require.ErrorIs(t, err, bundle.ErrPackageLayoutLoaderRequired)

	require.NoError(t, source.Close())
	require.NoError(t, source.Close())
	assert.NoFileExists(t, source.BundlePath)
	assert.NoFileExists(t, source.DefaultsPath)
	assert.NoFileExists(t, valuesPath)
	entries, err := os.ReadDir(workspace)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestPrepareDeploySourceFromTargetValidatesInputsAndCleansFailures(t *testing.T) {
	loader := &staticLoaderImpl{}
	_, err := bundle.PrepareDeploySourceFromTarget(t.Context(), iostreams.IOStreams{}, bundle.TargetDeploySourceOptions{})
	require.ErrorIs(t, err, bundle.ErrSourceRequired)
	_, err = bundle.PrepareDeploySourceFromTarget(t.Context(), iostreams.IOStreams{}, bundle.TargetDeploySourceOptions{Target: &recordingReadOnlyTarget{}})
	require.ErrorIs(t, err, bundle.ErrSourceRequired)
	_, err = bundle.PrepareDeploySourceFromTarget(t.Context(), iostreams.IOStreams{}, bundle.TargetDeploySourceOptions{Target: &recordingReadOnlyTarget{}, Reference: "v1"})
	require.ErrorIs(t, err, bundle.ErrPackageLayoutLoaderRequired)

	workspace := t.TempDir()
	_, err = bundle.PrepareDeploySourceFromTarget(t.Context(), iostreams.IOStreams{}, bundle.TargetDeploySourceOptions{
		Target: &failingReadOnlyTarget{}, Reference: "v1", Loader: loader, TmpDir: workspace,
	})
	require.ErrorIs(t, err, bundle.ErrPrepareDeploySource)
	entries, err := os.ReadDir(workspace)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

type failingReadOnlyTarget struct{ oras.ReadOnlyTarget }

func (f *failingReadOnlyTarget) Resolve(context.Context, string) (ocispec.Descriptor, error) {
	return ocispec.Descriptor{}, errors.New("target unavailable")
}

var _ oras.ReadOnlyTarget = (*recordingReadOnlyTarget)(nil)
var _ oras.ReadOnlyTarget = (*failingReadOnlyTarget)(nil)
