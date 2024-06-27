// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package test provides e2e tests for UDS.
package test

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/mod/modfile"

	"github.com/defenseunicorns/uds-cli/src/test"

	"github.com/defenseunicorns/zarf/src/pkg/message"
	"github.com/pterm/pterm"

	"github.com/defenseunicorns/uds-cli/src/config"
)

var (
	e2e test.UDSE2ETest //nolint:gochecknoglobals
)

const (
	applianceModeEnvVar     = "APPLIANCE_MODE"
	applianceModeKeepEnvVar = "APPLIANCE_MODE_KEEP"
	skipK8sEnvVar           = "SKIP_K8S"
)

// TestMain lets us customize the test run. See https://medium.com/goingogo/why-use-testmain-for-testing-in-go-dafb52b406bc.
func TestMain(m *testing.M) {
	// Work from the root directory of the project
	err := os.Chdir("../../../")
	if err != nil {
		fmt.Println(err)
	}

	// K3d use the intern package, which requires this to be set in go 1.19
	os.Setenv("ASSUME_NO_MOVING_GC_UNSAFE_RISK_IT_WITH", "go1.19")

	retCode, err := doAllTheThings(m)
	if err != nil {
		fmt.Println(err) //nolint:forbidigo
	}

	os.Exit(retCode)
}

// doAllTheThings just wraps what should go in TestMain. It's in its own function so it can
// [a] Not have a bunch of `os.Exit()` calls in it
// [b] Do defers properly
// [c] Handle errors cleanly
//
// It returns the return code passed from `m.Run()` and any error thrown.
func doAllTheThings(m *testing.M) (int, error) {
	var err error

	// Set up constants in the global variable that all the tests are able to access
	e2e.Arch = config.GetArch()
	e2e.UDSBinPath = path.Join("build", test.GetCLIName())
	e2e.ApplianceMode = os.Getenv(applianceModeEnvVar) == "true"
	e2e.ApplianceModeKeep = os.Getenv(applianceModeKeepEnvVar) == "true"
	e2e.RunClusterTests = os.Getenv(skipK8sEnvVar) != "true"

	// Validate that the UDS binary exists. If it doesn't that means the dev hasn't built it
	_, err = os.Stat(e2e.UDSBinPath)
	if err != nil {
		return 1, fmt.Errorf("zarf binary %s not found", e2e.UDSBinPath)
	}

	// Run the tests, with the cluster cleanup being deferred to the end of the function call
	returnCode := m.Run()

	isCi := os.Getenv("CI") == "true"
	if isCi {
		pterm.Println("::notice::UDS Command Log")
		// Print out the command history
		pterm.Println("::group::UDS Command Log")
		for _, cmd := range e2e.CommandLog {
			message.ZarfCommand(cmd) // todo: it's a UDS cmd but this links up with pterm in Zarf
		}
		pterm.Println("::endgroup::")
	}

	return returnCode, nil
}

// deployZarfInit deploys Zarf init (from a bundle!) if it hasn't already been deployed.
func deployZarfInit(t *testing.T) {
	if !zarfInitDeployed() {
		// get Zarf version from go.mod
		b, err := os.ReadFile("go.mod")
		require.NoError(t, err)
		f, err := modfile.Parse("go.mod", b, nil)
		require.NoError(t, err)
		var zarfVersion string
		for _, r := range f.Require {
			if r.Mod.Path == "github.com/defenseunicorns/zarf" {
				zarfVersion = r.Mod.Version
			}
		}
		e2e.DownloadZarfInitPkg(t, zarfVersion)

		bundleDir := "src/test/bundles/04-init"
		bundlePath := filepath.Join(bundleDir, fmt.Sprintf("uds-bundle-all-the-inits-%s-0.0.1.tar.zst", e2e.Arch))

		// Create
		cmd := strings.Split(fmt.Sprintf("create %s --confirm --insecure", bundleDir), " ")
		_, _, err = e2e.UDS(cmd...)
		require.NoError(t, err)

		// Deploy
		cmd = strings.Split(fmt.Sprintf("deploy %s --confirm -l=debug", bundlePath), " ")
		_, _, err = e2e.UDS(cmd...)
		require.NoError(t, err)
	}
}

func zarfInitDeployed() bool {
	cmd := strings.Split("zarf tools kubectl get deployments zarf-docker-registry --namespace zarf", " ")
	_, stderr, _ := e2e.UDS(cmd...)
	registryDeployed := !strings.Contains(stderr, "No resources found in zarf namespace") && !strings.Contains(stderr, "not found")

	cmd = strings.Split("zarf tools kubectl get deployments agent-hook --namespace zarf", " ")
	_, stderr, _ = e2e.UDS(cmd...)
	agentDeployed := !strings.Contains(stderr, "No resources found in zarf namespace") && !strings.Contains(stderr, "not found")
	return registryDeployed && agentDeployed
}
