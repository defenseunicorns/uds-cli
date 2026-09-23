// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build library && cluster_integration && !owned_cluster

package cluster_test

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/pkg/packager"
)

const clusterBundleFileName = "bundle.uds.hcl"

type clusterBundleFixture struct {
	Root          string
	BundleFile    string
	ArtifactPath  string
	BundleName    string
	Namespace     string
	BasePackage   string
	AppPackage    string
	BaseConfigMap string
	AppConfigMap  string
	BaseSource    string
	AppSource     string
	Config        *bundle.UDSBundleConfig
}

func createClusterBundleFixture(t *testing.T) clusterBundleFixture {
	t.Helper()
	identifier := clusterFixtureIdentifier(t)
	root := t.TempDir()
	bundleName := "uds-cli-302-" + identifier
	namespace := "uds-cli-302-" + identifier
	basePackage := "base" + identifier
	appPackage := "app" + identifier
	baseConfigMap := "base-config"
	appConfigMap := "app-config"
	baseSource := createClusterPackage(t, root, basePackage, namespace, baseConfigMap)
	appSource := createClusterPackage(t, root, appPackage, namespace, appConfigMap)

	valuesDir := filepath.Join(root, "values")
	require.NoError(t, os.MkdirAll(valuesDir, 0o755))
	values := []byte(`scalar: {{ .vars.scalar }}
nested:
  value: {{ .vars.nested.value }}
items:
{{- range .vars.items }}
  - {{ . }}
{{- end }}
`)
	require.NoError(t, os.WriteFile(filepath.Join(valuesDir, "base.yaml"), values, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(valuesDir, "app.yaml"), values, 0o600))

	bundleFile := filepath.Join(root, clusterBundleFileName)
	require.NoError(t, os.WriteFile(bundleFile, []byte(fmt.Sprintf(`uds {
  bundle_api_version = "uds.dev/v1alpha1"
}
metadata {
  name = %q
  version = "1.0.0"
}
package %q {
  source = %q
  namespace = %q
  values_files = ["values/base.yaml"]
  signature_verification { verify = false }
}
package %q {
  source = %q
  namespace = %q
  values_files = ["values/app.yaml"]
  depends_on = [package.%s]
  signature_verification { verify = false }
}
`, bundleName, basePackage, baseSource, namespace, appPackage, appSource, namespace, basePackage)), 0o600))

	config := clusterBundleConfig(t, false)
	created, err := bundle.Create(t.Context(), bundleFile, bundle.CreateOptions{
		Config:  config,
		Signing: bundle.SigningOptions{Mode: bundle.SigningModeUnsigned},
		Streams: iostreams.IOStreams{},
	})
	require.NoError(t, err)
	return clusterBundleFixture{
		Root:          root,
		BundleFile:    bundleFile,
		ArtifactPath:  created.OutputPath,
		BundleName:    bundleName,
		Namespace:     namespace,
		BasePackage:   basePackage,
		AppPackage:    appPackage,
		BaseConfigMap: baseConfigMap,
		AppConfigMap:  appConfigMap,
		BaseSource:    baseSource,
		AppSource:     appSource,
		Config:        config,
	}
}

func createClusterPackage(t *testing.T, root, packageName, namespace, configMapName string) string {
	t.Helper()
	packageDir := filepath.Join(root, packageName)
	chartDir := filepath.Join(packageDir, "charts", "configmap")
	templatesDir := filepath.Join(chartDir, "templates")
	require.NoError(t, os.MkdirAll(templatesDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(chartDir, "Chart.yaml"), []byte("apiVersion: v2\nname: configmap\nversion: 0.1.0\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(chartDir, "values.yaml"), []byte("scalar: default\nnested:\n  value: default\nitems: []\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(templatesDir, "configmap.yaml"), []byte(fmt.Sprintf(`apiVersion: v1
kind: ConfigMap
metadata:
  name: %s
data:
  scalar: {{ .Values.scalar | quote }}
  nested: {{ .Values.nested.value | quote }}
  items: {{ join "," .Values.items | quote }}
`, configMapName)), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(packageDir, "zarf.yaml"), []byte(fmt.Sprintf(`apiVersion: zarf.dev/v1alpha1
kind: ZarfPackageConfig
metadata:
  name: %s
  version: 1.0.0
components:
  - name: config
    required: true
    charts:
      - name: configmap
        localPath: charts/configmap
        namespace: %s
        releaseName: %s
        values:
          - sourcePath: .scalar
            targetPath: .scalar
          - sourcePath: .nested.value
            targetPath: .nested.value
          - sourcePath: .items
            targetPath: .items
`, packageName, namespace, packageName)), 0o600))

	packagePath, err := packager.Create(t.Context(), packageDir, root, packager.CreateOptions{
		CachePath: t.TempDir(),
		SkipSBOM:  true,
	})
	require.NoError(t, err)
	return packagePath
}

func clusterBundleConfig(t *testing.T, plainHTTP bool) *bundle.UDSBundleConfig {
	t.Helper()
	return &bundle.UDSBundleConfig{
		Options: &bundle.ConfigOptions{
			Architecture: runtime.GOARCH,
			Concurrency:  1,
			PlainHTTP:    plainHTTP,
			TmpDir:       t.TempDir(),
		},
		Variables: bundle.Variables{
			"scalar": "configured",
			"nested": bundle.Variables{"value": "nested-configured"},
			"items":  []any{"first", "second"},
		},
	}
}

func startClusterRegistry(t *testing.T) string {
	t.Helper()
	server := httptest.NewServer(registry.New())
	t.Cleanup(server.Close)
	return strings.TrimPrefix(server.URL, "http://")
}

func clusterFixtureIdentifier(t *testing.T) string {
	t.Helper()
	bytes := make([]byte, 4)
	_, err := rand.Read(bytes)
	require.NoError(t, err)
	return hex.EncodeToString(bytes)
}
