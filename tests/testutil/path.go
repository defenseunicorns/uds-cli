// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package testutil

import (
	"testing"

	internaltestutil "github.com/defenseunicorns/uds-cli/internal/testutil"
)

// TestDataPath returns an absolute path beneath tests/test_data.
func TestDataPath(relPath string) string {
	return internaltestutil.TestDataPath(relPath)
}

// UDSCLIPath returns the configured CLI binary path or skips the test.
func UDSCLIPath(t *testing.T, runHint string) string {
	return internaltestutil.UDSCLIPath(t, runHint)
}

// PrepareBundleDir copies bundle test data into a temporary directory.
func PrepareBundleDir(t *testing.T, testDataRelPath string) string {
	return internaltestutil.PrepareBundleDir(t, testDataRelPath)
}

// CreateBundleFromTestData creates a bundle from repository test data.
func CreateBundleFromTestData(t *testing.T, testDataRelPath, arch string) string {
	return internaltestutil.CreateBundleFromTestData(t, testDataRelPath, arch)
}

// CreateBundleFromTestDataWithDiagnostics creates a bundle and returns captured diagnostics.
func CreateBundleFromTestDataWithDiagnostics(t *testing.T, testDataRelPath, arch string) (string, string) {
	return internaltestutil.CreateBundleFromTestDataWithDiagnostics(t, testDataRelPath, arch)
}

// CreateBundleFromTestDataExpectError asserts that creating a test bundle fails.
func CreateBundleFromTestDataExpectError(t *testing.T, testDataRelPath, arch string) string {
	return internaltestutil.CreateBundleFromTestDataExpectError(t, testDataRelPath, arch)
}

// CreateBundleFromTestDataCLI creates a bundle through the configured CLI binary.
func CreateBundleFromTestDataCLI(t *testing.T, testDataRelPath, arch string) string {
	return internaltestutil.CreateBundleFromTestDataCLI(t, testDataRelPath, arch)
}

// CreateBundleFromTestDataCLIWithBinary creates a bundle through udsPath.
func CreateBundleFromTestDataCLIWithBinary(t *testing.T, udsPath, testDataRelPath, arch string) string {
	return internaltestutil.CreateBundleFromTestDataCLIWithBinary(t, udsPath, testDataRelPath, arch)
}

// CreateBundleFromTestDataCobra creates a bundle through Cobra command wiring.
func CreateBundleFromTestDataCobra(t *testing.T, testDataRelPath, arch string) string {
	return internaltestutil.CreateBundleFromTestDataCobra(t, testDataRelPath, arch)
}

// CopyDir recursively copies src into dst.
func CopyDir(src, dst string) error {
	return internaltestutil.CopyDir(src, dst)
}

// CheckDockerRunning skips the test when Docker is unavailable.
func CheckDockerRunning(t *testing.T, reason string) {
	internaltestutil.CheckDockerRunning(t, reason)
}

// DeleteK3dCluster deletes a test cluster if it exists.
func DeleteK3dCluster(t *testing.T, clusterName string) {
	internaltestutil.DeleteK3dCluster(t, clusterName)
}

// RunBundleDeploy runs bundle deploy through the supplied CLI binary.
func RunBundleDeploy(t *testing.T, udsPath, artifactPath string) {
	internaltestutil.RunBundleDeploy(t, udsPath, artifactPath)
}
