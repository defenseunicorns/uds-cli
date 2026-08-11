// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/defenseunicorns/uds-cli/internal/artifact"
	udsoci "github.com/defenseunicorns/uds-cli/internal/oci"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	"oras.land/oras-go/v2"
	oraci "oras.land/oras-go/v2/content/oci"
)

type ociIndex = udsoci.OciIndex
type ociManifest = udsoci.OciManifest
type ociDescriptor = udsoci.OciDescriptor
type ociImageManifest = udsoci.OciImageManifest

type trackingInspectTarget struct {
	oras.Target
	fetched []ocispec.Descriptor
}

func (t *trackingInspectTarget) Fetch(ctx context.Context, desc ocispec.Descriptor) (io.ReadCloser, error) {
	t.fetched = append(t.fetched, desc)
	return t.Target.Fetch(ctx, desc)
}

func TestInspectOptions_Validate(t *testing.T) {
	config := newTestConfig()
	artifactPath := filepath.Join(t.TempDir(), "bundle.tar.zst")
	require.NoError(t, os.WriteFile(artifactPath, []byte("artifact"), tmpFilePerm))

	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name:   "local artifact",
			source: artifactPath,
		},
		{
			name:   "OCI reference",
			source: "oci://example.com/uds/bundle:v1",
		},
		{
			name:   "OCI reference with tar suffix",
			source: "oci://example.com/uds/bundle:v1.tar.zst",
		},
		{
			name:   "localhost OCI reference",
			source: "localhost:5000/uds/bundle:v1",
		},
		{
			name:   "missing source",
			source: "",
			want:   "source must not be empty",
		},
		{
			name:   "source directory",
			source: t.TempDir(),
			want:   "source must be a .tar.zst bundle artifact or OCI reference",
		},
		{
			name:   "missing artifact path",
			source: filepath.Join(t.TempDir(), "missing.tar.zst"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := (InspectOptions{Source: tt.source, Config: config}).Validate()
			if tt.want == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tt.want)
		})
	}
}

func TestInspect_LocalArtifact(t *testing.T) {
	tarball := createArchTestBundle(t, "inspect-local", "1.0.0", runtime.GOARCH)
	result, err := Inspect(t.Context(), InspectOptions{
		Source:  tarball,
		Config:  newTestConfig(),
		Streams: iostreams.IOStreams{},
	})
	require.NoError(t, err)

	assert.Equal(t, "inspect-local", result.Name)
	assert.Equal(t, "1.0.0", result.Version)
	require.NotNil(t, result.BundleSignature)
	assert.Equal(t, BundleSignatureStatusNotChecked, result.BundleSignature.Status)
	require.Len(t, result.Packages, 1)
	assert.Equal(t, "pkg1", result.Packages[0].Name)
	assert.Equal(t, PackageSigningStatusUnknown, result.Packages[0].Signature.Signed)
	assert.Equal(t, PackageVerificationStatusSkipped, result.Packages[0].Signature.Verification)

	entries := readTarZstEntries(t, tarball)
	assert.Equal(t, digest.FromBytes(entries["oci/index.json"]).String(), result.ArtifactDigest)
}

func TestInspect_UsesArtifactArchitecture(t *testing.T) {
	tarball := createArchitectureTestBundle(t)
	inspectArch := "arm64"
	if runtime.GOARCH == inspectArch {
		inspectArch = "amd64"
	}

	result, err := Inspect(t.Context(), InspectOptions{
		Source:  tarball,
		Config:  newTestConfigWithArch(inspectArch),
		Streams: iostreams.IOStreams{},
	})
	require.NoError(t, err)
	require.Len(t, result.Packages, 1)
	assert.Equal(t, "localpkg-"+runtime.GOARCH, result.Packages[0].Source)
}

