package test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUDSCmd(t *testing.T) {
	_, _, err := e2e.UDS()
	require.NoError(t, err)
}

func TestUDSLogs(t *testing.T) {
	inspectRemote(t, "ghcr.io/defenseunicorns/packages/uds-cli/test/publish/ghcr-test:0.0.1")
	stderr, _, err := e2e.UDS("logs")
	require.NoError(t, err)
	require.Contains(t, stderr, "DEBUG")
	require.Contains(t, stderr, "UDSBundle")
}

func TestInvalidConfig(t *testing.T) {
	os.Setenv("UDS_CONFIG", filepath.Join("src/test/bundles/07-helm-overrides", "uds-config-invalid.yaml"))
	zarfPkgPath := "src/test/packages/helm"
	e2e.HelmDepUpdate(t, fmt.Sprintf("%s/unicorn-podinfo", zarfPkgPath))
	args := strings.Split(fmt.Sprintf("zarf package create %s -o %s --confirm", zarfPkgPath, zarfPkgPath), " ")
	_, stdErr, err := e2e.UDS(args...)
	require.Error(t, err)
	require.Contains(t, stdErr, "invalid config option: log_levelx")
	os.Unsetenv("UDS_CONFIG")
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
