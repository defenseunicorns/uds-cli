// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build cli

package package_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/defenseunicorns/uds-cli/internal/cli"
	"github.com/defenseunicorns/uds-cli/internal/mode"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/defenseunicorns/uds-cli/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/pkg/packager"
	"github.com/zarf-dev/zarf/src/pkg/packager/assemble"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
	"github.com/zarf-dev/zarf/src/pkg/packager/load"
)

func TestPackageModDisassembleAndReassemble(t *testing.T) {
	sourceDir := testutil.TestDataPath("packages/disassemble")
	loaded, err := load.Package(t.Context(), sourceDir, load.PackageOptions{
		DefinitionOptions: load.DefinitionOptions{SkipVersionCheck: true},
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, loaded.Close()) })
	pkgLayout, err := assemble.AssemblePackage(t.Context(), loaded, assemble.AssembleOptions{SkipSBOM: true})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pkgLayout.Cleanup()) })
	archivePath, err := pkgLayout.Archive(t.Context(), t.TempDir(), 0)
	require.NoError(t, err)

	worktree := filepath.Join(t.TempDir(), "source")
	tmpDir := t.TempDir()
	features := mode.FeatureSet{
		mode.FeatureNextMode:   true,
		mode.FeaturePackageMod: true,
	}
	streams, _, _, errOut := iostreams.NewTestIOStreams()
	root := cli.NewRootCommand(streams, features)
	root.SetArgs([]string{"package", "mod", "disassemble", archivePath, worktree, "--tmp-dir", tmpDir, "--concurrency", "1"})
	require.NoError(t, root.ExecuteContext(t.Context()))
	assert.Contains(t, errOut.String(), "port all edits to the upstream source")
	require.FileExists(t, filepath.Join(worktree, "zarf.yaml"))
	require.FileExists(t, filepath.Join(worktree, ".uds", "disassembly.json"))

	outputDir := t.TempDir()
	privateKey, _ := testutil.GenerateCosignKeyPair(t)
	t.Setenv("USER", "reassembly-test")
	t.Setenv("USERNAME", "reassembly-test")
	streams, _, out, _ := iostreams.NewTestIOStreams()
	root = cli.NewRootCommand(streams, features)
	root.SetArgs([]string{"package", "mod", "reassemble", worktree, "--output", outputDir, "--tmp-dir", tmpDir, "--concurrency", "1", "--signing-key", privateKey, "--with-build-machine-info"})
	require.NoError(t, root.ExecuteContext(t.Context()))

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
}
