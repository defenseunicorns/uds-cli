// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package monitor

import "testing"

func TestNewCommandIsRepeatable(t *testing.T) {
	first := NewCommand()
	second := NewCommand()
	if first == second {
		t.Fatal("NewCommand reused a command tree")
	}
	if err := first.PersistentFlags().Set("namespace", "first"); err != nil {
		t.Fatal(err)
	}
	if got := second.PersistentFlags().Lookup("namespace").Value.String(); got != "" {
		t.Fatalf("second namespace = %q, want empty", got)
	}
}
