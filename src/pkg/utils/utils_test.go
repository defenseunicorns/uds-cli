package utils

import (
	"testing"

	"github.com/stretchr/testify/require"
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

func Test_formatBundleName(t *testing.T) {
	tests := map[string]struct {
		src       string
		expected  string
		shouldErr bool // not used yet
	}{
		"valid": {
			src:      "wordpress",
			expected: "wordpress",
		},
		"valid mixed caps": {
			src:      "woRdprEsS",
			expected: "wordpress",
		},
		"valid spaces": {
			src:      "woRdprEsS version 1",
			expected: "wordpress-version-1",
		},
		"valid leading spaces": {
			// leading spaces trimmed but the space after the first "-" is
			// converted to "-"
			src:      "   - woRdprEsS version 1    ",
			expected: "--wordpress-version-1",
		},
		"invalid chars": {
			src:       "woRdprEsS*version 1",
			shouldErr: true,
		},
		"more invalid chars": {
			src:       "&*^woRdprEsS*version 1",
			shouldErr: true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			result, err := FormatBundleName(tt.src)
			if tt.shouldErr && err != nil {
				// expected error
				return
			}
			if tt.shouldErr && err == nil {
				t.Fatalf("expected error but got none")
			}
			if !tt.shouldErr && err != nil {
				t.Fatalf("got error but expected none: %s", err)
			}
			require.Equal(t, tt.expected, result)
		})
	}
}
