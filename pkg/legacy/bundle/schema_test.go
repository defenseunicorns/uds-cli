// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/legacy/config"
	"github.com/defenseunicorns/uds-cli/pkg/legacy/types"
	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/pkg/packager/assemble"
	"github.com/zarf-dev/zarf/src/pkg/packager/load"
)

func TestCreateAndDeployMixedSchemaPackages(t *testing.T) {
	root := t.TempDir()
	oldOptions := config.CommonOptions
	config.CommonOptions.Confirm = true
	config.CommonOptions.SkipSignatureValidation = true
	config.CommonOptions.CachePath = t.TempDir()
	config.CommonOptions.TempDirectory = t.TempDir()
	t.Cleanup(func() { config.CommonOptions = oldOptions })
	output := filepath.Join(root, "deployed.txt")
	alphaOutput := filepath.Join(root, "alpha-deployed.txt")
	alphaDir := filepath.Join(root, "alpha")
	require.NoError(t, os.Mkdir(alphaDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(alphaDir, "payload.txt"), []byte("###ZARF_VAR_MESSAGE###"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(alphaDir, "zarf.yaml"), []byte(fmt.Sprintf(`apiVersion: zarf.dev/v1alpha1
kind: ZarfPackageConfig
metadata:
  name: alpha
  version: 1.0.0
variables:
  - name: MESSAGE
    default: alpha-default
components:
  - name: required
    required: true
    files:
      - source: payload.txt
        target: %q
`, alphaOutput)), 0o600))
	alphaResolved, err := load.Package(t.Context(), alphaDir, load.PackageOptions{})
	require.NoError(t, err)
	alphaLayout, err := assemble.AssemblePackage(t.Context(), alphaResolved, assemble.AssembleOptions{SkipSBOM: true, CachePath: t.TempDir()})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, alphaLayout.Cleanup()) })
	alphaPackagePath, err := alphaLayout.Archive(t.Context(), root, 0)
	require.NoError(t, err)
	excluded := filepath.Join(root, "excluded.txt")
	selected := filepath.Join(root, "selected.txt")
	require.NoError(t, os.WriteFile(filepath.Join(root, "payload.txt"), []byte("{{ .Values.message }}"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "values.yaml"), []byte("message: beta-default"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "zarf.yaml"), []byte(fmt.Sprintf(`apiVersion: zarf.dev/v1beta1
kind: ZarfPackageConfig
metadata:
  name: beta
  version: 1.0.0
values:
  files: [values.yaml]
components:
  - name: required
    files:
      - source: payload.txt
        destination: %q
        enableTemplating: true
  - name: selected
    optional: true
    files:
      - source: payload.txt
        destination: %q
        enableTemplating: true
  - name: excluded
    optional: true
    files:
      - source: payload.txt
        destination: %q
        enableTemplating: true
`, output, selected, excluded)), 0o600))
	resolved, err := load.Package(t.Context(), root, load.PackageOptions{})
	require.NoError(t, err)
	pkgLayout, err := assemble.AssemblePackage(t.Context(), resolved, assemble.AssembleOptions{SkipSBOM: true, CachePath: t.TempDir()})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pkgLayout.Cleanup()) })
	packagePath, err := pkgLayout.Archive(t.Context(), root, 0)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, config.BundleYAML), []byte(fmt.Sprintf(`kind: UDSBundle
metadata:
  name: beta
  version: 1.0.0
packages:
  - name: alpha
    path: %q
    ref: 1.0.0
  - name: beta
    path: %q
    ref: 1.0.0
    optionalComponents: [selected]
`, alphaPackagePath, packagePath)), 0o600))
	b, err := New(&types.BundleConfig{CreateOpts: types.BundleCreateOptions{
		SourceDirectory: root, BundleFile: config.BundleYAML, Output: root,
	}})
	require.NoError(t, err)
	t.Cleanup(b.ClearPaths)
	require.NoError(t, b.Create(t.Context()))
	bundlePath := filepath.Join(root, fmt.Sprintf("uds-bundle-beta-%s-1.0.0.tar.zst", config.GetArch()))
	require.FileExists(t, bundlePath)
	b.cfg.DeployOpts.Source = bundlePath
	b.cfg.DeployOpts.SkipArchitectureCheck = true
	_, _, _, err = b.PreDeployValidation()
	require.NoError(t, err)
	definition, err := b.getMetadata(b.bundle.Packages[0])
	require.NoError(t, err)
	require.Equal(t, "zarf.dev/v1alpha1", definition.GetAPIVersion())
	definition, err = b.getMetadata(b.bundle.Packages[1])
	require.NoError(t, err)
	require.Equal(t, "zarf.dev/v1beta1", definition.APIVersion)
	require.True(t, definition.Components[0].Files[0].EnableTemplating)
	b.cfg.DeployOpts.SetVariables = map[string]string{"alpha.MESSAGE": "alpha-override"}
	require.NoError(t, b.Deploy(t.Context()))
	alphaData, err := os.ReadFile(alphaOutput)
	require.NoError(t, err)
	require.Equal(t, "alpha-override\n", string(alphaData))
	for _, path := range []string{output, selected} {
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, "beta-default\n", string(data))
	}
	require.NoFileExists(t, excluded)
}
