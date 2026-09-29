// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build cli

package bundle_test

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/stretchr/testify/require"

	bundleinternal "github.com/defenseunicorns/uds-cli/internal/bundle"
	udsoci "github.com/defenseunicorns/uds-cli/internal/oci"
	bundlepkg "github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	fixtureartifact "github.com/defenseunicorns/uds-cli/tests/fixtures/artifact"
	"github.com/defenseunicorns/uds-cli/tests/testutil"
)

func executeCLI(t *testing.T, input string, args ...string) (string, error) {
	t.Helper()
	streams, in, out, errOut := iostreams.NewTestIOStreams()
	if input != "" {
		_, err := in.WriteString(input)
		require.NoError(t, err)
	}
	err := testutil.ExecuteCLI(t.Context(), streams, args...)
	return out.String() + errOut.String(), err
}

type inspectResult struct {
	Name             string                  `json:"name" yaml:"name"`
	Version          string                  `json:"version" yaml:"version"`
	ArtifactDigest   string                  `json:"artifactDigest" yaml:"artifactDigest"`
	ReconfiguredFrom string                  `json:"reconfiguredFrom" yaml:"reconfiguredFrom"`
	BundleSignature  *bundleSignatureSummary `json:"bundleSignature" yaml:"bundleSignature"`
	Packages         []inspectPackage        `json:"packages" yaml:"packages"`
}

type bundleSignatureSummary struct {
	Status string `json:"status" yaml:"status"`
}

type inspectPackage struct {
	Name      string                   `json:"name" yaml:"name"`
	DependsOn []string                 `json:"dependsOn" yaml:"dependsOn"`
	Signature *packageSignatureSummary `json:"signature" yaml:"signature"`
}

type packageSignatureSummary struct {
	Signed       string `json:"signed" yaml:"signed"`
	Verification string `json:"verification" yaml:"verification"`
}

func createInspectArtifact(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	pkgDir := filepath.Join(root, "pkg")
	require.NoError(t, os.MkdirAll(pkgDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "zarf.yaml"), []byte("build:\n  signed: true\nmetadata:\n  name: test\n  version: 1.0.0\n  aggregateChecksum: e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "checksums.txt"), nil, 0o600))

	bundleFile := filepath.Join(root, bundleinternal.BundleFileName)
	require.NoError(t, os.WriteFile(bundleFile, []byte(`uds {
  bundle_api_version = "uds.dev/v1alpha1"
}
metadata {
  name    = "inspect-integration"
  version = "1.0.0"
}
package "pkg" {
  source = "pkg"
  signature_verification { verify = false }
}
`), 0o600))

	defaults := bundlepkg.ConfigOptions{
		Architecture: runtime.GOARCH,
		Concurrency:  10,
		LogLevel:     "info",
		TmpDir:       t.TempDir(),
	}
	result, err := bundlepkg.Create(t.Context(), bundleFile, bundlepkg.CreateOptions{
		Config:  &bundlepkg.UDSBundleConfig{Options: &defaults},
		Signing: bundlepkg.SigningOptions{Mode: bundlepkg.SigningModeUnsigned},
		Streams: iostreams.IOStreams{},
	})
	require.NoError(t, err)
	return result.OutputPath
}

func readBundleEntries(t *testing.T, tarPath string) (allPaths map[string]bool, small map[string][]byte) {
	t.Helper()
	entries, err := fixtureartifact.Read(t.Context(), tarPath)
	require.NoError(t, err)
	return entries.Paths, entries.Small
}

// bundleDefinitionContainsLayerTitle reports whether the bundle definition manifest
// (identified by artifactType == MediaTypeBundleDefinition) contains a layer with
// the given org.opencontainers.image.title AND the corresponding blob is present.
func bundleDefinitionContainsLayerTitle(t *testing.T, allPaths map[string]bool, small map[string][]byte, title string) bool {
	t.Helper()
	hasLayer, err := fixtureartifact.Entries{Paths: allPaths, Small: small}.HasLayerInArtifact(title, udsoci.MediaTypeBundleDefinition)
	require.NoError(t, err)
	return hasLayer
}

// startLocalTLSRegistry starts an in-memory OCI registry with a self-signed TLS certificate.
func startLocalTLSRegistry(t *testing.T) string {
	t.Helper()
	s := httptest.NewTLSServer(registry.New())
	t.Cleanup(s.Close)
	return strings.TrimPrefix(s.URL, "https://")
}

// extractLayerFromBundle extracts a layer's blob content by title from the bundle definition manifest.
func extractLayerFromBundle(t *testing.T, small map[string][]byte, title string) []byte {
	t.Helper()
	contents, err := fixtureartifact.Entries{Small: small}.LayerInArtifact(title, udsoci.MediaTypeBundleDefinition)
	require.NoError(t, err)
	return contents
}

// assertHasReconfiguredAnnotation verifies the bundle definition manifest has the provenance annotation.
func assertHasReconfiguredAnnotation(t *testing.T, small map[string][]byte) string {
	t.Helper()
	value := reconfiguredAnnotation(t, small)
	if !strings.HasPrefix(value, "sha256:") {
		t.Fatal("reconfigured-from annotation should be a sha256 digest")
	}
	return value
}

func reconfiguredAnnotation(t *testing.T, small map[string][]byte) string {
	t.Helper()

	idxBytes, ok := small["oci/index.json"]
	require.True(t, ok)

	var idx struct {
		Manifests []struct {
			Digest       string `json:"digest"`
			ArtifactType string `json:"artifactType"`
		} `json:"manifests"`
	}
	require.NoError(t, json.Unmarshal(idxBytes, &idx))

	for _, m := range idx.Manifests {
		if m.ArtifactType != udsoci.MediaTypeBundleDefinition {
			continue
		}
		hex := strings.TrimPrefix(m.Digest, "sha256:")
		manifestBytes, ok := small["oci/blobs/sha256/"+hex]
		require.True(t, ok)

		var manifest struct {
			Annotations map[string]string `json:"annotations"`
		}
		require.NoError(t, json.Unmarshal(manifestBytes, &manifest))

		value, has := manifest.Annotations[udsoci.AnnotationReconfiguredFrom]
		if !has {
			t.Fatal("bundle definition manifest missing reconfigured-from annotation")
		}
		return value
	}
	t.Fatal("bundle definition manifest not found")
	return ""
}
