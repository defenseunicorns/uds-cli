// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package test provides e2e tests for UDS.
package test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/require"
)

func TestBundleOptionalComponents(t *testing.T) {
	e2e.SetupDockerRegistry(t, 888)
	defer e2e.TeardownRegistry(t, 888)

	// create 2 Zarf pkgs to be bundled
	zarfPkgPath := "src/test/packages/podinfo-nginx"
	e2e.CreateZarfPkg(t, zarfPkgPath, false)

	zarfPkgPath = "src/test/packages/prometheus"
	pkg := filepath.Join(zarfPkgPath, fmt.Sprintf("zarf-package-prometheus-%s-0.0.1.tar.zst", e2e.Arch))
	e2e.CreateZarfPkg(t, zarfPkgPath, false)
<<<<<<< Updated upstream
	runCmd(t, fmt.Sprintf("zarf package publish %s oci://localhost:888 --plain-http --oci-concurrency=10 -l debug --no-progress", pkg))
=======
	runCmd(t, fmt.Sprintf("zarf package publish %s oci://localhost:888 --insecure --oci-concurrency=10 -l debug", pkg))
>>>>>>> Stashed changes

	// create bundle and publish
	bundleDir := "src/test/bundles/14-optional-components"
	bundleName := "optional-components"
	bundleTarballName := fmt.Sprintf("uds-bundle-%s-%s-0.0.1.tar.zst", bundleName, e2e.Arch)
	bundlePath := filepath.Join(bundleDir, bundleTarballName)
	runCmd(t, fmt.Sprintf("create %s --insecure --confirm -a %s", bundleDir, e2e.Arch))
	runCmd(t, fmt.Sprintf("publish %s localhost:888 --insecure", bundlePath))

	t.Run("look through contents of local bundle to ensure only selected components are present", func(t *testing.T) {
		// This functionality was removed in Zarf v0.55.2, see https://github.com/zarf-dev/zarf/issues/3829 for updates
		t.Skip("Test disabled: skipping optional component bundle content inspection")
		// local pkgs will have a correct pkg manifest (ie. missing non-selected optional component tarballs)
		// remote pkgs will not, they will contain non-selected optional component tarballs
		// because they already have a pkg manifest and we don't want to rewrite it
		introspectOptionalComponentsBundle(t)
	})

	t.Run("test local deploy", func(t *testing.T) {
		runCmd(t, fmt.Sprintf("deploy %s --retries 1 --confirm", bundlePath))
		runCmd(t, fmt.Sprintf("remove %s --confirm --insecure", bundlePath))
	})

	t.Run("test remote deploy + pulled deploy", func(t *testing.T) {
		pulledBundlePath := filepath.Join("build", bundleTarballName)
		pull(t, fmt.Sprintf("localhost:888/%s:0.0.1", bundleName), bundleTarballName)
		deployAndRemoveLocalAndRemoteInsecure(t, fmt.Sprintf("oci://localhost:888/%s:0.0.1", bundleName), pulledBundlePath)
	})
}

