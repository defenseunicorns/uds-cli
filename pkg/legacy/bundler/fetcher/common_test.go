// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package fetcher

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/legacy/config"
	godigest "github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/api"
	"github.com/zarf-dev/zarf/src/api/v1alpha1" //nolint:staticcheck // Preserve support for existing v1alpha1 packages.
	"github.com/zarf-dev/zarf/src/api/v1beta1"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
	"oras.land/oras-go/v2/registry/remote/auth"
)

func TestFilterPkgPathsSBOMs(t *testing.T) {
	tests := []struct {
		name       string
		apiVersion string
		sbomFiles  []string
	}{
		{
			name:       "alpha archive",
			apiVersion: v1alpha1.APIVersion,
			sbomFiles:  []string{layout.SBOMTar},
		},
		{
			name:       "beta resources",
			apiVersion: v1beta1.APIVersion,
			sbomFiles: []string{
				layout.ComponentSBOMResourcePath("selected"),
				layout.ComponentSBOMResourcePath("excluded"),
				layout.ImageSBOMResourcePath("ghcr.io/example/image:1"),
			},
		},
		{name: "alpha without SBOM", apiVersion: v1alpha1.APIVersion},
		{name: "beta without SBOM", apiVersion: v1beta1.APIVersion},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pkgDir := t.TempDir()
			files := map[string][]byte{
				filepath.Join(layout.ComponentsDir, "selected.tar"): []byte("selected"),
				filepath.Join(layout.ComponentsDir, "excluded.tar"): []byte("excluded"),
			}
			for _, path := range tt.sbomFiles {
				files[filepath.FromSlash(path)] = []byte("sbom")
			}

			var checksums string
			for path, contents := range files {
				fullPath := filepath.Join(pkgDir, path)
				require.NoError(t, os.MkdirAll(filepath.Dir(fullPath), 0o700))
				require.NoError(t, os.WriteFile(fullPath, contents, 0o600))
				checksums += fmt.Sprintf("%s %s\n", godigest.FromBytes(contents).Encoded(), filepath.ToSlash(path))
			}
			checksumBytes := []byte(checksums)
			definition := api.Package{
				APIVersion: tt.apiVersion,
				Kind:       api.ZarfPackageConfig,
				Metadata:   api.PackageMetadata{Name: "test", Version: "1.0.0"},
				Build:      api.BuildData{AggregateChecksum: godigest.FromBytes(checksumBytes).Encoded()},
				Components: []api.Component{
					{Name: "selected", Optional: true, Files: []api.File{{Source: "payload.txt", Destination: "/tmp/selected.txt"}}},
					{Name: "excluded", Optional: true},
				},
			}
			require.NoError(t, layout.WritePackageDefinition(filepath.Join(pkgDir, layout.ZarfYAML), definition))
			require.NoError(t, os.WriteFile(filepath.Join(pkgDir, layout.Checksums), checksumBytes, 0o600))

			pkgLayout, err := layout.LoadFromDir(t.Context(), pkgDir, layout.PackageLayoutOptions{
				VerificationStrategy: layout.VerifyNever,
			})
			require.NoError(t, err)
			require.Equal(t, len(tt.sbomFiles) > 0, pkgLayout.ContainsSBOM())

			got := filterPkgPaths(pkgLayout, nil, []api.Component{{Name: "selected"}})
			want := []string{
				filepath.Join(pkgDir, layout.ZarfYAML),
				filepath.Join(pkgDir, layout.Checksums),
				filepath.Join(pkgDir, layout.ComponentsDir, "selected.tar"),
			}
			for _, path := range tt.sbomFiles {
				want = append(want, filepath.Join(pkgDir, filepath.FromSlash(path)))
			}
			assert.ElementsMatch(t, want, got)
		})
	}
}

func TestNewZarfOCIRemotePreservesUDSUserAgent(t *testing.T) {
	originalVersion := config.CLIVersion
	originalInsecure := config.CommonOptions.Insecure
	t.Cleanup(func() {
		config.CLIVersion = originalVersion
		config.CommonOptions.Insecure = originalInsecure
	})
	config.CLIVersion = "test-version"
	config.CommonOptions.Insecure = false

	remote, err := NewZarfOCIRemote(context.Background(), "oci://example.com/repo:tag", ocispec.Platform{OS: "linux", Architecture: "amd64"})
	require.NoError(t, err)

	client, ok := remote.Repo().Client.(*auth.Client)
	require.True(t, ok)
	assert.Equal(t, "uds-cli/test-version", client.Header.Get("User-Agent"))
}
