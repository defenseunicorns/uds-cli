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
	"time"

	"github.com/fsnotify/fsnotify"
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

func TestSimpleBundleWithZarfAction(t *testing.T) {
	zarfPkgPath := "src/test/packages/no-cluster/real-simple"
	e2e.CreateZarfPkg(t, zarfPkgPath, false)
	os.Setenv("UDS_LOG_LEVEL", "debug")
	createLocal(t, "src/test/bundles/11-real-simple", e2e.Arch)
	_, stderr := deploy(t, fmt.Sprintf("src/test/bundles/11-real-simple/uds-bundle-real-simple-%s-0.0.1.tar.zst", e2e.Arch))
	require.Contains(t, stderr, "Log level set to debug")
}

func TestCreateWithNoPath(t *testing.T) {
	// need to use remote pkgs because we move the uds-bundle.yaml to the current directory
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

	// move the bundle to the current directory so we can test the create command with no path
	err := os.Link(fmt.Sprintf("src/test/bundles/02-variables/remote/%s", config.BundleYAML), config.BundleYAML)
	require.NoError(t, err)
	defer os.Remove(config.BundleYAML)
	defer os.Remove(fmt.Sprintf("uds-bundle-variables-%s-0.0.1.tar.zst", e2e.Arch))

	// create
	cmd := strings.Split("create --confirm --insecure", " ")
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
		Reference:  "0.0.1",
	}
	createLocal(t, bundleDir, e2e.Arch)
	inspectLocal(t, bundlePath)
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

	createLocal(t, bundleDir, e2e.Arch)
	inspectLocal(t, bundlePath)
	inspectLocalAndSBOMExtract(t, bundlePath)
	// Test with an "options only" config file
	os.Setenv("UDS_CONFIG", filepath.Join("src/test/bundles/01-uds-bundle", "uds-config.yaml"))
	deploy(t, bundlePath)
	remove(t, bundlePath)

	//Test create using custom tmpDir
	runCmd(t, "create "+bundleDir+" --tmpdir ./customtmp --confirm --insecure")

	// remove customtmp folder if it exists
	err := os.RemoveAll("./customtmp")
	require.NoError(t, err)

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

	createLocal(t, bundleDir, e2e.Arch)
	inspectLocal(t, bundlePath)
	inspectLocalAndSBOMExtract(t, bundlePath)

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

	createLocal(t, bundleDir, e2e.Arch)
	inspectLocal(t, bundlePath)
	inspectLocalAndSBOMExtract(t, bundlePath)

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
		Reference:  "0.0.1",
	}

	tarballPath := filepath.Join("build", fmt.Sprintf("uds-bundle-example-%s-0.0.1.tar.zst", e2e.Arch))
	bundlePath := "src/test/bundles/01-uds-bundle"
	createRemoteInsecure(t, bundlePath, bundleRef.Registry, e2e.Arch)

	// Test without oci prefix
	createRemoteInsecure(t, bundlePath, "localhost:888", e2e.Arch)

	pull(t, bundleRef.String(), tarballPath)
	inspectRemoteInsecure(t, bundleRef.String())
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

	createLocal(t, bundleDir, e2e.Arch)
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

	createLocal(t, bundleDir, e2e.Arch)
	inspectLocal(t, bundlePath)
	inspectLocalAndSBOMExtract(t, bundlePath)
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
	createLocal(t, bundleDir, e2e.Arch)

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
	createLocal(t, bundleDir, e2e.Arch)
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
		Reference:  "0.0.1",
	}
	createLocal(t, bundleDir, e2e.Arch) // todo: allow creating from both the folder containing and direct reference to uds-bundle.yaml
	publishInsecure(t, bundlePath, bundleRef.Registry)
	pull(t, bundleRef.String(), tarballPath)
	deploy(t, tarballPath)
	remove(t, tarballPath)

	// Test create -o with zarf package names that don't match the zarf package name in the bundle
	createRemoteInsecure(t, bundleDir, bundleRef.Registry, e2e.Arch)
	deployAndRemoveRemote(t, bundleRef.String(), tarballPath)
}

func TestBundleIndexInRemoteOnPublish(t *testing.T) {
	e2e.SetupDockerRegistry(t, 888)
	defer e2e.TeardownRegistry(t, 888)

	bundleDir := "src/test/bundles/06-ghcr"
	bundleName := "ghcr-test"
	bundlePathARM := filepath.Join(bundleDir, fmt.Sprintf("uds-bundle-%s-%s-0.0.1.tar.zst", bundleName, "arm64"))
	bundlePathAMD := filepath.Join(bundleDir, fmt.Sprintf("uds-bundle-%s-%s-0.0.1.tar.zst", bundleName, "amd64"))
	tarballPath := filepath.Join("build", fmt.Sprintf("uds-bundle-%s-%s-0.0.1.tar.zst", bundleName, e2e.Arch))

	// create and push bundles with different archs to the same OCI repo
	createLocal(t, bundleDir, "arm64")
	createLocal(t, bundleDir, "amd64")
	publishInsecure(t, bundlePathARM, "localhost:888")
	publishInsecure(t, bundlePathAMD, "localhost:888")

	// curl OCI registry for index
	index, err := queryIndex(t, "http://localhost:888", bundleName)
	require.NoError(t, err)
	validateMultiArchIndex(t, index)

	inspectRemoteInsecure(t, fmt.Sprintf("oci://localhost:888/%s:0.0.1", bundleName))
	pull(t, fmt.Sprintf("localhost:888/%s:0.0.1", bundleName), tarballPath) // test no oci prefix
	deployAndRemoveRemote(t, fmt.Sprintf("oci://localhost:888/%s:0.0.1", bundleName), tarballPath)

	// now test by running 'create -o' over the bundle that was published
	createRemoteInsecure(t, bundleDir, "oci://localhost:888", e2e.Arch)
	index, err = queryIndex(t, "http://localhost:888", bundleName)
	require.NoError(t, err)
	validateMultiArchIndex(t, index)
	inspectRemoteInsecure(t, fmt.Sprintf("oci://localhost:888/%s:0.0.1", bundleName))
	pull(t, fmt.Sprintf("localhost:888/%s:0.0.1", bundleName), tarballPath) // test no oci prefix
	deployAndRemoveRemote(t, fmt.Sprintf("oci://localhost:888/%s:0.0.1", bundleName), tarballPath)
}