// introspectOptionalComponentsBundle is a helper function that decompresses a bundle tarball and introspects the contents
// (has hardcoded checks meant for only the bundle in 14-optional-components)
func introspectOptionalComponentsBundle(t *testing.T) {
	// ensure a decompressed bundle doesn't already exist
	decompressionLoc := "build/decompressed-bundle"
	err := os.RemoveAll(decompressionLoc)
	if err != nil {
		return
	}
	defer e2e.CleanFiles(decompressionLoc)

	// decompress the bundle
	bundlePath := fmt.Sprintf("src/test/bundles/14-optional-components/uds-bundle-optional-components-%s-0.0.1.tar.zst", e2e.Arch)
	runCmd(t, fmt.Sprintf("zarf tools archiver decompress %s %s", bundlePath, decompressionLoc))

	// read in the bundle's index.json
	index := ocispec.Index{}
	bundleIndexBytes, err := os.ReadFile(filepath.Join(decompressionLoc, "index.json"))
	require.NoError(t, err)
	err = json.Unmarshal(bundleIndexBytes, &index)
	require.NoError(t, err)
	require.Equal(t, 1, len(index.Manifests))
	blobsDir := filepath.Join(decompressionLoc, "blobs", "sha256")

	// grab the bundle root manifest
	rootManifesBytes, err := os.ReadFile(filepath.Join(blobsDir, index.Manifests[0].Digest.Encoded()))
	require.NoError(t, err)
	bundleRootManifest := ocispec.Manifest{}
	err = json.Unmarshal(rootManifesBytes, &bundleRootManifest)
	require.NoError(t, err)

	// grab the second pkg (note that it came from a remote source)
	pkgManifestBytes, err := os.ReadFile(filepath.Join(blobsDir, bundleRootManifest.Layers[1].Digest.Encoded()))
	require.NoError(t, err)
	remotePkgManifest := ocispec.Manifest{}
	err = json.Unmarshal(pkgManifestBytes, &remotePkgManifest)
	require.NoError(t, err)

	// ensure kiwix not present in bundle bc we didn't specify its component in the optional components
	ensureImgNotPresent(t, "ghcr.io/kiwix/kiwix-serve", remotePkgManifest, blobsDir)

	// for this remote pkg, ensure component tars exist in img manifest, but not in the bundle
	componentName := "optional-kiwix"
	verifyComponentNotIncluded := false
	for _, desc := range remotePkgManifest.Layers {
		if strings.Contains(desc.Annotations[ocispec.AnnotationTitle], fmt.Sprintf("components/%s.tar", componentName)) {
			_, err = os.ReadFile(filepath.Join(blobsDir, desc.Digest.Encoded()))
			require.ErrorContains(t, err, "no such file or directory")
			verifyComponentNotIncluded = true
		}
	}
	require.True(t, verifyComponentNotIncluded)

	// grab the third pkg (note that it came from a local source)
	pkgManifestBytes, err = os.ReadFile(filepath.Join(blobsDir, bundleRootManifest.Layers[2].Digest.Encoded()))
	require.NoError(t, err)
	localPkgManifest := ocispec.Manifest{}
	err = json.Unmarshal(pkgManifestBytes, &localPkgManifest)
	require.NoError(t, err)

	// ensure podinfo not present in bundle bc we didn't specify its component in the optional components
	ensureImgNotPresent(t, "ghcr.io/stefanprodan/podinfo:6.4.0", localPkgManifest, blobsDir)

	// for this local pkg, ensure component tars DO NOT exist in img manifest
	componentName = "podinfo"
	verifyComponentNotIncluded = true
	for _, desc := range localPkgManifest.Layers {
		if strings.Contains(desc.Annotations[ocispec.AnnotationTitle], fmt.Sprintf("components/%s.tar", componentName)) {
			// component shouldn't exist in pkg manifest for locally sourced pkgs
			verifyComponentNotIncluded = false
		}
	}
	require.True(t, verifyComponentNotIncluded)
}

func ensureImgNotPresent(t *testing.T, imgName string, remotePkgManifest ocispec.Manifest, blobsDir string) {
	// used to verify that the img is not included in the bundle
	verifyImgNotIncluded := false

	// grab image index from pkg root manifest
	var imgIndex ocispec.Index
	for _, layer := range remotePkgManifest.Layers {
		if layer.Annotations[ocispec.AnnotationTitle] == "images/index.json" {
			imgIndexBytes, err := os.ReadFile(filepath.Join(blobsDir, layer.Digest.Encoded()))
			require.NoError(t, err)
			err = json.Unmarshal(imgIndexBytes, &imgIndex)
			require.NoError(t, err)

			// ensure specified img exists in the img index but isn't actually included in the bundle
			for _, desc := range imgIndex.Manifests {
				if strings.Contains(desc.Annotations[ocispec.AnnotationBaseImageName], imgName) {
					_, err = os.ReadFile(filepath.Join(blobsDir, desc.Digest.Encoded()))
					require.ErrorContains(t, err, "no such file or directory")
					verifyImgNotIncluded = true
					break
				}
			}
			break
		}
	}
	require.True(t, verifyImgNotIncluded)
}
