// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build library

package bundle_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigurationReachesPublicPackageHooks(t *testing.T) {
	source := preparedLibrarySource(t)
	source.DefaultsPath = ""
	config := libraryFixtureConfig(t)
	config.Options.LogLevel = "debug"
	config.Options.PlainHTTP = true
	config.Options.SkipTLSVerify = true
	config.Variables = bundle.Variables{
		"domain": "example.test", "enabled": true, "replicas": 2,
		"nested": bundle.Variables{"name": "api"},
		"map":    map[string]any{"port": 8080},
		"list":   []any{"first", bundle.Variables{"name": "second"}, map[string]any{"name": "third"}},
	}
	stop := errors.New("configuration captured")
	var observed *bundle.UDSBundleConfig
	var bundleDir string
	_, err := bundle.Deploy(t.Context(), source, bundle.DeployOptions{
		Config: config,
		PackageDeployHooks: bundle.PackageDeployHooks{PreDeploy: func(_ context.Context, _ *spec.Package, _ *bundle.ZarfPackageLayout, opts *bundle.DeployPackageOptions) error {
			observed, bundleDir = opts.Config, opts.BundleDir
			return stop
		}},
	})
	require.ErrorIs(t, err, stop)
	require.NotNil(t, observed)
	assert.Equal(t, config.Options, observed.Options)
	want, err := json.Marshal(config.Variables)
	require.NoError(t, err)
	got, err := json.Marshal(observed.Variables)
	require.NoError(t, err)
	assert.JSONEq(t, string(want), string(got))
	assert.Equal(t, filepath.Dir(source.BundlePath), bundleDir)
}

func TestDefaultsMergePreservesCallerConfiguration(t *testing.T) {
	source := preparedLibrarySource(t)
	source.DefaultsPath = filepath.Join(t.TempDir(), "defaults.uds.hcl")
	require.NoError(t, os.WriteFile(source.DefaultsPath, []byte(`variables = {
  domain = "default.test"
  nested = { keep = "default", override = "default" }
  names = ["default"]
}
`), 0o600))
	config := libraryFixtureConfig(t)
	config.Variables = bundle.Variables{
		"domain": "caller.test",
		"nested": map[string]any{"override": "caller"},
		"names":  []any{"caller"},
	}
	before, err := json.Marshal(config.Variables)
	require.NoError(t, err)
	var observed bundle.Variables
	stop := errors.New("merged defaults captured")
	_, err = bundle.Deploy(t.Context(), source, bundle.DeployOptions{
		Config: config,
		PackageDeployHooks: bundle.PackageDeployHooks{PreDeploy: func(_ context.Context, _ *spec.Package, _ *bundle.ZarfPackageLayout, opts *bundle.DeployPackageOptions) error {
			observed = opts.Config.Variables
			return stop
		}},
	})
	require.ErrorIs(t, err, stop)
	got, err := json.Marshal(observed)
	require.NoError(t, err)
	assert.JSONEq(t, `{"domain":"caller.test","nested":{"keep":"default","override":"caller"},"names":["caller"]}`, string(got))
	after, err := json.Marshal(config.Variables)
	require.NoError(t, err)
	assert.JSONEq(t, string(before), string(after))
}

