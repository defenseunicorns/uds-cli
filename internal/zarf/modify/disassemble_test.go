// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package modify

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
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/defenseunicorns/pkg/helpers/v2"
	packageoci "github.com/defenseunicorns/pkg/oci"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	goyaml "github.com/goccy/go-yaml"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/api"
	"github.com/zarf-dev/zarf/src/api/convert"
	"github.com/zarf-dev/zarf/src/api/v1alpha1"
	"github.com/zarf-dev/zarf/src/api/v1beta1"
	"github.com/zarf-dev/zarf/src/pkg/archive"
	"github.com/zarf-dev/zarf/src/pkg/images"
	"github.com/zarf-dev/zarf/src/pkg/packager/assemble"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
	"github.com/zarf-dev/zarf/src/pkg/packager/load"
	zarfschema "github.com/zarf-dev/zarf/src/pkg/schema"
	"github.com/zarf-dev/zarf/src/pkg/transform"
	"github.com/zarf-dev/zarf/src/pkg/zoci"
	zarftypes "github.com/zarf-dev/zarf/src/types"
	yamlv3 "gopkg.in/yaml.v3"
	chartloader "helm.sh/helm/v4/pkg/chart/v2/loader"
	"oras.land/oras-go/v2/content"
	contentoci "oras.land/oras-go/v2/content/oci"
)

