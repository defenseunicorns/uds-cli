// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package test provides e2e tests for UDS.
package test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/require"
	"oras.land/oras-go/v2/registry"
)

func TestSimpleBundleWithZarfAction(t *testing.T) {
	zarfPkgPath := "src/test/packages/no-cluster/real-simple"
	e2e.CreateZarfPkg(t, zarfPkgPath, false)
	os.Setenv("UDS_LOG_LEVEL", "debug")
	defer os.Unsetenv("UDS_LOG_LEVEL")
	createLocal(t, "src/test/bundles/11-real-simple", e2e.Arch)
	_, stderr := deploy(t, fmt.Sprintf("src/test/bundles/11-real-simple/uds-bundle-real-simple-%s-0.0.1.tar.zst", e2e.Arch))
	require.Contains(t, stderr, "Log level set to debug")
}

func TestSimpleBundleWithNameAndVersionFlags(t *testing.T) {
	zarfPkgPath := "src/test/packages/no-cluster/real-simple"
	e2e.CreateZarfPkg(t, zarfPkgPath, false)
	os.Setenv("UDS_LOG_LEVEL", "debug")
	defer os.Unsetenv("UDS_LOG_LEVEL")
	name, version := "name-from-flag", "version-from-flag"
	bundlePath := "src/test/bundles/11-real-simple"
	runCmd(t, fmt.Sprintf("create %s --confirm --name %s --version %s", bundlePath, name, version))
	_, stderr := deploy(t, fmt.Sprintf("src/test/bundles/11-real-simple/uds-bundle-%s-%s-%s.tar.zst", name, e2e.Arch, version))
	require.Contains(t, stderr, "Log level set to debug")
}

func TestBundleWithLocalAndRemotePkgs(t *testing.T) {
	deployZarfInit(t)
	e2e.SetupDockerRegistry(t, 888)
	defer e2e.TeardownRegistry(t, 888)
	e2e.SetupDockerRegistry(t, 889)
	defer e2e.TeardownRegistry(t, 889)
	e2e.CreateZarfPkg(t, "src/test/packages/podinfo", false)

	bundleDir := "src/test/bundles/03-local-and-remote"
	bundleTarballName := fmt.Sprintf("uds-bundle-test-local-and-remote-%s-0.0.1.tar.zst", e2e.Arch)
	bundlePath := filepath.Join(bundleDir, bundleTarballName)
	pulledBundlePath := filepath.Join("build", bundleTarballName)

	bundleRef := registry.Reference{
		Registry: "oci://localhost:888",
		// this info is derived from the bundle's metadata
		Repository: "test-local-and-remote",
		Reference:  "0.0.1",
	}
	createLocal(t, bundleDir, e2e.Arch)
	inspectLocal(t, bundlePath)
	deploy(t, bundlePath)
	remove(t, bundlePath)

	t.Run("Test pulling and deploying from the same registry", func(t *testing.T) {
		publishInsecure(t, bundlePath, bundleRef.Registry)
		pull(t, bundleRef.String(), bundleTarballName) // note that pull pulls the bundle into the build dir
		deploy(t, pulledBundlePath)
		remove(t, pulledBundlePath)
	})

	t.Run(" Test publishing and deploying from different registries", func(t *testing.T) {
		publishInsecure(t, bundlePath, bundleRef.Registry)
		pull(t, bundleRef.String(), bundleTarballName) // note that pull pulls the bundle into the build dir
		publishInsecure(t, pulledBundlePath, "oci://localhost:889")
		deployInsecure(t, bundleRef.String())
	})
}

func TestLocalBundleWithRemotePkgs(t *testing.T) {
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
	bundlePath := filepath.Join(bundleDir, fmt.Sprintf("uds-bundle-example-remote-%s-0.0.1.tar.zst", e2e.Arch))

	createLocal(t, bundleDir, e2e.Arch)
	inspectLocal(t, bundlePath)
	inspectLocalAndSBOMExtract(t, bundlePath)
	deploy(t, bundlePath)
	remove(t, bundlePath)
}

