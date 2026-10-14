// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build library

package bundle_test

import (
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	fixtureartifact "github.com/defenseunicorns/uds-cli/tests/fixtures/artifact"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const cacheWorkerEnv = "UDS_TEST_CACHE_WORKER"

var cacheWorkerPairs = [][2]string{{"create", "create"}, {"pull", "pull"}, {"create", "pull"}}

// TestConcurrentRemoteCacheReads covers reads from an already populated cache.
// It does not test competing writers, mutable index updates, or initial creation.
func TestConcurrentRemoteCacheReads(t *testing.T) {
	for _, pair := range cacheWorkerPairs {
		t.Run(strings.Join(pair[:], " and "), func(t *testing.T) {
			fixture, registryState := prepareConcurrentCache(t)
			before := libraryCachedBlobDigests(t, fixture.cacheDir)
			// Warm all content, then prohibit blob downloads. Each process
			// has a fresh workspace and its own cache-store instance: only
			// the shared cache can supply these package and bundle bytes.
			registryState.blockBlobs.Store(true)
			artifacts := runCacheProcesses(t, fixture.workers(t, pair))
			assert.Zero(t, registryState.blockedBlobs.Load())
			assert.Equal(t, before, libraryCachedBlobDigests(t, fixture.cacheDir))
			assertConcurrentCacheArtifacts(t, fixture.entries, artifacts)
		})
	}
}

// TestConcurrentRemoteCachePopulation is an opt-in reproduction of a specific
// upstream duplicate-writer gap. Passing it after a dependency update would not
// establish general concurrency safety for the shared cache.
func TestConcurrentRemoteCachePopulation(t *testing.T) {
	if os.Getenv("UDS_TEST_CACHE_CONCURRENT_WRITES") != "true" {
		// Keep the acceptance assertions below strict. The vendored cache
		// still returns ErrAlreadyExists when another writer wins, so this
		// scenario is pending an upstream dependency update, not passed.
		// Enable explicitly to reproduce the gap or validate that update.
		t.Skip("pending upstream duplicate-writer fix; set UDS_TEST_CACHE_CONCURRENT_WRITES=true to run the acceptance test")
	}
	for _, pair := range cacheWorkerPairs {
		t.Run(strings.Join(pair[:], " and "), func(t *testing.T) {
			fixture, registryState := prepareConcurrentCache(t)
			// Leave manifests and index metadata intact to isolate two
			// processes competing to populate one missing package blob.
			require.NoError(t, os.Remove(registryState.blobPath))
			registryState.compete.Store(true)
			artifacts := runCacheProcesses(t, fixture.workers(t, pair))
			assert.EqualValues(t, 2, registryState.misses.Load(), "both processes must encounter the same cache miss")
			contents, err := os.ReadFile(registryState.blobPath)
			require.NoError(t, err)
			assert.Equal(t, registryState.blobDigest, digest.FromBytes(contents))
			assertConcurrentCacheArtifacts(t, fixture.entries, artifacts)
		})
	}
}

// TestCacheWorkerProcess runs in separate copies of this Go test executable,
// not a UDS CLI binary. Every operation still uses only the public bundle API.
func TestCacheWorkerProcess(t *testing.T) {
	path := os.Getenv(cacheWorkerEnv)
	if path == "" {
		t.Skip("subprocess helper for the cache concurrency tests")
	}
	// The parent supplies a specification in its temporary output directory.
	// Scope the file read to that directory rather than opening an arbitrary
	// path passed through the environment.
	specRoot, err := os.OpenRoot(filepath.Dir(path))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, specRoot.Close()) })
	data, err := specRoot.ReadFile(filepath.Base(path))
	require.NoError(t, err)
	var worker cacheWorker
	require.NoError(t, json.Unmarshal(data, &worker))
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)
	// The local test server releases both processes together. This is a
	// harness barrier, not a lock around UDS or upstream cache operations.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, worker.ReadyURL, nil)
	require.NoError(t, err)
	response, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, http.StatusOK, response.StatusCode)
	config := libraryRegistryConfig(libraryFixtureConfig(t))
	config.Options.CacheDir = worker.CacheDir
	config.Options.TmpDir = worker.TmpDir
	var artifactPath string
	switch worker.Operation {
	case "create":
		result, err := bundle.Create(ctx, worker.Source, bundle.CreateOptions{
			Config: config, Signing: bundle.SigningOptions{Mode: bundle.SigningModeUnsigned},
		})
		require.NoError(t, err)
		artifactPath = result.OutputPath
	case "pull":
		result, err := bundle.Pull(ctx, worker.Source, worker.OutputDir, bundle.PullOptions{
			Config: config, Verification: bundle.VerificationPolicy{PublicKey: worker.PublicKey},
		})
		require.NoError(t, err)
		artifactPath = result.OutputPath
	default:
		t.Fatalf("unknown cache worker operation %q", worker.Operation)
	}
	// A successful operation must produce an artifact with usable package
	// content, not merely return nil after touching the shared cache.
	prepared, err := bundle.PrepareDeploySource(ctx, iostreams.IOStreams{}, artifactPath, worker.TmpDir, runtime.GOARCH)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, prepared.Close()) })
	for i := range prepared.Bundle.Packages {
		loaded, err := prepared.Loader.LoadPackageLayout(ctx, &prepared.Bundle.Packages[i], t.TempDir(), bundle.ZarfPackageLayoutLoadOptions{})
		require.NoError(t, err)
		assert.Equal(t, "concurrent-cache", loaded.Layout.PackageDefinition.Metadata.Name)
	}
	require.NoError(t, os.WriteFile(worker.ResultPath, []byte(artifactPath), 0o600))
}

