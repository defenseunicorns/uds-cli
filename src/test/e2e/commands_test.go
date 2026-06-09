// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package test provides e2e tests for UDS.
package test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/defenseunicorns/pkg/helpers/v2"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/message"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils"
	digest "github.com/opencontainers/go-digest"
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

type falsePositiveBlobProxy struct {
	serverURL          string
	registryHost       string
	targetRepositories map[string]struct{}
	proxy              *httputil.ReverseProxy

	mu            sync.RWMutex
	uploadedBlobs map[string]map[string]struct{}
	manifests     map[string]map[string]storedManifest
}

type storedManifest struct {
	contentType string
	body        []byte
	digest      string
}

func startFalsePositiveBlobProxy(t *testing.T, upstreamURL string, repositories []string) *falsePositiveBlobProxy {
	t.Helper()

	parsedURL, err := url.Parse(upstreamURL)
	require.NoError(t, err)

	targetRepositories := make(map[string]struct{}, len(repositories))
	for _, repo := range repositories {
		targetRepositories[repo] = struct{}{}
	}

	proxy := &falsePositiveBlobProxy{
		serverURL:          "",
		registryHost:       "",
		targetRepositories: targetRepositories,
		proxy:              httputil.NewSingleHostReverseProxy(parsedURL),
		uploadedBlobs:      make(map[string]map[string]struct{}),
		manifests:          make(map[string]map[string]storedManifest),
	}

	server := &http.Server{}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	proxy.serverURL = "http://" + listener.Addr().String()
	proxy.registryHost = listener.Addr().String()
	server.Handler = http.HandlerFunc(proxy.handle)

	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			panic(err)
		}
	}()

	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		require.NoError(t, server.Shutdown(shutdownCtx))
	})

	return proxy
}

func (p *falsePositiveBlobProxy) handle(w http.ResponseWriter, r *http.Request) {
	if repo, reference, ok := parseManifestRequest(r.URL.Path); ok && p.isTargetRepository(repo) {
		switch r.Method {
		case http.MethodPut:
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			_ = r.Body.Close()

			contentType := r.Header.Get("Content-Type")
			p.storeManifest(repo, reference, contentType, body)

			w.Header().Set("Docker-Content-Digest", digest.FromBytes(body).String())
			w.Header().Set("Location", r.URL.Path)
			w.Header().Set("Content-Length", "0")
			w.WriteHeader(http.StatusCreated)
			return
		case http.MethodGet, http.MethodHead:
			manifest, found := p.getManifest(repo, reference)
			if !found {
				http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
				return
			}

			w.Header().Set("Content-Type", manifest.contentType)
			w.Header().Set("Docker-Content-Digest", manifest.digest)
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(manifest.body)))
			if r.Method == http.MethodHead {
				w.WriteHeader(http.StatusOK)
				return
			}

			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(manifest.body)
			return
		}
	}

	if repo, digest, ok := parseBlobRequest(r.URL.Path); ok && p.isTargetRepository(repo) {
		switch r.Method {
		case http.MethodHead:
			w.Header().Set("Docker-Content-Digest", digest)
			w.Header().Set("Content-Length", "0")
			w.WriteHeader(http.StatusOK)
			return
		case http.MethodGet:
			if !p.hasUploadedBlob(repo, digest) {
				http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
				return
			}
		}
	}

	statusWriter := &statusCapturingResponseWriter{ResponseWriter: w}
	p.proxy.ServeHTTP(statusWriter, r)

	if repo, digest, ok := parseBlobUpload(r); ok && p.isTargetRepository(repo) && statusWriter.statusCode/100 == 2 {
		p.recordUploadedBlob(repo, digest)
	}
}

func (p *falsePositiveBlobProxy) isTargetRepository(repo string) bool {
	_, ok := p.targetRepositories[repo]
	return ok
}

func (p *falsePositiveBlobProxy) recordUploadedBlob(repo, digest string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if _, ok := p.uploadedBlobs[repo]; !ok {
		p.uploadedBlobs[repo] = make(map[string]struct{})
	}
	p.uploadedBlobs[repo][digest] = struct{}{}
}

