// Copyright 2024-2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package bundle contains functions for interacting with, managing and deploying UDS packages
package bundle

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/defenseunicorns/pkg/oci"
	"github.com/defenseunicorns/uds-cli/pkg/legacy/bundler/pusher"
	"github.com/defenseunicorns/uds-cli/pkg/legacy/config"
	"github.com/defenseunicorns/uds-cli/pkg/legacy/message"
	"github.com/defenseunicorns/uds-cli/pkg/legacy/types"
	"github.com/defenseunicorns/uds-cli/pkg/legacy/utils"
	"github.com/defenseunicorns/uds-cli/pkg/legacy/utils/boci"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/api"
	"github.com/zarf-dev/zarf/src/api/convert"
	"github.com/zarf-dev/zarf/src/api/v1alpha1" //nolint:staticcheck // Verify Legacy v1alpha1 package compatibility.
	"github.com/zarf-dev/zarf/src/api/v1beta1"
	zarfoci "github.com/zarf-dev/zarf/src/pkg/oci"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
	"github.com/zarf-dev/zarf/src/pkg/state"
	"github.com/zarf-dev/zarf/src/pkg/zoci"
	zarfTypes "github.com/zarf-dev/zarf/src/types"
	"oras.land/oras-go/v2/content"
	orasregistry "oras.land/oras-go/v2/registry"
)

