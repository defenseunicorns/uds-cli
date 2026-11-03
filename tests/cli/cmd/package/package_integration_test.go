// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build cli

package package_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	packageoci "github.com/defenseunicorns/pkg/oci"
	"github.com/defenseunicorns/uds-cli/internal/cli"
	"github.com/defenseunicorns/uds-cli/internal/mode"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/defenseunicorns/uds-cli/tests/testutil"
	"github.com/google/go-containerregistry/pkg/registry"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/pkg/packager"
	"github.com/zarf-dev/zarf/src/pkg/packager/assemble"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
	"github.com/zarf-dev/zarf/src/pkg/packager/load"
	"github.com/zarf-dev/zarf/src/pkg/zoci"
	zarftypes "github.com/zarf-dev/zarf/src/types"
)

func TestPackageModDisassembleAndReassemble(t *testing.T) {
	sourceDir := testutil.TestDataPath("packages/disassemble")
	loaded, err := load.Package(t.Context(), sourceDir, load.PackageOptions{
		DefinitionOptions: load.DefinitionOptions{SkipVersionCheck: true, CachePath: t.TempDir()},
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, loaded.Close()) })
	pkgLayout, err := assemble.AssemblePackage(t.Context(), loaded, assemble.AssembleOptions{SkipSBOM: true, CachePath: t.TempDir()})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pkgLayout.Cleanup()) })
	manifest, err := pkgLayout.Manifest()
	require.NoError(t, err)
	componentLayer := manifest.Locate(filepath.Join(layout.ComponentsDir, "app.tar"))
	require.NotEmpty(t, componentLayer.Digest)
	diagnostics := &packagePullLog{progressSeen: make(chan struct{})}
	registryHandler := registry.New()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/blobs/"+componentLayer.Digest.String()) {
			// Keep the pull active until its native reporter reaches the command's diagnostics stream.
			select {
			case <-diagnostics.progressSeen:
			case <-r.Context().Done():
				return
			}
		}
		registryHandler.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	ref := strings.TrimPrefix(server.URL, "http://") + "/test/disassemble:1.0.0"
	architecture := pkgLayout.Definition().Build.Architecture
	remote, err := zoci.NewRemoteWithOptions(t.Context(), ref, ocispec.Platform{Architecture: architecture, OS: packageoci.MultiOS}, zoci.RemoteClientOptions{
		RemoteOptions: zarftypes.RemoteOptions{PlainHTTP: true},
	})
	require.NoError(t, err)
	_, err = remote.PushPackage(t.Context(), pkgLayout, zoci.PublishOptions{Retries: 1, OCIConcurrency: 1})
	require.NoError(t, err)

	worktree := filepath.Join(t.TempDir(), "source")
	tmpDir := t.TempDir()
	cacheDir := t.TempDir()
	features := mode.FeatureSet{
		mode.FeatureNextMode:   true,
		mode.FeaturePackageMod: true,
	}
	var disassemblyOutput bytes.Buffer
	streams := iostreams.New(nil, &disassemblyOutput, diagnostics)
	root := cli.NewRootCommand(streams, features)
	root.SetArgs([]string{"package", "mod", "disassemble", "oci://" + ref, worktree, "--plain-http", "-a", architecture, "--tmp-dir", tmpDir, "--cache", cacheDir, "--concurrency", "1"})
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	require.NoError(t, root.ExecuteContext(ctx))
	assert.Contains(t, diagnostics.String(), "pulling package")
	assert.Contains(t, diagnostics.String(), "package pull in progress")
	assert.Contains(t, diagnostics.String(), "finished pulling package layers")
	assert.Contains(t, diagnostics.String(), "port all edits to the upstream source")
	assert.NotContains(t, disassemblyOutput.String(), "package pull in progress")
	require.FileExists(t, filepath.Join(worktree, "zarf.yaml"))
	require.FileExists(t, filepath.Join(worktree, ".uds", "disassembly.json"))

	t.Run("clean diff", func(t *testing.T) {
		streams, _, out, errOut := iostreams.NewTestIOStreams()
		root := cli.NewRootCommand(streams, features)
		root.SetArgs([]string{"package", "mod", "diff", worktree})
		require.NoError(t, root.ExecuteContext(t.Context()))
		assert.Contains(t, out.String(), "Package digest: sha256:")
		assert.Contains(t, out.String(), "Source hint:")
		assert.Contains(t, out.String(), "Working inventory digest: sha256:")
		assert.Contains(t, out.String(), "No changes.")
		assert.Empty(t, errOut.String())
	})

	outputDir := t.TempDir()
	privateKey, _ := testutil.GenerateCosignKeyPair(t)
	t.Setenv("USER", "reassembly-test")
	t.Setenv("USERNAME", "reassembly-test")
	streams, _, out, errOut := iostreams.NewTestIOStreams()
	root = cli.NewRootCommand(streams, features)
	root.SetArgs([]string{"package", "mod", "reassemble", worktree, "--output", outputDir, "--tmp-dir", tmpDir, "--cache", cacheDir, "--concurrency", "1", "--signing-key", privateKey, "--with-build-machine-info"})
	require.NoError(t, root.ExecuteContext(t.Context()))
	for _, message := range []string{"assembling package", "package signed successfully", "writing package to disk"} {
		assert.Contains(t, errOut.String(), message)
		assert.NotContains(t, out.String(), message)
	}

	matches, err := filepath.Glob(filepath.Join(outputDir, "zarf-package-*.tar.zst"))
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Contains(t, filepath.Base(matches[0]), "-disassembled")
	assert.Contains(t, out.String(), matches[0])
	_, err = os.Stat(matches[0])
	require.NoError(t, err)
	reassembled, err := packager.LoadPackage(t.Context(), matches[0], packager.LoadOptions{VerificationStrategy: layout.VerifyIfPossible})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, reassembled.Cleanup()) })
	require.FileExists(t, filepath.Join(reassembled.DirPath(), layout.Bundle))
	build := reassembled.AsV1alpha1().Build
	require.NotNil(t, build.Signed)
	assert.True(t, *build.Signed)
	assert.Equal(t, "reassembly-test", build.User)
	assert.NotEmpty(t, build.Terminal)

	t.Run("changed diff", func(t *testing.T) {
		definitionPath := filepath.Join(worktree, "zarf.yaml")
		definition, err := os.ReadFile(definitionPath)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(definitionPath, append(definition, []byte("\n# local workaround\n")...), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(worktree, "notes.txt"), []byte("reproduce the workaround upstream\n"), 0o600))
		payloads, err := filepath.Glob(filepath.Join(worktree, "components", "app", "files", "*"))
		require.NoError(t, err)
		require.Len(t, payloads, 1)
		require.NoError(t, os.Remove(payloads[0]))
		deletedPath, err := filepath.Rel(worktree, payloads[0])
		require.NoError(t, err)

		streams, _, out, errOut := iostreams.NewTestIOStreams()
		root := cli.NewRootCommand(streams, features)
		root.SetArgs([]string{"package", "mod", "diff", worktree})
		require.NoError(t, root.ExecuteContext(t.Context()))
		report := out.String()
		assert.Contains(t, report, "modified \"zarf.yaml\"")
		assert.Contains(t, report, "added \"notes.txt\"")
		assert.Contains(t, report, "deleted \""+filepath.ToSlash(deletedPath)+"\"")
		assert.Contains(t, report, "+# local workaround")
		assert.Contains(t, report, "+reproduce the workaround upstream")
		assert.Contains(t, report, "-offline payload")
		assert.Contains(t, report, "@@")
		assert.NotContains(t, report, "No changes.")
		assert.Empty(t, errOut.String())
		assert.Less(t, strings.Index(report, "deleted \""), strings.Index(report, "added \"notes.txt\""))
		assert.Less(t, strings.Index(report, "added \"notes.txt\""), strings.Index(report, "modified \"zarf.yaml\""))
	})

	for _, metadataState := range []string{"missing", "corrupt"} {
		t.Run(metadataState+" diff metadata", func(t *testing.T) {
			metadataPath := filepath.Join(worktree, ".uds", "disassembly.json")
			if metadataState == "missing" {
				require.NoError(t, os.Remove(metadataPath))
			} else {
				require.NoError(t, os.WriteFile(metadataPath, []byte("{not json"), 0o600))
			}
			streams, _, out, errOut := iostreams.NewTestIOStreams()
			root := cli.NewRootCommand(streams, features)
			root.SetArgs([]string{"package", "mod", "diff", worktree})
			require.Error(t, root.ExecuteContext(t.Context()))
			assert.Empty(t, out.String(), "validation must finish before any report is written")
			assert.Empty(t, errOut.String(), "the executable owns error reporting")
		})
	}
}

type packagePullLog struct {
	bytes.Buffer
	progressSeen chan struct{}
	once         sync.Once
}

func (w *packagePullLog) Write(p []byte) (int, error) {
	n, err := w.Buffer.Write(p)
	if strings.Contains(w.String(), "package pull in progress") {
		w.once.Do(func() { close(w.progressSeen) })
	}
	return n, err
}
