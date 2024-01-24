// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package test provides e2e tests for UDS.
package test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/require"
	"oras.land/oras-go/v2/registry"

	"github.com/defenseunicorns/uds-cli/src/config"
)

func zarfPublish(t *testing.T, path string, reg string) {
	args := strings.Split(fmt.Sprintf("zarf package publish %s oci://%s --insecure --oci-concurrency=10", path, reg), " ")
	_, _, err := e2e.UDS(args...)
	require.NoError(t, err)
}

func TestUDSCmd(t *testing.T) {
	_, _, err := e2e.UDS()
	require.NoError(t, err)
}

func TestCreateWithNoPath(t *testing.T) {
	zarfPkgPath1 := "src/test/packages/no-cluster/output-var"
	zarfPkgPath2 := "src/test/packages/no-cluster/receive-var"
	e2e.CreateZarfPkg(t, zarfPkgPath1, false)
	e2e.CreateZarfPkg(t, zarfPkgPath2, false)

	e2e.SetupDockerRegistry(t, 888)
	defer e2e.TeardownRegistry(t, 888)

	pkg := filepath.Join(zarfPkgPath1, fmt.Sprintf("zarf-package-output-var-%s-0.0.1.tar.zst", e2e.Arch))
	zarfPublish(t, pkg, "localhost:888")

	pkg = filepath.Join(zarfPkgPath2, fmt.Sprintf("zarf-package-receive-var-%s-0.0.1.tar.zst", e2e.Arch))
	zarfPublish(t, pkg, "localhost:888")

	err := os.Link(fmt.Sprintf("src/test/bundles/02-simple-vars/%s", config.BundleYAML), config.BundleYAML)
	require.NoError(t, err)
	defer os.Remove(config.BundleYAML)
	defer os.Remove(fmt.Sprintf("uds-bundle-simple-vars-%s-0.0.1.tar.zst", e2e.Arch))

	// create
	cmd := strings.Split(fmt.Sprintf("create --confirm --insecure"), " ")
	_, _, err = e2e.UDS(cmd...)
	require.NoError(t, err)
}

func TestBundleWithLocalAndRemotePkgs(t *testing.T) {
	deployZarfInit(t)
	e2e.SetupDockerRegistry(t, 888)
	defer e2e.TeardownRegistry(t, 888)
	e2e.CreateZarfPkg(t, "src/test/packages/podinfo", false)

	bundleDir := "src/test/bundles/03-local-and-remote"
	bundlePath := filepath.Join(bundleDir, fmt.Sprintf("uds-bundle-test-local-and-remote-%s-0.0.1.tar.zst", e2e.Arch))

	tarballPath := filepath.Join("build", fmt.Sprintf("uds-bundle-test-local-and-remote-%s-0.0.1.tar.zst", e2e.Arch))
	bundleRef := registry.Reference{
		Registry: "oci://localhost:888",
		// this info is derived from the bundle's metadata
		Repository: "test-local-and-remote",
		Reference:  fmt.Sprintf("0.0.1-%s", e2e.Arch),
	}
	createLocal(t, bundleDir)
	inspect(t, bundlePath)
	publishInsecure(t, bundlePath, bundleRef.Registry)
	pull(t, bundleRef.String(), tarballPath)
	deploy(t, tarballPath)
	remove(t, tarballPath)
}