func TestInspect_RequiresArtifactArchitecture(t *testing.T) {
	tarball := createArchTestBundle(t, "inspect-missing-arch", "1.0.0", runtime.GOARCH)
	entries := readTarZstEntries(t, tarball)

	var idx ociIndex
	require.NoError(t, json.Unmarshal(entries["oci/index.json"], &idx))
	idx.Annotations = nil
	indexBytes, err := json.Marshal(idx)
	require.NoError(t, err)
	entries["oci/index.json"] = indexBytes
	tampered := writeInspectTarZstEntries(t, entries)

	_, err = Inspect(t.Context(), InspectOptions{
		Source:  tampered,
		Config:  newTestConfig(),
		Streams: iostreams.IOStreams{},
	})
	require.ErrorContains(t, err, "does not record its architecture")
}

func TestInspect_LocalArtifactChecksOnlyPresentedMetadata(t *testing.T) {
	tarball := createArchTestBundle(t, "inspect-local-metadata", "1.0.0", runtime.GOARCH)
	entries := readTarZstEntries(t, tarball)

	var idx ociIndex
	require.NoError(t, json.Unmarshal(entries["oci/index.json"], &idx))
	var packageEntry ociManifest
	for _, entry := range idx.Manifests {
		if entry.ArtifactType != MediaTypeBundleDefinition {
			packageEntry = entry
			break
		}
	}
	packageManifestBytes := entries[filepath.Join("oci", "blobs", "sha256", digestToHex(t, packageEntry.Digest))]
	var packageManifest ociImageManifest
	require.NoError(t, json.Unmarshal(packageManifestBytes, &packageManifest))
	require.NotEmpty(t, packageManifest.Layers)

	layer := packageManifest.Layers[0]
	entries[filepath.Join("oci", "blobs", "sha256", digestToHex(t, layer.Digest))] = []byte("tampered package content")
	tampered := writeInspectTarZstEntries(t, entries)

	result, err := Inspect(t.Context(), InspectOptions{
		Source:  tampered,
		Config:  newTestConfig(),
		Streams: iostreams.IOStreams{},
	})
	require.NoError(t, err)
	assert.Equal(t, "inspect-local-metadata", result.Name)
}

func TestInspect_LocalArtifactRejectsCorruptMetadata(t *testing.T) {
	tarball := createPackageSignatureArtifact(t, true)
	entries := readTarZstEntries(t, tarball)

	var idx ociIndex
	require.NoError(t, json.Unmarshal(entries["oci/index.json"], &idx))
	var packageEntry ociManifest
	for _, entry := range idx.Manifests {
		if entry.ArtifactType != MediaTypeBundleDefinition {
			packageEntry = entry
			break
		}
	}
	packageManifestBytes := entries[filepath.Join("oci", "blobs", "sha256", digestToHex(t, packageEntry.Digest))]
	var packageManifest ociImageManifest
	require.NoError(t, json.Unmarshal(packageManifestBytes, &packageManifest))

	var metadataLayer ociDescriptor
	for _, layer := range packageManifest.Layers {
		if layer.Annotations[ocispec.AnnotationTitle] == "zarf.yaml" {
			metadataLayer = layer
			break
		}
	}
	require.NotEmpty(t, metadataLayer.Digest)
	entries[filepath.Join("oci", "blobs", "sha256", digestToHex(t, metadataLayer.Digest))] = []byte("tampered metadata")
	tampered := writeInspectTarZstEntries(t, entries)

	_, err := Inspect(t.Context(), InspectOptions{
		Source:  tampered,
		Config:  newTestConfig(),
		Streams: iostreams.IOStreams{},
	})
	require.ErrorContains(t, err, "size mismatch")
}

