package cmd

import (
	"testing"

	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/stretchr/testify/assert"
)

func TestValidateDevDeployFlags(t *testing.T) {
	testCases := []struct {
		name          string
		localBundle   bool
		DevDeployOpts types.BundleDevDeployOptions
		expectError   bool
	}{
		{
			name:        "Local bundle with --ref flag",
			localBundle: true,
			DevDeployOpts: types.BundleDevDeployOptions{
				Ref: map[string]string{"some-key": "some-ref"},
			},
			expectError: true,
		},
		{
			name:        "Remote bundle with --ref flag",
			localBundle: false,
			DevDeployOpts: types.BundleDevDeployOptions{
				Ref: map[string]string{"some-key": "some-ref"},
			},
			expectError: false,
		},
		{
			name:        "Local bundle with --flavor flag",
			localBundle: true,
			DevDeployOpts: types.BundleDevDeployOptions{
				Flavor: map[string]string{"some-key": "some-flavor"},
			},
			expectError: false,
		},
		{
			name:        "Remote bundle with --flavor flag",
			localBundle: false,
			DevDeployOpts: types.BundleDevDeployOptions{
				Flavor: map[string]string{"some-key": "some-flavor"},
			},
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			bundleCfg.DevDeployOpts = tc.DevDeployOpts

			err := validateDevDeployFlags(tc.localBundle)
			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestIsLocalBundle(t *testing.T) {
	testCases := []struct {
		name string
		src  string
		want bool
	}{
		{
			name: "Test with directory",
			src:  "../cmd/",
			want: true,
		},
		{
			name: "Test with .tar.zst file",
			src:  "/path/to/file.tar.zst",
			want: true,
		},
		{
			name: "Test with other file",
			src:  "/path/to/file.txt",
			want: false,
		},
		{
			name: "Test with registry",
			src:  "ghcr.io/defenseunicorns/uds-cli/nginx",
			want: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := isLocalBundle(tc.src)
			assert.Equal(t, tc.want, got)
		})
	}
}
