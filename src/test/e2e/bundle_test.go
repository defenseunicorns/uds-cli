// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package test provides e2e tests for UDS.
package test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/stretchr/testify/require"
	"oras.land/oras-go/v2/registry"

	"github.com/defenseunicorns/uds-cli/src/config"
)

func TestUDSCmd(t *testing.T) {
	_, _, err := e2e.UDS()
	require.NoError(t, err)
}

func TestUDSLogs(t *testing.T) {
	zarfPkgPath := "src/test/packages/no-cluster/real-simple"
	e2e.CreateZarfPkg(t, zarfPkgPath, false)
	createLocal(t, "src/test/bundles/11-real-simple", e2e.Arch)
	stdout, _, err := e2e.UDS("logs")
	require.NoError(t, err)
	require.Contains(t, stdout, "DEBUG")
	require.Contains(t, stdout, "UDSBundle")
}

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

func TestLocalBundleWithOutput(t *testing.T) {
	path := "src/test/packages/nginx"
	args := strings.Split(fmt.Sprintf("zarf package create %s -o %s --confirm", path, path), " ")
	_, _, err := e2e.UDS(args...)
	require.NoError(t, err)

	bundleDir := "src/test/bundles/09-uds-bundle-yml"
	destDir := "src/test/test/"
	bundlePath := filepath.Join(destDir, fmt.Sprintf("uds-bundle-yml-example-%s-0.0.1.tar.zst", e2e.Arch))
	createLocalWithOuputFlag(t, bundleDir, destDir, e2e.Arch)

	cmd := strings.Split(fmt.Sprintf("inspect %s", bundlePath), " ")
	_, _, err = e2e.UDS(cmd...)
	require.NoError(t, err)
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
	stdout, _, err := e2e.UDS(cmd...)
	require.NoError(t, err)
	require.Contains(t, stdout, "Cannot extract, no SBOMs found in bundle")
	require.Contains(t, stdout, "sboms.tar not found in Zarf pkg")
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
	stdout, _, err := e2e.UDS(cmd...)
	require.NoError(t, err)
	require.Contains(t, stdout, "Cannot extract, no SBOMs found in bundle")
	require.Contains(t, stdout, "sboms.tar not found in Zarf pkg")
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

func TestInvalidConfig(t *testing.T) {
	os.Setenv("UDS_CONFIG", filepath.Join("src/test/bundles/07-helm-overrides", "uds-config-invalid.yaml"))
	defer os.Unsetenv("UDS_CONFIG")
	zarfPkgPath := "src/test/packages/helm"
	e2e.HelmDepUpdate(t, fmt.Sprintf("%s/unicorn-podinfo", zarfPkgPath))
	args := strings.Split(fmt.Sprintf("zarf package create %s -o %s --confirm", zarfPkgPath, zarfPkgPath), " ")
	_, stdErr, _ := e2e.UDS(args...)
	count := strings.Count(stdErr, "invalid config option: log_levelx")
	require.Equal(t, 1, count, "The string 'invalid config option: log_levelx' should appear exactly once")
	require.NotContains(t, stdErr, "Usage:")
}

func TestInvalidBundle(t *testing.T) {
	deployZarfInit(t)
	zarfPkgPath := "src/test/packages/helm"
	e2e.HelmDepUpdate(t, fmt.Sprintf("%s/unicorn-podinfo", zarfPkgPath))
	e2e.CreateZarfPkg(t, zarfPkgPath, false)
	bundleDir := "src/test/bundles/07-helm-overrides/invalid"
	stderr := createLocalError(bundleDir, e2e.Arch)
	require.Contains(t, stderr, "unknown field")
}

func TestArchCheck(t *testing.T) {
	testArch := "arm64"

	// use arch that is different from system arch
	if e2e.Arch == "arm64" {
		testArch = "amd64"
	}

	deployZarfInit(t)
	zarfPkgPath := "src/test/packages/helm"
	e2e.CreateZarfPkgWithArch(t, zarfPkgPath, false, testArch)
	bundleDir := "src/test/bundles/07-helm-overrides"
	bundlePath := filepath.Join(bundleDir, fmt.Sprintf("uds-bundle-helm-overrides-%s-0.0.1.tar.zst", testArch))
	createLocal(t, bundleDir, testArch)
	cmd := strings.Split(fmt.Sprintf("deploy %s --confirm", bundlePath), " ")
	_, stderr, _ := e2e.UDS(cmd...)
	require.Contains(t, stderr, fmt.Sprintf("arch %s does not match cluster arch, [%s]", testArch, e2e.Arch))
}

func TestListImages(t *testing.T) {
	e2e.SetupDockerRegistry(t, 888)
	defer e2e.TeardownRegistry(t, 888)

	zarfPkgPath := "src/test/packages/prometheus"
	pkg := filepath.Join(zarfPkgPath, fmt.Sprintf("zarf-package-prometheus-%s-0.0.1.tar.zst", e2e.Arch))
	e2e.CreateZarfPkg(t, zarfPkgPath, false)
	zarfPublish(t, pkg, "localhost:888")

	zarfPkgPath = "src/test/packages/podinfo-nginx"
	e2e.CreateZarfPkg(t, zarfPkgPath, false)

	bundleDir := "src/test/bundles/14-optional-components"

	t.Run("list images on bundle YAML only", func(t *testing.T) {
		cmd := strings.Split(fmt.Sprintf("inspect %s --list-images --insecure", filepath.Join(bundleDir, config.BundleYAML)), " ")
		stdout, _, err := e2e.UDS(cmd...)
		require.NoError(t, err)
		require.Contains(t, stdout, "library/registry")
		require.Contains(t, stdout, "ghcr.io/defenseunicorns/zarf/agent")
		require.Contains(t, stdout, "nginx")
		require.Contains(t, stdout, "quay.io/prometheus/node-exporter")

		// ensure non-req'd components got filtered
		require.NotContains(t, stdout, "grafana")
		require.NotContains(t, stdout, "gitea")
		require.NotContains(t, stdout, "kiwix")
		require.NotContains(t, stdout, "podinfo")
	})

	t.Run("list images outputted to a file", func(t *testing.T) {
		args := strings.Split(fmt.Sprintf("inspect %s --list-images --insecure", filepath.Join(bundleDir, config.BundleYAML)), " ")
		cmd := exec.Command(e2e.UDSBinPath, args...)

		// open the out file for writing, and redirect the cmd output to that file
		filename := "./out.txt"
		outfile, err := os.Create(filename)
		require.NoError(t, err)
		defer outfile.Close()
		defer os.Remove(filename)

		cmd.Stdout = outfile

		err = cmd.Run()
		require.NoError(t, err)

		// read in the file and check its contents
		contents, err := os.ReadFile(filename)
		require.NoError(t, err)
		require.NotContains(t, string(contents), "\u001b") // ensure no color-related bytes
		require.Contains(t, string(contents), "library/registry")
	})
}

func TestListVariables(t *testing.T) {
	e2e.SetupDockerRegistry(t, 888)
	defer e2e.TeardownRegistry(t, 888)

	zarfPkgPath := "src/test/packages/prometheus"
	pkg := filepath.Join(zarfPkgPath, fmt.Sprintf("zarf-package-prometheus-%s-0.0.1.tar.zst", e2e.Arch))
	e2e.CreateZarfPkg(t, zarfPkgPath, false)
	zarfPublish(t, pkg, "localhost:888")

	zarfPkgPath = "src/test/packages/podinfo-nginx"
	e2e.CreateZarfPkg(t, zarfPkgPath, false)

	bundleDir := "src/test/bundles/14-optional-components"
	bundlePath := filepath.Join(bundleDir, fmt.Sprintf("uds-bundle-optional-components-%s-0.0.1.tar.zst", e2e.Arch))
	ociRef := "oci://localhost:888/test/bundles"
	createLocal(t, bundleDir, e2e.Arch)
	publishInsecure(t, bundlePath, ociRef)

	t.Run("list variables for local tarball with no color", func(t *testing.T) {
		cmd := strings.Split(fmt.Sprintf("inspect %s --list-variables --no-color", bundlePath), " ")
		stdout, _, err := e2e.UDS(cmd...)
		require.NoError(t, err)
		require.Contains(t, stdout, "prometheus:\n  variables: []\n")
	})

	t.Run("list variables for local tarball with color", func(t *testing.T) {
		cmd := strings.Split(fmt.Sprintf("inspect %s --list-variables", bundlePath), " ")
		stdout, _, err := e2e.UDS(cmd...)
		require.NoError(t, err)
		require.Contains(t, stdout, "\x1b")
	})

	t.Run("list variables for remote tarball", func(t *testing.T) {
		cmd := strings.Split(fmt.Sprintf("inspect %s --list-variables --insecure", fmt.Sprintf("%s/optional-components:0.0.1", ociRef)), " ")
		stdout, _, err := e2e.UDS(cmd...)
		require.NoError(t, err)

		ansiRegex := regexp.MustCompile("\x1b\\[[0-9;]*[a-zA-Z]")
		cleaned := ansiRegex.ReplaceAllString(stdout, "")
		require.Contains(t, cleaned, "prometheus:\n  variables: []\n")
	})
}
