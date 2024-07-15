package test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/stretchr/testify/require"
)

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