func TestBundleIndexInRemoteOnCreate(t *testing.T) {
	e2e.SetupDockerRegistry(t, 888)
	defer e2e.TeardownRegistry(t, 888)

	bundleDir := "src/test/bundles/06-ghcr"
	bundleName := "ghcr-test"
	tarballPath := filepath.Join("build", fmt.Sprintf("uds-bundle-%s-%s-0.0.1.tar.zst", bundleName, e2e.Arch))

	// create and push bundles with different archs to the same OCI repo
	createRemoteInsecure(t, bundleDir, "oci://localhost:888", "arm64")
	createRemoteInsecure(t, bundleDir, "oci://localhost:888", "amd64")

	// curl OCI registry for index
	index, err := queryIndex(t, "http://localhost:888", bundleName)
	require.NoError(t, err)
	validateMultiArchIndex(t, index)

	inspectRemoteInsecure(t, fmt.Sprintf("oci://localhost:888/%s:0.0.1", bundleName))
	pull(t, fmt.Sprintf("localhost:888/%s:0.0.1", bundleName), tarballPath) // test no oci prefix
	deployAndRemoveRemote(t, fmt.Sprintf("oci://localhost:888/%s:0.0.1", bundleName), tarballPath)

	// now test by publishing over the bundle that was created with 'create -o'
	createLocal(t, bundleDir, e2e.Arch)
	publishInsecure(t, tarballPath, "localhost:888")
	index, err = queryIndex(t, "http://localhost:888", bundleName)
	require.NoError(t, err)
	validateMultiArchIndex(t, index)
	inspectRemoteInsecure(t, fmt.Sprintf("oci://localhost:888/%s:0.0.1", bundleName))
	pull(t, fmt.Sprintf("localhost:888/%s:0.0.1", bundleName), tarballPath) // test no oci prefix
	deployAndRemoveRemote(t, fmt.Sprintf("oci://localhost:888/%s:0.0.1", bundleName), tarballPath)
}

func validateMultiArchIndex(t *testing.T, index ocispec.Index) {
	require.Equal(t, 2, len(index.Manifests))
	require.Equal(t, ocispec.MediaTypeImageIndex, index.MediaType)

	var checkedAMD, checkedARM bool
	for _, manifest := range index.Manifests {
		require.Equal(t, ocispec.MediaTypeImageManifest, manifest.MediaType)
		require.Equal(t, "multi", manifest.Platform.OS)
		if manifest.Platform.Architecture == "amd64" {
			require.Equal(t, "amd64", manifest.Platform.Architecture)
			checkedAMD = true
		} else {
			require.Equal(t, "arm64", manifest.Platform.Architecture)
			checkedARM = true
		}
	}
	require.True(t, checkedAMD)
	require.True(t, checkedARM)
}

func TestBundleTmpDir(t *testing.T) {
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

	//Test create using custom tmpDir
	tmpDirName := "customtmp"
	tmpDir := fmt.Sprintf("%s/%s", bundleDir, tmpDirName)

	err := os.Mkdir(tmpDir, 0755)
	if err != nil {
		t.Fatalf("error creating directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// create a file watcher for tmpDir
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatalf("error creating file watcher: %v", err)
	}
	defer watcher.Close()

	// Add the temporary directory to the watcher
	err = watcher.Add(tmpDir)
	if err != nil {
		t.Fatalf("error adding directory to watcher: %v", err)
	}

	// Channel to receive file change events
	done := make(chan bool)
	// Channel to receive errors
	errCh := make(chan error)

	// Watch for file creation in the temporary directory
	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Op&fsnotify.Create == fsnotify.Create {
					// Check if the directory is populated
					files, err := os.ReadDir(tmpDir)
					if err != nil {
						// Send error to the main test goroutine
						errCh <- fmt.Errorf("error reading directory: %v", err)
						return
					}
					if len(files) > 0 {
						// Directory is populated, send success signal to the main test goroutine
						done <- true
						return
					}
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				// Send error to the main test goroutine
				errCh <- fmt.Errorf("error watching directory: %v", err)
				return
			}
		}
	}()

	// run create command with tmpDir
	runCmd(t, "create "+bundleDir+" --tmpdir "+tmpDir+" --confirm --insecure")
	// run deploy command with tmpDir
	runCmd(t, "deploy "+bundlePath+" --tmpdir "+tmpDir+" --confirm --insecure")
	// run remove command with tmpDir
	runCmd(t, "remove "+bundlePath+" --tmpdir "+tmpDir+" --confirm --insecure")

	// handle errors and failures
	select {
	case err := <-errCh:
		t.Fatalf("error: %v", err)
	case <-done:
		t.Log("Directory is populated")
	case <-time.After(10 * time.Second): // Timeout after 10 seconds
		t.Fatal("timeout waiting for directory to get populated")
	}
}
