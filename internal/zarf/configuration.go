// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package zarf

import (
	bundleinternal "github.com/defenseunicorns/uds-cli/internal/bundle"
	"github.com/hashicorp/hcl/v2"
	zarfconfig "github.com/zarf-dev/zarf/src/config"
)

// UDSBundleConfig is the private resolved deployment configuration.
type UDSBundleConfig struct {
	Options   *bundleinternal.ConfigOptions `hcl:"options,block"`
	Variables bundleinternal.Variables
	Remain    hcl.Body `hcl:",remain"`
}

// ConfigureTempDir bridges a resolved temp directory into Zarf APIs that still
// read process-global configuration.
func ConfigureTempDir(tmpDir string) {
	zarfconfig.CommonOptions.TempDirectory = tmpDir
}
