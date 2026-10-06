// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package zarf

import (
	"fmt"
	"sync"

	bundleinternal "github.com/defenseunicorns/uds-cli/internal/bundle"
	"github.com/hashicorp/hcl/v2"
	zarfconfig "github.com/zarf-dev/zarf/src/config"
)

var zarfTempDir tempDirConfig

type tempDirConfig struct {
	sync.Mutex
	value string
	set   bool
}

// UDSBundleConfig is the private resolved deployment configuration.
type UDSBundleConfig struct {
	Options   *bundleinternal.ConfigOptions `hcl:"options,block"`
	Variables bundleinternal.Variables
	Remain    hcl.Body `hcl:",remain"`
}

// WithTempDir runs an operation while Zarf's process-global temporary directory
// matches the process configuration. The first call configures Zarf; later
// calls must use the same directory.
func WithTempDir[T any](tmpDir string, run func() (T, error)) (T, error) {
	zarfTempDir.Lock()
	if !zarfTempDir.set {
		zarfTempDir.value = tmpDir
		zarfTempDir.set = true
		zarfconfig.CommonOptions.TempDirectory = tmpDir
	}
	configured := zarfTempDir.value
	zarfTempDir.Unlock()

	if configured != tmpDir {
		var zero T
		return zero, fmt.Errorf("Zarf temporary directory is already configured as %q, cannot change it to %q", configured, tmpDir)
	}

	return run()
}

func withTempDir(tmpDir string, run func() error) error {
	_, err := WithTempDir(tmpDir, func() (struct{}, error) {
		return struct{}{}, run()
	})
	return err
}
