// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package disassemble

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/defenseunicorns/pkg/helpers/v2"
	packageoci "github.com/defenseunicorns/pkg/oci"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/api/v1alpha1"
	"github.com/zarf-dev/zarf/src/api/v1beta1"
	"github.com/zarf-dev/zarf/src/pkg/archive"
	"github.com/zarf-dev/zarf/src/pkg/packager/assemble"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
	"github.com/zarf-dev/zarf/src/pkg/packager/load"
	zarfschema "github.com/zarf-dev/zarf/src/pkg/schema"
	"github.com/zarf-dev/zarf/src/pkg/zoci"
	zarftypes "github.com/zarf-dev/zarf/src/types"
	"oras.land/oras-go/v2/content"
	contentoci "oras.land/oras-go/v2/content/oci"
)

func TestDisassembleRoundTripsThroughZarfOffline(t *testing.T) {
	sourceDir := prepareRoundTripFixture(t)
	resolved, err := load.PackageDefinition(t.Context(), sourceDir, load.DefinitionOptions{SkipVersionCheck: true})
	require.NoError(t, err)
	pkgLayout, err := assemble.AssemblePackage(t.Context(), resolved, sourceDir, assemble.AssembleOptions{
		SkipSBOM: true, OCIConcurrency: 1, CachePath: t.TempDir(),
	})
	require.NoError(t, err)
	defer func() { require.NoError(t, pkgLayout.Cleanup()) }()
	archivePath, err := pkgLayout.Archive(t.Context(), t.TempDir(), 0)
	require.NoError(t, err)

	outputDir := filepath.Join(t.TempDir(), "disassembled%source")
	result, err := Disassemble(t.Context(), Options{
		Source: archivePath, OutputDir: outputDir,
		Architecture: "amd64", TmpDir: t.TempDir(), Concurrency: 1,
	})
	require.NoError(t, err)
	assert.Equal(t, outputDir, result.OutputDir)

	generated, err := load.PackageDefinition(t.Context(), outputDir, load.DefinitionOptions{SkipVersionCheck: true})
	require.NoError(t, err)
	pkg := generated.PackageDefinition.AsV1alpha1()
	assert.Equal(t, "roundtrip", pkg.Metadata.Name)
	assert.Equal(t, "1.2.3-disassembled", pkg.Metadata.Version)
	require.Len(t, pkg.Components, 1)
	require.Len(t, pkg.Components[0].Charts, 1)
	chart := pkg.Components[0].Charts[0]
	assert.Equal(t, "app", chart.Name)
	assert.Equal(t, "1.0.0", chart.Version)
	assert.Empty(t, chart.URL)
	assert.Contains(t, chart.LocalPath, "app-1.0.0.tgz")
	require.FileExists(t, filepath.Join(outputDir, chart.LocalPath))
	require.Len(t, chart.ValuesFiles, 2)
	assert.Contains(t, chart.ValuesFiles[0], filepath.ToSlash("components/app/values/app/0-chart.yaml"))
	assert.Contains(t, chart.ValuesFiles[1], filepath.ToSlash("components/app/values/app/1-production-values.yaml"))
	assert.Len(t, chart.TemplatedValuesFiles, 1)
	require.Len(t, pkg.Components[0].Manifests, 1)
	manifest := pkg.Components[0].Manifests[0]
	assert.Len(t, manifest.Files, 1)
	require.Len(t, manifest.Kustomizations, 1)
	assert.True(t, manifest.IsTemplate())
	rendered, err := os.ReadFile(filepath.Join(outputDir, manifest.Kustomizations[0], "rendered.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(rendered), "{{ $labels.instance }}")
	assert.NotEqual(t, pkg.Components[0].Files[0].Source, pkg.Components[0].Files[1].Source)
	assert.NotEqual(t, pkg.Components[0].DataInjections[0].Source, pkg.Components[0].DataInjections[1].Source)
	require.Len(t, pkg.Components[0].Repos, 1)
	repoSource, err := url.Parse(pkg.Components[0].Repos[0])
	require.NoError(t, err)
	assert.Contains(t, repoSource.Path, filepath.ToSlash(outputDir))
	assert.Equal(t, []string{layout.ValuesYAML}, pkg.Values.Files)
	assert.Equal(t, layout.ValuesSchema, pkg.Values.Schema)
	assert.Equal(t, "documentation/guide.md", pkg.Documentation["guide"])

	reassembled, err := assemble.AssemblePackage(t.Context(), generated, outputDir, assemble.AssembleOptions{
		SkipSBOM: true, OCIConcurrency: 1, CachePath: t.TempDir(),
	})
	require.NoError(t, err)
	defer func() { require.NoError(t, reassembled.Cleanup()) }()
	assert.Equal(t, "roundtrip", reassembled.AsV1alpha1().Metadata.Name)
	assert.Equal(t, "1.2.3-disassembled", reassembled.AsV1alpha1().Metadata.Version)
}

func TestDisassemblePullsOCIPackage(t *testing.T) {
	server := httptest.NewServer(registry.New())
	t.Cleanup(server.Close)

	sourceDir := prepareRoundTripFixture(t)
	resolved, err := load.PackageDefinition(t.Context(), sourceDir, load.DefinitionOptions{SkipVersionCheck: true})
	require.NoError(t, err)
	pkgLayout, err := assemble.AssemblePackage(t.Context(), resolved, sourceDir, assemble.AssembleOptions{SkipSBOM: true, OCIConcurrency: 1, CachePath: t.TempDir()})
	require.NoError(t, err)
	defer func() { require.NoError(t, pkgLayout.Cleanup()) }()
	ref := strings.TrimPrefix(server.URL, "http://") + "/test/disassemble:1.0.0"
	remote, err := zoci.NewRemoteWithOptions(t.Context(), ref, ocispec.Platform{Architecture: "amd64", OS: packageoci.MultiOS}, zoci.RemoteClientOptions{
		RemoteOptions: zarftypes.RemoteOptions{PlainHTTP: true},
	})
	require.NoError(t, err)
	_, err = remote.PushPackage(t.Context(), pkgLayout, zoci.PublishOptions{Retries: 1, OCIConcurrency: 1})
	require.NoError(t, err)

	outputDir := filepath.Join(t.TempDir(), "output")
	_, err = Disassemble(t.Context(), Options{
		Source: "oci://" + ref, OutputDir: outputDir, Architecture: "amd64", PlainHTTP: true, TmpDir: t.TempDir(), Concurrency: 1,
	})
	require.NoError(t, err)
	generated, err := load.PackageDefinition(t.Context(), outputDir, load.DefinitionOptions{SkipVersionCheck: true})
	require.NoError(t, err)
	reassembled, err := assemble.AssemblePackage(t.Context(), generated, outputDir, assemble.AssembleOptions{SkipSBOM: true, OCIConcurrency: 1, CachePath: t.TempDir()})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, reassembled.Cleanup()) })
}