func TestRemoteBundleWithRemotePkgs(t *testing.T) {
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
		Repository: "example-remote",
		Reference:  "0.0.1",
	}

	bundleTarballName := fmt.Sprintf("uds-bundle-example-remote-%s-0.0.1.tar.zst", e2e.Arch)
	tarballPath := filepath.Join("build", bundleTarballName)
	bundlePath := "src/test/bundles/01-uds-bundle"
	createRemoteInsecure(t, bundlePath, bundleRef.Registry, e2e.Arch)

	// Test without oci prefix
	createRemoteInsecure(t, bundlePath, "localhost:888", e2e.Arch)

	pull(t, bundleRef.String(), bundleTarballName)
	inspectRemoteInsecure(t, bundleRef.String())
	inspectRemoteAndSBOMExtract(t, bundleRef.String())
	deployAndRemoveLocalAndRemoteInsecure(t, bundleRef.String(), tarballPath)

	bundleRef = registry.Reference{
		Registry:   "oci://localhost:888",
		Repository: "example-remote",
		Reference:  "0.0.1",
	}
	deployAndRemoveLocalAndRemoteInsecure(t, bundleRef.String(), tarballPath)
}

func TestBundleWithGitRepo(t *testing.T) {
	deployZarfInit(t)
	e2e.CreateZarfPkg(t, "src/test/packages/gitrepo", false)
	bundleDir := "src/test/bundles/05-gitrepo"
	bundlePath := filepath.Join(bundleDir, fmt.Sprintf("uds-bundle-gitrepo-%s-0.0.1.tar.zst", e2e.Arch))

	createLocal(t, bundleDir, e2e.Arch)
	deploy(t, bundlePath)
	remove(t, bundlePath)

	// TODO (holaday): no assertion; does this deploy make sense?
}

func TestBundleWithYmlFile(t *testing.T) {
	deployZarfInit(t)
	e2e.CreateZarfPkg(t, "src/test/packages/nginx", true)

	bundleDir := "src/test/bundles/09-uds-bundle-yml"
	bundlePath := filepath.Join(bundleDir, fmt.Sprintf("uds-bundle-yml-example-%s-0.0.1.tar.zst", e2e.Arch))

	createLocal(t, bundleDir, e2e.Arch)
	inspectLocal(t, bundlePath)
	inspectLocalAndSBOMExtract(t, bundlePath)
	os.Setenv("UDS_CONFIG", filepath.Join("src/test/bundles/09-uds-bundle-yml", "uds-config.yml"))
	defer os.Unsetenv("UDS_CONFIG")
	deploy(t, bundlePath)
	remove(t, bundlePath)
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
	bundleTarballName := fmt.Sprintf("uds-bundle-package-naming-%s-0.0.1.tar.zst", e2e.Arch)
	bundlePath := filepath.Join(bundleDir, bundleTarballName)
	tarballPath := filepath.Join("build", bundleTarballName)
	bundleRef := registry.Reference{
		Registry: "oci://localhost:888",
		// this info is derived from the bundle's metadata
		Repository: "package-naming",
		Reference:  "0.0.1",
	}
	createLocal(t, bundleDir, e2e.Arch) // todo: allow creating from both the folder containing and direct reference to uds-bundle.yaml
	publishInsecure(t, bundlePath, bundleRef.Registry)
	pull(t, bundleRef.String(), bundleTarballName)
	deploy(t, tarballPath)
	remove(t, tarballPath)

	// Test create -o with zarf package names that don't match the zarf package name in the bundle
	createRemoteInsecure(t, bundleDir, bundleRef.Registry, e2e.Arch)
	deployAndRemoveLocalAndRemoteInsecure(t, bundleRef.String(), tarballPath)
}

func TestBundleIndexInRemoteOnPublish(t *testing.T) {
	deployZarfInit(t)
	e2e.SetupDockerRegistry(t, 888)
	defer e2e.TeardownRegistry(t, 888)

	bundleDir := "src/test/bundles/06-ghcr"
	bundleName := "ghcr-test"
	bundleTarballName := fmt.Sprintf("uds-bundle-%s-%s-0.0.1.tar.zst", bundleName, e2e.Arch)
	bundlePathARM := filepath.Join(bundleDir, fmt.Sprintf("uds-bundle-%s-%s-0.0.1.tar.zst", bundleName, "arm64"))
	bundlePathAMD := filepath.Join(bundleDir, fmt.Sprintf("uds-bundle-%s-%s-0.0.1.tar.zst", bundleName, "amd64"))
	tarballPath := filepath.Join("build", bundleTarballName)

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
	pull(t, fmt.Sprintf("localhost:888/%s:0.0.1", bundleName), bundleTarballName) // test no oci prefix
	deployAndRemoveLocalAndRemoteInsecure(t, fmt.Sprintf("oci://localhost:888/%s:0.0.1", bundleName), tarballPath)

	// now test by running 'create -o' over the bundle that was published
	createRemoteInsecure(t, bundleDir, "oci://localhost:888", e2e.Arch)
	index, err = queryIndex(t, "http://localhost:888", bundleName)
	require.NoError(t, err)
	validateMultiArchIndex(t, index)
	inspectRemoteInsecure(t, fmt.Sprintf("oci://localhost:888/%s:0.0.1", bundleName))
	pull(t, fmt.Sprintf("localhost:888/%s:0.0.1", bundleName), bundleTarballName) // test no oci prefix
	deployAndRemoveLocalAndRemoteInsecure(t, fmt.Sprintf("oci://localhost:888/%s:0.0.1", bundleName), tarballPath)
}

