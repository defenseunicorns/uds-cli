// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build integration

package disassemble_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/defenseunicorns/uds-cli/tests/testutil"
	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/pkg/packager/assemble"
	"github.com/zarf-dev/zarf/src/pkg/packager/load"
)

func TestDisassembleAndRebuildPackageInBothModes(t *testing.T) {
	uds := testutil.UDSCLIPath(t, "run via 'uds run test:next-integration'")
	sourceDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "payload.txt"), []byte("offline payload\n"), 0o600))
	zarfYAML := fmt.Sprintf(`kind: ZarfPackageConfig
metadata:
  name: command-roundtrip
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
	defer func() { require.NoError(t, pkgLayout.Cleanup()) }()
	archivePath, err := pkgLayout.Archive(t.Context(), t.TempDir(), 0)
	require.NoError(t, err)

	tests := []struct {
		name     string
		nextMode bool
	}{
		{name: "Legacy"},
		{name: "Next", nextMode: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			outputDir := filepath.Join(t.TempDir(), "source")
			cmd := exec.Command(uds, "--features=NextMode=false", "dev", "disassemble", archivePath, outputDir)
			if tc.nextMode {
				cmd = exec.Command(uds, "--features=NextMode=true", "bundle", "dev", "disassemble", archivePath, outputDir)
			}
			output, err := cmd.CombinedOutput()
			require.NoErrorf(t, err, "disassemble output:\n%s", output)

			generated, err := load.PackageDefinition(t.Context(), outputDir, load.DefinitionOptions{SkipVersionCheck: true})
			require.NoError(t, err)
			reassembled, err := assemble.AssemblePackage(t.Context(), generated, outputDir, assemble.AssembleOptions{SkipSBOM: true})
			require.NoError(t, err)
			require.NoError(t, reassembled.Cleanup())
		})
	}
}
