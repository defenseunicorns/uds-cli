package test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/stretchr/testify/require"
)

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
		require.NotContains(t, string(contents), "\u001B") // ensure no color-related bytes
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

	t.Run("list variables for local tarball", func(t *testing.T) {
		cmd := strings.Split(fmt.Sprintf("inspect %s --list-variables", bundlePath), " ")
		_, stderr, err := e2e.UDS(cmd...)
		require.NoError(t, err)

		ansiRegex := regexp.MustCompile("\x1b\\[[0-9;]*[a-zA-Z]")
		cleaned := ansiRegex.ReplaceAllString(stderr, "")
		require.Contains(t, cleaned, "prometheus:\n  variables: []\n")
	})

	t.Run("list variables for remote tarball", func(t *testing.T) {
		cmd := strings.Split(fmt.Sprintf("inspect %s --list-variables --insecure", fmt.Sprintf("%s/optional-components:0.0.1", ociRef)), " ")
		_, stderr, err := e2e.UDS(cmd...)
		require.NoError(t, err)

		ansiRegex := regexp.MustCompile("\x1b\\[[0-9;]*[a-zA-Z]")
		cleaned := ansiRegex.ReplaceAllString(stderr, "")
		require.Contains(t, cleaned, "prometheus:\n  variables: []\n")
	})
}
