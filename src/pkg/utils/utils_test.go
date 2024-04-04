package utils

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_IsRegistryURL(t *testing.T) {
	type args struct {
		output string
	}
	tests := []struct {
		name        string
		description string
		args        args
		wantResult  bool
	}{
		{
			name:        "HasScheme",
			description: "Output has a scheme ://",
			args:        args{output: "oci://ghcr.io/defenseunicorns/dev"},
			wantResult:  true,
		},
		{
			name:        "HasDomain",
			description: "Output has no scheme but has domain",
			args:        args{output: "ghcr.io/defenseunicorns/dev"},
			wantResult:  true,
		},
		{
			name:        "HasMultiDomain",
			description: "Output has no scheme but has domain in form of example.example.com",
			args:        args{output: "registry.example.io/defenseunicorns/dev"},
			wantResult:  true,
		},
		{
			name:        "HasDomainAndNoPath",
			description: "Output has no scheme but has domain in form of example.example.com",
			args:        args{output: "registry.example.io"},
			wantResult:  true,
		},
		{
			name:        "HasPort",
			description: "Output has no scheme or domain (with .) but has port",
			args:        args{output: "localhost:31999"},
			wantResult:  true,
		},
		{
			name:        "HasPortWithTrailingSlash",
			description: "Output has no scheme or domain (with .) but has port with trailing /",
			args:        args{output: "localhost:31999/path"},
			wantResult:  true,
		},
		{
			name:        "IsLocalPath",
			description: "Output is to local path",
			args:        args{output: "local/path"},
			wantResult:  false,
		},
		{
			name:        "IsCurrentDirectory",
			description: "Output is current directory",
			args:        args{output: "."},
			wantResult:  false,
		},
		{
			name:        "IsHiddenDirectory",
			description: "Output is a hidden directory",
			args:        args{output: ".dev"},
			wantResult:  false,
		},
		{
			name:        "IsHiddenDirectoryWithSlashPrefix",
			description: "Output is a hidden directory nested in path",
			args:        args{output: "/pathto/.dev"},
			wantResult:  false,
		},
		{
			name:        "HasRareDotInLocalDirectoryPath",
			description: "Output is a hidden directory nested in path",
			args:        args{output: "/pathto/test.dev/"},
			wantResult:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actualResult := IsRegistryURL(tt.args.output)
			require.Equal(t, tt.wantResult, actualResult)
		})
	}
}
