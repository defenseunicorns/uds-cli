// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/legacy/types"
)

func TestFeatureGatesIsNotABundleOverride(t *testing.T) {
	t.Setenv("UDS_FEATURE_GATES", "NextMode=false")
	bundle := &Bundle{cfg: &types.BundleConfig{}}
	variables, _ := bundle.loadVariables(types.Package{Name: "package"}, nil)
	if _, found := variables["FEATURE_GATES"]; found {
		t.Fatal("UDS_FEATURE_GATES must not be imported as a Legacy bundle override")
	}
}
