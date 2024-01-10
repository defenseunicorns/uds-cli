// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package test provides e2e tests for UDS.
package test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/defenseunicorns/zarf/src/pkg/utils/exec"
	"github.com/stretchr/testify/require"
	"oras.land/oras-go/v2/registry"

	"github.com/defenseunicorns/uds-cli/src/config"
)

func zarfPublish(t *testing.T, path string, reg string) {
	cmd := "zarf"
	args := strings.Split(fmt.Sprintf("package publish %s oci://%s --insecure --oci-concurrency=10", path, reg), " ")
	tmp := exec.PrintCfg()
	_, _, err := exec.CmdWithContext(context.TODO(), tmp, cmd, args...)
	require.NoError(t, err)
}

func TestCreateWithNoPath(t *testing.T) {
	zarfPkgPath1 := "src/test/packages/no-cluster/output-var"
	zarfPkgPath2 := "src/test/packages/no-cluster/receive-var"
	e2e.CreateZarfPkg(t, zarfPkgPath1)
	e2e.CreateZarfPkg(t, zarfPkgPath2)

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
	e2e.CreateZarfPkg(t, "src/test/packages/podinfo")

	bundleDir := "src/test/bundles/03-local-and-remote"
	bundlePath := filepath.Join(bundleDir, fmt.Sprintf("uds-bundle-test-local-and-remote-%s-0.0.1.tar.zst", e2e.Arch))

	tarballPath := filepath.Join("build", fmt.Sprintf("uds-bundle-test-local-and-remote-%s-0.0.1.tar.zst", e2e.Arch))
	bundleRef := registry.Reference{
		Registry: "oci://localhost:888",
		// this info is derived from the bundle's metadata
		Repository: "test-local-and-remote",
		Reference:  fmt.Sprintf("0.0.1-%s", e2e.Arch),
	}
	createSecure(t, bundleDir)
	inspect(t, bundlePath)
	publishInsecure(t, bundlePath, bundleRef.Registry)
	pull(t, bundleRef.String(), tarballPath)
	deploy(t, tarballPath)
	remove(t, tarballPath)
}

func TestBundle(t *testing.T) {
	deployZarfInit(t)

	e2e.CreateZarfPkg(t, "src/test/packages/nginx")
	e2e.CreateZarfPkg(t, "src/test/packages/podinfo")

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

	e2e.CreateZarfPkg(t, "src/test/packages/nginx")
	e2e.CreateZarfPkg(t, "src/test/packages/podinfo")

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
	cmd := strings.Split("tools kubectl get deployments -A -o=jsonpath='{.items[*].metadata.name}'", " ")
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

	e2e.CreateZarfPkg(t, "src/test/packages/nginx")
	e2e.CreateZarfPkg(t, "src/test/packages/podinfo")

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
	cmd := strings.Split("tools kubectl get deployments -A -o=jsonpath='{.items[*].metadata.name}'", " ")
	deployments, _, _ := e2e.UDS(cmd...)
	require.Contains(t, deployments, "podinfo")
	require.NotContains(t, deployments, "nginx")

	// Deploy bundle --resume
	deployResumeFlag(t, bundlePath)
	cmd = strings.Split("tools kubectl get deployments -A -o=jsonpath='{.items[*].metadata.name}'", " ")
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
	cmd = strings.Split("tools kubectl get deployments -A -o=jsonpath='{.items[*].metadata.name}'", " ")
	deployments, _, _ = e2e.UDS(cmd...)
	require.NotContains(t, deployments, "podinfo")
	require.NotContains(t, deployments, "nginx")
}

func TestRemoteBundle(t *testing.T) {
	deployZarfInit(t)
	e2e.CreateZarfPkg(t, "src/test/packages/podinfo")

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
	e2e.CreateZarfPkg(t, "src/test/packages/gitrepo")
	bundleDir := "src/test/bundles/05-gitrepo"
	bundlePath := filepath.Join(bundleDir, fmt.Sprintf("uds-bundle-gitrepo-%s-0.0.1.tar.zst", e2e.Arch))

	create(t, bundleDir)
	deploy(t, bundlePath)
	remove(t, bundlePath)
}

func TestBundleWithYmlFile(t *testing.T) {
	deployZarfInit(t)

	e2e.CreateZarfPkg(t, "src/test/packages/nginx")
	e2e.CreateZarfPkg(t, "src/test/packages/podinfo")

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

	create(t, bundleDir) // todo: allow creating from both the folder containing and direct reference to uds-bundle.yaml
	inspect(t, bundlePath)
	inspectAndSBOMExtract(t, bundlePath)
	// Test with an "options only" config file
	os.Setenv("UDS_CONFIG", filepath.Join("src/test/bundles/09-uds-bundle-yml", "uds-config.yml"))
	deploy(t, bundlePath)
	remove(t, bundlePath)
}