func TestDisassemblePreservesV1beta1Definition(t *testing.T) {
	sourceDir := copyFixture(t, "v1beta1")
	resolved, err := load.PackageDefinition(t.Context(), sourceDir, load.DefinitionOptions{Flavor: "offline", SkipVersionCheck: true})
	require.NoError(t, err)
	pkgLayout, err := assemble.AssemblePackage(t.Context(), resolved, sourceDir, assemble.AssembleOptions{Flavor: "offline", SkipSBOM: true})
	require.NoError(t, err)
	defer func() { require.NoError(t, pkgLayout.Cleanup()) }()
	archivePath, err := pkgLayout.Archive(t.Context(), t.TempDir(), 0)
	require.NoError(t, err)

	outputDir := filepath.Join(t.TempDir(), "beta-output")
	_, err = Disassemble(t.Context(), Options{
		Source: archivePath, OutputDir: outputDir,
		Architecture: "amd64", TmpDir: t.TempDir(),
	})
	require.NoError(t, err)
	generated, err := load.PackageDefinition(t.Context(), outputDir, load.DefinitionOptions{SkipVersionCheck: true})
	generatedYAML, readErr := os.ReadFile(filepath.Join(outputDir, layout.ZarfYAML))
	require.NoError(t, readErr)
	require.NoErrorf(t, err, "generated zarf.yaml:\n%s", generatedYAML)
	assert.Equal(t, v1beta1.APIVersion, generated.PackageDefinition.OriginalAPIVersion())
	generatedBeta := generated.PackageDefinition.AsV1beta1()
	assert.Equal(t, "2.0.0-disassembled", generatedBeta.Metadata.Version)
	require.Len(t, generatedBeta.Components, 1)
	component := generatedBeta.Components[0]
	assert.Equal(t, v1beta1.ServiceAgent, component.Service)
	require.Len(t, component.Manifests, 1)
	require.NotNil(t, component.Manifests[0].Kustomize)
	assert.Contains(t, component.Manifests[0].Kustomize.Files[0], "components/app/manifests/raw/kustomization-0")
	assert.False(t, component.Manifests[0].Kustomize.AllowAnyDirectory)
	assert.False(t, component.Manifests[0].Kustomize.EnablePlugins)
	assert.True(t, component.Manifests[0].EnableTemplating)
	assert.Empty(t, component.Selector.Flavor)

	reassembled, err := assemble.AssemblePackage(t.Context(), generated, outputDir, assemble.AssembleOptions{SkipSBOM: true})
	require.NoError(t, err)
	defer func() { require.NoError(t, reassembled.Cleanup()) }()
	assert.Equal(t, v1beta1.APIVersion, reassembled.PackageDefinition.OriginalAPIVersion())
}

