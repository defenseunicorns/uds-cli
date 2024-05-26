// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package test contains e2e tests for UDS
package test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/defenseunicorns/pkg/helpers"
	"github.com/defenseunicorns/zarf/src/pkg/message"
	"github.com/defenseunicorns/zarf/src/pkg/utils/exec"
	"github.com/stretchr/testify/require"
)

// UDSE2ETest Struct holding common fields most of the tests will utilize.
type UDSE2ETest struct {
	UDSBinPath        string
	Arch              string
	ApplianceMode     bool
	ApplianceModeKeep bool
	RunClusterTests   bool
	CommandLog        []string
}

// GetCLIName looks at the OS and CPU architecture to determine which Zarf binary needs to be run.
func GetCLIName() string {
	var binaryName string
	if runtime.GOOS == "linux" {
		binaryName = "uds"
	} else if runtime.GOOS == "darwin" {
		if runtime.GOARCH == "arm64" {
			binaryName = "uds-mac-apple"
		} else {
			binaryName = "uds-mac-intel"
		}
	}
	return binaryName
}

var logRegex = regexp.MustCompile(`Saving log file to (?P<logFile>.*?\.log)`)

// UDS executes a UDS command.
func (e2e *UDSE2ETest) UDS(args ...string) (string, string, error) {
	e2e.CommandLog = append(e2e.CommandLog, strings.Join(args, " "))
	return exec.CmdWithContext(context.TODO(), exec.PrintCfg(), e2e.UDSBinPath, args...)
}

// UDSNoLog executes a UDS command with no logging.
func (e2e *UDSE2ETest) UDSNoLog(args ...string) (string, string, error) {
	return exec.CmdWithContext(context.TODO(), exec.Config{}, e2e.UDSBinPath, args...)
}

// CleanFiles removes files and directories that have been created during the test.
func (e2e *UDSE2ETest) CleanFiles(files ...string) {
	for _, file := range files {
		_ = os.RemoveAll(file)
	}
}

// GetMismatchedArch determines what architecture our tests are running on,
// and returns the opposite architecture.
func (e2e *UDSE2ETest) GetMismatchedArch() string {
	switch e2e.Arch {
	case "arm64":
		return "amd64"
	default:
		return "arm64"
	}
}

// GetLogFileContents gets the log file contents from a given run's std error.
func (e2e *UDSE2ETest) GetLogFileContents(t *testing.T, stdErr string) string {
	get, err := helpers.MatchRegex(logRegex, stdErr)
	require.NoError(t, err)
	logFile := get("logFile")
	logContents, err := os.ReadFile(logFile)
	require.NoError(t, err)
	return string(logContents)
}

// SetupDockerRegistry uses the host machine's docker daemon to spin up a local registry for testing purposes.
func (e2e *UDSE2ETest) SetupDockerRegistry(t *testing.T, port int) {
	// check if registry is already running on port
	_, _, err := exec.Cmd("docker", "inspect", fmt.Sprintf("registry-%d", port))
	if err == nil {
		fmt.Println("Registry already running, skipping setup")
		return
	}
	registryImage := "registry:2.8.3"
	err = exec.CmdWithPrint("docker", "run", "-d", "--restart=always", "-p", fmt.Sprintf("%d:5000", port), "--name", fmt.Sprintf("registry-%d", port), registryImage)
	require.NoError(t, err)

	// Check for registry health
	waitForRegistryHealth(t, port)
}

// waitForRegistryHealth pings the registry's health endpoint until a successful response or timeout
func waitForRegistryHealth(t *testing.T, port int) {
	healthURL := fmt.Sprintf("http://localhost:%d/v2/", port)
	maxDuration := 10 * time.Second  // Maximum time to wait for the registry to become healthy
	checkInterval := 1 * time.Second // Interval between health checks

	timeout := time.NewTimer(maxDuration)
	defer timeout.Stop()
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-timeout.C:
			t.Fatalf("Timeout waiting for registry at port %d to become healthy", port)
		case <-ticker.C:
			resp, err := http.Get(healthURL)
			if err == nil && resp.StatusCode == http.StatusOK {
				fmt.Printf("Registry at port %d is healthy\n", port)
				return
			}
			if resp != nil {
				resp.Body.Close()
			}
		}
	}
}

