// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHCLParserPublicFacade(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.uds.hcl")
	require.NoError(t, os.WriteFile(configPath, []byte(`
options {
  architecture = "arm64"
  concurrency  = 3
}
variables = {
  services = [
    { name = "api", settings = { enabled = true } },
  ]
}`), 0o600))

	cfg, err := NewHCLParser("", iostreams.IOStreams{}).ParseBundleConfig(t.Context(), configPath)
	require.NoError(t, err)
	require.NotNil(t, cfg.Options)
	assert.Equal(t, "arm64", cfg.Options.Architecture)
	assert.Equal(t, 3, cfg.Options.Concurrency)

	services, ok := cfg.Variables["services"].([]any)
	require.True(t, ok)
	require.Len(t, services, 1)
	service, ok := services[0].(Variables)
	require.True(t, ok)
	settings, ok := service["settings"].(Variables)
	require.True(t, ok)
	assert.Equal(t, "api", service["name"])
	assert.Equal(t, true, settings["enabled"])
}

func TestParseDefaultsAndMergeVariables(t *testing.T) {
	defaultsPath := filepath.Join(t.TempDir(), BundleDefaultsFileName)
	require.NoError(t, os.WriteFile(defaultsPath, []byte(`
variables = {
  image = { tag = "1.0.0", pull = "IfNotPresent" }
}`), 0o600))

	defaults, err := ParseDefaults(t.Context(), defaultsPath)
	require.NoError(t, err)
	merged := MergeVariables(defaults, Variables{
		"image": Variables{"tag": "2.0.0"},
		"ports": []any{float64(8080)},
	})

	image, ok := merged["image"].(Variables)
	require.True(t, ok)
	assert.Equal(t, Variables{"tag": "2.0.0", "pull": "IfNotPresent"}, image)
	assert.Equal(t, []any{float64(8080)}, merged["ports"])

	image["tag"] = "changed"
	assert.Equal(t, "1.0.0", defaults["image"].(Variables)["tag"])
}

func TestConfigConversionsPreserveRecursiveVariables(t *testing.T) {
	cfg := &UDSBundleConfig{
		Global: &GlobalOptions{LogLevel: "debug", Prompt: true},
		Options: &ConfigOptions{
			LogLevel: "warn", Architecture: "amd64", PlainHTTP: true,
			SkipTLSVerify: true, UDSCache: "/cache", TmpDir: "/tmp", Concurrency: 5,
		},
		Variables: Variables{
			"nested": map[string]any{"items": []any{map[string]any{"value": "ok"}}},
		},
	}

	got := fromInternalConfig(toInternalConfig(cfg))
	assert.Equal(t, cfg.Global, got.Global)
	assert.Equal(t, cfg.Options, got.Options)

	nested, ok := got.Variables["nested"].(Variables)
	require.True(t, ok)
	items, ok := nested["items"].([]any)
	require.True(t, ok)
	item, ok := items[0].(Variables)
	require.True(t, ok)
	assert.Equal(t, "ok", item["value"])
}

func TestVariablesFlatten(t *testing.T) {
	got, err := (Variables{
		"domain":   "uds.dev",
		"replicas": float64(3),
		"enabled":  true,
		"nested":   Variables{"ignored": true},
	}).Flatten()
	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		"DOMAIN": "uds.dev", "REPLICAS": "3", "ENABLED": "true",
	}, got)
}