// Disassembly selectively rewrites package source fields instead of round-tripping
// definitions wholesale. Fingerprinting Zarf's source schemas makes every change
// require an explicit preservation or localization review. Post-create provenance
// files and Zarf's internal package layout are intentionally outside this boundary.
func TestZarfPackageSchemaChangesRequireDisassemblyReview(t *testing.T) {
	tests := []struct {
		name   string
		schema []byte
		want   string
	}{
		{name: "v1alpha1", schema: zarfschema.GetV1Alpha1Schema(), want: "e46b466b366ba42fa171de0cf27302ced24c3b3416bcbaa8a1da93c2383dfd1b"},
		{name: "v1beta1", schema: zarfschema.GetV1Beta1Schema(), want: "ff49e63d52cbce0f2c795537e418d747541a5081d97af65ab6f131331cbfec50"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			digest := canonicalSchemaDigest(t, tc.schema)
			if tc.want != digest {
				t.Fatalf("Zarf %s package source schema changed: review each new or modified field for preservation or localization before updating the fingerprint to %s", tc.name, digest)
			}
		})
	}
}

func TestDisassembleClearsResolvedFlavorSelectors(t *testing.T) {
	sourceDir := copyFixture(t, "flavored")
	resolved, err := load.PackageDefinition(t.Context(), sourceDir, load.DefinitionOptions{Flavor: "offline", SkipVersionCheck: true})
	require.NoError(t, err)
	pkgLayout, err := assemble.AssemblePackage(t.Context(), resolved, sourceDir, assemble.AssembleOptions{Flavor: "offline", SkipSBOM: true})
	require.NoError(t, err)
	defer func() { require.NoError(t, pkgLayout.Cleanup()) }()
	archivePath, err := pkgLayout.Archive(t.Context(), t.TempDir(), 0)
	require.NoError(t, err)

	outputDir := filepath.Join(t.TempDir(), "output")
	_, err = Disassemble(t.Context(), Options{Source: archivePath, OutputDir: outputDir, Architecture: "amd64", TmpDir: t.TempDir()})
	require.NoError(t, err)
	generated, err := load.PackageDefinition(t.Context(), outputDir, load.DefinitionOptions{SkipVersionCheck: true})
	require.NoError(t, err)
	require.Len(t, generated.PackageDefinition.AsV1alpha1().Components, 1)
	assert.Empty(t, generated.PackageDefinition.AsV1alpha1().Components[0].Only.Flavor)

	reassembled, err := assemble.AssemblePackage(t.Context(), generated, outputDir, assemble.AssembleOptions{SkipSBOM: true})
	require.NoError(t, err)
	defer func() { require.NoError(t, reassembled.Cleanup()) }()
}

