// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package testutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	bundlepkg "github.com/defenseunicorns/uds-cli/pkg/bundle"
	bundlecmd "github.com/defenseunicorns/uds-cli/pkg/cmd/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
)

func TestDataPath(relPath string) string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("cannot determine testutil file location")
	}
	root := filepath.Join(filepath.Dir(thisFile), "..", "..")
	return filepath.Join(root, "tests", "test_data", relPath)
}

func UDSCLIPath(t *testing.T, runHint string) string {
	t.Helper()
	path := os.Getenv("UDS_CLI_PATH")
	if path == "" {
		if runHint == "" {
			runHint = "run via 'maru run build-local' first"
		}
		t.Skipf("UDS_CLI_PATH not set; %s", runHint)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("UDS_CLI_PATH binary not found at %s: %v", path, err)
	}
	return path
}

func PrepareBundleDir(t *testing.T, testDataRelPath string) string {
	t.Helper()

	srcDir := TestDataPath(testDataRelPath)
	bundleFile := filepath.Join(srcDir, "bundle.uds.hcl")
	_, err := os.Stat(bundleFile)
	require.NoError(t, err, "bundle.uds.hcl must exist in %s", srcDir)

	dir := t.TempDir()
	require.NoError(t, CopyDir(srcDir, dir))
	return dir
}

// CreateBundleFromTestData creates a bundle from a copied test fixture and
// returns the resulting archive path.
func CreateBundleFromTestData(t *testing.T, testDataRelPath, arch string) string {
	t.Helper()

	_, result, _, err := createBundleFromTestData(t, testDataRelPath, arch)
	require.NoError(t, err)

	_, err = os.Stat(result.OutputPath)
	require.NoError(t, err, "expected bundle output file to exist")
	return result.OutputPath
}

// CreateBundleFromTestDataWithDiagnostics creates a bundle from a copied test
// fixture and returns its archive path and captured diagnostic output.
func CreateBundleFromTestDataWithDiagnostics(t *testing.T, testDataRelPath, arch string) (string, string) {
	t.Helper()

	_, result, diagnostics, err := createBundleFromTestData(t, testDataRelPath, arch)
	require.NoError(t, err)

	_, err = os.Stat(result.OutputPath)
	require.NoError(t, err, "expected bundle output file to exist")
	return result.OutputPath, diagnostics
}

// CreateBundleFromTestDataExpectError creates a bundle from a copied test
// fixture, asserts that creation fails, and returns the copied fixture
// directory so callers can assert that no archive was written.
func CreateBundleFromTestDataExpectError(t *testing.T, testDataRelPath, arch string) string {
	t.Helper()

	dir, _, _, err := createBundleFromTestData(t, testDataRelPath, arch)
	require.Error(t, err)
	return dir
}

func createBundleFromTestData(t *testing.T, testDataRelPath, arch string) (string, *bundlepkg.CreateResult, string, error) {
	t.Helper()

	dir := PrepareBundleDir(t, testDataRelPath)
	streams, _, _, errOut := iostreams.NewTestIOStreams()

	resolver := bundlecmd.NewConfigResolver()
	opts := resolver.Defaults()
	opts.Architecture = arch
	global := &bundlepkg.GlobalOptions{LogLevel: opts.LogLevel}
	result, err := bundlepkg.Create(t.Context(), bundlepkg.CreateOptions{
		Config:     &bundlepkg.UDSBundleConfig{Global: global, Options: &opts},
		BundleFile: filepath.Join(dir, "bundle.uds.hcl"),
		Streams:    streams,
	})
	return dir, result, errOut.String(), err
}

func CreateBundleFromTestDataCLI(t *testing.T, testDataRelPath, arch string) string {
	t.Helper()
	return CreateBundleFromTestDataCLIWithBinary(t, UDSCLIPath(t, "run via 'maru run test:integration'"), testDataRelPath, arch)
}

func CreateBundleFromTestDataCLIWithBinary(t *testing.T, udsPath, testDataRelPath, arch string) string {
	t.Helper()

	dir := PrepareBundleDir(t, testDataRelPath)
	args := []string{"bundle", "create", "--architecture", arch, dir}
	cmd := exec.Command(udsPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	require.NoError(t, cmd.Run(), "bundle create should succeed")

	matches, err := filepath.Glob(filepath.Join(dir, "*.tar.zst"))
	require.NoError(t, err)
	require.Len(t, matches, 1, "expected exactly one bundle artifact in %s", dir)
	_, err = os.Stat(matches[0])
	require.NoError(t, err, "expected bundle output file to exist")
	return matches[0]
}

func CreateBundleFromTestDataCobra(t *testing.T, testDataRelPath, arch string) string {
	t.Helper()

	dir := PrepareBundleDir(t, testDataRelPath)
	streams, _, out, _ := iostreams.NewTestIOStreams()
	root := bundlecmd.NewBundleCommand(streams)
	root.SetArgs([]string{"create", "--architecture", arch, dir})
	require.NoError(t, root.Execute())
	assert.Contains(t, out.String(), "Bundle Name:")

	matches, err := filepath.Glob(filepath.Join(dir, "*.tar.zst"))
	require.NoError(t, err)
	require.Len(t, matches, 1, "expected exactly one bundle artifact in %s", dir)
	_, err = os.Stat(matches[0])
	require.NoError(t, err, "expected bundle output file to exist")
	return matches[0]
}

func CopyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	})
}

func CheckDockerRunning(t *testing.T, reason string) {
	t.Helper()
	cmd := exec.Command("docker", "info")
	if err := cmd.Run(); err != nil {
		if reason == "" {
			reason = "tests require Docker"
		}
		t.Skip(reason)
	}
}

func DeleteK3dCluster(t *testing.T, clusterName string) {
	t.Helper()
	cmd := exec.Command("k3d", "cluster", "delete", clusterName)
	output, err := cmd.CombinedOutput()
	if err == nil {
		return
	}
	out := string(output)
	if strings.Contains(out, "No cluster(s) found") || strings.Contains(out, "No nodes found for given cluster") {
		return
	}
	t.Fatalf("failed to delete k3d cluster %q: %v\n%s", clusterName, err, out)
}

func RunBundleDeploy(t *testing.T, udsPath, artifactPath string) {
	t.Helper()
	args := []string{"bundle", "deploy", artifactPath}
	cmd := exec.Command(udsPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	require.NoError(t, cmd.Run(), "bundle deploy should succeed for %s", artifactPath)
}