func TestBundleIndexInRemoteOnCreate(t *testing.T) {
	deployZarfInit(t)
	e2e.SetupDockerRegistry(t, 888)
	defer e2e.TeardownRegistry(t, 888)

	bundleDir := "src/test/bundles/06-ghcr"
	bundleName := "ghcr-test"
	bundleTarballName := fmt.Sprintf("uds-bundle-%s-%s-0.0.1.tar.zst", bundleName, e2e.Arch)
	tarballPath := filepath.Join("build", bundleTarballName)

	// create and push bundles with different archs to the same OCI repo
	createRemoteInsecure(t, bundleDir, "oci://localhost:888", "arm64")
	createRemoteInsecure(t, bundleDir, "oci://localhost:888", "amd64")

	// curl OCI registry for index
	index, err := queryIndex(t, "http://localhost:888", bundleName)
	require.NoError(t, err)
	validateMultiArchIndex(t, index)

	inspectRemoteInsecure(t, fmt.Sprintf("oci://localhost:888/%s:0.0.1", bundleName))
	pull(t, fmt.Sprintf("localhost:888/%s:0.0.1", bundleName), bundleTarballName) // test no oci prefix
	deployAndRemoveLocalAndRemoteInsecure(t, fmt.Sprintf("oci://localhost:888/%s:0.0.1", bundleName), tarballPath)

	// now test by publishing over the bundle that was created with 'create -o'
	createLocal(t, bundleDir, e2e.Arch)
	publishInsecure(t, tarballPath, "localhost:888")
	index, err = queryIndex(t, "http://localhost:888", bundleName)
	require.NoError(t, err)
	validateMultiArchIndex(t, index)
	inspectRemoteInsecure(t, fmt.Sprintf("oci://localhost:888/%s:0.0.1", bundleName))
	pull(t, fmt.Sprintf("localhost:888/%s:0.0.1", bundleName), bundleTarballName) // test no oci prefix
	deployAndRemoveLocalAndRemoteInsecure(t, fmt.Sprintf("oci://localhost:888/%s:0.0.1", bundleName), tarballPath)
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

func TestBundleWithComposedPkgComponent(t *testing.T) {
	deployZarfInit(t)
	e2e.SetupDockerRegistry(t, 888)
	defer e2e.TeardownRegistry(t, 888)
	zarfPkgPath := "src/test/packages/prometheus"
	pkg := filepath.Join(zarfPkgPath, fmt.Sprintf("zarf-package-prometheus-%s-0.0.1.tar.zst", e2e.Arch))
	e2e.CreateZarfPkg(t, zarfPkgPath, false)
	zarfPublish(t, pkg, "localhost:888")

	bundleDir := "src/test/bundles/13-composable-component"
	bundleName := "with-composed"
	bundlePath := filepath.Join(bundleDir, fmt.Sprintf("uds-bundle-%s-%s-0.0.1.tar.zst", bundleName, e2e.Arch))
	createLocal(t, bundleDir, e2e.Arch)
	deploy(t, bundlePath)
	remove(t, bundlePath)
}

func TestBundleTmpDir(t *testing.T) {
	deployZarfInit(t)
	e2e.CreateZarfPkg(t, "src/test/packages/podinfo", false)

	bundleDir := "src/test/bundles/03-local-and-remote"
	bundlePath := filepath.Join(bundleDir, fmt.Sprintf("uds-bundle-test-local-and-remote-%s-0.0.1.tar.zst", e2e.Arch))

	// Test create using custom tmpDir
	tmpDirName := "customtmp"
	tmpDir := fmt.Sprintf("%s/%s", bundleDir, tmpDirName)

	err := os.Mkdir(tmpDir, 0o755)
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
	// remove customtmp folder if it exists
	err = os.RemoveAll("./customtmp")
	require.NoError(t, err)
}
