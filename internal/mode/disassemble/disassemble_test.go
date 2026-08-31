// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package disassemble

import (
	"bytes"
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

	packageoci "github.com/defenseunicorns/pkg/oci"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/api"
	"github.com/zarf-dev/zarf/src/api/v1alpha1"
	"github.com/zarf-dev/zarf/src/api/v1beta1"
	"github.com/zarf-dev/zarf/src/pkg/archive"
	"github.com/zarf-dev/zarf/src/pkg/packager/assemble"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
	"github.com/zarf-dev/zarf/src/pkg/packager/load"
	"github.com/zarf-dev/zarf/src/pkg/zoci"
	zarftypes "github.com/zarf-dev/zarf/src/types"
	"oras.land/oras-go/v2/content"
	contentoci "oras.land/oras-go/v2/content/oci"
)

func TestDisassembleRoundTripsThroughZarfOffline(t *testing.T) {
	sourceDir := writeSourcePackage(t)
	resolved, err := load.PackageDefinition(t.Context(), sourceDir, load.DefinitionOptions{SkipVersionCheck: true})
	require.NoError(t, err)
	pkgLayout, err := assemble.AssemblePackage(t.Context(), resolved, sourceDir, assemble.AssembleOptions{
		SkipSBOM: true, OCIConcurrency: 1, CachePath: t.TempDir(),
	})
	require.NoError(t, err)
	defer func() { require.NoError(t, pkgLayout.Cleanup()) }()

	outputDir := filepath.Join(t.TempDir(), "disassembled%source")
	result, err := Disassemble(t.Context(), Options{
		Source: pkgLayout.DirPath(), OutputDir: outputDir,
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
	assert.Contains(t, chart.ValuesFiles[0], filepath.ToSlash("components/app/values/app/values-0-chart.yaml"))
	assert.Contains(t, chart.ValuesFiles[1], filepath.ToSlash("components/app/values/app/values-1-production-values.yaml"))
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

	sourceDir := writeSourcePackage(t)
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

	for _, source := range []string{"oci://" + ref, ref} {
		t.Run(source, func(t *testing.T) {
			outputDir := filepath.Join(t.TempDir(), "output")
			_, err := Disassemble(t.Context(), Options{
				Source: source, OutputDir: outputDir, Architecture: "amd64", PlainHTTP: true, TmpDir: t.TempDir(), Concurrency: 1,
			})
			require.NoError(t, err)
			generated, err := load.PackageDefinition(t.Context(), outputDir, load.DefinitionOptions{SkipVersionCheck: true})
			require.NoError(t, err)
			reassembled, err := assemble.AssemblePackage(t.Context(), generated, outputDir, assemble.AssembleOptions{SkipSBOM: true, OCIConcurrency: 1, CachePath: t.TempDir()})
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, reassembled.Cleanup()) })
		})
	}
}