func TestBundle(t *testing.T) {
	deployZarfInit(t)

	e2e.CreateZarfPkg(t, "src/test/packages/nginx", false)
	e2e.CreateZarfPkg(t, "src/test/packages/podinfo", false)

	e2e.SetupDockerRegistry(t, 888)
	defer e2e.TeardownRegistry(t, 888)
	e2e.SetupDockerRegistry(t, 889)
	defer e2e.TeardownRegistry(t, 889)

	pkg := fmt.Sprintf("src/test/packages/nginx/zarf-package-nginx-%s-0.0.1.tar.zst", e2e.Arch)
	zarfPublish(t, pkg, "localhost:888")

	pkg = fmt.Sprintf("src/test/packages/podinfo/zarf-package-podinfo-%s-0.0.1.tar.zst", e2e.Arch)
	zarfPublish(t, pkg, "localhost:889")

	bundleDir := "src/test/bundles/01-uds-bundle"
	bundlePath := filepath.Join(bundleDir, fmt.Sprintf("uds-bundle-example-%s-0.0.1.tar.zst", e2e.Arch))

	create(t, bundleDir) // todo: allow creating from both the folder containing and direct reference to uds-bundle.yaml
	inspect(t, bundlePath)
	inspectAndSBOMExtract(t, bundlePath)
	// Test with an "options only" config file
	os.Setenv("UDS_CONFIG", filepath.Join("src/test/bundles/01-uds-bundle", "uds-config.yaml"))
	deploy(t, bundlePath)
	remove(t, bundlePath)
}

func TestPackagesFlag(t *testing.T) {
	deployZarfInit(t)

	e2e.CreateZarfPkg(t, "src/test/packages/nginx", false)
	e2e.CreateZarfPkg(t, "src/test/packages/podinfo", false)

	e2e.SetupDockerRegistry(t, 888)
	defer e2e.TeardownRegistry(t, 888)
	e2e.SetupDockerRegistry(t, 889)
	defer e2e.TeardownRegistry(t, 889)

	pkg := fmt.Sprintf("src/test/packages/nginx/zarf-package-nginx-%s-0.0.1.tar.zst", e2e.Arch)
	zarfPublish(t, pkg, "localhost:888")

	pkg = fmt.Sprintf("src/test/packages/podinfo/zarf-package-podinfo-%s-0.0.1.tar.zst", e2e.Arch)
	zarfPublish(t, pkg, "localhost:889")

	bundleDir := "src/test/bundles/01-uds-bundle"
	bundlePath := filepath.Join(bundleDir, fmt.Sprintf("uds-bundle-example-%s-0.0.1.tar.zst", e2e.Arch))

	create(t, bundleDir)
	inspect(t, bundlePath)
	inspectAndSBOMExtract(t, bundlePath)

	// Test only podinfo deploy
	deployPackagesFlag(bundlePath, "podinfo")
	cmd := strings.Split("zarf tools kubectl get deployments -A -o=jsonpath='{.items[*].metadata.name}'", " ")
	deployments, _, _ := e2e.UDS(cmd...)
	require.Contains(t, deployments, "podinfo")
	require.NotContains(t, deployments, "nginx")

	remove(t, bundlePath)

	// Test both podinfo and nginx deploy
	deployPackagesFlag(bundlePath, "podinfo,nginx")
	deployments, _, _ = e2e.UDS(cmd...)
	require.Contains(t, deployments, "podinfo")
	require.Contains(t, deployments, "nginx")

	// Remove only podinfo
	removePackagesFlag(bundlePath, "podinfo")
	deployments, _, _ = e2e.UDS(cmd...)
	require.NotContains(t, deployments, "podinfo")
	require.Contains(t, deployments, "nginx")

	// Remove nginx
	removePackagesFlag(bundlePath, "nginx")
	deployments, _, _ = e2e.UDS(cmd...)
	require.NotContains(t, deployments, "podinfo")
	require.NotContains(t, deployments, "nginx")

	// Test invalid package deploy
	_, stderr := deployPackagesFlag(bundlePath, "podinfo,nginx,peanuts")
	require.Contains(t, stderr, "invalid zarf packages specified by --packages")

	// Test invalid package remove
	_, stderr = removePackagesFlag(bundlePath, "podinfo,nginx,peanuts")
	require.Contains(t, stderr, "invalid zarf packages specified by --packages")
}

