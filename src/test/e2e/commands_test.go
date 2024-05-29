// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

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

	"github.com/defenseunicorns/pkg/helpers"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/zarf/src/pkg/message"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This file contains helpers for running UDS CLI commands (ie. uds create/deploy/etc with various flags and options)

func zarfPublish(t *testing.T, path string, reg string) {
	args := strings.Split(fmt.Sprintf("zarf package publish %s oci://%s --insecure --oci-concurrency=10 -l debug --no-progress", path, reg), " ")
	_, _, err := e2e.UDS(args...)
	require.NoError(t, err)
}

func createLocal(t *testing.T, bundlePath string, arch string) {
	cmd := strings.Split(fmt.Sprintf("create %s --insecure --confirm -a %s", bundlePath, arch), " ")
	_, _, err := e2e.UDS(cmd...)
	require.NoError(t, err)
}

func createLocalError(bundlePath string, arch string) (stderr string) {
	cmd := strings.Split(fmt.Sprintf("create %s --insecure --confirm -a %s", bundlePath, arch), " ")
	_, stderr, _ = e2e.UDS(cmd...)
	return stderr
}

func createLocalWithOuputFlag(t *testing.T, bundlePath string, destPath string, arch string) {
	cmd := strings.Split(fmt.Sprintf("create %s -o %s --insecure --confirm -a %s", bundlePath, destPath, arch), " ")
	_, _, err := e2e.UDS(cmd...)
	require.NoError(t, err)
}

func createRemoteInsecure(t *testing.T, bundlePath, registry, arch string) {
	cmd := strings.Split(fmt.Sprintf("create %s -o %s --confirm --insecure -a %s", bundlePath, registry, arch), " ")
	_, _, err := e2e.UDS(cmd...)
	require.NoError(t, err)
}

func createRemote(t *testing.T, bundlePath, registry, arch string) {
	cmd := strings.Split(fmt.Sprintf("create %s -o %s --confirm -a %s", bundlePath, registry, arch), " ")
	_, _, err := e2e.UDS(cmd...)
	require.NoError(t, err)
}

func inspectRemoteInsecure(t *testing.T, ref string) {
	cmd := strings.Split(fmt.Sprintf("inspect %s --insecure --sbom", ref), " ")
	_, _, err := e2e.UDS(cmd...)
	require.NoError(t, err)
	_, err = os.Stat(config.BundleSBOMTar)
	require.NoError(t, err)
	err = os.Remove(config.BundleSBOMTar)
	require.NoError(t, err)
}

func inspectRemote(t *testing.T, ref string) {
	cmd := strings.Split(fmt.Sprintf("inspect %s --sbom", ref), " ")
	_, _, err := e2e.UDS(cmd...)
	require.NoError(t, err)
	_, err = os.Stat(config.BundleSBOMTar)
	require.NoError(t, err)
	err = os.Remove(config.BundleSBOMTar)
	require.NoError(t, err)
}

func inspectRemoteAndSBOMExtract(t *testing.T, ref string) {
	cmd := strings.Split(fmt.Sprintf("inspect %s --insecure --sbom --extract", ref), " ")
	_, _, err := e2e.UDS(cmd...)
	require.NoError(t, err)
	_, err = os.Stat(config.BundleSBOM)
	require.NoError(t, err)
	err = os.RemoveAll(config.BundleSBOM)
	require.NoError(t, err)
}

func inspectLocal(t *testing.T, tarballPath string) {
	cmd := strings.Split(fmt.Sprintf("inspect %s --sbom", tarballPath), " ")
	_, _, err := e2e.UDS(cmd...)
	require.NoError(t, err)
	_, err = os.Stat(config.BundleSBOMTar)
	require.NoError(t, err)
	err = os.Remove(config.BundleSBOMTar)
	require.NoError(t, err)
}

func inspectLocalAndSBOMExtract(t *testing.T, tarballPath string) {
	cmd := strings.Split(fmt.Sprintf("inspect %s --sbom --extract", tarballPath), " ")
	_, _, err := e2e.UDS(cmd...)
	require.NoError(t, err)
	_, err = os.Stat(config.BundleSBOM)
	require.NoError(t, err)
	err = os.RemoveAll(config.BundleSBOM)
	require.NoError(t, err)
}

func deploy(t *testing.T, tarballPath string) (stdout string, stderr string) {
	cmd := strings.Split(fmt.Sprintf("deploy %s --retries 1 --confirm", tarballPath), " ")
	stdout, stderr, err := e2e.UDS(cmd...)
	require.NoError(t, err)
	return stdout, stderr
}

func devDeploy(t *testing.T, bundlePath string) (stdout string, stderr string) {
	cmd := strings.Split(fmt.Sprintf("dev deploy %s --confirm", bundlePath), " ")
	stdout, stderr, err := e2e.UDS(cmd...)
	require.NoError(t, err)
	return stdout, stderr
}