type cacheWorker struct {
	Operation, Source, CacheDir, TmpDir, OutputDir, PublicKey, ReadyURL, ResultPath string
}

type concurrentCacheFixture struct {
	cacheDir, packageRef, bundleRef, publicKey, readyURL string
	entries                                              fixtureartifact.Entries
}

func (f concurrentCacheFixture) workers(t *testing.T, pair [2]string) []cacheWorker {
	t.Helper()
	workers := make([]cacheWorker, 0, len(pair))
	for _, operation := range pair {
		outputDir := t.TempDir()
		source := f.bundleRef
		if operation == "create" {
			source = writePackageVerificationBundle(t, outputDir, f.packageRef, f.publicKey)
		}
		workers = append(workers, cacheWorker{
			Operation: operation, Source: source, CacheDir: f.cacheDir,
			TmpDir: t.TempDir(), OutputDir: outputDir, PublicKey: f.publicKey,
			ReadyURL: f.readyURL, ResultPath: filepath.Join(outputDir, "result.txt"),
		})
	}
	return workers
}

// cacheRaceRegistry synchronizes network responses to reproduce a duplicate
// writer without changing the cache API or relying on arbitrary sleeps.
type cacheRaceRegistry struct {
	handler                        http.Handler
	ready, bothMissed              chan struct{}
	arrivals, misses, blockedBlobs atomic.Int64
	blockBlobs, compete            atomic.Bool
	blobPath                       string
	blobDigest                     digest.Digest
}

func (s *cacheRaceRegistry) serve(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/cache-test/ready" {
		if s.arrivals.Add(1) == 2 {
			close(s.ready)
		}
		select {
		case <-s.ready:
			w.WriteHeader(http.StatusOK)
		case <-ctx.Done():
			http.Error(w, ctx.Err().Error(), http.StatusRequestTimeout)
		}
		return
	}
	if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/blobs/") {
		if s.blockBlobs.Load() {
			s.blockedBlobs.Add(1)
			http.Error(w, "blob downloads forbidden during shared cache reads", http.StatusForbidden)
			return
		}
		if s.compete.Load() && filepath.Base(r.URL.Path) == s.blobDigest.String() {
			arrival := s.misses.Add(1)
			if arrival == 2 {
				close(s.bothMissed)
			}
			select {
			case <-s.bothMissed:
			case <-ctx.Done():
				http.Error(w, ctx.Err().Error(), http.StatusRequestTimeout)
				return
			}
			if arrival == 2 {
				// Both cache lookups have missed. Let the first process finish
				// caching, then serve the second response. Its cache Push must
				// tolerate another writer having published that blob meanwhile.
				if err := waitForCachedBlob(ctx, s.blobPath); err != nil {
					http.Error(w, err.Error(), http.StatusRequestTimeout)
					return
				}
			}
		}
	}
	s.handler.ServeHTTP(w, r)
}

