// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package oci

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsOCIReference(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{name: "explicit scheme", input: "oci://registry.example/bundle:v1", want: true},
		{name: "explicit scheme with tar suffix", input: "oci://registry.example/bundle:v1.tar.zst", want: true},
		{name: "localhost with port", input: "localhost:5000/bundle:v1", want: true},
		{name: "localhost without port", input: "localhost/bundle:v1", want: true},
		{name: "scheme-less registry with tar suffix", input: "registry.example/bundle:v1.tar.zst", want: true},
		{name: "local archive", input: "./bundle.tar.zst", want: false},
		{name: "HCL file", input: "bundle.uds.hcl", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsOCIReference(tt.input))
		})
	}
}