func TestDisassembleRoundTripsImagesOffline(t *testing.T) {
	const image = "registry.invalid/offline/app:v1"
	sourceDir := copyFixture(t, "offline-image")
	imageArchive := filepath.Join(sourceDir, "images.tar")
	writeImageArchive(t, imageArchive, image)
	resolved, err := load.PackageDefinition(t.Context(), sourceDir, load.DefinitionOptions{SkipVersionCheck: true})
	require.NoError(t, err)
	pkgLayout, err := assemble.AssemblePackage(t.Context(), resolved, sourceDir, assemble.AssembleOptions{SkipSBOM: true})
	require.NoError(t, err)
	defer func() { require.NoError(t, pkgLayout.Cleanup()) }()
	archivePath, err := pkgLayout.Archive(t.Context(), t.TempDir(), 0)
	require.NoError(t, err)

	outputDir := filepath.Join(t.TempDir(), "output")
	_, err = Disassemble(t.Context(), Options{Source: archivePath, OutputDir: outputDir, Architecture: "amd64", TmpDir: t.TempDir()})
	require.NoError(t, err)
	generated, err := load.PackageDefinition(t.Context(), outputDir, load.DefinitionOptions{SkipVersionCheck: true})
	require.NoError(t, err)
	require.Len(t, generated.PackageDefinition.AsV1alpha1().Components[0].ImageArchives, 1)
	assert.Equal(t, []string{image}, generated.PackageDefinition.AsV1alpha1().Components[0].ImageArchives[0].Images)

	reassembled, err := assemble.AssemblePackage(t.Context(), generated, outputDir, assemble.AssembleOptions{SkipSBOM: true})
	require.NoError(t, err)
	defer func() { require.NoError(t, reassembled.Cleanup()) }()
	indexBytes, err := os.ReadFile(filepath.Join(reassembled.GetImageDirPath(), ocispec.ImageIndexFile))
	require.NoError(t, err)
	var index ocispec.Index
	require.NoError(t, json.Unmarshal(indexBytes, &index))
	require.Len(t, index.Manifests, 1)
	assert.Equal(t, image, index.Manifests[0].Annotations[ocispec.AnnotationRefName])
}

func TestDisassembleFailureDoesNotPublishPartialOutput(t *testing.T) {
	parent := t.TempDir()
	outputDir := filepath.Join(parent, "output")
	_, err := Disassemble(t.Context(), Options{
		Source: filepath.Join(parent, "missing.tar.zst"), OutputDir: outputDir,
		TmpDir: t.TempDir(),
	})
	require.Error(t, err)
	assert.NoDirExists(t, outputDir)
	entries, readErr := os.ReadDir(parent)
	require.NoError(t, readErr)
	assert.Empty(t, entries)
}

func TestDisassembleSeparatesPackageDocumentationFromComponentAssets(t *testing.T) {
	sourceDir := copyFixture(t, "namespace-collision")
	resolved, err := load.PackageDefinition(t.Context(), sourceDir, load.DefinitionOptions{SkipVersionCheck: true})
	require.NoError(t, err)
	pkgLayout, err := assemble.AssemblePackage(t.Context(), resolved, sourceDir, assemble.AssembleOptions{SkipSBOM: true})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pkgLayout.Cleanup()) })
	archivePath, err := pkgLayout.Archive(t.Context(), t.TempDir(), 0)
	require.NoError(t, err)

	outputDir := filepath.Join(t.TempDir(), "output")
	_, err = Disassemble(t.Context(), Options{Source: archivePath, OutputDir: outputDir, Architecture: "amd64", TmpDir: t.TempDir()})
	require.NoError(t, err)
	generated, err := load.PackageDefinition(t.Context(), outputDir, load.DefinitionOptions{SkipVersionCheck: true})
	require.NoError(t, err)
	generatedPkg := generated.PackageDefinition.AsV1alpha1()
	assert.Equal(t, "documentation/files", generatedPkg.Documentation["guide"])
	assert.Contains(t, generatedPkg.Components[0].Files[0].Source, "components/documentation/files/")

	reassembled, err := assemble.AssemblePackage(t.Context(), generated, outputDir, assemble.AssembleOptions{SkipSBOM: true})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, reassembled.Cleanup()) })
}

func TestNormalizeMetadataMarksModifiedSourceOnce(t *testing.T) {
	metadata := v1alpha1.ZarfMetadata{Version: "1.2.3", AggregateChecksum: "checksum"}
	normalizeMetadata(&metadata)
	normalizeMetadata(&metadata)
	assert.Equal(t, "1.2.3-disassembled", metadata.Version)
	assert.Empty(t, metadata.AggregateChecksum)

	empty := v1alpha1.ZarfMetadata{}
	normalizeMetadata(&empty)
	assert.Equal(t, "disassembled", empty.Version)
}

