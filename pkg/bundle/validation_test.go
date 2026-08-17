// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"strings"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// validValidationConfig returns baseline valid public configuration for tests.
func validValidationConfig() *UDSBundleConfig {
	return &UDSBundleConfig{
		Global:  &GlobalOptions{LogLevel: "info"},
		Options: &ConfigOptions{Concurrency: 10},
	}
}

func TestValidateOperationOptions(t *testing.T) {
	tests := []struct {
		name     string
		validate func() error
		wantErr  string
	}{
		{name: "deploy requires config", validate: func() error { return (DeployOptions{}).Validate() }, wantErr: "config is required"},
		{name: "deploy requires bundle source", validate: func() error { return (DeployOptions{Config: validValidationConfig()}).Validate() }, wantErr: "at least one of BundlePath or Bundle must be provided"},
		{name: "deploy accepts bundle path", validate: func() error {
			return (DeployOptions{Config: validValidationConfig(), BundlePath: "/bundle"}).Validate()
		}},
		{name: "deploy accepts bundle", validate: func() error {
			return (DeployOptions{Config: validValidationConfig(), Bundle: &spec.UDSBundle{}}).Validate()
		}},
		{name: "deploy package requires config", validate: func() error { return (DeployPackageOptions{}).Validate() }, wantErr: "config is required"},
		{name: "deploy package requires bundle directory", validate: func() error {
			return (DeployPackageOptions{Config: validValidationConfig()}).Validate()
		}, wantErr: "BundleDir is required"},
		{name: "deploy package accepts valid options", validate: func() error {
			return (DeployPackageOptions{Config: validValidationConfig(), BundleDir: "/bundle"}).Validate()
		}},
		{name: "load accepts options", validate: func() error { return (LoadOptions{}).Validate() }},
		{name: "remove requires config", validate: func() error { return (RemoveOptions{}).Validate() }, wantErr: "config is required"},
		{name: "remove requires bundle source", validate: func() error { return (RemoveOptions{Config: validValidationConfig()}).Validate() }, wantErr: "at least one of BundlePath or Bundle must be provided"},
		{name: "remove accepts bundle path", validate: func() error {
			return (RemoveOptions{Config: validValidationConfig(), BundlePath: "/bundle"}).Validate()
		}},
		{name: "remove accepts bundle", validate: func() error {
			return (RemoveOptions{Config: validValidationConfig(), Bundle: &spec.UDSBundle{}}).Validate()
		}},
		{name: "remove package requires config", validate: func() error { return (RemovePackageOptions{}).Validate() }, wantErr: "config is required"},
		{name: "remove package accepts config", validate: func() error {
			return (RemovePackageOptions{Config: validValidationConfig()}).Validate()
		}},
		{name: "create requires config", validate: func() error { return (CreateOptions{}).Validate() }, wantErr: "config is required"},
		{name: "create requires bundle file", validate: func() error { return (CreateOptions{Config: validValidationConfig()}).Validate() }, wantErr: "BundleFile is required"},
		{name: "create accepts bundle file", validate: func() error {
			return (CreateOptions{Config: validValidationConfig(), BundleFile: "bundle.uds.hcl", Signing: SigningOptions{Mode: SigningModeUnsigned}}).Validate()
		}},
		{name: "pull requires config", validate: func() error { return (PullOptions{}).Validate() }, wantErr: "config is required"},
		{name: "pull accepts config", validate: func() error { return (PullOptions{Config: validValidationConfig()}).Validate() }},
		{name: "archive deploy source requires verification policy", validate: func() error {
			return (PrepareDeploySourceOptions{Path: "bundle.tar.zst", Config: validValidationConfig()}).Validate()
		}, wantErr: "exactly one"},
		{name: "archive deploy source accepts explicit bypass", validate: func() error {
			return (PrepareDeploySourceOptions{Path: "bundle.tar.zst", Config: validValidationConfig(), SkipSignatureVerification: true}).Validate()
		}},
		{name: "push requires config", validate: func() error { return (PushOptions{}).Validate() }, wantErr: "config is required"},
		{name: "push accepts config", validate: func() error { return (PushOptions{Config: validValidationConfig()}).Validate() }},
		{name: "reconfigure requires source", validate: func() error { return (ReconfigureOptions{DefaultsFile: "defaults.uds.hcl", Suffix: "-v2"}).Validate() }, wantErr: "source is required"},
		{name: "reconfigure requires defaults", validate: func() error { return (ReconfigureOptions{Source: "bundle.tar.zst", Suffix: "-v2"}).Validate() }, wantErr: "defaults file is required"},
		{name: "reconfigure rejects invalid suffix", validate: func() error {
			return (ReconfigureOptions{Source: "bundle.tar.zst", DefaultsFile: "defaults.uds.hcl", Suffix: "v2"}).Validate()
		}, wantErr: "invalid suffix"},
		{name: "reconfigure accepts valid options", validate: func() error {
			return (ReconfigureOptions{Source: "bundle.tar.zst", DefaultsFile: "defaults.uds.hcl", Suffix: "-v2", Signing: SigningOptions{Mode: SigningModeUnsigned}, SkipSignatureVerification: true}).Validate()
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestSigningValidation(t *testing.T) {
	tests := []struct {
		name     string
		validate func() error
		wantErr  string
	}{
		{name: "key requires key", validate: func() error { return (SigningOptions{Mode: SigningModeKey}).Validate() }, wantErr: "signing key is required"},
		{name: "keyless accepted", validate: func() error { return (SigningOptions{Mode: SigningModeKeyless}).Validate() }},
		{name: "keyless rejects signing key", validate: func() error {
			return (SigningOptions{Mode: SigningModeKeyless, Key: "key"}).Validate()
		}, wantErr: "signing key cannot be combined with keyless signing"},
		{name: "unsigned accepted", validate: func() error { return (SigningOptions{Mode: SigningModeUnsigned}).Validate() }},
		{name: "policy requires one method", validate: func() error { return (VerificationPolicy{}).Validate() }, wantErr: "exactly one"},
		{name: "policy rejects both methods", validate: func() error {
			return (VerificationPolicy{PublicKey: "key", Keyless: &KeylessVerification{}}).Validate()
		}, wantErr: "exactly one"},
		{name: "keyless requires identity", validate: func() error {
			return (VerificationPolicy{Keyless: &KeylessVerification{CertificateOIDCIssuer: "issuer"}}).Validate()
		}, wantErr: "certificate identity"},
		{name: "keyless requires issuer", validate: func() error {
			return (VerificationPolicy{Keyless: &KeylessVerification{CertificateIdentity: "identity"}}).Validate()
		}, wantErr: "certificate OIDC issuer"},
		{name: "keyless accepted", validate: func() error {
			return (VerificationPolicy{Keyless: &KeylessVerification{CertificateIdentity: "identity", CertificateOIDCIssuer: "issuer"}}).Validate()
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestPullOptions_ValidateBundleVerification(t *testing.T) {
	tests := []struct {
		name    string
		opts    PullOptions
		wantErr string
	}{
		{name: "policy required", wantErr: "exactly one"},
		{name: "key policy accepted", opts: PullOptions{Verification: VerificationPolicy{PublicKey: "key"}}},
		{name: "explicit bypass accepted", opts: PullOptions{SkipSignatureVerification: true}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.opts.validateBundleVerification()
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

// validationBundle returns a bundle with representative dependency relationships.
func validationBundle() *spec.UDSBundle {
	packageWithDependencies := func(name string, dependencies ...string) spec.Package {
		refs := make([]spec.PackageRef, len(dependencies))
		for i, dependency := range dependencies {
			refs[i] = spec.PackageRef{Name: dependency}
		}
		return spec.Package{Name: name, Source: "oci://example.com/" + name + ":v1", DependsOn: refs}
	}

	return &spec.UDSBundle{Packages: []spec.Package{
		packageWithDependencies("core"),
		packageWithDependencies("nginx", "core"),
		packageWithDependencies("podinfo", "nginx", "core"),
		packageWithDependencies("standalone"),
	}}
}

func TestValidateRemovalSafety(t *testing.T) {
	b := validationBundle()
	tests := []struct {
		name         string
		remove       []string
		wantContains []string
	}{
		{name: "full bundle removal is safe"},
		{name: "leaf package is safe", remove: []string{"podinfo"}},
		{name: "isolated package is safe", remove: []string{"standalone"}},
		{name: "root with dependents is blocked", remove: []string{"core"}, wantContains: []string{"cannot remove package(s) with bundle dependents", `"core" is required by: nginx, podinfo`}},
		{name: "middle with dependent is blocked", remove: []string{"nginx"}, wantContains: []string{`"nginx" is required by: podinfo`}},
		{name: "complete dependency chain is safe", remove: []string{"core", "nginx", "podinfo"}},
		{name: "partial chain is blocked", remove: []string{"core", "nginx"}, wantContains: []string{`"core" is required by: podinfo`, `"nginx" is required by: podinfo`}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRemovalSafety(t.Context(), iostreams.IOStreams{}, b, tt.remove)
			if len(tt.wantContains) == 0 {
				require.NoError(t, err)
				return
			}
			for _, want := range tt.wantContains {
				require.ErrorContains(t, err, want)
			}
		})
	}
}

func TestValidateDeploySafety(t *testing.T) {
	b := validationBundle()
	tests := []struct {
		name         string
		deploy       []string
		wantContains []string
	}{
		{name: "full bundle deploy is safe"},
		{name: "root package is safe", deploy: []string{"core"}},
		{name: "isolated package is safe", deploy: []string{"standalone"}},
		{name: "dependency and dependent are safe", deploy: []string{"core", "nginx"}},
		{name: "full dependency chain is safe", deploy: []string{"core", "nginx", "podinfo"}},
		{name: "missing root is blocked", deploy: []string{"nginx"}, wantContains: []string{"cannot deploy package(s) with unselected dependencies", `"nginx" requires: core`}},
		{name: "missing all dependencies is blocked", deploy: []string{"podinfo"}, wantContains: []string{`"podinfo" requires: core, nginx`}},
		{name: "shared missing dependency is reported", deploy: []string{"nginx", "podinfo"}, wantContains: []string{`"nginx" requires: core`, `"podinfo" requires: core`}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDeploySafety(t.Context(), iostreams.IOStreams{}, b, tt.deploy)
			if len(tt.wantContains) == 0 {
				require.NoError(t, err)
				return
			}
			for _, want := range tt.wantContains {
				require.ErrorContains(t, err, want)
			}
		})
	}
}

func TestFormatDependencyError(t *testing.T) {
	violations := map[string][]string{
		"core":  {"nginx", "podinfo"},
		"nginx": {"podinfo"},
	}
	err := formatDependencyError("cannot remove package(s) with bundle dependents", "is required by", violations)
	require.Error(t, err)

	msg := err.Error()
	assert.Contains(t, msg, "cannot remove package(s) with bundle dependents:")
	assert.Contains(t, msg, `"core" is required by: nginx, podinfo`)
	assert.Contains(t, msg, `"nginx" is required by: podinfo`)
	assert.NotContains(t, msg, "--force")
	assert.Less(t, strings.Index(msg, `"core"`), strings.Index(msg, `"nginx"`))

	var violationError *DependencyViolationError
	require.ErrorAs(t, err, &violationError)
	assert.Equal(t, "cannot remove package(s) with bundle dependents", violationError.Header)
	assert.Equal(t, "is required by", violationError.Relation)
	assert.Equal(t, violations, violationError.Violations)
}