func TestDisassembleRoundTripsThroughZarfOffline(t *testing.T) {
	sourceDir := prepareRoundTripFixture(t)
	packageArchitecture := "arm64"
	if runtime.GOARCH == packageArchitecture {
		packageArchitecture = "amd64"
	}
	setPackageArchitecture(t, sourceDir, packageArchitecture)

	pkgLayout := assembleTestPackage(t, sourceDir, load.DefinitionOptions{SkipVersionCheck: true}, assemble.AssembleOptions{
		SkipSBOM: true, OCIConcurrency: 1, CachePath: t.TempDir(),
	})
	defer func() { require.NoError(t, pkgLayout.Cleanup()) }()
	archivePath, err := pkgLayout.Archive(t.Context(), t.TempDir(), 0)
	require.NoError(t, err)

	outputDir := filepath.Join(t.TempDir(), "disassembled%source")
	var warnings []string
	result, err := Disassemble(t.Context(), Options{
		PackageOptions: testPackageOptions(t),
		Source:         archivePath,
		OutputDir:      outputDir,
		Warn: func(msg string, _ ...any) {
			warnings = append(warnings, msg)
		},
	})
	require.NoError(t, err)
	assert.Equal(t, outputDir, result)
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "last resort")
	assert.Contains(t, warnings[0], "upstream source")

	generated, err := load.PackageDefinition(t.Context(), outputDir, load.DefinitionOptions{SkipVersionCheck: true})
	require.NoError(t, err)
	pkg := generated
	assert.Equal(t, "roundtrip", pkg.Metadata.Name)
	assert.Equal(t, "1.2.3-disassembled", pkg.Metadata.Version)
	assert.Equal(t, packageArchitecture, pkg.Metadata.Architecture)
	require.Len(t, pkg.Components, 1)
	require.Len(t, pkg.Components[0].Charts, 1)
	chart := pkg.Components[0].Charts[0]
	assert.Equal(t, "app", chart.Name)
	assert.Equal(t, "1.0.0", chart.LegacyVersion)
	require.NotNil(t, chart.Local)
	assert.Nil(t, chart.HelmRepository)
	assert.Equal(t, "components/app/charts/app", chart.Local.Path)
	require.DirExists(t, filepath.Join(outputDir, chart.Local.Path))
	require.FileExists(t, filepath.Join(outputDir, chart.Local.Path, "Chart.yaml"))
	require.FileExists(t, filepath.Join(outputDir, chart.Local.Path, "templates", "configmap.yaml"))
	require.NoFileExists(t, filepath.Join(outputDir, chart.Local.Path, "Chart.lock"))
	require.FileExists(t, filepath.Join(outputDir, chart.Local.Path, "charts", "child", "Chart.yaml"))
	expandedChart, err := chartloader.Load(filepath.Join(outputDir, chart.Local.Path))
	require.NoError(t, err)
	require.Len(t, expandedChart.Metadata.Dependencies, 1)
	assert.Empty(t, expandedChart.Metadata.Dependencies[0].Repository)
	assert.Equal(t, "renamed-child", expandedChart.Metadata.Dependencies[0].Alias)
	require.Len(t, chart.ValuesFiles, 3)
	assert.Contains(t, chart.ValuesFiles[0].Path, filepath.ToSlash("components/app/values/app/0-chart.yaml"))
	assert.Contains(t, chart.ValuesFiles[1].Path, filepath.ToSlash("components/app/values/app/1-production-values.yaml"))
	assert.True(t, chart.ValuesFiles[2].EnableTemplating)
	require.Len(t, pkg.Components[0].Manifests, 1)
	manifest := pkg.Components[0].Manifests[0]
	require.Len(t, manifest.Files, 2)
	assert.Contains(t, manifest.Files[0], filepath.ToSlash("manifests/raw/0-configmap.yaml"))
	assert.Contains(t, manifest.Files[1], filepath.ToSlash("manifests/raw/1-experimental-install.yaml"))
	require.Len(t, manifest.Kustomize.Files, 1)
	assert.True(t, manifest.EnableTemplating)
	rendered, err := os.ReadFile(filepath.Join(outputDir, manifest.Kustomize.Files[0], "rendered.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(rendered), "{{ $labels.instance }}")
	assert.Equal(t, "components/app/files/0-config.yaml", pkg.Components[0].Files[0].Source)
	assert.Equal(t, "components/app/files/1-config.yaml", pkg.Components[0].Files[1].Source)
	assert.Equal(t, "components/app/data/0-payload.txt", pkg.Components[0].DataInjections[0].Source)
	assert.Equal(t, "components/app/data/1-payload.txt", pkg.Components[0].DataInjections[1].Source)
	for _, file := range pkg.Components[0].Files {
		assert.FileExists(t, filepath.Join(outputDir, file.Source))
	}
	for _, data := range pkg.Components[0].DataInjections {
		assert.FileExists(t, filepath.Join(outputDir, data.Source))
	}
	require.Len(t, pkg.Components[0].Repositories, 1)
	repoSource, err := url.Parse(pkg.Components[0].Repositories[0].URL)
	require.NoError(t, err)
	assert.Contains(t, repoSource.Path, filepath.ToSlash(outputDir))
	repo, err := git.PlainOpen(repoSource.Path)
	require.NoError(t, err)
	worktree, err := repo.Worktree()
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(repoSource.Path, "README.md"), []byte("edited offline repository\n"), 0o600))
	_, err = worktree.Add("README.md")
	require.NoError(t, err)
	_, err = worktree.Commit("edit offline repository", &git.CommitOptions{Author: &object.Signature{
		Name: "UDS Test", Email: "test@example.com", When: time.Unix(2, 0),
	}})
	require.NoError(t, err)
	assert.Equal(t, []string{layout.ValuesYAML}, pkg.Values.Files)
	assert.Equal(t, layout.ValuesSchema, pkg.Values.Schema)
	assert.Equal(t, "documentation/guide.md", pkg.Documentation["guide"])
	generatedYAML, err := os.ReadFile(filepath.Join(outputDir, layout.ZarfYAML))
	require.NoError(t, err)
	assert.NotContains(t, string(generatedYAML), "\nbuild:")
	assert.Contains(t, string(generatedYAML), "\ncomponents:\n  - name: app\n")
	disassemblyJSON, err := os.ReadFile(filepath.Join(outputDir, disassemblyMetadataDir, disassemblyMetadataFile))
	require.NoError(t, err)
	assert.JSONEq(t, fmt.Sprintf("{\n  \"formatVersion\": \"v1alpha1\",\n  \"architecture\": %q,\n  \"flavor\": \"\"\n}\n", packageArchitecture), string(disassemblyJSON))

	reassembled := assembleTestPackage(t, outputDir, load.DefinitionOptions{SkipVersionCheck: true}, assemble.AssembleOptions{
		SkipSBOM: true, OCIConcurrency: 1, CachePath: t.TempDir(),
	})
	defer func() { require.NoError(t, reassembled.Cleanup()) }()
	reassembledRepositoryRoot, err := reassembled.GetComponentDir(t.Context(), t.TempDir(), pkg.Components[0].Name, layout.RepoComponentDir)
	require.NoError(t, err)
	reassembledRepositoryName, err := transform.GitURLtoFolderName(pkg.Components[0].Repositories[0].URL)
	require.NoError(t, err)
	reassembledREADME, err := os.ReadFile(filepath.Join(reassembledRepositoryRoot, reassembledRepositoryName, "README.md"))
	require.NoError(t, err)
	assert.Equal(t, "edited offline repository\n", string(reassembledREADME))
	assert.Equal(t, "roundtrip", reassembled.AsV1alpha1().Metadata.Name)
	assert.Equal(t, "1.2.3-disassembled", reassembled.AsV1alpha1().Metadata.Version)
	assert.Equal(t, packageArchitecture, reassembled.AsV1alpha1().Build.Architecture)
}