func prepareConcurrentCache(t *testing.T) (concurrentCacheFixture, *cacheRaceRegistry) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	state := &cacheRaceRegistry{handler: registry.New(), ready: make(chan struct{}), bothMissed: make(chan struct{})}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { state.serve(ctx, w, r) }))
	t.Cleanup(server.Close)
	// Cancel blocked handlers before closing the server, including on failure.
	t.Cleanup(cancel)
	privateKey, publicKey := libraryKeyPair(t)
	packagePath := createLibraryPackageWithSigning(t, t.TempDir(), "concurrent-cache", privateKey)
	packageRef := "oci://" + publishLibraryPackage(t, packagePath, strings.TrimPrefix(server.URL, "http://"))
	config := libraryRegistryConfig(libraryFixtureConfig(t))
	config.Options.CacheDir = filepath.Join(t.TempDir(), "cache")
	bundleFile := writePackageVerificationBundle(t, t.TempDir(), packageRef, publicKey)
	created, err := bundle.Create(ctx, bundleFile, bundle.CreateOptions{Config: config, Signing: bundle.SigningOptions{Mode: bundle.SigningModeUnsigned}})
	require.NoError(t, err)
	require.NoError(t, bundle.Sign(ctx, bundle.SignOptions{
		Source: created.OutputPath, Config: config, TmpDir: t.TempDir(),
		Signing: bundle.SigningOptions{Mode: bundle.SigningModeKey, Key: privateKey},
	}))
	bundleRef := strings.TrimPrefix(server.URL, "http://") + "/bundles/concurrent-cache:v1"
	_, err = bundle.Push(ctx, created.OutputPath, bundleRef, bundle.PushOptions{Config: config})
	require.NoError(t, err)
	_, err = bundle.Pull(ctx, bundleRef, t.TempDir(), bundle.PullOptions{Config: config, Verification: bundle.VerificationPolicy{PublicKey: publicKey}})
	require.NoError(t, err)
	entries, err := fixtureartifact.Read(ctx, created.OutputPath)
	require.NoError(t, err)
	contents, err := entries.LayerInArtifact("components/main.tar", "")
	require.NoError(t, err)
	state.blobDigest = digest.FromBytes(contents)
	state.blobPath = filepath.Join(config.Options.CacheDir, "bundle-oci", "images", "blobs", "sha256", state.blobDigest.Encoded())
	require.FileExists(t, state.blobPath)
	return concurrentCacheFixture{
		cacheDir: config.Options.CacheDir, packageRef: packageRef, bundleRef: bundleRef,
		publicKey: publicKey, readyURL: server.URL + "/cache-test/ready", entries: entries,
	}, state
}

func runCacheProcesses(t *testing.T, workers []cacheWorker) []string {
	t.Helper()
	executable, err := os.Executable()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)
	type outcome struct {
		worker cacheWorker
		output []byte
		err    error
	}
	done := make(chan outcome, len(workers))
	commands := make([]*exec.Cmd, 0, len(workers))
	for _, worker := range workers {
		data, err := json.Marshal(worker)
		require.NoError(t, err)
		path := filepath.Join(worker.OutputDir, "worker.json")
		require.NoError(t, os.WriteFile(path, data, 0o600))
		cmd := exec.CommandContext(ctx, executable, "-test.run=^TestCacheWorkerProcess$", "-test.count=1", "-test.timeout=45s")
		cmd.Env = append(os.Environ(), cacheWorkerEnv+"="+path)
		commands = append(commands, cmd)
	}
	// Complete all fallible setup before starting children, so a setup
	// assertion cannot leave a process waiting alone at the registry barrier.
	for i, cmd := range commands {
		// Each goroutine exits when its child process finishes or the deadline
		// kills it. Drain every outcome before the test can return.
		go func() {
			output, err := cmd.CombinedOutput()
			done <- outcome{worker: workers[i], output: output, err: err}
		}()
	}
	results := make([]outcome, 0, len(workers))
	for range workers {
		results = append(results, <-done)
	}
	artifacts := make([]string, 0, len(workers))
	for _, result := range results {
		require.NoError(t, result.err, "%s cache worker failed:\n%s", result.worker.Operation, result.output)
		path, err := os.ReadFile(result.worker.ResultPath)
		require.NoError(t, err)
		artifacts = append(artifacts, string(path))
		entries, err := os.ReadDir(result.worker.TmpDir)
		require.NoError(t, err)
		assert.Empty(t, entries, "each process must clean its operation workspace, not the shared cache")
	}
	return artifacts
}

func waitForCachedBlob(ctx context.Context, path string) error {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := os.Stat(path); err == nil {
			return nil
		} else if !os.IsNotExist(err) {
			return err
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func libraryCachedBlobDigests(t *testing.T, cacheDir string) map[string]digest.Digest {
	t.Helper()
	blobs := map[string]digest.Digest{}
	root, err := os.OpenRoot(filepath.Join(cacheDir, "bundle-oci", "images", "blobs"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, root.Close()) })
	require.NoError(t, fs.WalkDir(root.FS(), ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, err := root.ReadFile(path)
		if err != nil {
			return err
		}
		blobs[path] = digest.FromBytes(data)
		return nil
	}))
	require.NotEmpty(t, blobs)
	return blobs
}

func assertConcurrentCacheArtifacts(t *testing.T, original fixtureartifact.Entries, artifacts []string) {
	t.Helper()
	assert.Len(t, artifacts, 2, "both competing operations must succeed")
	for _, path := range artifacts {
		entries, err := fixtureartifact.Read(t.Context(), path)
		require.NoError(t, err)
		assert.Equal(t, original.BlobPaths(), entries.BlobPaths())
		assert.Equal(t, original.Small["oci/index.json"], entries.Small["oci/index.json"])
	}
}
