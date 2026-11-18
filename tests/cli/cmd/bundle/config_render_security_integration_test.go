// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build cli

package bundle_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"

	"github.com/defenseunicorns/uds-cli/internal/cli"
	"github.com/defenseunicorns/uds-cli/internal/mode"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
)

func TestConfigRenderInvalidOptionDoesNotExposeFileContents(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "config.uds.hcl")
	const secret = "security-regression-secret-78319"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "secret.txt"), []byte(secret), 0o600))
	require.NoError(t, os.WriteFile(input, []byte("options { log_level = file(\"secret.txt\") }\n"), 0o600))

	streams, _, stdout, stderr := iostreams.NewTestIOStreams()
	root := cli.NewRootCommand(streams, mode.FeatureSet{})
	root.SetArgs([]string{"bundle", "config", "render", input})
	err := root.ExecuteContext(t.Context())
	require.Error(t, err)
	require.Contains(t, err.Error(), "log level")
	require.NotContains(t, err.Error(), secret)
	require.NotContains(t, stdout.String(), secret)
	require.NotContains(t, stderr.String(), secret)
	require.Empty(t, stdout.String())
}

func TestConfigRenderErrorsDoNotExposeFileContents(t *testing.T) {
	for _, tc := range []struct {
		name, secret, config, diagnostic string
	}{
		{"nested file", "missing-secret-path-78319", `variables = { value = file(file("secret.txt")) }`, "file"},
		{"invalid concurrency", "-987654321", `options { concurrency = file("secret.txt") }`, "concurrency"},
		{"invalid variable value", "secret-object-key-78319", `variables = { (file("secret.txt")) = null }`, "variable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			input := filepath.Join(dir, "config.uds.hcl")
			require.NoError(t, os.WriteFile(filepath.Join(dir, "secret.txt"), []byte(tc.secret), 0o600))
			require.NoError(t, os.WriteFile(input, []byte(tc.config+"\n"), 0o600))

			streams, _, stdout, stderr := iostreams.NewTestIOStreams()
			root := cli.NewRootCommand(streams, mode.FeatureSet{})
			root.SetArgs([]string{"bundle", "config", "render", input})
			err := root.ExecuteContext(t.Context())
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.diagnostic)
			require.NotContains(t, err.Error(), tc.secret)
			require.NotContains(t, stdout.String(), tc.secret)
			require.NotContains(t, stderr.String(), tc.secret)
			require.Empty(t, stdout.String())
		})
	}
}

func TestConfigRenderCompactNumericValues(t *testing.T) {
	for _, tc := range []struct {
		name, config string
	}{
		{"architecture", "options { architecture = 1e10000 }\n"},
		{"tiny variable", "variables = { value = 1e-10000 }\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			input := filepath.Join(dir, "config.uds.hcl")
			require.NoError(t, os.WriteFile(input, []byte(tc.config), 0o600))
			streams, _, stdout, stderr := iostreams.NewTestIOStreams()
			root := cli.NewRootCommand(streams, mode.FeatureSet{})
			root.SetArgs([]string{"bundle", "config", "render", input})
			err := root.ExecuteContext(t.Context())
			if err != nil {
				// A bounded rejection is also valid when the value exceeds render limits.
				require.Contains(t, err.Error(), "number")
				require.Less(t, len(err.Error()), 4096)
				require.Less(t, stderr.Len(), 4096)
				require.Empty(t, stdout.String())
				return
			}
			require.Empty(t, stderr.String())
			// The compact output must still represent the original value.
			file, diags := hclsyntax.ParseConfig(stdout.Bytes(), "rendered.hcl", hcl.InitialPos)
			require.False(t, diags.HasErrors(), "%s", diags.Error())
			body := file.Body.(*hclsyntax.Body)
			var value cty.Value
			if tc.name == "architecture" {
				// Architecture is a string, so numeric-to-string conversion expands it.
				require.Less(t, stdout.Len(), 12000)
				value, diags = body.Blocks[0].Body.Attributes["architecture"].Expr.Value(nil)
				require.False(t, diags.HasErrors(), "%s", diags.Error())
				number, err := cty.ParseNumberVal("1e10000")
				require.NoError(t, err)
				expected, err := convert.Convert(number, cty.String)
				require.NoError(t, err)
				require.True(t, value.RawEquals(expected))
			} else {
				require.Less(t, stdout.Len(), 4096)
				value, diags = body.Attributes["variables"].Expr.Value(nil)
				require.False(t, diags.HasErrors(), "%s", diags.Error())
				expected, err := cty.ParseNumberVal("1e-10000")
				require.NoError(t, err)
				require.True(t, value.GetAttr("value").RawEquals(expected))
			}
		})
	}
}

func TestConfigRenderRejectsGiganticNumericOption(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "config.uds.hcl")
	// This compact input would expand to 1,000,001 digits if serialized in decimal.
	require.NoError(t, os.WriteFile(input, []byte("options { concurrency = 1e1000000 }\n"), 0o600))

	streams, _, stdout, stderr := iostreams.NewTestIOStreams()
	root := cli.NewRootCommand(streams, mode.FeatureSet{})
	root.SetArgs([]string{"bundle", "config", "render", input})
	err := root.ExecuteContext(t.Context())
	require.Error(t, err)
	require.Empty(t, stdout.String())
	// Bound diagnostics rather than relying on a machine-dependent timing limit.
	require.Less(t, len(err.Error()), 4096)
	require.Less(t, stderr.Len(), 4096)
}
