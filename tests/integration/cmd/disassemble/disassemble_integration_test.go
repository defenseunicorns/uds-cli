// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build integration

package disassemble_test

import (
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/defenseunicorns/uds-cli/tests/testutil"
	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/pkg/packager/assemble"
	"github.com/zarf-dev/zarf/src/pkg/packager/load"
)

func TestDisassembleAndRebuildPackage(t *testing.T) {
	uds := testutil.UDSCLIPath(t, "run via 'uds run test:next-integration'")
	sourceDir := testutil.TestDataPath("packages/disassemble")
	resolved, err := load.PackageDefinition(t.Context(), sourceDir, load.DefinitionOptions{SkipVersionCheck: true})
	require.NoError(t, err)
	pkgLayout, err := assemble.AssemblePackage(t.Context(), resolved, sourceDir, assemble.AssembleOptions{SkipSBOM: true})
	require.NoError(t, err)
	defer func() { require.NoError(t, pkgLayout.Cleanup()) }()
	archivePath, err := pkgLayout.Archive(t.Context(), t.TempDir(), 0)
	require.NoError(t, err)

	outputDir := filepath.Join(t.TempDir(), "source")
	cmd := exec.Command(uds, "--features=NextMode=true", "bundle", "dev", "disassemble", archivePath, outputDir)
	output, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "disassemble output:\n%s", output)

	generated, err := load.PackageDefinition(t.Context(), outputDir, load.DefinitionOptions{SkipVersionCheck: true})
	require.NoError(t, err)
	reassembled, err := assemble.AssemblePackage(t.Context(), generated, outputDir, assemble.AssembleOptions{SkipSBOM: true})
	require.NoError(t, err)
	require.NoError(t, reassembled.Cleanup())
}
