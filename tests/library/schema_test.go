// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build library

package bundle_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/pkg/packager/assemble"
	"github.com/zarf-dev/zarf/src/pkg/packager/load"
)

func TestCreateAndDeployMixedSchemaPackages(t *testing.T) {
	root := t.TempDir()
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
	require.NoError(t, os.WriteFile(filepath.Join(root, "payload.txt"), []byte("{{ .Values.message }}"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "values.yaml"), []byte("message: beta-default"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "override.yaml"), []byte("message: '{{ .vars.message }}'"), 0o600))
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
  - name: excluded
    optional: true
    files:
      - source: payload.txt
        destination: %q
        enableTemplating: true
`, output, excluded)), 0o600))
	resolved, err := load.Package(t.Context(), root, load.PackageOptions{})
	require.NoError(t, err)
	pkgLayout, err := assemble.AssemblePackage(t.Context(), resolved, assemble.AssembleOptions{SkipSBOM: true, CachePath: t.TempDir()})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pkgLayout.Cleanup()) })
	packagePath, err := pkgLayout.Archive(t.Context(), root, 0)
	require.NoError(t, err)
	bundlePath := filepath.Join(root, libraryBundleFileName)
	require.NoError(t, os.WriteFile(bundlePath, []byte(fmt.Sprintf(`uds { bundle_api_version = "uds.dev/v1alpha1" }
metadata {
  name = "beta"
  version = "1.0.0"
}
package "alpha" {
  source = %q
  signature_verification { verify = false }
}
package "beta" {
  source = %q
  values_files = ["override.yaml"]
  signature_verification { verify = false }
}
`, alphaPackagePath, packagePath)), 0o600))
	config := libraryFixtureConfig(t)
	config.Variables = map[string]any{"message": "beta-override"}
	created, err := bundle.Create(t.Context(), bundlePath, bundle.CreateOptions{
		Config: config, Signing: bundle.SigningOptions{Mode: bundle.SigningModeUnsigned},
	})
	require.NoError(t, err)
	source, err := bundle.PrepareDeploySource(t.Context(), iostreams.IOStreams{}, created.OutputPath, t.TempDir(), config.Options.Architecture)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, source.Close()) })
	var inspectedPackages []string
	_, err = bundle.Deploy(t.Context(), source, bundle.DeployOptions{
		Config: config,
		PackageDeployHooks: bundle.PackageDeployHooks{PreDeploy: func(_ context.Context, pkg *spec.Package, layout *bundle.ZarfPackageLayout, _ *bundle.DeployPackageOptions) error {
			if pkg.Name == "alpha" {
				require.Equal(t, "zarf.dev/v1alpha1", layout.PackageDefinition.GetAPIVersion())
			} else {
				require.Equal(t, "zarf.dev/v1beta1", layout.PackageDefinition.APIVersion)
				require.Len(t, layout.PackageDefinition.Components, 1)
				require.True(t, layout.PackageDefinition.Components[0].Files[0].EnableTemplating)
			}
			inspectedPackages = append(inspectedPackages, pkg.Name)
			return nil
		}},
	})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"alpha", "beta"}, inspectedPackages)
	alphaData, err := os.ReadFile(alphaOutput)
	require.NoError(t, err)
	require.Equal(t, "beta-override\n", string(alphaData))
	data, err := os.ReadFile(output)
	require.NoError(t, err)
	require.Equal(t, "beta-override\n", string(data))
	require.NoFileExists(t, excluded)
}