func TestResumeFlag(t *testing.T) {
	deployZarfInit(t)

	e2e.CreateZarfPkg(t, "src/test/packages/nginx", false)
	e2e.CreateZarfPkg(t, "src/test/packages/podinfo", false)

	e2e.SetupDockerRegistry(t, 888)
	defer e2e.TeardownRegistry(t, 888)
	e2e.SetupDockerRegistry(t, 889)
	defer e2e.TeardownRegistry(t, 889)

	pkg := fmt.Sprintf("src/test/packages/nginx/zarf-package-nginx-%s-0.0.1.tar.zst", e2e.Arch)
	zarfPublish(t, pkg, "localhost:888")

	pkg = fmt.Sprintf("src/test/packages/podinfo/zarf-package-podinfo-%s-0.0.1.tar.zst", e2e.Arch)
	zarfPublish(t, pkg, "localhost:889")

	bundleDir := "src/test/bundles/01-uds-bundle"
	bundlePath := filepath.Join(bundleDir, fmt.Sprintf("uds-bundle-example-%s-0.0.1.tar.zst", e2e.Arch))

	create(t, bundleDir)
	inspect(t, bundlePath)
	inspectAndSBOMExtract(t, bundlePath)

	// Deploy only podinfo from bundle
	deployPackagesFlag(bundlePath, "podinfo")
	cmd := strings.Split("zarf tools kubectl get deployments -A -o=jsonpath='{.items[*].metadata.name}'", " ")
	deployments, _, _ := e2e.UDS(cmd...)
	require.Contains(t, deployments, "podinfo")
	require.NotContains(t, deployments, "nginx")

	// Deploy bundle --resume
	deployResumeFlag(t, bundlePath)
	cmd = strings.Split("zarf tools kubectl get deployments -A -o=jsonpath='{.items[*].metadata.name}'", " ")
	deployments, _, _ = e2e.UDS(cmd...)
	require.Contains(t, deployments, "podinfo")
	require.Contains(t, deployments, "nginx")

	// Remove only podinfo
	removePackagesFlag(bundlePath, "podinfo")
	deployments, _, _ = e2e.UDS(cmd...)
	require.NotContains(t, deployments, "podinfo")
	require.Contains(t, deployments, "nginx")

	// Remove bundle
	remove(t, bundlePath)
	cmd = strings.Split("zarf tools kubectl get deployments -A -o=jsonpath='{.items[*].metadata.name}'", " ")
	deployments, _, _ = e2e.UDS(cmd...)
	require.NotContains(t, deployments, "podinfo")
	require.NotContains(t, deployments, "nginx")
}

func TestRemoteBundle(t *testing.T) {
	deployZarfInit(t)
	e2e.CreateZarfPkg(t, "src/test/packages/nginx", false)
	e2e.CreateZarfPkg(t, "src/test/packages/podinfo", false)

	e2e.SetupDockerRegistry(t, 888)
	defer e2e.TeardownRegistry(t, 888)
	e2e.SetupDockerRegistry(t, 889)
	defer e2e.TeardownRegistry(t, 889)

	pkg := fmt.Sprintf("src/test/packages/nginx/zarf-package-nginx-%s-0.0.1.tar.zst", e2e.Arch)
	zarfPublish(t, pkg, "localhost:888")

	pkg = fmt.Sprintf("src/test/packages/podinfo/zarf-package-podinfo-%s-0.0.1.tar.zst", e2e.Arch)
	zarfPublish(t, pkg, "localhost:889")

	bundleRef := registry.Reference{
		Registry: "oci://localhost:888",
		// this info is derived from the bundle's metadata
		Repository: "example",
		Reference:  fmt.Sprintf("0.0.1-%s", e2e.Arch),
	}

	tarballPath := filepath.Join("build", fmt.Sprintf("uds-bundle-example-%s-0.0.1.tar.zst", e2e.Arch))
	bundlePath := "src/test/bundles/01-uds-bundle"
	createRemote(t, bundlePath, bundleRef.Registry)

	// Test without oci prefix
	createRemote(t, bundlePath, "localhost:888")

	pull(t, bundleRef.String(), tarballPath)
	inspectRemote(t, bundleRef.String())
	inspectRemoteAndSBOMExtract(t, bundleRef.String())
	deployAndRemoveRemote(t, bundleRef.String(), tarballPath)

	// Test without architecture specified
	bundleRef = registry.Reference{
		Registry: "oci://localhost:888",
		// this info is derived from the bundle's metadata
		Repository: "example",
		Reference:  "0.0.1",
	}
	deployAndRemoveRemote(t, bundleRef.String(), tarballPath)
}

