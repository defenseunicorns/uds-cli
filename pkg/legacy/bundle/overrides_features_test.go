// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/legacy/types"
)

func TestUDSFeaturesIsABundleOverride(t *testing.T) {
	t.Setenv("UDS_FEATURES", "enabled")
	bundle := &Bundle{cfg: &types.BundleConfig{}}
	variables, _ := bundle.loadVariables(types.Package{Name: "package"}, nil)
	if got := variables["FEATURES"]; got != "enabled" {
		t.Fatalf("FEATURES = %q, want %q", got, "enabled")
	}
}