func TestValidateOutputDirRejectsContent(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "existing"), []byte("data"), 0o600))
	require.ErrorContains(t, validateOutputDir(dir), "must be empty")
}

func prepareRoundTripFixture(t *testing.T) string {
	t.Helper()
	valuesServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := w.Write([]byte("remote: true\n")); err != nil {
			t.Errorf("write remote values response: %v", err)
		}
	}))
	t.Cleanup(valuesServer.Close)
	dir := copyFixture(t, "roundtrip")
	repoURL := initGitRepository(t, filepath.Join(dir, "repository"))
	template, err := os.ReadFile(filepath.Join(dir, "zarf.yaml.tmpl"))
	require.NoError(t, err)
	definition := strings.NewReplacer(
		"__REMOTE_VALUES_URL__", valuesServer.URL+"/production-values.yaml?token=secret",
		"__REPOSITORY_URL__", repoURL,
	).Replace(string(template))
	// dir is a test-owned path beneath t.TempDir.
	//nolint:gosec // G703 reports the parameterized fixture path as tainted.
	require.NoError(t, os.WriteFile(filepath.Join(dir, layout.ZarfYAML), []byte(definition), 0o600))
	return dir
}

func initGitRepository(t *testing.T, path string) string {
	t.Helper()
	repo, err := git.PlainInit(path, false)
	require.NoError(t, err)
	worktree, err := repo.Worktree()
	require.NoError(t, err)
	_, err = worktree.Add("README.md")
	require.NoError(t, err)
	_, err = worktree.Commit("initial", &git.CommitOptions{Author: &object.Signature{
		Name: "UDS Test", Email: "test@example.com", When: time.Unix(1, 0),
	}})
	require.NoError(t, err)
	return "file://" + filepath.ToSlash(path)
}

func copyFixture(t *testing.T, name string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, helpers.CreatePathAndCopy(filepath.Join("testdata", name), dir))
	return dir
}

func canonicalSchemaDigest(t *testing.T, schema []byte) string {
	t.Helper()
	var document any
	require.NoError(t, json.Unmarshal(schema, &document))
	canonical, err := json.Marshal(document)
	require.NoError(t, err)
	return fmt.Sprintf("%x", sha256.Sum256(canonical))
}

func writeImageArchive(t *testing.T, archivePath, ref string) {
	t.Helper()
	ctx := t.Context()
	imageDir := t.TempDir()
	store, err := contentoci.NewWithContext(ctx, imageDir)
	require.NoError(t, err)
	push := func(mediaType string, data []byte) ocispec.Descriptor {
		t.Helper()
		desc := content.NewDescriptorFromBytes(mediaType, data)
		require.NoError(t, store.Push(ctx, desc, bytes.NewReader(data)))
		return desc
	}
	layerData := []byte("offline image layer")
	layer := push(ocispec.MediaTypeImageLayer, layerData)
	configData := []byte(fmt.Sprintf(`{"architecture":"amd64","os":"linux","rootfs":{"type":"layers","diff_ids":[%q]}}`, layer.Digest.String()))
	config := push(ocispec.MediaTypeImageConfig, configData)
	manifestData, err := json.Marshal(ocispec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageManifest,
		Config:    config,
		Layers:    []ocispec.Descriptor{layer},
	})
	require.NoError(t, err)
	manifest := content.NewDescriptorFromBytes(ocispec.MediaTypeImageManifest, manifestData)
	manifest.Annotations = map[string]string{
		ocispec.AnnotationRefName:       ref,
		ocispec.AnnotationBaseImageName: ref,
	}
	require.NoError(t, store.Push(ctx, manifest, bytes.NewReader(manifestData)))
	require.NoError(t, store.Tag(ctx, manifest, ref))
	entries, err := os.ReadDir(imageDir)
	require.NoError(t, err)
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		paths = append(paths, filepath.Join(imageDir, entry.Name()))
	}
	require.NoError(t, archive.Compress(ctx, paths, archivePath, archive.CompressOpts{}))
}