func TestBundleWithGitRepo(t *testing.T) {
	deployZarfInit(t)
	e2e.CreateZarfPkg(t, "src/test/packages/gitrepo", false)
	bundleDir := "src/test/bundles/05-gitrepo"
	bundlePath := filepath.Join(bundleDir, fmt.Sprintf("uds-bundle-gitrepo-%s-0.0.1.tar.zst", e2e.Arch))

	create(t, bundleDir)
	deploy(t, bundlePath)
	remove(t, bundlePath)
}

func TestBundleWithYmlFile(t *testing.T) {
	deployZarfInit(t)

	e2e.CreateZarfPkg(t, "src/test/packages/nginx", true)
	e2e.CreateZarfPkg(t, "src/test/packages/podinfo", true)

	e2e.SetupDockerRegistry(t, 888)
	defer e2e.TeardownRegistry(t, 888)
	e2e.SetupDockerRegistry(t, 889)
	defer e2e.TeardownRegistry(t, 889)

	pkg := fmt.Sprintf("src/test/packages/nginx/zarf-package-nginx-%s-0.0.1.tar.zst", e2e.Arch)
	zarfPublish(t, pkg, "localhost:888")

	pkg = fmt.Sprintf("src/test/packages/podinfo/zarf-package-podinfo-%s-0.0.1.tar.zst", e2e.Arch)
	zarfPublish(t, pkg, "localhost:889")

	bundleDir := "src/test/bundles/09-uds-bundle-yml"
	bundlePath := filepath.Join(bundleDir, fmt.Sprintf("uds-bundle-yml-example-%s-0.0.1.tar.zst", e2e.Arch))

	create(t, bundleDir)
	inspect(t, bundlePath)
	inspectAndSBOMExtract(t, bundlePath)
	// Test with an "options only" config file
	os.Setenv("UDS_CONFIG", filepath.Join("src/test/bundles/09-uds-bundle-yml", "uds-config.yml"))
	deploy(t, bundlePath)
	remove(t, bundlePath)
}

func TestLocalBundleWithNoSBOM(t *testing.T) {
	path := "src/test/packages/nginx"
	args := strings.Split(fmt.Sprintf("zarf package create %s -o %s --skip-sbom --confirm", path, path), " ")
	_, _, err := e2e.UDS(args...)
	require.NoError(t, err)

	bundleDir := "src/test/bundles/09-uds-bundle-yml"
	bundlePath := filepath.Join(bundleDir, fmt.Sprintf("uds-bundle-yml-example-%s-0.0.1.tar.zst", e2e.Arch))
	create(t, bundleDir)

	cmd := strings.Split(fmt.Sprintf("inspect %s --sbom --extract", bundlePath), " ")
	_, stderr, err := e2e.UDS(cmd...)
	require.NoError(t, err)
	require.Contains(t, stderr, "Cannot extract, no SBOMs found in bundle")
	require.Contains(t, stderr, "sboms.tar not found in Zarf pkg")
}

func TestRemoteBundleWithNoSBOM(t *testing.T) {
	path := "src/test/packages/nginx"
	args := strings.Split(fmt.Sprintf("zarf package create %s -o %s --skip-sbom --confirm", path, path), " ")
	_, _, err := e2e.UDS(args...)
	require.NoError(t, err)

	e2e.SetupDockerRegistry(t, 888)
	defer e2e.TeardownRegistry(t, 888)
	bundleDir := "src/test/bundles/09-uds-bundle-yml"
	bundlePath := filepath.Join(bundleDir, fmt.Sprintf("uds-bundle-yml-example-%s-0.0.1.tar.zst", e2e.Arch))
	create(t, bundleDir)
	publishInsecure(t, bundlePath, "localhost:888")

	cmd := strings.Split(fmt.Sprintf("inspect %s --sbom --extract", bundlePath), " ")
	_, stderr, err := e2e.UDS(cmd...)
	require.NoError(t, err)
	require.Contains(t, stderr, "Cannot extract, no SBOMs found in bundle")
	require.Contains(t, stderr, "sboms.tar not found in Zarf pkg")
}

