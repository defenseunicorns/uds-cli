// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package testutil

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestBootstrapCommandsCreateThenDeployBundle(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	bootstrap := "/fixtures/00-cluster-bootstrap"
	got := bootstrapCommands(workspace, bootstrap)
	want := [][]string{
		{"create", bootstrap, "--confirm", "--output", workspace},
		{"deploy", filepath.Join(workspace, "uds-bundle-test-cluster-bootstrap-*.tar.zst"), "--confirm", "--no-progress"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("bootstrap commands = %#v, want %#v", got, want)
	}
}