func TestInspect_LocalReconfiguredArtifactReportsProvenance(t *testing.T) {
	tarball := createTestBundle(t, `uds {
  bundle_api_version = "uds.dev/v1alpha1"
}
metadata {
  name    = "inspect-reconfigure"
  version = "1.0.0"
}
package "pkg1" {
  source = "localpkg"
}
`, `variables = { old = "value" }`)
	reconfigured, err := runLocalReconfigure(t, tarball, writeDefaultsFile(t, `variables = { new = "value" }`), "-custom")
	require.NoError(t, err)

	result, err := Inspect(t.Context(), InspectOptions{
		Source:  reconfigured.OutputPath,
		Config:  newTestConfig(),
		Streams: iostreams.IOStreams{},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, result.ReconfiguredFrom)
}

func TestInspect_RejectsNilTarget(t *testing.T) {
	_, err := inspect(t.Context(), InspectOptions{
		Source: "example.com/test/bundle:v1",
		Config: newTestConfig(),
	}, func(context.Context, string, *InspectOptions) (oras.Target, error) {
		return nil, nil
	})
	require.ErrorContains(t, err, "inspect target is nil")
}

func TestReadLocalBlobRejectsOversizedMetadata(t *testing.T) {
	data := []byte("metadata")
	d := digest.FromBytes(data)
	blobDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(blobDir, d.Encoded()), data, tmpFilePerm))

	const maxMetadataSize = 16 << 20
	_, err := udsoci.ReadLocalBlob(blobDir, ocispec.Descriptor{Digest: d, Size: maxMetadataSize + 1}, maxMetadataSize)
	require.ErrorContains(t, err, "exceeds maximum allowed size")
}

func TestInspect_RemoteReferenceMatchesLocalArtifact(t *testing.T) {
	tarball := createArchTestBundle(t, "inspect-remote", "1.0.0", runtime.GOARCH)
	localResult, err := Inspect(t.Context(), InspectOptions{
		Source:  tarball,
		Config:  newTestConfig(),
		Streams: iostreams.IOStreams{},
	})
	require.NoError(t, err)

	store, err := oraci.New(t.TempDir())
	require.NoError(t, err)
	ref := "example.com/test/inspect-remote:1.0.0"
	pushArchTestBundle(t, store, ref, tarball)

	result, err := inspect(t.Context(), InspectOptions{
		Source:  ref,
		Config:  newTestConfig(),
		Streams: iostreams.IOStreams{},
	}, func(context.Context, string, *InspectOptions) (oras.Target, error) {
		return store, nil
	})
	require.NoError(t, err)
	assert.Equal(t, localResult, result)
}

func TestInspect_RemoteReferenceFetchesMetadataOnly(t *testing.T) {
	tarball := createArchTestBundle(t, "inspect-metadata", "1.0.0", runtime.GOARCH)
	store, err := oraci.New(t.TempDir())
	require.NoError(t, err)
	ref := "example.com/test/inspect-metadata:1.0.0"
	pushArchTestBundle(t, store, ref, tarball)

	tracking := &trackingInspectTarget{Target: store}
	_, err = inspect(t.Context(), InspectOptions{
		Source:  ref,
		Config:  newTestConfig(),
		Streams: iostreams.IOStreams{},
	}, func(context.Context, string, *InspectOptions) (oras.Target, error) {
		return tracking, nil
	})
	require.NoError(t, err)

	entries := readTarZstEntries(t, tarball)
	var idx ociIndex
	require.NoError(t, json.Unmarshal(entries["oci/index.json"], &idx))
	var packageManifestEntry ociManifest
	for _, entry := range idx.Manifests {
		if entry.ArtifactType != MediaTypeBundleDefinition {
			packageManifestEntry = entry
			break
		}
	}
	manifestBytes := entries[filepath.Join("oci", "blobs", "sha256", digestToHex(t, packageManifestEntry.Digest))]
	var manifest ociImageManifest
	require.NoError(t, json.Unmarshal(manifestBytes, &manifest))

	for _, fetched := range tracking.fetched {
		for _, layer := range manifest.Layers {
			assert.NotEqual(t, layer.Digest, fetched.Digest, "inspect must not fetch package content layers")
		}
	}
}