func TestDisassemblePullsOCIPackage(t *testing.T) {
	nonHostArchitecture := "arm64"
	if runtime.GOARCH == nonHostArchitecture {
		nonHostArchitecture = "amd64"
	}
	for _, tc := range []struct {
		name                 string
		packageArchitecture  string
		selectedArchitecture string
	}{
		{name: "workstation default", packageArchitecture: runtime.GOARCH},
		{name: "non-host selection", packageArchitecture: nonHostArchitecture, selectedArchitecture: nonHostArchitecture},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(registry.New())
			t.Cleanup(server.Close)
			sourceDir := prepareRoundTripFixture(t)
			setPackageArchitecture(t, sourceDir, tc.packageArchitecture)
			pkgLayout := assembleTestPackage(t, sourceDir, load.DefinitionOptions{SkipVersionCheck: true}, assemble.AssembleOptions{SkipSBOM: true, OCIConcurrency: 1, CachePath: t.TempDir()})
			t.Cleanup(func() { require.NoError(t, pkgLayout.Cleanup()) })
			ref := strings.TrimPrefix(server.URL, "http://") + "/test/disassemble:1.0.0"
			remote, err := zoci.NewRemoteWithOptions(t.Context(), ref, ocispec.Platform{Architecture: tc.packageArchitecture, OS: packageoci.MultiOS}, zoci.RemoteClientOptions{
				RemoteOptions: zarftypes.RemoteOptions{PlainHTTP: true},
			})
			require.NoError(t, err)
			_, err = remote.PushPackage(t.Context(), pkgLayout, zoci.PublishOptions{Retries: 1, OCIConcurrency: 1})
			require.NoError(t, err)

			outputDir := filepath.Join(t.TempDir(), "output")
			_, err = Disassemble(t.Context(), Options{
				PackageOptions: PackageOptions{PlainHTTP: true, TmpDir: os.TempDir(), Concurrency: 1},
				Source:         "oci://" + ref,
				Architecture:   tc.selectedArchitecture,
				OutputDir:      outputDir,
			})
			require.NoError(t, err)
			metadata, err := readDisassemblyMetadata(outputDir)
			require.NoError(t, err)
			assert.Equal(t, tc.packageArchitecture, metadata.Architecture)
			reassembled := assembleTestPackage(t, outputDir, load.DefinitionOptions{SkipVersionCheck: true}, assemble.AssembleOptions{SkipSBOM: true, OCIConcurrency: 1, CachePath: t.TempDir()})
			t.Cleanup(func() { require.NoError(t, reassembled.Cleanup()) })
			assert.Equal(t, tc.packageArchitecture, reassembled.Definition().Build.Architecture)
		})
	}
}

func TestDisassemblePreservesV1beta1Definition(t *testing.T) {
	sourceDir := copyFixture(t, "v1beta1")
	pkgLayout := assembleTestPackage(t, sourceDir, load.DefinitionOptions{Flavor: "offline", SkipVersionCheck: true}, assemble.AssembleOptions{Flavor: "offline", SkipSBOM: true})
	defer func() { require.NoError(t, pkgLayout.Cleanup()) }()
	archivePath, err := pkgLayout.Archive(t.Context(), t.TempDir(), 0)
	require.NoError(t, err)

	outputDir := filepath.Join(t.TempDir(), "beta-output")
	_, err = Disassemble(t.Context(), Options{
		PackageOptions: testPackageOptions(t),
		Source:         archivePath,
		OutputDir:      outputDir,
	})
	require.NoError(t, err)
	generated, err := load.PackageDefinition(t.Context(), outputDir, load.DefinitionOptions{Flavor: "offline", SkipVersionCheck: true})
	generatedYAML, readErr := os.ReadFile(filepath.Join(outputDir, layout.ZarfYAML))
	require.NoError(t, readErr)
	require.NoErrorf(t, err, "generated zarf.yaml:\n%s", generatedYAML)
	assert.Equal(t, v1beta1.APIVersion, generated.GetAPIVersion())
	generatedBeta := convert.PackageToV1beta1(generated)
	assert.Equal(t, "2.0.0-disassembled", generatedBeta.Metadata.Version)
	assert.Equal(t, "amd64", generatedBeta.Metadata.Architecture)
	require.Len(t, generatedBeta.Components, 1)
	component := generatedBeta.Components[0]
	assert.Equal(t, v1beta1.ServiceAgent, component.Service)
	require.Len(t, component.Manifests, 1)
	assert.Contains(t, component.Manifests[0].Kustomize.Files[0], "components/app/manifests/raw/0-kustomize")
	assert.False(t, component.Manifests[0].Kustomize.AllowAnyDirectory)
	assert.False(t, component.Manifests[0].Kustomize.EnablePlugins)
	assert.True(t, component.Manifests[0].EnableTemplating)
	assert.Equal(t, "offline", component.Selector.Flavor)
	assert.NotContains(t, string(generatedYAML), "\nbuild:")
	assert.Contains(t, string(generatedYAML), "\ncomponents:\n  - name: app\n")

	reassembled := assembleTestPackage(t, outputDir, load.DefinitionOptions{Flavor: "offline", SkipVersionCheck: true}, assemble.AssembleOptions{Flavor: "offline", SkipSBOM: true})
	defer func() { require.NoError(t, reassembled.Cleanup()) }()
	assert.Equal(t, v1beta1.APIVersion, reassembled.Definition().GetAPIVersion())
}

