package test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBundleDeploy(t *testing.T) {
	deployZarfInit(t)
	e2e.CreateZarfPkg(t, "src/test/packages/podinfo", false)

	source := "ghcr.io/defenseunicorns/packages/uds-cli/test/publish/ghcr-test:0.0.1"
	stdout, _ := deployWithTUI(t, source)
	require.Contains(t, stdout, "Validating bundle")
	require.Contains(t, stdout, "UDS Bundle: ghcr-test")
	require.Contains(t, stdout, "Verifying podinfo package (0%)")
	require.Contains(t, stdout, "Downloading podinfo package (0%)")
	require.Contains(t, stdout, "Deploying podinfo package (1 / 1 components)")
	require.Contains(t, stdout, "Deploying nginx package (1 / 1 components)")
	require.Contains(t, stdout, "✔ Package podinfo deployed")
	require.Contains(t, stdout, "✔ Package nginx deployed")
	require.Contains(t, stdout, "Verifying nginx package (0%)")
	require.Contains(t, stdout, "Downloading nginx package (0%)")
	require.Contains(t, stdout, "✨ Bundle ghcr-test deployed successfully")
	remove(t, source)
}

func TestBundleDeployWithBadSource(t *testing.T) {
	deployZarfInit(t)
	e2e.CreateZarfPkg(t, "src/test/packages/podinfo", false)

	source := "a.bad.source:0.0.1"
	stdout, _ := deployWithTUI(t, source)
	require.Contains(t, stdout, "❌ Error deploying bundle: a.bad.source:0.0.1: not found")
}

func TestBundleDeployWithBadPkg(t *testing.T) {
	deployZarfInit(t)

	// deploy a good pkg
	source := "ghcr.io/defenseunicorns/packages/uds-cli/test/publish/ghcr-test:0.0.1 --packages=nginx"
	stdout, _ := deployWithTUI(t, source)
	require.Contains(t, stdout, "✨ Bundle ghcr-test deployed successfully")

	// attempt to deploy a conflicting pkg
	e2e.CreateZarfPkg(t, "src/test/packages/gitrepo", false)
	bundleDir := "src/test/bundles/05-gitrepo"
	bundlePath := filepath.Join(bundleDir, fmt.Sprintf("uds-bundle-gitrepo-%s-0.0.1.tar.zst", e2e.Arch))

	createLocal(t, bundleDir, e2e.Arch)
	stdout, _ = deployWithTUI(t, bundlePath)
	require.Contains(t, stdout, "❌ Error deploying bundle: unable to deploy component \"nginx-remote\": unable to install helm chart")
	require.Contains(t, stdout, "Run uds logs to view deployment logs")
	remove(t, source)
}