func createArchitectureTestBundle(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeMinimalZarfPackage(t, filepath.Join(dir, "localpkg-"+runtime.GOARCH))
	bundleFile := filepath.Join(dir, BundleFileName)
	require.NoError(t, os.WriteFile(bundleFile, []byte(`uds {
  bundle_api_version = "uds.dev/v1alpha1"
}
metadata {
  name    = "inspect-arch"
  version = "1.0.0"
}
package "pkg1" {
  source = "localpkg-${sys.arch}"
  signature_verification { verify = false }
}
`), tmpFilePerm))

	created, err := Create(t.Context(), CreateOptions{
		Config:     newTestConfigWithArch(runtime.GOARCH),
		BundleFile: bundleFile,
		Streams:    iostreams.IOStreams{},
	})
	require.NoError(t, err)
	return created.OutputPath
}

func createPackageSignatureArtifact(t *testing.T, signed bool) string {
	t.Helper()
	root := t.TempDir()
	pkgDir := filepath.Join(root, "pkg")
	require.NoError(t, os.MkdirAll(pkgDir, tempDirPerm))
	signedValue := "false"
	if signed {
		signedValue = "true"
	}
	require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "zarf.yaml"), []byte("build:\n  signed: "+signedValue+"\n"), tmpFilePerm))

	bundleFile := filepath.Join(root, BundleFileName)
	require.NoError(t, os.WriteFile(bundleFile, []byte(`uds {
  bundle_api_version = "uds.dev/v1alpha1"
}
metadata { name = "signature-summary" }
package "pkg" {
  source = "pkg"
  signature_verification { verify = false }
}
`), tmpFilePerm))

	config := newTestConfig()
	config.Options.TmpDir = t.TempDir()
	created, err := Create(t.Context(), CreateOptions{
		Config:     config,
		BundleFile: bundleFile,
		Streams:    iostreams.IOStreams{},
	})
	require.NoError(t, err)
	return created.OutputPath
}

func TestInspect_PackageSignatureSummary(t *testing.T) {
	result, err := Inspect(t.Context(), InspectOptions{
		Source:  createPackageSignatureArtifact(t, true),
		Config:  newTestConfig(),
		Streams: iostreams.IOStreams{},
	})
	require.NoError(t, err)
	require.Len(t, result.Packages, 1)
	assert.Equal(t, PackageSigningStatusSigned, result.Packages[0].Signature.Signed)
	assert.Equal(t, PackageVerificationStatusSkipped, result.Packages[0].Signature.Verification)
}

func TestInspect_PackageSignatureSummaryUnsigned(t *testing.T) {
	result, err := Inspect(t.Context(), InspectOptions{
		Source:  createPackageSignatureArtifact(t, false),
		Config:  newTestConfig(),
		Streams: iostreams.IOStreams{},
	})
	require.NoError(t, err)
	require.Len(t, result.Packages, 1)
	assert.Equal(t, PackageSigningStatusUnsigned, result.Packages[0].Signature.Signed)
	assert.Equal(t, PackageVerificationStatusSkipped, result.Packages[0].Signature.Verification)
}

func createArchTestBundle(t *testing.T, name, version, arch string) string {
	t.Helper()
	dir := t.TempDir()
	writeMinimalZarfPackage(t, filepath.Join(dir, "localpkg-"+arch))
	bundleFile := filepath.Join(dir, BundleFileName)
	require.NoError(t, os.WriteFile(bundleFile, []byte(fmt.Sprintf(`uds {
  bundle_api_version = "uds.dev/v1alpha1"
}
metadata {
  name    = %q
  version = %q
}
package "pkg1" {
  source = "localpkg-%s"
  signature_verification { verify = false }
}
`, name, version, arch)), tmpFilePerm))

	created, err := Create(t.Context(), CreateOptions{
		Config:     newTestConfigWithArch(arch),
		BundleFile: bundleFile,
		Streams:    iostreams.IOStreams{},
	})
	require.NoError(t, err)
	return created.OutputPath
}

