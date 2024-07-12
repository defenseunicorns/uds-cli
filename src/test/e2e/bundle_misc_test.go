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
	zarfPkgPath := "src/test/packages/no-cluster/real-simple"
	e2e.CreateZarfPkg(t, zarfPkgPath, false)
	createLocal(t, "src/test/bundles/11-real-simple", e2e.Arch)
	stdout, _, err := e2e.UDS("logs")
	require.NoError(t, err)
	require.Contains(t, stdout, "DEBUG")
	require.Contains(t, stdout, "UDSBundle")
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
