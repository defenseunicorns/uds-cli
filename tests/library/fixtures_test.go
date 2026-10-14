// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build library

package bundle_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/pkg/packager"
	"github.com/zarf-dev/zarf/src/types"
	orasregistry "oras.land/oras-go/v2/registry"
)

const libraryBundleFileName = "bundle.uds.hcl"

func createLibraryArtifact(t *testing.T) string {
	t.Helper()
	return createLibraryBundle(t).ArtifactPath
}

type libraryBundleFixture struct {
	Root         string
	BundleFile   string
	ArtifactPath string
	BaseSource   string
	AppSource    string
	Config       *bundle.UDSBundleConfig
	CreateResult *bundle.CreateResult
}

func createLibraryBundle(t *testing.T) libraryBundleFixture {
	t.Helper()
	return createLibraryBundleWithOptions(t, true, true)
}

func createLibraryBundleWithoutOptionalComponent(t *testing.T) libraryBundleFixture {
	t.Helper()
	return createLibraryBundleWithOptions(t, false, true)
}

func createLibraryBundleWithoutDefaults(t *testing.T) libraryBundleFixture {
	t.Helper()
	return createLibraryBundleWithOptions(t, true, false)
}

func createLibraryBundleWithOptions(t *testing.T, includeOptional, includeDefaults bool) libraryBundleFixture {
	t.Helper()

	root := t.TempDir()
	baseSource := createLibraryPackage(t, root, "base")
	appSource := createLibraryPackage(t, root, "app")
	require.NoError(t, os.Mkdir(filepath.Join(root, "values"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "values", "app.yaml"), []byte("replicas: 1\n"), 0o600))
	if includeDefaults {
		require.NoError(t, os.WriteFile(filepath.Join(root, "defaults.uds.hcl"), []byte("variables = { fixture = \"default\" }\n"), 0o600))
	}
	optionalComponents := ""
	if includeOptional {
		optionalComponents = "  optional_components = [\"optional\"]\n"
	}

	bundleFile := filepath.Join(root, libraryBundleFileName)
	require.NoError(t, os.WriteFile(bundleFile, []byte(fmt.Sprintf(`uds {
  bundle_api_version = "uds.dev/v1alpha1"
}
metadata {
  name        = "library-fixture"
  description = "public library fixture"
  version     = "1.0.0"
}
package "base" {
  source = %q
  namespace = "base"
  signature_verification { verify = false }
}
package "app" {
  source = %q
  namespace = "app"
  depends_on = [package.base]
  values_files = ["values/app.yaml"]
%s
  signature_verification { verify = false }
}
`, baseSource, appSource, optionalComponents)), 0o600))

	config := libraryFixtureConfig(t)
	result, err := bundle.Create(t.Context(), bundleFile, bundle.CreateOptions{
		Config:  config,
		Signing: bundle.SigningOptions{Mode: bundle.SigningModeUnsigned},
		Streams: iostreams.IOStreams{},
	})
	require.NoError(t, err)
	return libraryBundleFixture{
		Root:         root,
		BundleFile:   bundleFile,
		ArtifactPath: result.OutputPath,
		BaseSource:   baseSource,
		AppSource:    appSource,
		Config:       config,
		CreateResult: result,
	}
}

func createLibraryPackage(t *testing.T, root, name string) string {
	t.Helper()
	return createLibraryPackageForArchitectureAndSigning(t, root, name, runtime.GOARCH, "")
}

func createLibraryPackageWithSigning(t *testing.T, root, name, signingKey string) string {
	t.Helper()
	return createLibraryPackageForArchitectureAndSigning(t, root, name, runtime.GOARCH, signingKey)
}

func createLibraryPackageForArchitecture(t *testing.T, root, name, architecture string) string {
	t.Helper()
	return createLibraryPackageForArchitectureAndSigning(t, root, name, architecture, "")
}

