// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package utils

import (
	"testing"

	"github.com/stretchr/testify/require"
	zarfUtils "github.com/zarf-dev/zarf/src/pkg/utils"
)

func Test_IsRegistryURL(t *testing.T) {
	tests := []struct {
		name        string
		description string
		output      string
		wantResult  bool
	}{
		{
			name:        "HasScheme",
			description: "Output has a scheme ://",
			output:      "oci://ghcr.io/defenseunicorns/dev",
			wantResult:  true,
		},
		{
			name:        "HasDomain",
			description: "Output has no scheme but has domain",
			output:      "ghcr.io/defenseunicorns/dev",
			wantResult:  true,
		},
		{
			name:        "HasMultiDomain",
			description: "Output has no scheme but has domain in form of example.example.com",
			output:      "registry.example.io/defenseunicorns/dev",
			wantResult:  true,
		},
		{
			name:        "HasDomainAndNoPath",
			description: "Output has no scheme but has domain in form of example.example.com",
			output:      "registry.example.io",
			wantResult:  true,
		},
		{
			name:        "HasPort",
			description: "Output has no scheme or domain (with .) but has port",
			output:      "localhost:31999",
			wantResult:  true,
		},
		{
			name:        "HasPortWithTrailingSlash",
			description: "Output has no scheme or domain (with .) but has port with trailing /",
			output:      "localhost:31999/path",
			wantResult:  true,
		},
		{
			name:        "IsLocalPath",
			description: "Output is to local path",
			output:      "local/path",
			wantResult:  false,
		},
		{
			name:        "IsCurrentDirectory",
			description: "Output is current directory",
			output:      ".",
			wantResult:  false,
		},
		{
			name:        "IsHiddenDirectory",
			description: "Output is a hidden directory",
			output:      ".dev",
			wantResult:  false,
		},
		{
			name:        "IsHiddenDirectoryWithSlashPrefix",
			description: "Output is a hidden directory nested in path",
			output:      "/pathto/.dev",
			wantResult:  false,
		},
		{
			name:        "HasRareDotInLocalDirectoryPath",
			description: "Output has a rare dot in local directory path",
			output:      "/pathto/test.dev/",
			wantResult:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsRegistryURL(tt.output)
			require.Equal(t, tt.wantResult, result, tt.description)
		})
	}
}

func TestVerifyBlobOptionsFromKey(t *testing.T) {
	tests := []struct {
		name    string
		keyPath string
		wantNil bool
	}{
		{name: "empty key path returns nil", keyPath: "", wantNil: true},
		{name: "non-empty key path sets Key", keyPath: "/path/to/key.pub"},
		{name: "any non-empty string sets Key", keyPath: "mykey"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := VerifyBlobOptionsFromKey(tt.keyPath)
			if tt.wantNil {
				require.Nil(t, result)
				return
			}
			require.NotNil(t, result)
			require.Equal(t, tt.keyPath, result.Key)

			// Verify that other fields are set to their default values
			defaults := zarfUtils.DefaultVerifyBlobOptions()
			defaults.Key = tt.keyPath
			require.Equal(t, defaults, *result)
		})
	}
}

func TestResolveVerifyBlobOptions(t *testing.T) {
	customOpts := zarfUtils.VerifyBlobOptions{}
	customOpts.Key = "/path/to/key.pub"

	tests := []struct {
		name string
		opts *zarfUtils.VerifyBlobOptions
		want zarfUtils.VerifyBlobOptions
	}{
		{
			name: "nil input returns defaults",
			opts: nil,
			want: zarfUtils.DefaultVerifyBlobOptions(),
		},
		{
			name: "non-nil input returned as-is",
			opts: &customOpts,
			want: customOpts,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolveVerifyBlobOptions(tt.opts)
			require.Equal(t, tt.want, result)
		})
	}
}
