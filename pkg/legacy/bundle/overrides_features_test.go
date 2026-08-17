// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/legacy/types"
)

func TestFeaturesIsNotABundleOverride(t *testing.T) {
	t.Setenv("UDS_FEATURES", "NextMode=false")
	bundle := &Bundle{cfg: &types.BundleConfig{}}
	variables, _ := bundle.loadVariables(types.Package{Name: "package"}, nil)
	if _, found := variables["FEATURES"]; found {
		t.Fatal("UDS_FEATURES must not be imported as a Legacy bundle override")
	}
}
