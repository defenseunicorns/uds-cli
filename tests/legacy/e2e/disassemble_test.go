// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/pkg/packager/assemble"
	"github.com/zarf-dev/zarf/src/pkg/packager/load"
)

func TestDevDisassembleAndRebuildPackage(t *testing.T) {
	sourceDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "payload.txt"), []byte("offline payload\n"), 0o600))
	zarfYAML := fmt.Sprintf(`kind: ZarfPackageConfig
metadata:
  name: legacy-command-roundtrip
  version: 1.0.0
  architecture: %s
components:
  - name: app
    required: true
    files:
      - source: payload.txt
        target: /tmp/payload.txt
`, runtime.GOARCH)
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "zarf.yaml"), []byte(zarfYAML), 0o600))
	resolved, err := load.PackageDefinition(t.Context(), sourceDir, load.DefinitionOptions{SkipVersionCheck: true})
	require.NoError(t, err)
	pkgLayout, err := assemble.AssemblePackage(t.Context(), resolved, sourceDir, assemble.AssembleOptions{SkipSBOM: true})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pkgLayout.Cleanup()) })
	archivePath, err := pkgLayout.Archive(t.Context(), t.TempDir(), 0)
	require.NoError(t, err)

	outputDir := filepath.Join(t.TempDir(), "source")
	stdout, stderr, err := e2e.UDS("--features=NextMode=false", "dev", "disassemble", archivePath, outputDir)
	require.NoErrorf(t, err, "stdout:\n%s\nstderr:\n%s", stdout, stderr)

	generated, err := load.PackageDefinition(t.Context(), outputDir, load.DefinitionOptions{SkipVersionCheck: true})
	require.NoError(t, err)
	reassembled, err := assemble.AssemblePackage(t.Context(), generated, outputDir, assemble.AssembleOptions{SkipSBOM: true})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, reassembled.Cleanup()) })
}