// TeardownRegistry removes the local registry.
func (e2e *UDSE2ETest) TeardownRegistry(t *testing.T, port int) {
	// remove the local registry
	err := exec.CmdWithPrint("docker", "rm", "-f", fmt.Sprintf("registry-%d", port))
	require.NoError(t, err)
}

// GetUdsVersion returns the current build version
func (e2e *UDSE2ETest) GetUdsVersion(t *testing.T) string {
	// Get the version of the CLI
	stdOut, stdErr, err := e2e.UDS("version")
	require.NoError(t, err, stdOut, stdErr)
	return strings.Trim(stdOut, "\n")
}

// DownloadZarfInitPkg downloads the zarf init pkg used for testing if it doesn't already exist (todo: makefile?)
func (e2e *UDSE2ETest) DownloadZarfInitPkg(t *testing.T, zarfVersion string) {
	filename := fmt.Sprintf("zarf-init-%s-%s.tar.zst", e2e.Arch, zarfVersion)
	zarfReleaseURL := fmt.Sprintf("https://github.com/defenseunicorns/zarf/releases/download/%s/%s", zarfVersion, filename)
	outputDir := "src/test/packages"

	// Check if the file already exists
	if _, err := os.Stat(outputDir + "/" + filename); err == nil {
		fmt.Println("Zarf init pkg already exists. Skipping download.")
		return
	}
	downloadSpinner := message.NewProgressSpinner("Downloading Zarf init package %s", zarfVersion)
	err := downloadFile(zarfReleaseURL, outputDir)
	downloadSpinner.Successf("Downloaded Zarf init package %s", zarfVersion)
	require.NoError(t, err)
}

// CreateZarfPkg creates a Zarf package in the given path
func (e2e *UDSE2ETest) CreateZarfPkg(t *testing.T, path string, forceCreate bool) {
	//  check if pkg already exists
	pattern := fmt.Sprintf("%s/*-%s-*.tar.zst", path, e2e.Arch)
	matches, err := filepath.Glob(pattern)
	require.NoError(t, err)
	if !forceCreate && len(matches) > 0 {
		fmt.Println("Zarf pkg already exists, skipping create")
		return
	}
	args := strings.Split(fmt.Sprintf("zarf package create %s -o %s --confirm", path, path), " ")
	_, _, err = e2e.UDS(args...)
	require.NoError(t, err)
}

// DeleteZarfPkg deletes a Zarf package from the given path
func (e2e *UDSE2ETest) DeleteZarfPkg(t *testing.T, path string) {
	//  check if pkg already exists
	pattern := fmt.Sprintf("%s/*-%s-*.tar.zst", path, e2e.Arch)
	matches, err := filepath.Glob(pattern)
	require.NoError(t, err)
	if len(matches) > 0 {
		fmt.Println("Deleting Zarf pkg")
		for _, match := range matches {
			os.Remove(match)
		}
		return
	}
}

func downloadFile(url string, outputDir string) error {
	response, err := http.Get(url)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", response.StatusCode)
	}

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return err
	}

	outputFileName := filepath.Base(url)
	outputFilePath := filepath.Join(outputDir, outputFileName)

	outputFile, err := os.Create(outputFilePath)
	if err != nil {
		return err
	}
	defer outputFile.Close()

	_, err = io.Copy(outputFile, response.Body)
	if err != nil {
		return err
	}

	return nil
}

// GetGitRevision returns the current git revision
func (e2e *UDSE2ETest) GetGitRevision() (string, error) {
	out, _, err := exec.Cmd("git", "rev-parse", "--short", "HEAD")
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(out), nil
}

// HelmDepUpdate runs 'helm dependency update .' on the given path
func (e2e *UDSE2ETest) HelmDepUpdate(t *testing.T, path string) {
	cmd := "helm"
	args := strings.Split("dependency update .", " ")
	tmp := exec.PrintCfg()
	tmp.Dir = path
	_, _, err := exec.CmdWithContext(context.TODO(), tmp, cmd, args...)
	require.NoError(t, err)
}