func TestGetOCIValidatedSource(t *testing.T) {
	const source = "origin.example/team/bundle:v1"
	candidates := []string{"oci://" + source, GHCRUDSBundlePath + source, GHCRDeliveryBundlePath + source, GHCRPackagesPath + source}
	auth := base64.StdEncoding.EncodeToString([]byte("test-user:test-password"))
	dockerConfig := t.TempDir()
	t.Setenv("DOCKER_CONFIG", dockerConfig)
	require.NoError(t, os.WriteFile(filepath.Join(dockerConfig, "config.json"), []byte(`{"auths":{"origin.example":{"auth":"`+auth+`"}}}`), 0o600))
	oldInsecure, oldArch := config.CommonOptions.Insecure, config.CLIArch
	config.CommonOptions.Insecure, config.CLIArch = false, "amd64"
	t.Cleanup(func() { config.CommonOptions.Insecure, config.CLIArch = oldInsecure, oldArch })
	t.Run("invalid reference", func(t *testing.T) {
		_, err := getOCIValidatedSource("invalid ref")
		require.EqualError(t, err, "invalid ref: not found")
	})

	for _, tt := range []struct {
		name        string
		available   int
		kind        string
		status      int
		wantError   string
		wantNext    bool
		requireAuth bool
	}{
		{name: "explicit Legacy manifest", available: 0},
		{name: "authenticated Legacy manifest", available: 0, requireAuth: true},
		{name: "Legacy multi-architecture index", available: 0, kind: "legacy"},
		{name: "UDS bundle fallback", available: 1},
		{name: "delivery fallback", available: 2},
		{name: "packages fallback", available: 3},
		{name: "missing reference preserves first error", available: -1, wantError: "origin.example/team/bundle:v1"},
		{name: "authorization error preserved", available: 0, status: http.StatusForbidden, wantError: "403"},
		{name: "malformed index error preserved", available: 0, kind: "malformed", wantError: "unexpected end of JSON input"},
		{name: "Legacy architecture mismatch preserved", available: 0, kind: "wrong architecture", wantError: "no matching manifest"},
		{name: "Next child stops fallback", available: 0, kind: "next child", wantNext: true},
		{name: "Next root stops fallback", available: 0, kind: "next root", wantNext: true},
		{name: "Next fallback stops further lookup", available: 1, kind: "next child", wantNext: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			manifest := []byte(`{"schemaVersion":2,"mediaType":"application/vnd.oci.image.manifest.v1+json","config":{},"layers":[]}`)
			manifestDesc := content.NewDescriptorFromBytes(ocispec.MediaTypeImageManifest, manifest)
			body, mediaType := manifest, ocispec.MediaTypeImageManifest
			if tt.kind != "" {
				index := ocispec.Index{Versioned: specs.Versioned{SchemaVersion: 2}, MediaType: ocispec.MediaTypeImageIndex}
				manifestDesc.Platform = &ocispec.Platform{OS: oci.MultiOS, Architecture: "amd64"}
				index.Manifests = []ocispec.Descriptor{manifestDesc}
				switch tt.kind {
				case "wrong architecture":
					index.Manifests[0].Platform.Architecture = "arm64"
				case "next child":
					index.ArtifactType = "application/vnd.defenseunicorns.uds.bundle.v1"
				case "next root":
					index.Manifests[0].MediaType = ocispec.MediaTypeImageIndex
					index.Manifests[0].ArtifactType = "application/vnd.defenseunicorns.uds.bundle.v1"
				}
				var err error
				body, err = json.Marshal(index)
				require.NoError(t, err)
				mediaType = ocispec.MediaTypeImageIndex
				if tt.kind == "malformed" {
					body = []byte(`{`)
				}
			}
			desc := content.NewDescriptorFromBytes(mediaType, body)
			attempts := make(chan string, 100)
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				for i, candidate := range candidates {
					ref, err := orasregistry.ParseReference(strings.TrimPrefix(candidate, "oci://"))
					if err != nil || r.Host != ref.Registry || r.URL.Path != "/v2/"+ref.Repository+"/manifests/"+ref.Reference {
						continue
					}
					attempts <- candidate
					if i != tt.available {
						w.WriteHeader(http.StatusNotFound)
						return
					}
					if tt.status != 0 {
						w.WriteHeader(tt.status)
						return
					}
					if tt.requireAuth && r.Header.Get("Authorization") != "Basic "+auth {
						w.Header().Set("WWW-Authenticate", `Basic realm="test registry"`)
						w.WriteHeader(http.StatusUnauthorized)
						return
					}
					break
				}
				if strings.Contains(r.URL.Path, "/manifests/") {
					w.Header().Set("Content-Type", mediaType)
					w.Header().Set("Docker-Content-Digest", desc.Digest.String())
					w.Header().Set("Content-Length", strconv.Itoa(len(body)))
					if r.Method != http.MethodHead {
						_, _ = w.Write(body)
					}
					return
				}
				w.WriteHeader(http.StatusNotFound)
			}))
			t.Cleanup(server.Close)
			transport := server.Client().Transport.(*http.Transport).Clone()
			transport.Proxy = nil
			transport.TLSClientConfig.ServerName = server.Certificate().DNSNames[0]
			transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
			}
			originalTransport := http.DefaultTransport
			http.DefaultTransport = transport
			t.Cleanup(func() { http.DefaultTransport = originalTransport; transport.CloseIdleConnections() })

			got, err := getOCIValidatedSource(source)
			switch {
			case tt.wantNext:
				require.ErrorIs(t, err, utils.ErrNextBundle)
			case tt.wantError != "":
				require.ErrorContains(t, err, tt.wantError)
			default:
				require.NoError(t, err)
				require.Equal(t, candidates[tt.available], got)
			}
			var visited []string
			for len(attempts) > 0 {
				candidate := <-attempts
				if len(visited) == 0 || candidate != visited[len(visited)-1] {
					visited = append(visited, candidate)
				}
			}
			last := tt.available
			if tt.wantError != "" {
				last = len(candidates) - 1
			}
			require.Equal(t, candidates[:last+1], visited)
		})
	}
}