func writeInspectTarZstEntries(t *testing.T, entries map[string][]byte) string {
	t.Helper()
	root := t.TempDir()
	for name, data := range entries {
		path := filepath.Join(root, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), tempDirPerm))
		require.NoError(t, os.WriteFile(path, data, tmpFilePerm))
	}

	output := filepath.Join(t.TempDir(), "inspect.tar.zst")
	require.NoError(t, artifact.WriteTarZst(t.Context(), iostreams.IOStreams{}, output, root))
	return output
}

func pushArchTestBundle(t *testing.T, dst oras.Target, ref, tarball string) {
	t.Helper()
	workspace := t.TempDir()
	require.NoError(t, artifact.ExtractTarZst(t.Context(), iostreams.IOStreams{}, tarball, workspace))
	cfg := newTestConfig()
	cfg.Options.TmpDir = t.TempDir()
	_, err := NewDefaultPusher().PushBundle(t.Context(), workspace, ref, PushOptions{Config: cfg, PushHooks: pushTo(dst)})
	require.NoError(t, err)
}

func digestToHex(t *testing.T, value string) string {
	t.Helper()
	parts := strings.SplitN(value, ":", 2)
	require.Len(t, parts, 2, "invalid digest format: %s", value)
	return parts[1]
}

// pkgRef creates a package dependency reference for inspect tests.
func pkgRef(name string) PackageRef {
	return PackageRef{Name: name}
}

func TestToInspectResult(t *testing.T) {
	b := &UDSBundle{
		Metadata: Metadata{
			Name:        "test-bundle",
			Description: "A test bundle",
			Version:     "2.0.0",
		},
		Packages: []Package{
			{
				Name:      "db",
				Source:    "oci://ghcr.io/org/db:1.0",
				Namespace: "database",
				DependsOn: nil,
			},
			{
				Name:        "api",
				Source:      "oci://ghcr.io/org/api:2.0",
				DependsOn:   []PackageRef{pkgRef("db")},
				ValuesFiles: []string{"values/api.yaml"},
			},
		},
	}

	result, err := toInspectResult(t.Context(), b, iostreams.IOStreams{})
	require.NoError(t, err)

	assert.Equal(t, "test-bundle", result.Name)
	assert.Equal(t, "A test bundle", result.Description)
	assert.Equal(t, "2.0.0", result.Version)
	require.Len(t, result.Packages, 2)

	assert.Equal(t, "db", result.Packages[0].Name)
	assert.Equal(t, "oci://ghcr.io/org/db:1.0", result.Packages[0].Source)
	assert.Equal(t, "database", result.Packages[0].Namespace)
	assert.Nil(t, result.Packages[0].DependsOn)

	assert.Equal(t, "api", result.Packages[1].Name)
	assert.Equal(t, "oci://ghcr.io/org/api:2.0", result.Packages[1].Source)
	assert.Empty(t, result.Packages[1].Namespace)
	assert.Equal(t, []string{"db"}, result.Packages[1].DependsOn)
	assert.Equal(t, []string{"values/api.yaml"}, result.Packages[1].ValuesFiles)
}

// TestToInspectResult_DAGOrder verifies that toInspectResult returns packages
// in DAG (topological) order, not declaration order. Packages declared
// "out of order" in the HCL file must appear sorted by dependency level, and
// packages within the same level must be sorted alphabetically for determinism.
func TestToInspectResult_DAGOrder(t *testing.T) {
	// Declaration order: D, C, B, A — completely reversed from deployment order.
	// Dependency chain: A (no deps) → B depends on A → C depends on B → D depends on C
	b := &UDSBundle{
		Metadata: Metadata{Name: "dag-test"},
		Packages: []Package{
			{Name: "D", Source: "oci://example.com/D:v1", DependsOn: []PackageRef{pkgRef("C")}},
			{Name: "C", Source: "oci://example.com/C:v1", DependsOn: []PackageRef{pkgRef("B")}},
			{Name: "B", Source: "oci://example.com/B:v1", DependsOn: []PackageRef{pkgRef("A")}},
			{Name: "A", Source: "oci://example.com/A:v1"},
		},
	}

	result, err := toInspectResult(t.Context(), b, iostreams.IOStreams{})
	require.NoError(t, err)

	require.Len(t, result.Packages, 4)
	assert.Equal(t, "A", result.Packages[0].Name, "level 0: A has no dependencies")
	assert.Equal(t, "B", result.Packages[1].Name, "level 1: B depends on A")
	assert.Equal(t, "C", result.Packages[2].Name, "level 2: C depends on B")
	assert.Equal(t, "D", result.Packages[3].Name, "level 3: D depends on C")
}