func TestDisassemblePreservesV1beta1Definition(t *testing.T) {
	sourceDir := t.TempDir()
	writeFile(t, filepath.Join(sourceDir, "payload.txt"), "payload\n")
	writeFile(t, filepath.Join(sourceDir, layout.ZarfYAML), `apiVersion: zarf.dev/v1beta1
kind: ZarfPackageConfig
metadata:
  name: beta-roundtrip
  version: 2.0.0
components:
  - name: app
    files:
      - source: payload.txt
        destination: /tmp/payload.txt
`)
	resolved, err := load.PackageDefinition(t.Context(), sourceDir, load.DefinitionOptions{SkipVersionCheck: true})
	require.NoError(t, err)
	pkgLayout, err := assemble.AssemblePackage(t.Context(), resolved, sourceDir, assemble.AssembleOptions{SkipSBOM: true})
	require.NoError(t, err)
	defer func() { require.NoError(t, pkgLayout.Cleanup()) }()

	outputDir := filepath.Join(t.TempDir(), "beta-output")
	_, err = Disassemble(t.Context(), Options{
		Source: pkgLayout.DirPath(), OutputDir: outputDir,
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

	reassembled, err := assemble.AssemblePackage(t.Context(), generated, outputDir, assemble.AssembleOptions{SkipSBOM: true})
	require.NoError(t, err)
	defer func() { require.NoError(t, reassembled.Cleanup()) }()
	assert.Equal(t, v1beta1.APIVersion, reassembled.PackageDefinition.OriginalAPIVersion())
}

func TestDisassembleClearsResolvedFlavorSelectors(t *testing.T) {
	sourceDir := t.TempDir()
	writeFile(t, filepath.Join(sourceDir, "payload.txt"), "payload\n")
	writeFile(t, filepath.Join(sourceDir, layout.ZarfYAML), `kind: ZarfPackageConfig
metadata:
  name: flavored
  version: 1.0.0
  architecture: amd64
components:
  - name: app
    required: true
    only:
      flavor: offline
    files:
      - source: payload.txt
        target: /tmp/payload.txt
`)
	resolved, err := load.PackageDefinition(t.Context(), sourceDir, load.DefinitionOptions{Flavor: "offline", SkipVersionCheck: true})
	require.NoError(t, err)
	pkgLayout, err := assemble.AssemblePackage(t.Context(), resolved, sourceDir, assemble.AssembleOptions{Flavor: "offline", SkipSBOM: true})
	require.NoError(t, err)
	defer func() { require.NoError(t, pkgLayout.Cleanup()) }()

	outputDir := filepath.Join(t.TempDir(), "output")
	_, err = Disassemble(t.Context(), Options{Source: pkgLayout.DirPath(), OutputDir: outputDir, Architecture: "amd64", TmpDir: t.TempDir()})
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
	sourceDir := t.TempDir()
	imageArchive := filepath.Join(sourceDir, "images.tar")
	writeImageArchive(t, imageArchive, image)
	pkg := v1alpha1.ZarfPackage{
		APIVersion: v1alpha1.APIVersion,
		Kind:       v1alpha1.ZarfPackageConfig,
		Metadata:   v1alpha1.ZarfMetadata{Name: "offline-image", Version: "1.0.0", Architecture: "amd64"},
		Components: []v1alpha1.ZarfComponent{{
			Name:          "app",
			ImageArchives: []v1alpha1.ImageArchive{{Path: "images.tar", Images: []string{image}}},
		}},
	}
	require.NoError(t, layout.WritePackageDefinition(filepath.Join(sourceDir, layout.ZarfYAML), api.NewPackageDefinitionFromV1alpha1(pkg)))
	resolved, err := load.PackageDefinition(t.Context(), sourceDir, load.DefinitionOptions{SkipVersionCheck: true})
	require.NoError(t, err)
	pkgLayout, err := assemble.AssemblePackage(t.Context(), resolved, sourceDir, assemble.AssembleOptions{SkipSBOM: true})
	require.NoError(t, err)
	defer func() { require.NoError(t, pkgLayout.Cleanup()) }()

	outputDir := filepath.Join(t.TempDir(), "output")
	_, err = Disassemble(t.Context(), Options{Source: pkgLayout.DirPath(), OutputDir: outputDir, Architecture: "amd64", TmpDir: t.TempDir()})
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

func TestLocalizedDefinitionPreservesV1beta1OnlyFields(t *testing.T) {
	original := api.NewPackageDefinitionFromV1beta1(v1beta1.Package{
		APIVersion: v1beta1.APIVersion,
		Kind:       v1beta1.ZarfPackageConfig,
		Metadata:   v1beta1.PackageMetadata{Name: "beta", Version: "1.0.0"},
		Components: []v1beta1.Component{{
			Name: "agent", ComponentSpec: v1beta1.ComponentSpec{
				Service: v1beta1.ServiceAgent,
				Manifests: []v1beta1.Manifest{{
					Name: "app", EnableTemplating: true,
					Kustomize: &v1beta1.KustomizeManifest{Files: []string{"original"}, AllowAnyDirectory: true, EnablePlugins: true},
				}},
			},
		}},
	})
	localized := original.AsV1alpha1()
	localized.Metadata.Version = "1.0.0-disassembled"
	localized.Components[0].Manifests[0].Kustomizations = []string{"agent/manifests/app/kustomization-0"}

	definition, err := localizedDefinition(original, localized)
	require.NoError(t, err)
	assert.Equal(t, v1beta1.APIVersion, definition.OriginalAPIVersion())
	component := definition.AsV1beta1().Components[0]
	assert.Equal(t, v1beta1.ServiceAgent, component.Service)
	require.NotNil(t, component.Manifests[0].Kustomize)
	assert.Equal(t, []string{"agent/manifests/app/kustomization-0"}, component.Manifests[0].Kustomize.Files)
	assert.False(t, component.Manifests[0].Kustomize.AllowAnyDirectory)
	assert.False(t, component.Manifests[0].Kustomize.EnablePlugins)
	assert.True(t, component.Manifests[0].EnableTemplating)
	assert.Empty(t, component.Selector.Flavor)
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
	sourceDir := t.TempDir()
	writeFile(t, filepath.Join(sourceDir, "docs", "files"), "documentation\n")
	writeFile(t, filepath.Join(sourceDir, "payload.txt"), "payload\n")
	pkg := v1alpha1.ZarfPackage{
		APIVersion:    v1alpha1.APIVersion,
		Kind:          v1alpha1.ZarfPackageConfig,
		Metadata:      v1alpha1.ZarfMetadata{Name: "namespace-collision", Version: "1.0.0", Architecture: "amd64"},
		Documentation: map[string]string{"guide": "docs/files"},
		Components: []v1alpha1.ZarfComponent{{
			Name:  "documentation",
			Files: []v1alpha1.ZarfFile{{Source: "payload.txt", Target: "/tmp/payload.txt"}},
		}},
	}
	require.NoError(t, layout.WritePackageDefinition(filepath.Join(sourceDir, layout.ZarfYAML), api.NewPackageDefinitionFromV1alpha1(pkg)))
	resolved, err := load.PackageDefinition(t.Context(), sourceDir, load.DefinitionOptions{SkipVersionCheck: true})
	require.NoError(t, err)
	pkgLayout, err := assemble.AssemblePackage(t.Context(), resolved, sourceDir, assemble.AssembleOptions{SkipSBOM: true})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pkgLayout.Cleanup()) })

	outputDir := filepath.Join(t.TempDir(), "output")
	_, err = Disassemble(t.Context(), Options{Source: pkgLayout.DirPath(), OutputDir: outputDir, Architecture: "amd64", TmpDir: t.TempDir()})
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

func TestPortableAssetName(t *testing.T) {
	tests := []struct {
		source string
		want   string
	}{
		{source: "https://example.test/production-values.yaml?token=secret", want: "production-values.yaml"},
		{source: "https://example.test/CON", want: "_CON"},
		{source: `bad<name>?.yaml`, want: "bad-name--.yaml"},
	}
	for _, tc := range tests {
		t.Run(tc.source, func(t *testing.T) {
			assert.Equal(t, tc.want, portableAssetName(tc.source, "values.yaml"))
		})
	}
}

func TestFindRepoPathReportsAttemptedMappings(t *testing.T) {
	_, err := findRepoPath(t.TempDir(), "https://github.com/example/repo.git")
	require.Error(t, err)
	require.ErrorContains(t, err, "tried")
	assert.NotContains(t, err.Error(), "%!w(<nil>)")
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

func writeSourcePackage(t *testing.T) string {
	t.Helper()
	valuesServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := w.Write([]byte("remote: true\n")); err != nil {
			t.Errorf("write remote values response: %v", err)
		}
	}))
	t.Cleanup(valuesServer.Close)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "values", "package.yaml"), "message: hello\n")
	writeFile(t, filepath.Join(dir, "values", "package.schema.json"), `{"type":"object","properties":{"message":{"type":"string"}}}`)
	writeFile(t, filepath.Join(dir, "values", "chart.yaml"), "replicaCount: 1\n")
	writeFile(t, filepath.Join(dir, "values", "chart-templated.yaml"), "message: '###ZARF_PKG_TMPL_MESSAGE###'\n")
	writeFile(t, filepath.Join(dir, "inputs", "one", "config.yaml"), "source: one\n")
	writeFile(t, filepath.Join(dir, "inputs", "two", "config.yaml"), "source: two\n")
	writeFile(t, filepath.Join(dir, "data", "one", "payload.txt"), "source: one\n")
	writeFile(t, filepath.Join(dir, "data", "two", "payload.txt"), "source: two\n")
	writeFile(t, filepath.Join(dir, "manifests", "configmap.yaml"), "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: raw\n")
	writeFile(t, filepath.Join(dir, "kustomize", "kustomization.yaml"), "resources:\n  - configmap.yaml\n")
	writeFile(t, filepath.Join(dir, "kustomize", "configmap.yaml"), "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: rendered\ndata:\n  query: '{{ $labels.instance }}'\n")
	writeFile(t, filepath.Join(dir, "docs", "guide.md"), "# Guide\n")
	writeFile(t, filepath.Join(dir, "chart", "Chart.yaml"), "apiVersion: v2\nname: upstream-app\nversion: 1.0.0\n")
	writeFile(t, filepath.Join(dir, "chart", "templates", "configmap.yaml"), "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: app\n")
	repoURL := writeGitRepository(t, filepath.Join(dir, "repository"))

	required := true
	template := true
	pkg := v1alpha1.ZarfPackage{
		APIVersion: v1alpha1.APIVersion,
		Kind:       v1alpha1.ZarfPackageConfig,
		Metadata: v1alpha1.ZarfMetadata{
			Name: "roundtrip", Version: "1.2.3", Architecture: "amd64",
		},
		Values: v1alpha1.ZarfValues{
			Files: []string{"values/package.yaml"}, Schema: "values/package.schema.json",
		},
		Documentation: map[string]string{"guide": "docs/guide.md"},
		Components: []v1alpha1.ZarfComponent{{
			Name: "app", Required: &required,
			Charts: []v1alpha1.ZarfChart{{
				Name: "app", Version: "1.0.0", Namespace: "app", LocalPath: "chart",
				ValuesFiles: []string{"values/chart.yaml", valuesServer.URL + "/production-values.yaml?token=secret"}, TemplatedValuesFiles: []string{"values/chart-templated.yaml"},
			}},
			Files: []v1alpha1.ZarfFile{
				{Source: "inputs/one/config.yaml", Target: "/etc/one/config.yaml"},
				{Source: "inputs/two/config.yaml", Target: "/etc/two/config.yaml"},
			},
			DataInjections: []v1alpha1.ZarfDataInjection{
				{Source: "data/one/payload.txt", Target: v1alpha1.ZarfContainerTarget{Namespace: "app", Selector: "app=one", Container: "app", Path: "/tmp/payload.txt"}},
				{Source: "data/two/payload.txt", Target: v1alpha1.ZarfContainerTarget{Namespace: "app", Selector: "app=two", Container: "app", Path: "/opt/payload.txt"}},
			},
			Manifests: []v1alpha1.ZarfManifest{{
				Name: "raw", Files: []string{"manifests/configmap.yaml"}, Kustomizations: []string{"kustomize"}, Template: &template,
			}},
			Repos: []string{repoURL},
		}},
	}
	require.NoError(t, layout.WritePackageDefinition(filepath.Join(dir, layout.ZarfYAML), api.NewPackageDefinitionFromV1alpha1(pkg)))
	return dir
}

func writeGitRepository(t *testing.T, path string) string {
	t.Helper()
	repo, err := git.PlainInit(path, false)
	require.NoError(t, err)
	writeFile(t, filepath.Join(path, "README.md"), "offline repository\n")
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

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o600))
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