func Test_validateBundleVars(t *testing.T) {
	tests := []struct {
		name        string
		description string
		packages    []types.Package
		wantErr     bool
	}{
		{
			name:        "ImportMatchesExport",
			description: "import matches export",
			packages: []types.Package{
				{Name: "foo", Exports: []types.BundleVariableExport{{Name: "foo"}}},
				{Name: "bar", Imports: []types.BundleVariableImport{{Name: "foo", Package: "foo"}}},
			},
			wantErr: false,
		},
		{
			name:        "ImportDoesntMatchExport",
			description: "error when import doesn't match export",
			packages: []types.Package{
				{Name: "foo", Exports: []types.BundleVariableExport{{Name: "foo"}}},
				{Name: "bar", Imports: []types.BundleVariableImport{{Name: "bar", Package: "foo"}}},
			},
			wantErr: true,
		},
		{
			name:        "FirstPkgHasImport",
			description: "error when first pkg has an import",
			packages: []types.Package{
				{Name: "foo", Imports: []types.BundleVariableImport{{Name: "foo", Package: "foo"}}},
			},
			wantErr: true,
		},
		{
			name:        "PackageNamesMustMatch",
			description: "error when package name doesn't match",
			packages: []types.Package{
				{Name: "foo", Exports: []types.BundleVariableExport{{Name: "foo"}}},
				{Name: "bar", Imports: []types.BundleVariableImport{{Name: "foo", Package: "baz"}}},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateBundleVars(tt.packages)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestCrossRegistryPackageWithImageLoadsFromBundle(t *testing.T) {
	oldNoProgress := message.NoProgress
	message.NoProgress = true
	t.Cleanup(func() { message.NoProgress = oldNoProgress })

	ctx := t.Context()
	srcServer := httptest.NewServer(registry.New())
	t.Cleanup(srcServer.Close)
	dstServer := httptest.NewServer(registry.New())
	t.Cleanup(dstServer.Close)
	srcRef := strings.TrimPrefix(srcServer.URL, "http://") + "/test/package:v1"
	dstRef := strings.TrimPrefix(dstServer.URL, "http://") + "/test/bundle:v1"
	platform := ocispec.Platform{Architecture: config.GetArch(), OS: zarfoci.MultiOS}
	options := zoci.RemoteClientOptions{RemoteOptions: zarfTypes.RemoteOptions{PlainHTTP: true}}
	src, err := zoci.NewRemote(ctx, srcRef, platform, options)
	require.NoError(t, err)
	dst, err := zoci.NewRemote(ctx, dstRef, platform, options)
	require.NoError(t, err)

	pushPackageFile := func(data []byte, title string) ocispec.Descriptor {
		t.Helper()
		desc := content.NewDescriptorFromBytes(layout.ZarfLayerMediaTypeBlob, data)
		require.NoError(t, src.Repo().Push(ctx, desc, bytes.NewReader(data)))
		desc.Annotations = map[string]string{ocispec.AnnotationTitle: title}
		return desc
	}
	marshal := func(value any) []byte {
		t.Helper()
		data, err := json.Marshal(value)
		require.NoError(t, err)
		return data
	}
	imageFile := func(mediaType string, data []byte) (ocispec.Descriptor, ocispec.Descriptor) {
		t.Helper()
		imageDesc := content.NewDescriptorFromBytes(mediaType, data)
		packageDesc := pushPackageFile(data, filepath.ToSlash(filepath.Join(layout.ImagesBlobsDir, imageDesc.Digest.Encoded())))
		return imageDesc, packageDesc
	}

	imageConfig, imageConfigFile := imageFile(ocispec.MediaTypeImageConfig, []byte(`{"architecture":"amd64","os":"linux"}`))
	imageLayer, imageLayerFile := imageFile(ocispec.MediaTypeImageLayer, []byte("image layer"))
	imageManifestData := marshal(ocispec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		Config:    imageConfig,
		Layers:    []ocispec.Descriptor{imageLayer},
	})
	imageManifest, imageManifestFile := imageFile(ocispec.MediaTypeImageManifest, imageManifestData)
	imageIndexData := marshal(ocispec.Index{
		Versioned: specs.Versioned{SchemaVersion: 2},
		Manifests: []ocispec.Descriptor{{
			MediaType: imageManifest.MediaType,
			Digest:    imageManifest.Digest,
			Size:      imageManifest.Size,
			Platform:  &ocispec.Platform{Architecture: "amd64", OS: "linux"},
		}},
	})
	imageIndex, imageIndexFile := imageFile(ocispec.MediaTypeImageIndex, imageIndexData)
	const imageName = "docker.io/library/test:v1"
	packageImageIndex := pushPackageFile(marshal(ocispec.Index{
		Versioned: specs.Versioned{SchemaVersion: 2},
		Manifests: []ocispec.Descriptor{{
			MediaType:   imageIndex.MediaType,
			Digest:      imageIndex.Digest,
			Size:        imageIndex.Size,
			Annotations: map[string]string{ocispec.AnnotationBaseImageName: imageName},
		}},
	}), layout.IndexPath)
	zarfYAML := pushPackageFile([]byte("kind: ZarfPackageConfig\nmetadata:\n  name: test\n  version: v1\ncomponents:\n  - name: main\n    required: true\n    images:\n      - "+imageName+"\n"), layout.ZarfYAML)
	component := pushPackageFile([]byte("component"), "components/main.tar")
	unused := pushPackageFile([]byte("unused"), "components/unused.tar")
	ociLayout := pushPackageFile([]byte(`{"imageLayoutVersion":"1.0.0"}`), layout.OCILayoutPath)
	packageConfig := pushPackageFile([]byte(`{"architecture":"amd64","ociVersion":"1.0.1"}`), "")
	packageRoot := &zarfoci.Manifest{Manifest: ocispec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		Config:    packageConfig,
		Layers:    []ocispec.Descriptor{zarfYAML, component, unused, packageImageIndex, ociLayout, imageIndexFile, imageManifestFile, imageConfigFile, imageLayerFile},
	}}
	_, err = boci.ToOCIRemote(packageRoot, ocispec.MediaTypeImageManifest, src.OrasRemote)
	require.NoError(t, err)

	packagePusher := pusher.NewPkgPusher(types.Package{Name: "test", Repository: srcRef, Ref: "v1"}, pusher.Config{
		PkgRootManifest: packageRoot,
		RemoteSrc:       *src,
		RemoteDst:       *dst,
	})
	packageDesc, err := packagePusher.Push()
	require.NoError(t, err)
	exists, err := dst.Repo().Exists(ctx, unused)
	require.NoError(t, err)
	require.False(t, exists)

	bundleConfigBytes := []byte(`{"architecture":"amd64","ociVersion":"1.0.1"}`)
	bundleConfig := content.NewDescriptorFromBytes(layout.ZarfLayerMediaTypeBlob, bundleConfigBytes)
	require.NoError(t, dst.Repo().Push(ctx, bundleConfig, bytes.NewReader(bundleConfigBytes)))
	bundleYAML := []byte("kind: UDSBundle\nmetadata:\n  name: test-bundle\n  version: v1\n  architecture: " + config.GetArch() + "\npackages:\n  - name: test\n    ref: test@" + packageDesc.Digest.String() + "\n")
	bundleYAMLDesc := content.NewDescriptorFromBytes(layout.ZarfLayerMediaTypeBlob, bundleYAML)
	require.NoError(t, dst.Repo().Push(ctx, bundleYAMLDesc, bytes.NewReader(bundleYAML)))
	bundleYAMLDesc.Annotations = map[string]string{ocispec.AnnotationTitle: config.BundleYAML}
	_, err = boci.ToOCIRemote(ocispec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		Config:    bundleConfig,
		Layers:    []ocispec.Descriptor{packageDesc, bundleYAMLDesc},
	}, ocispec.MediaTypeImageManifest, dst.OrasRemote)
	require.NoError(t, err)

	oldCachePath := config.CommonOptions.CachePath
	config.CommonOptions.CachePath = t.TempDir()
	t.Cleanup(func() { config.CommonOptions.CachePath = oldCachePath })
	legacyRemote, err := oci.NewOrasRemote(dstRef, platform, oci.WithPlainHTTP(true))
	require.NoError(t, err)
	bundleRoot, err := legacyRemote.FetchRoot(ctx)
	require.NoError(t, err)
	provider := &ociProvider{src: "oci://" + dstRef, dst: t.TempDir(), OrasRemote: legacyRemote, rootManifest: bundleRoot}
	loaded, paths, err := provider.LoadBundle(types.BundlePullOptions{}, 1)
	require.NoError(t, err)
	require.Equal(t, "test-bundle", loaded.Metadata.Name)
	require.Len(t, loaded.Packages, 1)
	require.Equal(t, "test@"+packageDesc.Digest.String(), loaded.Packages[0].Ref)
	for _, desc := range []ocispec.Descriptor{packageImageIndex, imageIndexFile, imageManifestFile, imageConfigFile, imageLayerFile} {
		path := paths[desc.Digest.Encoded()]
		require.NotEmpty(t, path, "missing loaded image file %s", desc.Digest)
		loadedBytes, err := os.ReadFile(path)
		require.NoError(t, err)
		sourceBytes, err := content.FetchAll(ctx, src.Repo(), desc)
		require.NoError(t, err)
		require.Equal(t, sourceBytes, loadedBytes)
	}
}

func TestPackageManifestDigest(t *testing.T) {
	tests := []struct {
		name           string
		ref            string
		expectedDigest string
		expectedError  string
	}{
		{
			name:           "returns manifest digest",
			ref:            "ghcr.io/example/package:0.0.1@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			expectedDigest: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		},
		{
			name:          "rejects ref without digest",
			ref:           "ghcr.io/example/package:0.0.1",
			expectedError: "reference is missing a manifest digest",
		},
		{
			name:          "rejects empty digest",
			ref:           "ghcr.io/example/package:0.0.1@sha256:",
			expectedError: "reference is missing a manifest digest",
		},
		{
			name:          "rejects malformed digest",
			ref:           "ghcr.io/example/package:0.0.1@sha256:abc123",
			expectedError: "invalid manifest digest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifestDigest, err := packageManifestDigest(types.Package{Name: "test-package", Ref: tt.ref})

			if tt.expectedError != "" {
				require.ErrorContains(t, err, tt.expectedError)
				return
			}
			require.NoError(t, err)
			require.Equal(t, "sha256:"+tt.expectedDigest, manifestDigest.String())
		})
	}
}

func Test_validateOverrides(t *testing.T) {
	tests := []struct {
		name          string
		description   string
		bundlePackage types.Package
		zarfPackage   api.Package
		wantErr       bool
	}{
		{
			name:        "validOverride",
			description: "Respective components and charts exist for override",
			bundlePackage: types.Package{
				Name: "foo", Overrides: map[string]map[string]types.BundleChartOverrides{"component": {"chart": {}}}},
			zarfPackage: api.Package{
				Components: []api.Component{
					{Name: "component", Charts: []api.Chart{{Name: "chart"}}},
				},
			},
			wantErr: false,
		},
		{
			name:        "validOverrideMultipleComponents",
			description: "Respective components and charts exist for override when multiple charts and components are present",
			bundlePackage: types.Package{
				Name: "foo", Overrides: map[string]map[string]types.BundleChartOverrides{"component-a": {"chart-1": {}}}},
			zarfPackage: api.Package{
				Components: []api.Component{
					{Name: "component-a", Charts: []api.Chart{{Name: "chart-1"}, {Name: "chart-2"}}},
					{Name: "component-b", Charts: []api.Chart{{Name: "chart-b"}}},
				},
			},
			wantErr: false,
		},
		{
			name:        "invalidComponentOverride",
			description: "Component does not exist for override",
			bundlePackage: types.Package{
				Name: "foo", Overrides: map[string]map[string]types.BundleChartOverrides{"hell-unleashed": {"chart": {}}}},
			zarfPackage: api.Package{
				Components: []api.Component{
					{Name: "hello-world", Charts: []api.Chart{{Name: "chart"}}},
				},
			},
			wantErr: true,
		},
		{
			name:        "invalidChartOverride",
			description: "Chart does not exist for override",
			bundlePackage: types.Package{
				Name: "foo", Overrides: map[string]map[string]types.BundleChartOverrides{"component": {"hell-unleashed": {}}}},
			zarfPackage: api.Package{
				Components: []api.Component{
					{Name: "component", Charts: []api.Chart{{Name: "hello-world"}}},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateOverrides(tt.bundlePackage, tt.zarfPackage)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func Test_getPkgPath(t *testing.T) {
	tests := []struct {
		name   string
		pkg    types.Package
		arch   string
		srcDir string
		want   string
	}{
		{
			name:   "init full path",
			pkg:    types.Package{Name: "init", Ref: "0.0.1", Path: "../fake/path/custom-init.tar.zst"},
			arch:   "fake64",
			srcDir: "/mock/source",
			want:   "/mock/fake/path/custom-init.tar.zst",
		},
		{
			name:   "init directory only path",
			pkg:    types.Package{Name: "init", Ref: "0.0.1", Path: "../fake/path"},
			arch:   "fake64",
			srcDir: "/mock/source",
			want:   "/mock/fake/path/zarf-init-fake64-0.0.1.tar.zst",
		},
		{
			name:   "full path",
			pkg:    types.Package{Name: "nginx", Ref: "0.0.1", Path: "./fake/zarf-package-nginx-fake64-0.0.1.tar.zst"},
			arch:   "fake64",
			srcDir: "/mock/source",
			want:   "/mock/source/fake/zarf-package-nginx-fake64-0.0.1.tar.zst",
		},
		{
			name:   "directory only path",
			pkg:    types.Package{Name: "nginx", Ref: "0.0.1", Path: "fake"},
			arch:   "fake64",
			srcDir: "/mock/source",
			want:   "/mock/source/fake/zarf-package-nginx-fake64-0.0.1.tar.zst",
		},
		{
			name:   "absolute path",
			pkg:    types.Package{Name: "nginx", Ref: "0.0.1", Path: "/fake/zarf-package-nginx-fake64-0.0.1.tar.zst"},
			arch:   "fake64",
			srcDir: "/mock/source",
			want:   "/fake/zarf-package-nginx-fake64-0.0.1.tar.zst",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, err := utils.GetPkgPath(tt.pkg, tt.arch, tt.srcDir)
			require.NoError(t, err)
			require.Equal(t, tt.want, path)
		})
	}
}

func Test_GetPackagesInBundle(t *testing.T) {
	tests := []struct {
		name           string
		packages       []types.Package
		expectedNames  []string
		expectedRefs   []string
		expectedLength int
	}{
		{
			name:           "single package",
			packages:       []types.Package{{Name: "test", Ref: "0.0.1", Path: "../fake/path/custom-init.tar.zst"}},
			expectedNames:  []string{"test"},
			expectedRefs:   []string{"0.0.1"},
			expectedLength: 1,
		},
		{
			name:           "multiple packages",
			packages:       []types.Package{{Name: "test", Ref: "0.0.1", Path: "../fake/path/custom-init.tar.zst"}, {Name: "test2", Ref: "1.2.3", Path: "../fake/path/custom-init.tar.zst"}},
			expectedNames:  []string{"test", "test2"},
			expectedRefs:   []string{"0.0.1", "1.2.3"},
			expectedLength: 2,
		},
		{
			name:           "no packages",
			packages:       []types.Package{},
			expectedNames:  []string{},
			expectedRefs:   []string{},
			expectedLength: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bundleCfg := types.BundleConfig{}
			bundleCfg.DeployOpts.Source = "fake"
			bndlClient, _ := New(&bundleCfg)
			bndlClient.bundle.Packages = tt.packages

			require.Equal(t, tt.expectedLength, len(bndlClient.GetPackages()))
			for i, pkg := range bndlClient.GetPackages() {
				require.Equal(t, tt.expectedNames[i], pkg.Name)
				require.Equal(t, tt.expectedRefs[i], pkg.Ref)
			}
		})
	}
}

func Test_GetBundleMetadata(t *testing.T) {
	bundleCfg := types.BundleConfig{}
	bundleCfg.DeployOpts.Source = "fake"
	bndlClient, _ := New(&bundleCfg)
	bndlClient.bundle.Metadata = types.UDSMetadata{
		Name:        "test-metadata",
		Description: "test-description",
		Version:     "0.0.1",
	}

	require.Equal(t, "test-metadata", bndlClient.GetMetadata().Name)
	require.Equal(t, "test-description", bndlClient.GetMetadata().Description)
	require.Equal(t, "0.0.1", bndlClient.GetMetadata().Version)
}

func Test_deployedPackageIsSuccessful(t *testing.T) {
	boolPtr := func(v bool) *bool {
		return &v
	}

	tests := []struct {
		name               string
		description        string
		definition         v1alpha1.ZarfPackage
		deployedComponents []state.DeployedComponent
		want               bool
	}{
		{
			name:        "SuccessAllSucceededRequiredPresent",
			description: "returns true when all deployed components succeeded and required components are present",
			definition: v1alpha1.ZarfPackage{
				Components: []v1alpha1.ZarfComponent{
					{Name: "component-a", Required: boolPtr(true)},
					{Name: "component-b"},
				},
			},
			deployedComponents: []state.DeployedComponent{
				{Name: "component-a", Status: state.ComponentStatusSucceeded},
				{Name: "component-b", Status: state.ComponentStatusSucceeded},
			},
			want: true,
		},
		{
			name:        "FailRequiredMissing",
			description: "returns false when a required component is missing from deployed components",
			definition: v1alpha1.ZarfPackage{
				Components: []v1alpha1.ZarfComponent{
					{Name: "component-a", Required: boolPtr(true)},
					{Name: "component-b"},
				},
			},
			deployedComponents: []state.DeployedComponent{
				{Name: "component-b", Status: state.ComponentStatusSucceeded},
			},
			want: false,
		},
		{
			name:        "FailDeployedComponentNotSucceeded",
			description: "returns false when any deployed component is not succeeded",
			definition: v1alpha1.ZarfPackage{
				Components: []v1alpha1.ZarfComponent{
					{Name: "component-a", Required: boolPtr(true)},
				},
			},
			deployedComponents: []state.DeployedComponent{
				{Name: "component-a", Status: state.ComponentStatusFailed},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Run("legacy Data", func(t *testing.T) {
				pkg := state.DeployedPackage{Name: "test", DeployedComponents: tt.deployedComponents}
				pkg.Data = tt.definition //nolint:staticcheck // Exercise Definition's fallback for state saved before PackageData existed.
				require.Equal(t, tt.want, deployedPackageIsSuccessful(pkg))
			})
			t.Run("current PackageData", func(t *testing.T) {
				pkg := state.DeployedPackage{Name: "test", DeployedComponents: tt.deployedComponents}
				definition := convert.PackageFromV1alpha1(tt.definition) //nolint:staticcheck // Exercise compatibility with existing v1alpha1 deployment records.
				definition.APIVersion = v1beta1.APIVersion
				require.NoError(t, pkg.SetPackageDefinition(definition))
				require.NotEmpty(t, pkg.PackageData)
				require.Equal(t, tt.want, deployedPackageIsSuccessful(pkg))
			})
		})
	}
}