func createLibraryPackageForArchitectureAndSigning(t *testing.T, root, name, architecture, signingKey string) string {
	t.Helper()

	packageDir := filepath.Join(root, name)
	require.NoError(t, os.Mkdir(packageDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(packageDir, "payload.txt"), []byte(name+"\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(packageDir, "zarf.yaml"), []byte(fmt.Sprintf(`apiVersion: zarf.dev/v1alpha1
kind: ZarfPackageConfig
metadata:
  name: %s
  version: 1.0.0
  architecture: %s
components:
  - name: main
    required: true
    files:
      - source: payload.txt
        target: /tmp/%s.txt
  - name: optional
    default: false
    files:
      - source: payload.txt
        target: /tmp/%s-optional.txt
`, name, architecture, name, name)), 0o600))

	packagePath, err := packager.Create(t.Context(), packageDir, root, packager.CreateOptions{
		CachePath:      t.TempDir(),
		SigningKeyPath: signingKey,
		SkipSBOM:       true,
	})
	require.NoError(t, err)
	return packagePath
}

func libraryFixtureConfig(t *testing.T) *bundle.UDSBundleConfig {
	t.Helper()
	return libraryFixtureConfigForArchitecture(t, runtime.GOARCH)
}

func libraryFixtureConfigForArchitecture(t *testing.T, architecture string) *bundle.UDSBundleConfig {
	t.Helper()
	return &bundle.UDSBundleConfig{Options: &bundle.ConfigOptions{
		Architecture: architecture,
		Concurrency:  1,
		TmpDir:       t.TempDir(),
	}}
}

func startLibraryRegistry(t *testing.T) string {
	t.Helper()
	server := httptest.NewServer(registry.New())
	t.Cleanup(server.Close)
	return strings.TrimPrefix(server.URL, "http://")
}

type libraryCountingRegistry struct {
	host      string
	downloads atomic.Int64
}

// startLibraryCountingRegistry runs an in-process localhost registry. Only blob
// GETs count as downloads; tag resolution and signature discovery stay available.
func startLibraryCountingRegistry(t *testing.T) *libraryCountingRegistry {
	t.Helper()
	remote := &libraryCountingRegistry{}
	handler := registry.New()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/blobs/") {
			remote.downloads.Add(1)
		}
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	remote.host = strings.TrimPrefix(server.URL, "http://")
	return remote
}

func publishSignedLibraryBundle(t *testing.T, registryHost string) (libraryBundleFixture, string, bundle.VerificationPolicy) {
	t.Helper()
	fixture := createLibraryBundle(t)
	privateKey, publicKey := libraryKeyPair(t)
	require.NoError(t, bundle.Sign(t.Context(), bundle.SignOptions{
		Source:  fixture.ArtifactPath,
		Signing: bundle.SigningOptions{Mode: bundle.SigningModeKey, Key: privateKey},
		Config:  fixture.Config,
		TmpDir:  t.TempDir(),
	}))
	ref := registryHost + "/library/cached-pull:v1"
	_, err := bundle.Push(t.Context(), fixture.ArtifactPath, ref, bundle.PushOptions{Config: libraryRegistryConfig(fixture.Config)})
	require.NoError(t, err)
	return fixture, ref, bundle.VerificationPolicy{PublicKey: publicKey}
}

func publishLibraryPackage(t *testing.T, packagePath, registryHost string) string {
	t.Helper()
	local, err := packager.LoadPackage(t.Context(), packagePath, packager.LoadOptions{
		Architecture: runtime.GOARCH,
		CachePath:    t.TempDir(),
		RemoteOptions: types.RemoteOptions{
			PlainHTTP: true,
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, local.Cleanup()) })

	published, err := packager.PublishPackage(t.Context(), local, orasregistry.Reference{
		Registry:   registryHost,
		Repository: "library",
	}, packager.PublishPackageOptions{
		Retries: 1,
		RemoteOptions: types.RemoteOptions{
			PlainHTTP: true,
		},
	})
	require.NoError(t, err)
	return published.String()
}