func TestGlobalCompatibilityOptionsPreserveOperationBehavior(t *testing.T) {
	artifact := createLibraryArtifact(t)
	config := libraryFixtureConfig(t)
	before, err := bundle.Inspect(t.Context(), bundle.InspectOptions{Source: artifact, Config: config})
	require.NoError(t, err)
	config.Global = &bundle.GlobalOptions{LogLevel: "debug", Prompt: true}
	after, err := bundle.Inspect(t.Context(), bundle.InspectOptions{Source: artifact, Config: config})
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func TestPublicOptionsValidateConfiguration(t *testing.T) {
	checks := []struct {
		name     string
		validate func(*bundle.UDSBundleConfig) error
	}{
		{"create", func(c *bundle.UDSBundleConfig) error {
			return (bundle.CreateOptions{Config: c, Signing: bundle.SigningOptions{Mode: bundle.SigningModeUnsigned}}).Validate()
		}},
		{"deploy", func(c *bundle.UDSBundleConfig) error { return (bundle.DeployOptions{Config: c}).Validate() }},
		{"package deploy", func(c *bundle.UDSBundleConfig) error {
			return (bundle.DeployPackageOptions{Config: c, BundleDir: "."}).Validate()
		}},
		{"remove", func(c *bundle.UDSBundleConfig) error { return (bundle.RemoveOptions{Config: c}).Validate() }},
		{"pull", func(c *bundle.UDSBundleConfig) error { return (bundle.PullOptions{Config: c}).Validate() }},
		{"push", func(c *bundle.UDSBundleConfig) error { return (bundle.PushOptions{Config: c}).Validate() }},
		{"inspect", func(c *bundle.UDSBundleConfig) error {
			return (bundle.InspectOptions{Config: c, Source: "bundle.tar.zst"}).Validate()
		}},
		{"reconfigure", func(c *bundle.UDSBundleConfig) error {
			return (bundle.ReconfigureOptions{Config: c, Suffix: "-configured", Signing: bundle.SigningOptions{Mode: bundle.SigningModeUnsigned}}).Validate()
		}},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			require.NoError(t, check.validate(libraryFixtureConfig(t)))
			require.ErrorIs(t, check.validate(nil), bundle.ErrInvalidConfig)
			require.ErrorIs(t, check.validate(&bundle.UDSBundleConfig{}), bundle.ErrInvalidConfig)
			invalid := libraryFixtureConfig(t)
			invalid.Options.Concurrency = 0
			require.ErrorIs(t, check.validate(invalid), bundle.ErrInvalidConfig)
		})
	}
	require.ErrorIs(t, (bundle.DeployPackageOptions{Config: libraryFixtureConfig(t)}).Validate(), bundle.ErrBundleDirRequired)
}

func TestDeployTemplatesPublicVariables(t *testing.T) {
	for _, missing := range []bool{false, true} {
		name := "nested maps and lists"
		if missing {
			name = "missing variable"
		}
		t.Run(name, func(t *testing.T) {
			source := preparedLibrarySource(t)
			values := filepath.Join(t.TempDir(), "values.yaml")
			require.NoError(t, os.WriteFile(values, []byte(`replicas: {{ .vars.replicas }}
name: {{ .vars.nested.name }}
items:
{{ range .vars.items }}  - {{ . }}
{{ end }}`), 0o600))
			source.Bundle.Packages[1].ValuesFiles = []string{values}
			config := libraryFixtureConfig(t)
			config.Variables = bundle.Variables{
				"replicas": 2,
				"nested":   map[string]any{"name": "consumer"},
				"items":    []any{"first", "second"},
			}
			if missing {
				delete(config.Variables, "nested")
			}
			stop := errors.New("valid values reached package hook")
			called := false
			_, err := bundle.Deploy(t.Context(), source, bundle.DeployOptions{
				Config: config, Packages: []string{"app"}, Force: true,
				PackageDeployHooks: bundle.PackageDeployHooks{PreDeploy: func(context.Context, *spec.Package, *bundle.ZarfPackageLayout, *bundle.DeployPackageOptions) error {
					called = true
					return stop
				}},
			})
			require.ErrorIs(t, err, bundle.ErrDeployBundle)
			if missing {
				require.NotErrorIs(t, err, stop)
				require.ErrorContains(t, err, "nested")
				assert.False(t, called)
			} else {
				require.ErrorIs(t, err, stop)
				assert.True(t, called)
			}
			entries, err := os.ReadDir(config.Options.TmpDir)
			require.NoError(t, err)
			assert.Empty(t, entries)
			assert.FileExists(t, values)
		})
	}
}
