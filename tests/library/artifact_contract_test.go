// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build library

package bundle_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	fixtureartifact "github.com/defenseunicorns/uds-cli/tests/fixtures/artifact"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
	godigest "github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const libraryPackageMetadataWithoutSigning = `apiVersion: zarf.dev/v1alpha1
kind: ZarfPackageConfig
metadata:
  name: package-with-unknown-signing-status
  version: 1.0.0
components: []
`

func TestCreateAndInspectPublicContract(t *testing.T) {
	fixture := createLibraryBundle(t)

	require.NotNil(t, fixture.CreateResult)
	assert.Equal(t, "library-fixture", fixture.CreateResult.BundleName)
	assert.Equal(t, filepath.Join(fixture.Root, fmt.Sprintf("uds-bundle-library-fixture-%s-1.0.0.tar.zst", runtime.GOARCH)), fixture.CreateResult.OutputPath)
	require.FileExists(t, fixture.CreateResult.OutputPath)
	entries, err := fixtureartifact.Read(t.Context(), fixture.CreateResult.OutputPath)
	require.NoError(t, err)
	assert.True(t, entries.Paths["oci/oci-layout"])
	assert.True(t, entries.Paths["oci/index.json"])
	assert.NotEmpty(t, entries.BlobPaths())
	hasBundleDefinition, err := entries.HasLayerInArtifact(libraryBundleFileName, fixtureartifact.BundleDefinitionMediaType)
	require.NoError(t, err)
	assert.True(t, hasBundleDefinition)
	hasDefaults, err := entries.HasLayerInArtifact("defaults.uds.hcl", fixtureartifact.BundleDefinitionMediaType)
	require.NoError(t, err)
	assert.True(t, hasDefaults)

	inspected, err := bundle.Inspect(t.Context(), bundle.InspectOptions{Source: fixture.ArtifactPath, Config: fixture.Config})
	require.NoError(t, err)
	assert.Equal(t, "library-fixture", inspected.Name)
	assert.Equal(t, "public library fixture", inspected.Description)
	assert.Equal(t, "1.0.0", inspected.Version)
	assert.NotEmpty(t, inspected.ArtifactDigest)
	assert.Empty(t, inspected.ReconfiguredFrom)
	require.NotNil(t, inspected.BundleSignature)
	assert.Equal(t, bundle.BundleSignatureStatusNotChecked, inspected.BundleSignature.Status)
	require.NotNil(t, inspected.Bundle)
	require.Len(t, inspected.Packages, 2)
	assert.Equal(t, []string{"base", "app"}, []string{inspected.Packages[0].Name, inspected.Packages[1].Name})
	assert.Equal(t, fixture.BaseSource, inspected.Packages[0].Source)
	assert.Equal(t, fixture.AppSource, inspected.Packages[1].Source)
	assert.Equal(t, "base", inspected.Packages[0].Namespace)
	assert.Equal(t, "app", inspected.Packages[1].Namespace)
	assert.Empty(t, inspected.Packages[0].DependsOn)
	assert.Equal(t, []string{"base"}, inspected.Packages[1].DependsOn)
	assert.Equal(t, []string{"values/app.yaml"}, inspected.Packages[1].ValuesFiles)
	assert.Equal(t, []string{"optional"}, inspected.Bundle.Packages[1].OptionalComponents)
	require.NotNil(t, inspected.Packages[0].Signature)
	require.NotNil(t, inspected.Packages[1].Signature)
	assert.Equal(t, bundle.PackageSigningStatusUnsigned, inspected.Packages[0].Signature.Signed)
	assert.Equal(t, bundle.PackageVerificationStatusSkipped, inspected.Packages[0].Signature.Verification)
	assert.Equal(t, bundle.PackageSigningStatusUnsigned, inspected.Packages[1].Signature.Signed)
	assert.Equal(t, bundle.PackageVerificationStatusSkipped, inspected.Packages[1].Signature.Verification)
	assert.Equal(t, []string{"main", "optional"}, libraryPackageComponents(t, fixture, 1))
	assert.Contains(t, libraryPreparedDefaults(t, fixture.ArtifactPath, fixture.Config), "default")
}

func TestPushPullAndInspectPublicContract(t *testing.T) {
	fixture := createLibraryBundle(t)
	config := libraryRegistryConfig(fixture.Config)
	ref := fmt.Sprintf("%s/library/fixture:v1.0.0", startLibraryRegistry(t))
	original, err := bundle.Inspect(t.Context(), bundle.InspectOptions{Source: fixture.ArtifactPath, Config: fixture.Config})
	require.NoError(t, err)

	pushed, err := bundle.Push(t.Context(), fixture.ArtifactPath, ref, bundle.PushOptions{Config: config})
	require.NoError(t, err)
	require.NotNil(t, pushed)
	assert.Equal(t, ref, pushed.OCIReference)

	targetDir := t.TempDir()
	pulled, err := bundle.Pull(t.Context(), pushed.OCIReference, targetDir, bundle.PullOptions{
		Config:                    config,
		SkipSignatureVerification: true,
	})
	require.NoError(t, err)
	require.NotNil(t, pulled)
	assert.Equal(t, ref, pulled.OCIReference)
	assert.Equal(t, targetDir, filepath.Dir(pulled.OutputPath))
	require.FileExists(t, pulled.OutputPath)

	for _, source := range []string{fixture.ArtifactPath, ref, pulled.OutputPath} {
		inspected, err := bundle.Inspect(t.Context(), bundle.InspectOptions{
			Source:                    source,
			Config:                    config,
			SkipSignatureVerification: true,
		})
		require.NoError(t, err)
		assert.Equal(t, "library-fixture", inspected.Name)
		assert.Equal(t, "1.0.0", inspected.Version)
		assert.Equal(t, original.ArtifactDigest, inspected.ArtifactDigest)
		assert.Equal(t, []string{"base", "app"}, []string{inspected.Packages[0].Name, inspected.Packages[1].Name})
		assert.Equal(t, bundle.BundleSignatureStatusSkipped, inspected.BundleSignature.Status)
	}
	originalEntries, err := fixtureartifact.Read(t.Context(), fixture.ArtifactPath)
	require.NoError(t, err)
	pulledEntries, err := fixtureartifact.Read(t.Context(), pulled.OutputPath)
	require.NoError(t, err)
	assert.Equal(t, originalEntries.BlobPaths(), pulledEntries.BlobPaths())
	assert.Equal(t, originalEntries.Small["oci/index.json"], pulledEntries.Small["oci/index.json"])

	_, err = bundle.Pull(t.Context(), ref, t.TempDir(), bundle.PullOptions{
		Config:       config,
		Verification: bundle.VerificationPolicy{PublicKey: "unused"},
	})
	require.ErrorIs(t, err, bundle.ErrPullBundle)
	require.ErrorIs(t, err, bundle.ErrBundleNotSigned)
}

func TestInspectStatusConstants(t *testing.T) {
	assert.Equal(t, "verified", bundle.BundleSignatureStatusVerified)
	assert.Equal(t, "not_checked", bundle.BundleSignatureStatusUnverified)
	assert.Equal(t, bundle.BundleSignatureStatusNotChecked, bundle.BundleSignatureStatusUnverified)
	assert.Equal(t, "skipped", bundle.BundleSignatureStatusSkipped)
	assert.Equal(t, "signed", bundle.PackageSigningStatusSigned)
	assert.Equal(t, "unsigned", bundle.PackageSigningStatusUnsigned)
	assert.Equal(t, "unknown", bundle.PackageSigningStatusUnknown)
	assert.Equal(t, "verified", bundle.PackageVerificationStatusVerified)
	assert.Equal(t, "skipped", bundle.PackageVerificationStatusSkipped)
	assert.Equal(t, "unknown", bundle.PackageVerificationStatusUnknown)
}

func TestInspectUnknownPackageMetadata(t *testing.T) {
	fixture := createLibraryBundle(t)
	unknown := rewriteLibraryArtifact(t, fixture.ArtifactPath, func(root string) {
		rewriteUnknownPackageMetadata(t, root)
	})

	inspected, err := bundle.Inspect(t.Context(), bundle.InspectOptions{Source: unknown, Config: fixture.Config})
	require.NoError(t, err)
	for _, pkg := range inspected.Packages {
		require.NotNil(t, pkg.Signature)
		assert.Equal(t, bundle.PackageSigningStatusUnknown, pkg.Signature.Signed)
		assert.Equal(t, bundle.PackageVerificationStatusUnknown, pkg.Signature.Verification)
	}
}

func libraryRegistryConfig(config *bundle.UDSBundleConfig) *bundle.UDSBundleConfig {
	copy := *config
	options := *config.Options
	options.PlainHTTP = true
	copy.Options = &options
	return &copy
}

func TestCreateOptionalComponentCanBeExcluded(t *testing.T) {
	fixture := createLibraryBundleWithoutOptionalComponent(t)
	inspected, err := bundle.Inspect(t.Context(), bundle.InspectOptions{Source: fixture.ArtifactPath, Config: fixture.Config})
	require.NoError(t, err)
	assert.Empty(t, inspected.Bundle.Packages[1].OptionalComponents)
	assert.Equal(t, []string{"main"}, libraryPackageComponents(t, fixture, 1))
}

func TestCreateInspectsSignedVerifiedPackage(t *testing.T) {
	privateKey, publicKey := libraryKeyPair(t)
	root := t.TempDir()
	source := createLibraryPackageWithSigning(t, root, "signed", privateKey)
	bundleFile := writePackageVerificationBundle(t, root, source, publicKey)
	config := libraryFixtureConfig(t)

	created, err := bundle.Create(t.Context(), bundleFile, bundle.CreateOptions{
		Config:  config,
		Signing: bundle.SigningOptions{Mode: bundle.SigningModeUnsigned},
	})
	require.NoError(t, err)
	inspected, err := bundle.Inspect(t.Context(), bundle.InspectOptions{Source: created.OutputPath, Config: config})
	require.NoError(t, err)
	require.NotNil(t, inspected.BundleSignature)
	require.Len(t, inspected.Packages, 1)
	require.NotNil(t, inspected.Packages[0].Signature)
	assert.Equal(t, bundle.BundleSignatureStatusNotChecked, inspected.BundleSignature.Status)
	assert.Equal(t, bundle.PackageSigningStatusSigned, inspected.Packages[0].Signature.Signed)
	assert.Equal(t, bundle.PackageVerificationStatusVerified, inspected.Packages[0].Signature.Verification)
}

func TestCreateVerifiesSignedPackageFromOCI(t *testing.T) {
	privateKey, publicKey := libraryKeyPair(t)
	packageRoot := t.TempDir()
	packagePath := createLibraryPackageWithSigning(t, packageRoot, "signed-oci", privateKey)
	ref := "oci://" + publishLibraryPackage(t, packagePath, startLibraryRegistry(t))
	config := libraryRegistryConfig(libraryFixtureConfig(t))

	root := t.TempDir()
	bundleFile := writePackageVerificationBundle(t, root, ref, publicKey)
	created, err := bundle.Create(t.Context(), bundleFile, bundle.CreateOptions{
		Config:  config,
		Signing: bundle.SigningOptions{Mode: bundle.SigningModeUnsigned},
	})
	require.NoError(t, err)
	require.NotNil(t, created)
	assert.Equal(t, "package-verification", created.BundleName)
	require.FileExists(t, created.OutputPath)
	inspected, err := bundle.Inspect(t.Context(), bundle.InspectOptions{Source: created.OutputPath, Config: config})
	require.NoError(t, err)
	require.Len(t, inspected.Packages, 1)
	require.NotNil(t, inspected.Packages[0].Signature)
	assert.Equal(t, bundle.PackageSigningStatusSigned, inspected.Packages[0].Signature.Signed)
	assert.Equal(t, bundle.PackageVerificationStatusVerified, inspected.Packages[0].Signature.Verification)

	_, wrongPublicKey := libraryKeyPair(t)
	wrongRoot := t.TempDir()
	wrongBundle := writePackageVerificationBundle(t, wrongRoot, ref, wrongPublicKey)
	failed, err := bundle.Create(t.Context(), wrongBundle, bundle.CreateOptions{
		Config:  config,
		Signing: bundle.SigningOptions{Mode: bundle.SigningModeUnsigned},
	})
	require.ErrorIs(t, err, bundle.ErrCreateBundle)
	assert.Nil(t, failed)
	artifacts, err := filepath.Glob(filepath.Join(wrongRoot, "*.tar.zst"))
	require.NoError(t, err)
	assert.Empty(t, artifacts)
}

func TestCreatePublicContractSupportsMultipleArchitectures(t *testing.T) {
	for _, architecture := range []string{"amd64", "arm64"} {
		t.Run(architecture, func(t *testing.T) {
			root := t.TempDir()
			source := createLibraryPackageForArchitecture(t, root, "multiarch", architecture)
			bundleFile := filepath.Join(root, libraryBundleFileName)
			require.NoError(t, os.WriteFile(bundleFile, []byte(fmt.Sprintf(`uds {
  bundle_api_version = "uds.dev/v1alpha1"
}
metadata {
  name = "multiarch"
  version = "1.0.0"
}
package "multiarch" {
  source = %q
  signature_verification { verify = false }
}
`, source)), 0o600))
			config := libraryFixtureConfigForArchitecture(t, architecture)

			created, err := bundle.Create(t.Context(), bundleFile, bundle.CreateOptions{
				Config:  config,
				Signing: bundle.SigningOptions{Mode: bundle.SigningModeUnsigned},
			})
			require.NoError(t, err)
			require.NotNil(t, created)
			assert.Equal(t, filepath.Join(root, fmt.Sprintf("uds-bundle-multiarch-%s-1.0.0.tar.zst", architecture)), created.OutputPath)
			require.FileExists(t, created.OutputPath)
			assert.Equal(t, architecture, libraryPackageArchitecture(t, created.OutputPath, architecture))
		})
	}
}

func TestCreateRejectsWrongPackageVerificationKey(t *testing.T) {
	privateKey, _ := libraryKeyPair(t)
	_, wrongPublicKey := libraryKeyPair(t)
	root := t.TempDir()
	source := createLibraryPackageWithSigning(t, root, "signed", privateKey)
	bundleFile := writePackageVerificationBundle(t, root, source, wrongPublicKey)

	created, err := bundle.Create(t.Context(), bundleFile, bundle.CreateOptions{
		Config:  libraryFixtureConfig(t),
		Signing: bundle.SigningOptions{Mode: bundle.SigningModeUnsigned},
	})
	require.ErrorIs(t, err, bundle.ErrCreateBundle)
	assert.Nil(t, created)
	assert.NoFileExists(t, filepath.Join(root, fmt.Sprintf("uds-bundle-package-verification-%s-1.0.0.tar.zst", runtime.GOARCH)))
}

func libraryPackageComponents(t *testing.T, fixture libraryBundleFixture, packageIndex int) []string {
	t.Helper()
	source, err := bundle.PrepareDeploySource(t.Context(), iostreams.IOStreams{}, fixture.ArtifactPath, t.TempDir(), runtime.GOARCH)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, source.Close()) })
	require.NotNil(t, source.Loader)
	require.Len(t, source.Bundle.Packages, packageIndex+1)

	loaded, err := source.Loader.LoadPackageLayout(t.Context(), &source.Bundle.Packages[packageIndex], t.TempDir(), bundle.ZarfPackageLayoutLoadOptions{})
	require.NoError(t, err)
	definition := loaded.Layout.PackageDefinition.AsV1alpha1()
	assert.Equal(t, runtime.GOARCH, definition.Metadata.Architecture)
	components := make([]string, len(definition.Components))
	for i, component := range definition.Components {
		components[i] = component.Name
	}
	return components
}

func libraryPackageArchitecture(t *testing.T, artifactPath, architecture string) string {
	t.Helper()
	source, err := bundle.PrepareDeploySource(t.Context(), iostreams.IOStreams{}, artifactPath, t.TempDir(), architecture)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, source.Close()) })
	require.NotNil(t, source.Loader)
	require.Len(t, source.Bundle.Packages, 1)

	loaded, err := source.Loader.LoadPackageLayout(t.Context(), &source.Bundle.Packages[0], t.TempDir(), bundle.ZarfPackageLayoutLoadOptions{})
	require.NoError(t, err)
	return loaded.Layout.PackageDefinition.AsV1alpha1().Metadata.Architecture
}

func rewriteUnknownPackageMetadata(t *testing.T, root string) {
	t.Helper()
	indexPath := filepath.Join(root, "oci", "index.json")
	indexBytes, err := os.ReadFile(indexPath)
	require.NoError(t, err)
	var index ocispec.Index
	require.NoError(t, json.Unmarshal(indexBytes, &index))

	var hclRewritten, packageMetadataRewritten bool
	for i := range index.Manifests {
		manifestPath := filepath.Join(root, "oci", "blobs", "sha256", index.Manifests[i].Digest.Hex())
		manifestBytes, err := os.ReadFile(manifestPath)
		require.NoError(t, err)
		var manifest ocispec.Manifest
		require.NoError(t, json.Unmarshal(manifestBytes, &manifest))
		changed := false
		for j := range manifest.Layers {
			title := manifest.Layers[j].Annotations[ocispec.AnnotationTitle]
			if title != libraryBundleFileName && title != "zarf.yaml" {
				continue
			}
			var contents []byte
			switch title {
			case libraryBundleFileName:
				layerPath := filepath.Join(root, "oci", "blobs", "sha256", manifest.Layers[j].Digest.Hex())
				contents, err = os.ReadFile(layerPath)
				require.NoError(t, err)
				contents = removeLibraryPackageVerification(t, contents)
				hclRewritten = true
			case "zarf.yaml":
				contents = []byte(libraryPackageMetadataWithoutSigning)
				packageMetadataRewritten = true
			}
			writeLibraryBlob(t, root, &manifest.Layers[j], contents)
			changed = true
		}
		if changed {
			updated, err := json.Marshal(manifest)
			require.NoError(t, err)
			writeLibraryBlob(t, root, &index.Manifests[i], updated)
		}
	}
	require.True(t, hclRewritten)
	require.True(t, packageMetadataRewritten)
	updatedIndex, err := json.Marshal(index)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(indexPath, updatedIndex, 0o600))
}

func removeLibraryPackageVerification(t *testing.T, contents []byte) []byte {
	t.Helper()
	file, diagnostics := hclwrite.ParseConfig(contents, libraryBundleFileName, hcl.InitialPos)
	require.False(t, diagnostics.HasErrors())
	removed := false
	for _, block := range file.Body().Blocks() {
		if block.Type() != "package" {
			continue
		}
		for _, child := range block.Body().Blocks() {
			if child.Type() == "signature_verification" {
				removed = block.Body().RemoveBlock(child) || removed
			}
		}
	}
	require.True(t, removed)
	return file.Bytes()
}

func writeLibraryBlob(t *testing.T, root string, descriptor *ocispec.Descriptor, contents []byte) {
	t.Helper()
	digest := godigest.FromBytes(contents)
	blobsRoot, err := os.OpenRoot(filepath.Join(root, "oci", "blobs", "sha256"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, blobsRoot.Close()) })
	require.NoError(t, blobsRoot.WriteFile(digest.Hex(), contents, 0o600))
	descriptor.Digest = digest
	descriptor.Size = int64(len(contents))
}

func writePackageVerificationBundle(t *testing.T, root, source, publicKey string) string {
	t.Helper()
	bundleFile := filepath.Join(root, libraryBundleFileName)
	require.NoError(t, os.WriteFile(bundleFile, []byte(fmt.Sprintf(`uds {
  bundle_api_version = "uds.dev/v1alpha1"
}
metadata {
  name = "package-verification"
  version = "1.0.0"
}
package "signed" {
  source = %q
  signature_verification { public_key = %q }
}
`, source, publicKey)), 0o600))
	return bundleFile
}
