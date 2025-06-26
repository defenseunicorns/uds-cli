// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package test provides e2e tests for UDS.
package test

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/defenseunicorns/pkg/helpers/v2"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/message"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This file contains helpers for running UDS CLI commands (ie. uds create/deploy/etc with various flags and options)

func inspectRemoteInsecure(t *testing.T, ref string, bundleName string) {
	runCmd(t, fmt.Sprintf("inspect %s --insecure --sbom", ref))
	_, err := os.Stat(fmt.Sprintf("%s-%s", bundleName, config.BundleSBOMTar))
	require.NoError(t, err)
	err = os.Remove(fmt.Sprintf("%s-%s", bundleName, config.BundleSBOMTar))
	require.NoError(t, err)
}

func inspectRemote(t *testing.T, path, bundleName, ref string) {
	// ensure slash at end of path unless it's empty
	if path != "" && !strings.HasSuffix(path, "/") {
		path = path + "/"
	}

	fullBundleRef := fmt.Sprintf("%s%s:%s", path, bundleName, ref)
	runCmd(t, fmt.Sprintf("inspect %s --sbom", fullBundleRef))
	_, err := os.Stat(fmt.Sprintf("%s-%s", bundleName, config.BundleSBOMTar))
	require.NoError(t, err)
	err = os.Remove(fmt.Sprintf("%s-%s", bundleName, config.BundleSBOMTar))
	require.NoError(t, err)
}

func inspectRemoteAndSBOMExtract(t *testing.T, bundleName, ref string) {
	runCmd(t, fmt.Sprintf("inspect %s --insecure --sbom --extract", ref))
	sbomName := fmt.Sprintf("%s-%s", bundleName, config.BundleSBOM)
	_, err := os.Stat(sbomName)
	require.NoError(t, err)
	err = os.RemoveAll(sbomName)
	require.NoError(t, err)
}

func inspectLocal(t *testing.T, tarballPath string, bundleName string) {
	stdout, _ := runCmd(t, fmt.Sprintf("inspect %s --sbom --no-color", tarballPath))
	require.NotContains(t, stdout, "\x1b")
	_, err := os.Stat(fmt.Sprintf("%s-%s", bundleName, config.BundleSBOMTar))
	require.NoError(t, err)
	err = os.Remove(fmt.Sprintf("%s-%s", bundleName, config.BundleSBOMTar))
	require.NoError(t, err)
}

func inspectLocalAndSBOMExtract(t *testing.T, bundleName, tarballPath string) {
	runCmd(t, fmt.Sprintf("inspect %s --sbom --extract", tarballPath))
	sbomDir := fmt.Sprintf("%s-%s", bundleName, config.BundleSBOM)
	_, err := os.Stat(sbomDir)
	require.NoError(t, err)
	err = os.RemoveAll(sbomDir)
	require.NoError(t, err)
}

func runCmd(t *testing.T, input string) (stdout string, stderr string) {
	cmd := strings.Split(input, " ")
	stdout, stderr, err := e2e.UDS(cmd...)
	require.NoError(t, err)
	return stdout, stderr
}

func runCmdWithErr(input string) (stdout string, stderr string, err error) {
	cmd := strings.Split(input, " ")
	stdout, stderr, err = e2e.UDS(cmd...)
	return stdout, stderr, err
}

func deployAndRemoveLocalAndRemoteInsecure(t *testing.T, ref string, tarballPath string) {
	// test both paths because we want to test that the pulled tarball works as well
	t.Run(
		"deploy+remove bundle via OCI",
		func(t *testing.T) {
			runCmd(t, fmt.Sprintf("deploy %s --insecure --confirm", ref))
			runCmd(t, fmt.Sprintf("remove %s --confirm --insecure", ref))
		},
	)

	t.Run(
		"deploy+remove bundle via local tarball",
		func(t *testing.T) {
			runCmd(t, fmt.Sprintf("deploy %s --confirm", tarballPath))
			runCmd(t, fmt.Sprintf("remove %s --confirm", tarballPath))
		},
	)
}

func shasMatch(t *testing.T, path string, expected string) {
	actual, err := helpers.GetSHA256OfFile(path)
	require.NoError(t, err)
	require.Equal(t, expected, actual)
}

func pull(t *testing.T, ref string, tarballName string) {
	if !strings.HasSuffix(tarballName, "tar.zst") {
		t.Fatalf("second arg to pull() must be the name a bundle tarball, got %s", tarballName)
	}
	// todo: output somewhere other than build?
	runCmd(t, fmt.Sprintf("pull %s -o build --insecure --oci-concurrency=10", ref))

	decompressed := "build/decompressed-bundle"
	defer e2e.CleanFiles(decompressed)

	runCmd(t, fmt.Sprintf("zarf tools archiver decompress %s %s", filepath.Join("build", tarballName), decompressed))

	index := ocispec.Index{}
	b, err := os.ReadFile(filepath.Join(decompressed, "index.json"))
	require.NoError(t, err)
	err = json.Unmarshal(b, &index)
	require.NoError(t, err)

	require.Equal(t, 1, len(index.Manifests))

	blobsDir := filepath.Join(decompressed, "blobs", "sha256")

	for _, desc := range index.Manifests {
		sha := desc.Digest.Encoded()
		shasMatch(t, filepath.Join(blobsDir, sha), desc.Digest.Encoded())

		manifest := ocispec.Manifest{}
		b, err := os.ReadFile(filepath.Join(blobsDir, sha))
		require.NoError(t, err)
		err = json.Unmarshal(b, &manifest)
		require.NoError(t, err)

		for _, layer := range manifest.Layers {
			sha := layer.Digest.Encoded()
			path := filepath.Join(blobsDir, sha)
			if assert.FileExists(t, path) {
				shasMatch(t, path, layer.Digest.Encoded())
			} else {
				t.Logf("layer dne, but it might be part of a component that is not included in this bundle: \n %#+v", layer)
			}
		}
	}
}

func queryIndex(t *testing.T, registryURL, bundlePath string) (ocispec.Index, error) {
	url := fmt.Sprintf("%s/v2/%s/manifests/0.0.1", registryURL, bundlePath)
	req, err := http.NewRequest("GET", url, nil)
	require.NoError(t, err)
	req.Header.Set("Accept", ocispec.MediaTypeImageIndex)
	if registryURL == "https://ghcr.io" {
		// requires a base64 Github token (can be a PAT)
		token := os.Getenv("GITHUB_TOKEN")
		encodedToken := base64.StdEncoding.EncodeToString([]byte(token))
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", encodedToken))
	}
	client := http.DefaultClient
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	index := ocispec.Index{}
	if err != nil {
		return index, err
	}
	if strings.Contains(string(body), "errors") {
		require.Fail(t, fmt.Sprintf("Received the following error from GHCR: %s", string(body)))
	}
	err = json.Unmarshal(body, &index)
	return index, err
}

func removeZarfInit() {
	_, _, err := runCmdWithErr("zarf tools kubectl delete namespace zarf")
	message.WarnErr(err, "Failed to delete zarf namespace")
	_, _, err = runCmdWithErr("zarf tools kubectl delete mutatingwebhookconfiguration.admissionregistration.k8s.io/zarf")
	message.WarnErr(err, "Failed to delete zarf webhook")
}