// TestToInspectResult_DAGOrder_Deterministic verifies that packages within the
// same DAG level are sorted alphabetically for deterministic output.
func TestToInspectResult_DAGOrder_Deterministic(t *testing.T) {
	// Diamond: A (no deps), B and C both depend on A, D depends on B and C.
	// Declaration order deliberately puts C before B to test alphabetical sorting.
	b := &UDSBundle{
		Metadata: Metadata{Name: "diamond"},
		Packages: []Package{
			{Name: "D", Source: "oci://example.com/D:v1", DependsOn: []PackageRef{pkgRef("B"), pkgRef("C")}},
			{Name: "C", Source: "oci://example.com/C:v1", DependsOn: []PackageRef{pkgRef("A")}},
			{Name: "A", Source: "oci://example.com/A:v1"},
			{Name: "B", Source: "oci://example.com/B:v1", DependsOn: []PackageRef{pkgRef("A")}},
		},
	}

	result, err := toInspectResult(t.Context(), b, iostreams.IOStreams{})
	require.NoError(t, err)

	require.Len(t, result.Packages, 4)
	assert.Equal(t, "A", result.Packages[0].Name, "level 0: A has no dependencies")
	assert.Equal(t, "B", result.Packages[1].Name, "level 1: B before C alphabetically")
	assert.Equal(t, "C", result.Packages[2].Name, "level 1: C after B alphabetically")
	assert.Equal(t, "D", result.Packages[3].Name, "level 2: D depends on B and C")
}

func TestToInspectResult_Empty(t *testing.T) {
	b := &UDSBundle{Metadata: Metadata{Name: "empty"}}

	result, err := toInspectResult(t.Context(), b, iostreams.IOStreams{})
	require.NoError(t, err)
	assert.Equal(t, "empty", result.Name)
	assert.Empty(t, result.Packages)
}

func TestPackageSummary_NoDependsOnOmittedInJSON(t *testing.T) {
	pkg := PackageSummary{Name: "solo", Source: "oci://example.com/solo:v1"}
	data, err := json.Marshal(pkg)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "dependsOn")
}

func TestInspectResult_JSONRoundTrip(t *testing.T) {
	original := &InspectResult{
		Name:    "test",
		Version: "1.0",
		Packages: []PackageSummary{
			{Name: "pkg1", Source: "oci://example.com/pkg:v1", DependsOn: []string{"pkg0"}},
		},
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded InspectResult
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, original.Name, decoded.Name)
	assert.Equal(t, original.Version, decoded.Version)
	require.Len(t, decoded.Packages, 1)
	assert.Equal(t, "pkg1", decoded.Packages[0].Name)
}

func TestInspectResult_YAMLRoundTrip(t *testing.T) {
	original := &InspectResult{
		Name:    "test",
		Version: "1.0",
		Packages: []PackageSummary{
			{Name: "pkg1", Source: "oci://example.com/pkg:v1"},
		},
	}

	data, err := yaml.Marshal(original)
	require.NoError(t, err)

	var decoded InspectResult
	require.NoError(t, yaml.Unmarshal(data, &decoded))
	assert.Equal(t, original.Name, decoded.Name)
	assert.Equal(t, original.Version, decoded.Version)
}