func (p *falsePositiveBlobProxy) storeManifest(repo, reference, contentType string, body []byte) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if _, ok := p.manifests[repo]; !ok {
		p.manifests[repo] = make(map[string]storedManifest)
	}

	manifest := storedManifest{
		contentType: contentType,
		body:        append([]byte(nil), body...),
		digest:      digest.FromBytes(body).String(),
	}

	p.manifests[repo][reference] = manifest
	p.manifests[repo][manifest.digest] = manifest
}

func (p *falsePositiveBlobProxy) getManifest(repo, reference string) (storedManifest, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	repoManifests, ok := p.manifests[repo]
	if !ok {
		return storedManifest{}, false
	}

	manifest, ok := repoManifests[reference]
	return manifest, ok
}

func (p *falsePositiveBlobProxy) hasUploadedBlob(repo, digest string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	blobs, ok := p.uploadedBlobs[repo]
	if !ok {
		return false
	}
	_, ok = blobs[digest]
	return ok
}

func parseBlobRequest(path string) (repo string, digest string, ok bool) {
	if !strings.HasPrefix(path, "/v2/") {
		return "", "", false
	}

	trimmed := strings.TrimPrefix(path, "/v2/")
	parts := strings.SplitN(trimmed, "/blobs/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}

	return parts[0], parts[1], true
}

func parseManifestRequest(path string) (repo string, reference string, ok bool) {
	if !strings.HasPrefix(path, "/v2/") {
		return "", "", false
	}

	trimmed := strings.TrimPrefix(path, "/v2/")
	parts := strings.SplitN(trimmed, "/manifests/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}

	return parts[0], parts[1], true
}

func parseBlobUpload(r *http.Request) (repo string, digest string, ok bool) {
	if r.Method != http.MethodPut || !strings.HasPrefix(r.URL.Path, "/v2/") {
		return "", "", false
	}

	trimmed := strings.TrimPrefix(r.URL.Path, "/v2/")
	parts := strings.SplitN(trimmed, "/blobs/uploads/", 2)
	if len(parts) != 2 || parts[0] == "" {
		return "", "", false
	}

	digest = r.URL.Query().Get("digest")
	if digest == "" {
		return "", "", false
	}

	return parts[0], digest, true
}

func getBundleLayerDigest(t *testing.T, bundlePath string) string {
	t.Helper()

	bundleFile, err := os.Open(bundlePath)
	require.NoError(t, err)
	defer bundleFile.Close()

	var index ocispec.Index
	err = config.BundleArchiveFormat.Extract(context.TODO(), bundleFile, utils.ExtractJSON(&index, "index.json"))
	require.NoError(t, err)
	require.Len(t, index.Manifests, 1)

	rootManifestDigest := index.Manifests[0].Digest.Encoded()
	manifestPath := filepath.Join(config.BlobsDir, rootManifestDigest)

	bundleFile, err = os.Open(bundlePath)
	require.NoError(t, err)
	defer bundleFile.Close()

	var manifest ocispec.Manifest
	err = config.BundleArchiveFormat.Extract(context.TODO(), bundleFile, utils.ExtractJSON(&manifest, manifestPath))
	require.NoError(t, err)
	require.NotEmpty(t, manifest.Layers)

	return manifest.Layers[0].Digest.String()
}

func queryBlobStatus(t *testing.T, registryURL, repository, digest string) int {
	t.Helper()

	url := fmt.Sprintf("%s/v2/%s/blobs/%s", registryURL, repository, digest)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	_, err = io.Copy(io.Discard, resp.Body)
	require.NoError(t, err)

	return resp.StatusCode
}

type statusCapturingResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (w *statusCapturingResponseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *statusCapturingResponseWriter) Write(data []byte) (int, error) {
	if w.statusCode == 0 {
		w.statusCode = http.StatusOK
	}
	return w.ResponseWriter.Write(data)
}