func TestDisassembleRemovesDeprecatedMigrationFields(t *testing.T) {
	sourceDir := copyFixture(t, "deprecated")
	pkgLayout := assembleTestPackage(t, sourceDir, load.DefinitionOptions{SkipVersionCheck: true}, assemble.AssembleOptions{SkipSBOM: true})
	defer func() { require.NoError(t, pkgLayout.Cleanup()) }()

	// Recreate an older package definition that predates migration markers and
	// stores only the deprecated representation.
	pkg := pkgLayout.AsV1alpha1()
	pkg.Build.Migrations = nil
	pkg.Components[0].DeprecatedScripts = v1alpha1.DeprecatedZarfComponentScripts{
		Prepare: []string{"echo legacy-create"},
		Before:  []string{"echo legacy-deploy"},
	}
	pkg.Components[0].Actions.OnCreate.Before = nil
	pkg.Components[0].Actions.OnDeploy.Before = nil
	pkg.Components[0].Actions.OnDeploy.After[0].DeprecatedSetVariable = "RESULT"
	pkg.Components[0].Actions.OnDeploy.After[0].SetVariables = nil
	contents, err := goyaml.MarshalWithOptions(pkg, goyaml.IndentSequence(true), goyaml.UseLiteralStyleIfMultiline(true))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(pkgLayout.DirPath(), layout.ZarfYAML), contents, helpers.ReadWriteUser))
	archivePath, err := pkgLayout.Archive(t.Context(), t.TempDir(), 0)
	require.NoError(t, err)

	outputDir := filepath.Join(t.TempDir(), "output")
	_, err = Disassemble(t.Context(), Options{PackageOptions: testPackageOptions(t), Source: archivePath, OutputDir: outputDir})
	require.NoError(t, err)
	generatedYAML, err := os.ReadFile(filepath.Join(outputDir, layout.ZarfYAML))
	require.NoError(t, err)
	assert.NotContains(t, string(generatedYAML), "\nscripts:")
	assert.NotContains(t, string(generatedYAML), "setVariable:")

	generated, err := load.PackageDefinition(t.Context(), outputDir, load.DefinitionOptions{SkipVersionCheck: true})
	require.NoError(t, err)
	component := generated.Components[0]
	assert.Empty(t, component.Actions.OnCreate)
	require.Len(t, component.Actions.OnDeploy.Before, 1)
	assert.Equal(t, "echo legacy-deploy", component.Actions.OnDeploy.Before[0].Cmd)
	require.Len(t, component.Actions.OnDeploy.After, 1)
	require.Len(t, component.Actions.OnDeploy.After[0].SetVariables, 1)
	assert.Equal(t, "RESULT", component.Actions.OnDeploy.After[0].SetVariables[0].Name)
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
		{name: "v1beta1", schema: zarfschema.GetV1Beta1Schema(), want: "ae2c2b0069c7634afe2a87456a58e8601a8ee626bba087d25e26610410254a2d"},
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

func TestDisassemblePreservesFlavorSelectors(t *testing.T) {
	for _, tc := range []struct {
		name        string
		unversioned bool
		wantVersion string
	}{
		{name: "versioned", wantVersion: "1.0.0-disassembled"},
		{name: "unversioned", unversioned: true, wantVersion: "disassembled"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sourceDir := copyFixture(t, "flavored")
			definitionPath := filepath.Join(sourceDir, layout.ZarfYAML)
			contents, err := os.ReadFile(definitionPath)
			require.NoError(t, err)
			contents = bytes.Replace(contents, []byte("    files:\n      - source: payload.txt\n        target: /tmp/payload.txt\n"), nil, 1)
			if tc.unversioned {
				contents = bytes.Replace(contents, []byte("  version: 1.0.0\n"), nil, 1)
			}
			//nolint:gosec // G703 treats the test-owned fixture path beneath t.TempDir as attacker-controlled.
			require.NoError(t, os.WriteFile(definitionPath, contents, helpers.ReadWriteUser))
			pkgLayout := assembleTestPackage(t, sourceDir, load.DefinitionOptions{Flavor: "offline", SkipVersionCheck: true}, assemble.AssembleOptions{Flavor: "offline", SkipSBOM: true})
			t.Cleanup(func() { require.NoError(t, pkgLayout.Cleanup()) })
			archivePath, err := pkgLayout.Archive(t.Context(), t.TempDir(), 0)
			require.NoError(t, err)

			outputDir := filepath.Join(t.TempDir(), "output")
			_, err = Disassemble(t.Context(), Options{PackageOptions: testPackageOptions(t), Source: archivePath, OutputDir: outputDir})
			require.NoError(t, err)
			generated, err := load.PackageDefinition(t.Context(), outputDir, load.DefinitionOptions{Flavor: "offline", SkipVersionCheck: true})
			require.NoError(t, err)
			require.Len(t, generated.Components, 1)
			assert.Equal(t, "offline", generated.Components[0].Selector.Flavor)
			assert.Equal(t, tc.wantVersion, generated.Metadata.Version)

			metadata, err := readDisassemblyMetadata(outputDir)
			require.NoError(t, err)
			assert.Equal(t, disassemblyMetadata{FormatVersion: "v1alpha1", Architecture: "amd64", Flavor: "offline"}, metadata)

			result, err := Reassemble(t.Context(), ReassembleOptions{PackageOptions: testPackageOptions(t), SourceDir: outputDir, Output: t.TempDir()})
			require.NoError(t, err)
			require.FileExists(t, result)
			reassembled, err := loadPackageSource(t.Context(), Options{PackageOptions: PackageOptions{Concurrency: 1}, Source: result})
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, reassembled.Cleanup()) })
			assert.Equal(t, tc.wantVersion, reassembled.Definition().Metadata.Version)
			assert.Equal(t, "offline", reassembled.AsV1alpha1().Build.Flavor)
			assert.Equal(t, "amd64", reassembled.AsV1alpha1().Build.Architecture)
		})
	}
}

