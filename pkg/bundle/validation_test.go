// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// validBaseConfig returns a fully-valid UDSBundleConfig used as the starting
// point for table-driven validation tests.
func validBaseConfig() *UDSBundleConfig {
	return &UDSBundleConfig{
		Global:  &GlobalOptions{LogLevel: "info"},
		Options: &ConfigOptions{Concurrency: 10},
	}
}

func TestValidateConfig_Structure(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *UDSBundleConfig
		wantErr string
	}{
		{
			name:    "nil config",
			cfg:     nil,
			wantErr: "config is required",
		},
		{
			name: "nil Global",
			cfg: &UDSBundleConfig{
				Options: &ConfigOptions{Concurrency: 10},
			},
			wantErr: "config.Global is required",
		},
		{
			name: "nil Options",
			cfg: &UDSBundleConfig{
				Global: &GlobalOptions{},
			},
			wantErr: "config.Options is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateConfig(tt.cfg)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestValidateConfig_Valid(t *testing.T) {
	require.NoError(t, ValidateConfig(validBaseConfig()))
}

func TestValidateLogLevel(t *testing.T) {
	tests := []struct {
		name    string
		level   string
		wantErr string
	}{
		{name: "empty allowed (defaults)", level: ""},
		{name: "debug accepted", level: "debug"},
		{name: "info accepted", level: "info"},
		{name: "warn accepted", level: "warn"},
		{name: "error accepted", level: "error"},
		{name: "invalid rejected", level: "loud", wantErr: "unknown log level"},
		{name: "typo rejected", level: "verbose", wantErr: "unknown log level"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validBaseConfig()
			cfg.Global.LogLevel = tt.level
			err := ValidateConfig(cfg)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestValidateConcurrency(t *testing.T) {
	tests := []struct {
		name        string
		concurrency int
		wantErr     string
	}{
		{name: "zero rejected", concurrency: 0, wantErr: "concurrency must be >= 1"},
		{name: "negative rejected", concurrency: -1, wantErr: "concurrency must be >= 1"},
		{name: "negative-large rejected", concurrency: -100, wantErr: "concurrency must be >= 1"},
		{name: "above max rejected", concurrency: MaxConcurrency + 1, wantErr: fmt.Sprintf("concurrency must be <= %d", MaxConcurrency)},
		{name: "way above max rejected", concurrency: 9999, wantErr: fmt.Sprintf("concurrency must be <= %d", MaxConcurrency)},
		{name: "lower bound accepted", concurrency: 1},
		{name: "default accepted", concurrency: 10},
		{name: "upper bound accepted", concurrency: MaxConcurrency},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validBaseConfig()
			cfg.Options.Concurrency = tt.concurrency
			err := ValidateConfig(cfg)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestValidateTmpDir(t *testing.T) {
	t.Run("empty allowed", func(t *testing.T) {
		cfg := validBaseConfig()
		cfg.Options.TmpDir = ""
		require.NoError(t, ValidateConfig(cfg))
	})

	t.Run("existing directory accepted", func(t *testing.T) {
		cfg := validBaseConfig()
		cfg.Options.TmpDir = t.TempDir()
		require.NoError(t, ValidateConfig(cfg))
	})

	t.Run("nonexistent path rejected", func(t *testing.T) {
		cfg := validBaseConfig()
		cfg.Options.TmpDir = "/nonexistent/path/tmp"
		err := ValidateConfig(cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "tmp_dir")
		assert.Contains(t, err.Error(), "directory does not exist")
	})

	t.Run("file rejected", func(t *testing.T) {
		f := filepath.Join(t.TempDir(), "afile")
		require.NoError(t, os.WriteFile(f, []byte("x"), 0o644))
		cfg := validBaseConfig()
		cfg.Options.TmpDir = f
		err := ValidateConfig(cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "tmp_dir")
		assert.Contains(t, err.Error(), "path is not a directory")
	})
}

func TestValidateConfig_StopsOnFirstError(t *testing.T) {
	// Confirms ValidateConfig short-circuits: a nil structure should be reported
	// before field-level errors are evaluated.
	err := ValidateConfig(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config is required")
	assert.NotContains(t, err.Error(), "concurrency")
}

func TestValidatePackageNames(t *testing.T) {
	packages := []Package{
		{Name: "core"},
		{Name: "nginx"},
		{Name: "podinfo"},
	}

	tests := []struct {
		name            string
		names           []string
		wantErrContains []string
	}{
		{
			name:  "nil names passes",
			names: nil,
		},
		{
			name:  "empty names passes",
			names: []string{},
		},
		{
			name:  "valid single name",
			names: []string{"nginx"},
		},
		{
			name:  "valid multiple names",
			names: []string{"core", "podinfo"},
		},
		{
			name:            "unknown package name includes available packages",
			names:           []string{"bogus"},
			wantErrContains: []string{"unknown packages", "available packages"},
		},
		{
			name:            "mix of valid and invalid",
			names:           []string{"core", "invalid"},
			wantErrContains: []string{"unknown packages", "available packages"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePackageNames(tt.names, packages)
			if len(tt.wantErrContains) == 0 {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			for _, s := range tt.wantErrContains {
				assert.Contains(t, err.Error(), s)
			}
		})
	}
}

func TestValidateDeployOptions(t *testing.T) {
	tests := []struct {
		name    string
		opts    DeployOptions
		wantErr string
	}{
		{name: "nil config rejected", opts: DeployOptions{}, wantErr: "config is required"},
		{name: "no bundle path or bundle rejected", opts: DeployOptions{Config: validBaseConfig()}, wantErr: "at least one of BundlePath or Bundle must be provided"},
		{name: "bundle path accepted", opts: DeployOptions{Config: validBaseConfig(), BundlePath: "/some/path"}},
		{name: "bundle accepted", opts: DeployOptions{Config: validBaseConfig(), Bundle: &UDSBundle{}}},
		{name: "both set accepted", opts: DeployOptions{Config: validBaseConfig(), BundlePath: "/some/path", Bundle: &UDSBundle{}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.opts.Validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}

func TestValidateDeployPackageOptions(t *testing.T) {
	tests := []struct {
		name    string
		opts    DeployPackageOptions
		wantErr string
	}{
		{name: "nil config rejected", opts: DeployPackageOptions{}, wantErr: "config is required"},
		{name: "empty BundleDir rejected", opts: DeployPackageOptions{Config: validBaseConfig()}, wantErr: "BundleDir is required"},
		{name: "valid opts accepted", opts: DeployPackageOptions{Config: validBaseConfig(), BundleDir: "/some/dir"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.opts.Validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}

func TestValidateRemoveOptions(t *testing.T) {
	tests := []struct {
		name    string
		opts    RemoveOptions
		wantErr string
	}{
		{name: "nil config rejected", opts: RemoveOptions{}, wantErr: "config is required"},
		{name: "no bundle path or bundle rejected", opts: RemoveOptions{Config: validBaseConfig()}, wantErr: "at least one of BundlePath or Bundle must be provided"},
		{name: "bundle path accepted", opts: RemoveOptions{Config: validBaseConfig(), BundlePath: "/some/path"}},
		{name: "bundle accepted", opts: RemoveOptions{Config: validBaseConfig(), Bundle: &UDSBundle{}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.opts.Validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}

func TestValidateRemovePackageOptions(t *testing.T) {
	tests := []struct {
		name    string
		opts    RemovePackageOptions
		wantErr string
	}{
		{name: "nil config rejected", opts: RemovePackageOptions{}, wantErr: "config is required"},
		{name: "valid config accepted", opts: RemovePackageOptions{Config: validBaseConfig()}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.opts.Validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}

func TestValidateCreateOptions(t *testing.T) {
	tests := []struct {
		name    string
		opts    CreateOptions
		wantErr string
	}{
		{name: "nil config rejected", opts: CreateOptions{}, wantErr: "config is required"},
		{name: "empty BundleFile rejected", opts: CreateOptions{Config: validBaseConfig()}, wantErr: "BundleFile is required"},
		{name: "valid opts accepted", opts: CreateOptions{Config: validBaseConfig(), BundleFile: "/some/bundle.uds.hcl"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.opts.Validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}

func TestValidateCreatePackageOptions(t *testing.T) {
	tests := []struct {
		name    string
		opts    CreatePackageOptions
		wantErr string
	}{
		{name: "nil config rejected", opts: CreatePackageOptions{}, wantErr: "config is required"},
		{name: "empty BlobDir rejected", opts: CreatePackageOptions{Config: validBaseConfig(), BundleDir: "/some/dir"}, wantErr: "BlobDir is required"},
		{name: "empty BundleDir rejected", opts: CreatePackageOptions{Config: validBaseConfig(), BlobDir: "/some/blobs"}, wantErr: "BundleDir is required"},
		{name: "valid opts accepted", opts: CreatePackageOptions{Config: validBaseConfig(), BlobDir: "/some/blobs", BundleDir: "/some/dir"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.opts.Validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}

func TestValidatePullOptions(t *testing.T) {
	tests := []struct {
		name    string
		opts    PullOptions
		wantErr string
	}{
		{name: "nil config rejected", opts: PullOptions{}, wantErr: "config is required"},
		{name: "valid config accepted", opts: PullOptions{Config: validBaseConfig()}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.opts.Validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}

func TestValidatePushOptions(t *testing.T) {
	tests := []struct {
		name    string
		opts    PushOptions
		wantErr string
	}{
		{name: "nil config rejected", opts: PushOptions{}, wantErr: "config is required"},
		{name: "valid config accepted", opts: PushOptions{Config: validBaseConfig()}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.opts.Validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}

func TestValidateReconfigureOptions(t *testing.T) {
	tests := []struct {
		name    string
		opts    ReconfigureOptions
		wantErr string
	}{
		{name: "empty source rejected", opts: ReconfigureOptions{DefaultsFile: "defaults.uds.hcl", Suffix: "-v2"}, wantErr: "source is required"},
		{name: "empty defaults file rejected", opts: ReconfigureOptions{Source: "bundle.tar.zst", Suffix: "-v2"}, wantErr: "defaults file is required"},
		{name: "invalid suffix no leading dash", opts: ReconfigureOptions{Source: "bundle.tar.zst", DefaultsFile: "defaults.uds.hcl", Suffix: "nosuffix"}, wantErr: "invalid suffix"},
		{name: "empty suffix rejected", opts: ReconfigureOptions{Source: "bundle.tar.zst", DefaultsFile: "defaults.uds.hcl", Suffix: ""}, wantErr: "invalid suffix"},
		{name: "valid opts accepted", opts: ReconfigureOptions{Source: "bundle.tar.zst", DefaultsFile: "defaults.uds.hcl", Suffix: "-v2"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.opts.Validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}

// TestValidateRemovalSafety covers the public dependency-safety check the CLI
// invokes from RemoveOptions.Validate(). The fixture below mirrors a typical
// UDS Core layering:
//
//	core         (no deps)
//	nginx        depends on core
//	podinfo      depends on nginx, core
//	standalone   (no deps, unrelated)
func TestValidateRemovalSafety(t *testing.T) {
	b := bundleWith(
		pkg("core"),
		pkg("nginx", "core"),
		pkg("podinfo", "nginx", "core"),
		pkg("standalone"),
	)

	tests := []struct {
		name    string
		remove  []string
		wantErr string
		// wantContains lets us assert specific blocker phrasing without
		// pinning the entire (multiline) error string.
		wantContains []string
	}{
		{
			name:   "empty filter is safe (full bundle removal)",
			remove: nil,
		},
		{
			name:   "leaf package is safe",
			remove: []string{"podinfo"},
		},
		{
			name:   "isolated package is safe",
			remove: []string{"standalone"},
		},
		{
			name:   "removing root with dependents is blocked",
			remove: []string{"core"},
			wantContains: []string{
				"cannot remove package(s) with bundle dependents",
				`"core" is required by: nginx, podinfo`,
			},
		},
		{
			name:         "removing middle blocks via upper level",
			remove:       []string{"nginx"},
			wantContains: []string{`"nginx" is required by: podinfo`},
		},
		{
			name:   "removing all dependents along with root is safe",
			remove: []string{"core", "nginx", "podinfo"},
		},
		{
			name:   "partial chain still has a blocker",
			remove: []string{"core", "nginx"},
			wantContains: []string{
				`"core" is required by: podinfo`,
				`"nginx" is required by: podinfo`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRemovalSafety(t.Context(), iostreams.IOStreams{}, b, tt.remove)
			if len(tt.wantContains) == 0 && tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			for _, s := range tt.wantContains {
				assert.Contains(t, err.Error(), s)
			}
		})
	}
}

// TestValidateDeploySafety covers the deploy dependency-safety check.
// The fixture mirrors a typical UDS Core layering:
//
//	core         (no deps)
//	nginx        depends on core
//	podinfo      depends on nginx, core
//	standalone   (no deps, unrelated)
func TestValidateDeploySafety(t *testing.T) {
	b := bundleWith(
		pkg("core"),
		pkg("nginx", "core"),
		pkg("podinfo", "nginx", "core"),
		pkg("standalone"),
	)

	tests := []struct {
		name            string
		deploy          []string
		wantErrContains []string
	}{
		{
			name:   "empty filter is safe (full bundle deploy)",
			deploy: nil,
		},
		{
			name:   "root package is safe (no deps)",
			deploy: []string{"core"},
		},
		{
			name:   "isolated package is safe",
			deploy: []string{"standalone"},
		},
		{
			name:   "deploying dep and dependent is safe",
			deploy: []string{"core", "nginx"},
		},
		{
			name:   "deploying full chain is safe",
			deploy: []string{"core", "nginx", "podinfo", "standalone"},
		},
		{
			name:   "deploying leaf whose all deps are selected is safe",
			deploy: []string{"core", "nginx", "podinfo"},
		},
		{
			name:   "deploying dependent without its dep is blocked",
			deploy: []string{"nginx"},
			wantErrContains: []string{
				"cannot deploy package(s) with unselected dependencies",
				`"nginx" requires: core`,
			},
		},
		{
			name:   "deploying podinfo without its deps is blocked",
			deploy: []string{"podinfo"},
			wantErrContains: []string{
				"cannot deploy package(s) with unselected dependencies",
				`"podinfo" requires: core, nginx`,
			},
		},
		{
			name:   "deploying podinfo with nginx but without core is blocked",
			deploy: []string{"nginx", "podinfo"},
			wantErrContains: []string{
				`"nginx" requires: core`,
				`"podinfo" requires: core`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDeploySafety(t.Context(), iostreams.IOStreams{}, b, tt.deploy)
			if len(tt.wantErrContains) == 0 {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			for _, s := range tt.wantErrContains {
				assert.Contains(t, err.Error(), s)
			}
		})
	}
}

// TestFormatDependencyError verifies sort stability and message shape. The
// formatter underpins ValidateRemovalSafety's and ValidateDeploySafety's
// user-facing output.
func TestFormatDependencyError(t *testing.T) {
	blockers := map[string][]string{
		"core":  {"nginx", "podinfo"},
		"nginx": {"podinfo"},
	}
	err := formatDependencyError("cannot remove package(s) with bundle dependents", "is required by", blockers)
	require.Error(t, err)

	msg := err.Error()
	assert.Contains(t, msg, "cannot remove package(s) with bundle dependents:")
	assert.Contains(t, msg, `"core" is required by: nginx, podinfo`)
	assert.Contains(t, msg, `"nginx" is required by: podinfo`)
	assert.NotContains(t, msg, "--force")
	// "core" line should appear before "nginx" line (sorted).
	assert.Less(t, strings.Index(msg, `"core"`), strings.Index(msg, `"nginx"`))

	// The error is a typed *DependencyViolationError, so library consumers can
	// inspect the structured violations via errors.As instead of parsing the message.
	var dve *DependencyViolationError
	require.ErrorAs(t, err, &dve)
	assert.Equal(t, "cannot remove package(s) with bundle dependents", dve.Header)
	assert.Equal(t, "is required by", dve.Relation)
	assert.Equal(t, blockers, dve.Violations)
}