func devDeployPackages(t *testing.T, tarballPath string, packages string) (stdout string, stderr string) {
	cmd := strings.Split(fmt.Sprintf("dev deploy %s --packages %s", tarballPath, packages), " ")
	stdout, stderr, err := e2e.UDS(cmd...)
	require.NoError(t, err)
	return stdout, stderr
}

func runCmd(t *testing.T, input string) (stdout string, stderr string) {
	cmd := strings.Split(input, " ")
	stdout, stderr, err := e2e.UDS(cmd...)
	require.NoError(t, err)
	return stdout, stderr
}

func deployPackagesFlag(tarballPath string, packages string) (stdout string, stderr string) {
	cmd := strings.Split(fmt.Sprintf("deploy %s --confirm -l=debug --packages %s", tarballPath, packages), " ")
	stdout, stderr, _ = e2e.UDS(cmd...)
	return stdout, stderr
}

func deployResumeFlag(t *testing.T, tarballPath string) {
	cmd := strings.Split(fmt.Sprintf("deploy %s --confirm -l=debug --resume", tarballPath), " ")
	_, _, err := e2e.UDS(cmd...)
	require.NoError(t, err)
}

func remove(t *testing.T, source string) {
	cmd := strings.Split(fmt.Sprintf("remove %s --confirm --insecure", source), " ")
	_, _, err := e2e.UDS(cmd...)
	require.NoError(t, err)
}

func removePackagesFlag(tarballPath string, packages string) (stdout string, stderr string) {
	cmd := strings.Split(fmt.Sprintf("remove %s --confirm --insecure --packages %s", tarballPath, packages), " ")
	stdout, stderr, _ = e2e.UDS(cmd...)
	return stdout, stderr
}

func deployInsecure(t *testing.T, ref string) {
	cmd := strings.Split(fmt.Sprintf("deploy %s --insecure --confirm", ref), " ")
	_, _, err := e2e.UDS(cmd...)
	require.NoError(t, err)
}

func removeInsecure(t *testing.T, remote string) {
	cmd := strings.Split(fmt.Sprintf("remove %s --insecure --confirm", remote), " ")
	_, _, err := e2e.UDS(cmd...)
	require.NoError(t, err)
}

func deployAndRemoveLocalAndRemoteInsecure(t *testing.T, ref string, tarballPath string) {
	var cmd []string
	// test both paths because we want to test that the pulled tarball works as well
	t.Run(
		"deploy+remove bundle via OCI",
		func(t *testing.T) {
			cmd = strings.Split(fmt.Sprintf("deploy %s --insecure --confirm", ref), " ")
			_, _, err := e2e.UDS(cmd...)
			require.NoError(t, err)

			cmd = strings.Split(fmt.Sprintf("remove %s --confirm --insecure", ref), " ")
			_, _, err = e2e.UDS(cmd...)
			require.NoError(t, err)
		},
	)

	t.Run(
		"deploy+remove bundle via local tarball",
		func(t *testing.T) {
			cmd = strings.Split(fmt.Sprintf("deploy %s --confirm", tarballPath), " ")
			_, _, err := e2e.UDS(cmd...)
			require.NoError(t, err)

			cmd = strings.Split(fmt.Sprintf("remove %s --confirm", tarballPath), " ")
			_, _, err = e2e.UDS(cmd...)
			require.NoError(t, err)
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
	cmd := strings.Split(fmt.Sprintf("pull %s -o build --insecure --oci-concurrency=10", ref), " ")
	_, _, err := e2e.UDS(cmd...)
	require.NoError(t, err)

	decompressed := "build/decompressed-bundle"
	defer e2e.CleanFiles(decompressed)

	cmd = []string{"zarf", "tools", "archiver", "decompress", filepath.Join("build", tarballName), decompressed}
	_, _, err = e2e.UDS(cmd...)
	require.NoError(t, err)

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

func publish(t *testing.T, bundlePath, ociPath string) {
	cmd := strings.Split(fmt.Sprintf("publish %s %s --oci-concurrency=10", bundlePath, ociPath), " ")
	_, _, err := e2e.UDS(cmd...)
	require.NoError(t, err)
}

func publishInsecure(t *testing.T, bundlePath, ociPath string) {
	cmd := strings.Split(fmt.Sprintf("publish %s %s --insecure", bundlePath, ociPath), " ")
	_, _, err := e2e.UDS(cmd...)
	require.NoError(t, err)
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
	cmd := strings.Split("zarf tools kubectl delete namespace zarf", " ")
	_, _, err := e2e.UDS(cmd...)
	message.WarnErr(err, "Failed to delete zarf namespace")
	cmd = strings.Split("zarf tools kubectl delete mutatingwebhookconfiguration.admissionregistration.k8s.io/zarf", " ")
	_, _, err = e2e.UDS(cmd...)
	message.WarnErr(err, "Failed to delete zarf webhook")
}