func TestPackageNaming(t *testing.T) {
	deployZarfInit(t)

	e2e.CreateZarfPkg(t, "src/test/packages/nginx", false)
	e2e.CreateZarfPkg(t, "src/test/packages/podinfo", false)

	e2e.SetupDockerRegistry(t, 888)
	defer e2e.TeardownRegistry(t, 888)
	e2e.SetupDockerRegistry(t, 889)
	defer e2e.TeardownRegistry(t, 889)

	pkg := fmt.Sprintf("src/test/packages/nginx/zarf-package-nginx-%s-0.0.1.tar.zst", e2e.Arch)
	zarfPublish(t, pkg, "localhost:888")

	pkg = fmt.Sprintf("src/test/packages/podinfo/zarf-package-podinfo-%s-0.0.1.tar.zst", e2e.Arch)
	zarfPublish(t, pkg, "localhost:889")

	bundleDir := "src/test/bundles/10-package-naming"
	bundlePath := filepath.Join(bundleDir, fmt.Sprintf("uds-bundle-package-naming-%s-0.0.1.tar.zst", e2e.Arch))
	tarballPath := filepath.Join("build", fmt.Sprintf("uds-bundle-package-naming-%s-0.0.1.tar.zst", e2e.Arch))
	bundleRef := registry.Reference{
		Registry: "oci://localhost:888",
		// this info is derived from the bundle's metadata
		Repository: "package-naming",
		Reference:  fmt.Sprintf("0.0.1-%s", e2e.Arch),
	}
	create(t, bundleDir) // todo: allow creating from both the folder containing and direct reference to uds-bundle.yaml
	publishInsecure(t, bundlePath, bundleRef.Registry)
	pull(t, bundleRef.String(), tarballPath)
	deploy(t, tarballPath)
	remove(t, tarballPath)

	// Test create -o with zarf package names that don't match the zarf package name in the bundle
	createRemote(t, bundleDir, bundleRef.Registry)
	deployAndRemoveRemote(t, bundleRef.String(), tarballPath)
}

func TestBundleIndexInRemoteOnPublish(t *testing.T) {
	e2e.SetupDockerRegistry(t, 888)
	defer e2e.TeardownRegistry(t, 888)

	bundleDir := "src/test/bundles/06-ghcr"
	bundleName := "ghcr-test"
	bundlePath := filepath.Join(bundleDir, fmt.Sprintf("uds-bundle-%s-%s-0.0.1.tar.zst", bundleName, e2e.Arch))
	create(t, bundleDir)
	publishInsecure(t, bundlePath, "localhost:888")

	// curl OCI registry for index
	index, err := queryIndex(t, "http://localhost:888", bundleName)

	require.NoError(t, err)
	require.Equal(t, 1, len(index.Manifests))
	require.Equal(t, ocispec.MediaTypeImageIndex, index.MediaType)
	require.Equal(t, ocispec.MediaTypeImageManifest, index.Manifests[0].MediaType)
	require.Equal(t, e2e.Arch, index.Manifests[0].Platform.Architecture)
	require.Equal(t, "multi", index.Manifests[0].Platform.OS)
}

func TestBundleIndexInRemoteOnCreate(t *testing.T) {
	e2e.SetupDockerRegistry(t, 888)
	defer e2e.TeardownRegistry(t, 888)

	bundleDir := "src/test/bundles/06-ghcr"
	bundleName := "ghcr-test"
	createRemote(t, bundleDir, "oci://localhost:888")

	// curl OCI registry for index
	index, err := queryIndex(t, "http://localhost:888", bundleName)

	require.NoError(t, err)
	require.Equal(t, 1, len(index.Manifests))
	require.Equal(t, ocispec.MediaTypeImageIndex, index.MediaType)
	require.Equal(t, ocispec.MediaTypeImageManifest, index.Manifests[0].MediaType)
	require.Equal(t, e2e.Arch, index.Manifests[0].Platform.Architecture)
	require.Equal(t, "multi", index.Manifests[0].Platform.OS)
}