func TestReassembleRejectsInvalidDisassembledSource(t *testing.T) {
	tests := []struct {
		name      string
		prepare   func(t *testing.T) string
		wantError string
	}{
		{
			name: "version suffix",
			prepare: func(t *testing.T) string {
				sourceDir := copyFixture(t, "flavored")
				require.NoError(t, writeDisassemblyMetadata(sourceDir, "amd64", "offline"))
				return sourceDir
			},
			wantError: `must end with "disassembled"`,
		},
		{
			name: "architecture mismatch",
			prepare: func(t *testing.T) string {
				sourceDir := copyFixture(t, "flavored")
				definitionPath := filepath.Join(sourceDir, layout.ZarfYAML)
				contents, err := os.ReadFile(definitionPath)
				require.NoError(t, err)
				contents = bytes.Replace(contents, []byte("version: 1.0.0"), []byte("version: 1.0.0-disassembled"), 1)
				//nolint:gosec // G703 treats the test-owned fixture path beneath t.TempDir as attacker-controlled.
				require.NoError(t, os.WriteFile(definitionPath, contents, helpers.ReadWriteUser))
				require.NoError(t, writeDisassemblyMetadata(sourceDir, "arm64", "offline"))
				return sourceDir
			},
			wantError: `package architecture "amd64" does not match disassembly metadata architecture "arm64"`,
		},
		{
			name: "unsupported metadata version",
			prepare: func(t *testing.T) string {
				sourceDir := copyFixture(t, "flavored")
				metadataDir := filepath.Join(sourceDir, disassemblyMetadataDir)
				require.NoError(t, os.MkdirAll(metadataDir, helpers.ReadWriteExecuteUser))
				require.NoError(t, os.WriteFile(
					filepath.Join(metadataDir, disassemblyMetadataFile),
					[]byte("{\"formatVersion\":\"v2\",\"architecture\":\"amd64\",\"flavor\":\"offline\"}\n"),
					helpers.ReadWriteUser,
				))
				return sourceDir
			},
			wantError: `unsupported disassembly metadata format version "v2"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Reassemble(t.Context(), ReassembleOptions{PackageOptions: testPackageOptions(t), SourceDir: tc.prepare(t), Output: t.TempDir()})
			require.ErrorContains(t, err, tc.wantError)
		})
	}
}

func TestDisassembleRoundTripsImagesOffline(t *testing.T) {
	const image = "registry.invalid/offline/app:v1"
	for _, tc := range []struct {
		name        string
		annotations map[string]string
	}{
		{name: "current references", annotations: map[string]string{
			ocispec.AnnotationRefName: image, ocispec.AnnotationBaseImageName: image,
		}},
		{name: "legacy base-name only", annotations: map[string]string{
			ocispec.AnnotationBaseImageName: image,
		}},
		{name: "preserve existing reference", annotations: map[string]string{
			ocispec.AnnotationRefName: image, ocispec.AnnotationBaseImageName: "registry.invalid/provenance/base:v0",
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sourceDir := copyFixture(t, "offline-image")
			imageArchive := filepath.Join(sourceDir, "images.tar")
			writeImageArchive(t, imageArchive, image)
			pkgLayout := assembleTestPackage(t, sourceDir, load.DefinitionOptions{SkipVersionCheck: true, CachePath: t.TempDir()}, assemble.AssembleOptions{SkipSBOM: true, CachePath: t.TempDir()})
			t.Cleanup(func() { require.NoError(t, pkgLayout.Cleanup()) })
			sourceIndex := setPackageImageAnnotations(t, pkgLayout, tc.annotations)
			archivePath, err := pkgLayout.Archive(t.Context(), t.TempDir(), 0)
			require.NoError(t, err)

			outputDir := filepath.Join(t.TempDir(), "output")
			_, err = Disassemble(t.Context(), Options{PackageOptions: testPackageOptions(t), Source: archivePath, OutputDir: outputDir})
			require.NoError(t, err)
			generated, err := load.PackageDefinition(t.Context(), outputDir, load.DefinitionOptions{SkipVersionCheck: true, CachePath: t.TempDir()})
			require.NoError(t, err)
			require.Len(t, generated.Components[0].ImageArchives, 1)
			imageArchive = filepath.Join(outputDir, generated.Components[0].ImageArchives[0].Path)
			assert.Equal(t, []string{image}, generated.Components[0].ImageArchives[0].Images)

			manifests, err := images.GetManifestsFromArchive(t.Context(), imageArchive)
			require.NoError(t, err)
			require.Len(t, manifests, 1)
			assert.Equal(t, image, manifests[0].Annotations[ocispec.AnnotationRefName])
			assert.Equal(t, tc.annotations[ocispec.AnnotationBaseImageName], manifests[0].Annotations[ocispec.AnnotationBaseImageName])
			assert.Equal(t, sourceIndex.Manifests[0].Digest, manifests[0].Digest)

			reassembled := assembleTestPackage(t, outputDir, load.DefinitionOptions{SkipVersionCheck: true, CachePath: t.TempDir()}, assemble.AssembleOptions{SkipSBOM: true, CachePath: t.TempDir()})
			t.Cleanup(func() { require.NoError(t, reassembled.Cleanup()) })
			manifests, err = images.GetManifestsFromArchive(t.Context(), reassembled.GetImageDirPath())
			require.NoError(t, err)
			require.Len(t, manifests, 1)
			assert.Equal(t, image, manifests[0].Annotations[ocispec.AnnotationRefName])
			assert.Equal(t, sourceIndex.Manifests[0].Digest, manifests[0].Digest)
		})
	}
}

func setPackageImageAnnotations(t *testing.T, pkgLayout *layout.PackageLayout, annotations map[string]string) ocispec.Index {
	t.Helper()
	root, err := os.OpenRoot(pkgLayout.DirPath())
	require.NoError(t, err)
	defer func() { require.NoError(t, root.Close()) }()
	indexPath := filepath.Join(layout.ImagesDir, ocispec.ImageIndexFile)
	original, err := root.ReadFile(indexPath)
	require.NoError(t, err)
	var index ocispec.Index
	require.NoError(t, json.Unmarshal(original, &index))
	require.Len(t, index.Manifests, 1)
	index.Manifests[0].Annotations = annotations
	updated, err := json.Marshal(index)
	require.NoError(t, err)
	require.NoError(t, root.WriteFile(indexPath, updated, helpers.ReadWriteUser))

	// Model a complete package built with these annotations, preserving the
	// integrity checks exercised by Disassemble's normal package loader.
	checksums, err := root.ReadFile(layout.Checksums)
	require.NoError(t, err)
	rel := filepath.ToSlash(filepath.Join(layout.ImagesDir, ocispec.ImageIndexFile))
	oldLine := fmt.Sprintf("%x %s\n", sha256.Sum256(original), rel)
	newLine := fmt.Sprintf("%x %s\n", sha256.Sum256(updated), rel)
	require.Contains(t, string(checksums), oldLine)
	lines := strings.Split(strings.TrimSuffix(strings.Replace(string(checksums), oldLine, newLine, 1), "\n"), "\n")
	slices.Sort(lines)
	checksums = []byte(strings.Join(lines, "\n") + "\n")
	require.NoError(t, root.WriteFile(layout.Checksums, checksums, helpers.ReadWriteUser))
	definition := pkgLayout.Definition()
	definition.Build.AggregateChecksum = fmt.Sprintf("%x", sha256.Sum256(checksums))
	require.NoError(t, layout.WritePackageDefinition(filepath.Join(pkgLayout.DirPath(), layout.ZarfYAML), definition))
	return index
}

func TestDisassembleFailureDoesNotPublishPartialOutput(t *testing.T) {
	parent := t.TempDir()
	outputDir := filepath.Join(parent, "output")
	var warnings []string
	_, err := Disassemble(t.Context(), Options{
		PackageOptions: testPackageOptions(t),
		Source:         filepath.Join(parent, "missing.tar.zst"),
		OutputDir:      outputDir,
		Warn: func(msg string, _ ...any) {
			warnings = append(warnings, msg)
		},
	})
	require.Error(t, err)
	assert.NoDirExists(t, outputDir)
	assert.Empty(t, warnings)
	entries, readErr := os.ReadDir(parent)
	require.NoError(t, readErr)
	assert.Empty(t, entries)
}

func TestDisassembleSeparatesPackageDocumentationFromComponentAssets(t *testing.T) {
	sourceDir := copyFixture(t, "namespace-collision")
	definition, err := load.PackageDefinition(t.Context(), sourceDir, load.DefinitionOptions{SkipVersionCheck: true})
	require.NoError(t, err)
	definition.Documentation["second"] = definition.Documentation["guide"]
	require.NoError(t, writeSourceDefinition(filepath.Join(sourceDir, layout.ZarfYAML), definition))
	pkgLayout := assembleTestPackage(t, sourceDir, load.DefinitionOptions{SkipVersionCheck: true}, assemble.AssembleOptions{SkipSBOM: true})
	t.Cleanup(func() { require.NoError(t, pkgLayout.Cleanup()) })
	archivePath, err := pkgLayout.Archive(t.Context(), t.TempDir(), 0)
	require.NoError(t, err)

	outputDir := filepath.Join(t.TempDir(), "output")
	_, err = Disassemble(t.Context(), Options{PackageOptions: testPackageOptions(t), Source: archivePath, OutputDir: outputDir})
	require.NoError(t, err)
	generated, err := load.PackageDefinition(t.Context(), outputDir, load.DefinitionOptions{SkipVersionCheck: true})
	require.NoError(t, err)
	generatedPkg := generated
	assert.Equal(t, "documentation/guide-files", generatedPkg.Documentation["guide"])
	assert.Equal(t, "documentation/second-files", generatedPkg.Documentation["second"])
	for _, path := range generatedPkg.Documentation {
		contents, err := os.ReadFile(filepath.Join(outputDir, path))
		require.NoError(t, err)
		assert.Equal(t, "documentation\n", string(contents))
	}
	assert.Contains(t, generatedPkg.Components[0].Files[0].Source, "components/documentation/files/")

	reassembled := assembleTestPackage(t, outputDir, load.DefinitionOptions{SkipVersionCheck: true}, assemble.AssembleOptions{SkipSBOM: true})
	t.Cleanup(func() { require.NoError(t, reassembled.Cleanup()) })
}

func TestDisassembleRejectsEscapingDocumentationNames(t *testing.T) {
	for _, tc := range []struct {
		name string
		key  func(string) string
	}{
		{name: "traversal", key: func(path string) string {
			return strings.Repeat("../", 20) + strings.TrimPrefix(filepath.ToSlash(path), "/")
		}},
		{name: "absolute", key: filepath.ToSlash},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pkgLayout := assembleTestPackage(t, copyFixture(t, "namespace-collision"), load.DefinitionOptions{SkipVersionCheck: true}, assemble.AssembleOptions{SkipSBOM: true})
			t.Cleanup(func() { require.NoError(t, pkgLayout.Cleanup()) })
			canaryDir := t.TempDir()
			canaries := []string{filepath.Join(canaryDir, "first-files"), filepath.Join(canaryDir, "second-files")}
			definition := pkgLayout.AsV1alpha1()
			definition.Documentation = make(map[string]string)
			for _, canary := range canaries {
				require.NoError(t, os.WriteFile(canary, []byte("untouched\n"), 0o600))
				definition.Documentation[tc.key(strings.TrimSuffix(canary, "-files"))] = "docs/files"
			}
			contents, err := goyaml.Marshal(definition)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(pkgLayout.DirPath(), layout.ZarfYAML), contents, 0o600))
			archivePath, err := pkgLayout.Archive(t.Context(), t.TempDir(), 0)
			require.NoError(t, err)
			parent := t.TempDir()
			outputDir := filepath.Join(parent, "output")
			_, err = Disassemble(t.Context(), Options{PackageOptions: testPackageOptions(t), Source: archivePath, OutputDir: outputDir})
			require.ErrorContains(t, err, "invalid filename")
			assert.NoDirExists(t, outputDir)
			for _, canary := range canaries {
				contents, err := os.ReadFile(canary)
				require.NoError(t, err)
				assert.Equal(t, "untouched\n", string(contents))
			}
			entries, err := os.ReadDir(parent)
			require.NoError(t, err)
			assert.Empty(t, entries, "failed disassembly must remove its output staging directory")
		})
	}
}

func TestNormalizeMetadataMarksModifiedSourceOnce(t *testing.T) {
	// Repeated normalization must not append the marker again, including for unversioned packages.
	metadata := api.PackageMetadata{Version: "1.2.3"}
	normalizeMetadata(&metadata)
	normalizeMetadata(&metadata)
	assert.Equal(t, "1.2.3-disassembled", metadata.Version)

	empty := api.PackageMetadata{}
	normalizeMetadata(&empty)
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
	assetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		content := "remote: true\n"
		if strings.HasSuffix(r.URL.Path, "/experimental-install.yaml") {
			content = "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: remote\n"
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Errorf("write remote asset response: %v", err)
		}
	}))
	t.Cleanup(assetServer.Close)
	dir := copyFixture(t, "roundtrip")
	repoURL := initGitRepository(t, filepath.Join(dir, "repository"))
	template, err := os.ReadFile(filepath.Join(dir, "zarf.yaml.tmpl"))
	require.NoError(t, err)
	definition := strings.NewReplacer(
		"__REMOTE_VALUES_URL__", assetServer.URL+"/production-values.yaml?token=secret",
		"__REMOTE_MANIFEST_URL__", assetServer.URL+"/experimental-install.yaml?token=secret",
		"__REPOSITORY_URL__", repoURL,
	).Replace(string(template))
	// dir is a test-owned path beneath t.TempDir.
	//nolint:gosec // G703 reports the parameterized fixture path as tainted.
	require.NoError(t, os.WriteFile(filepath.Join(dir, layout.ZarfYAML), []byte(definition), 0o600))
	return dir
}

func setPackageArchitecture(t *testing.T, sourceDir, architecture string) {
	t.Helper()
	definitionPath := filepath.Join(sourceDir, layout.ZarfYAML)
	definition, err := os.ReadFile(definitionPath)
	require.NoError(t, err)
	definition = bytes.Replace(definition, []byte("architecture: amd64"), []byte("architecture: "+architecture), 1)
	//nolint:gosec // G703 treats the test-owned fixture path beneath t.TempDir as attacker-controlled.
	require.NoError(t, os.WriteFile(definitionPath, definition, 0o600))
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

func testPackageOptions(t *testing.T) PackageOptions {
	t.Helper()
	return PackageOptions{TmpDir: os.TempDir(), Concurrency: 1}
}

func assembleTestPackage(t *testing.T, sourceDir string, definitionOpts load.DefinitionOptions, assembleOpts assemble.AssembleOptions) *layout.PackageLayout {
	t.Helper()
	loaded, err := load.Package(t.Context(), sourceDir, load.PackageOptions{DefinitionOptions: definitionOpts})
	require.NoError(t, err)
	defer func() { require.NoError(t, loaded.Close()) }()
	pkgLayout, err := assemble.AssemblePackage(t.Context(), loaded, assembleOpts)
	require.NoError(t, err)
	return pkgLayout
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

func TestWriteSourceDefinitionFormatting(t *testing.T) {
	for _, version := range []string{"", v1alpha1.APIVersion, v1beta1.APIVersion} {
		name := version
		if name == "" {
			name = "unversioned alpha"
		}
		t.Run(name, func(t *testing.T) {
			pkg := api.Package{
				APIVersion: version,
				Kind:       api.PackageKind(v1alpha1.ZarfPackageConfig),
				Metadata:   api.PackageMetadata{Name: "readable", Version: "1.0.0"},
				Components: []api.Component{{Name: "first"}, {Name: "second"}, {Name: "third"}},
				Values:     api.Values{Files: []string{"values.yaml"}, Schema: "values.schema.json"},
				Documentation: map[string]string{
					"guide": "guide.md",
				},
			}
			alphaFields := ""
			metadataSuffix, componentSuffix := "", ""
			if version != v1beta1.APIVersion {
				pkg.Constants = []api.Constant{{Name: "FIXED", Value: "fixed"}}
				pkg.Variables = []api.InteractiveVariable{{Variable: api.Variable{Name: "INPUT"}, Default: "input"}}
				alphaFields = "\nconstants:\n  - name: FIXED\n    value: fixed\n\nvariables:\n  - name: INPUT\n    default: input\n"
				metadataSuffix = "  allowNamespaceOverride: true\n"
				componentSuffix = "    required: true\n"
			}
			path := filepath.Join(t.TempDir(), layout.ZarfYAML)
			require.NoError(t, writeSourceDefinition(path, pkg))
			contents, err := os.ReadFile(path)
			require.NoError(t, err)
			prefix := ""
			if version != "" {
				prefix = "apiVersion: " + version + "\n\n"
			}
			want := prefix + "kind: ZarfPackageConfig\nmetadata:\n  name: readable\n  version: 1.0.0\n" + metadataSuffix +
				"\ncomponents:\n  - name: first\n" + componentSuffix + "\n  - name: second\n" + componentSuffix +
				"\n  - name: third\n" + componentSuffix + alphaFields +
				"\nvalues:\n  files:\n    - values.yaml\n  schema: values.schema.json\n\ndocumentation:\n  guide: guide.md\n"
			assert.Equal(t, want, string(contents))
		})
	}
}

func TestWriteSourceDefinitionFormattingPreservesMultilineScalars(t *testing.T) {
	for _, version := range []string{"", v1alpha1.APIVersion, v1beta1.APIVersion} {
		for _, trailingNewlines := range []int{0, 1, 2, 3} {
			t.Run(fmt.Sprintf("%s/trailing_newlines_%d", version, trailingNewlines), func(t *testing.T) {
				text := "literal text\nmetadata: embedded\ncomponents:\n  - name: embedded\n\nkind: embedded" + strings.Repeat("\n", trailingNewlines)
				command := "cat <<'EOF'\n" + text + "\nEOF" + strings.Repeat("\n", trailingNewlines)
				pkg := api.Package{
					APIVersion: version,
					Kind:       api.PackageKind(v1alpha1.ZarfPackageConfig),
					Metadata:   api.PackageMetadata{Name: "literal", Description: text},
					Components: []api.Component{
						{Name: "command", Actions: api.ComponentActions{OnDeploy: api.ActionSet{Before: []api.Action{{Cmd: command}}}}},
						{Name: "description", Description: text},
					},
					Values:        api.Values{Files: []string{"values.yaml"}},
					Documentation: map[string]string{"guide": text},
				}
				var definition any = convert.PackageToV1alpha1(pkg)
				if version == v1beta1.APIVersion {
					definition = convert.PackageToV1beta1(pkg)
				}
				original, err := goyaml.MarshalWithOptions(definition, goyaml.IndentSequence(true), goyaml.UseLiteralStyleIfMultiline(true))
				require.NoError(t, err)
				path := filepath.Join(t.TempDir(), layout.ZarfYAML)
				require.NoError(t, writeSourceDefinition(path, pkg))
				contents, err := os.ReadFile(path)
				require.NoError(t, err)

				// A separate YAML implementation checks that spacing preserves the
				// entire document, including keep-chomp scalar trailing newlines.
				var before, after map[string]any
				require.NoError(t, yamlv3.Unmarshal(original, &before))
				require.NoError(t, yamlv3.Unmarshal(contents, &after))
				assert.Equal(t, before, after)
				metadata := after["metadata"].(map[string]any)
				assert.Equal(t, text, metadata["description"])
				components := after["components"].([]any)
				description := components[1].(map[string]any)
				assert.Equal(t, text, description["description"])
				actions := components[0].(map[string]any)["actions"].(map[string]any)
				beforeActions := actions["onDeploy"].(map[string]any)["before"].([]any)
				assert.Equal(t, command, beforeActions[0].(map[string]any)["cmd"])
				assert.Contains(t, string(contents), "description: |")
				assert.Contains(t, string(contents), "cmd: |")
				assert.Contains(t, string(contents), "\ncomponents:\n  - name: command\n")
			})
		}
	}
}
