// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsOCIRef(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		// Explicit scheme — always true
		{"oci://ghcr.io/org/bundle:v1", true},
		{"https://ghcr.io/org/bundle:v1", true},
		{"docker://ghcr.io/org/bundle:v1", true},

		// Bare registry refs — true (domain + slash, no file cues)
		{"ghcr.io/org/bundle:v1", true},
		{"ghcr.io/org/bundle", true},
		{"registry.example.com/org/pkg:tag", true},

		// File paths — false
		{"bundle.uds.hcl", false},
		{"./bundle.uds.hcl", false},
		{"../bundle.uds.hcl", false},
		{"/absolute/path/bundle.uds.hcl", false},
		{"some/dir/bundle.uds.hcl", false},
		{"path/to/package.tar.zst", false},
		{"path/to/package.yaml", false},
		{"path/to/package.yml", false},

		// Ambiguous but correctly classified
		{"plain-name", false},               // no dot, no slash → not OCI
		{"name with spaces/repo:tag", false}, // spaces → not OCI
		{"local-dir", false},                // no dot → not OCI
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, IsOCIRef(tt.input))
		})
	}
}
