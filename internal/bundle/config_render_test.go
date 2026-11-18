// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
)

func TestRenderDecodeEvaluatesExpressionsOnce(t *testing.T) {
	src := []byte("variables = { value = file(\"x\") }\noptions { log_level = file(\"x\") }")
	file, diags := hclsyntax.ParseConfig(src, "config", hcl.InitialPos)
	require.False(t, diags.HasErrors(), diags.Error())
	calls := 0
	ctx := &hcl.EvalContext{Functions: map[string]function.Function{
		"file": function.New(&function.Spec{
			Params: []function.Parameter{{Name: "path", Type: cty.String}},
			Type:   function.StaticReturnType(cty.String),
			Impl: func(_ []cty.Value, _ cty.Type) (cty.Value, error) {
				calls++
				return cty.StringVal("info"), nil
			},
		}),
	}}
	values := make(map[int]cty.Value)
	_, err := decodeBundleConfig(&renderBody{Body: file.Body, values: values}, ctx, "config")
	require.NoError(t, err)
	out := hclwrite.NewEmptyFile()
	require.NoError(t, renderConfigBody(out.Body(), file.Body.(*hclsyntax.Body), values))
	assert.Equal(t, 2, calls)
	assert.Len(t, values, 2)
}

func TestRenderRejectsLargeOptionBeforeSerialization(t *testing.T) {
	for _, src := range []string{
		"options { concurrency = 1e100000000 }",
		"options { architecture = 1e1000000 }",
		`options { architecture = "${1e1000000}" }`,
		"options { architecture = 1e1000 * 1e1000 }",
		"options { architecture = true ? \"ok\" : 1e1000000 }",
		"variables = { x = 1e-1000000 }",
		"variables = { x = [{ y = 1e-1000000 }] }",
	} {
		t.Run(src, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.uds.hcl")
			require.NoError(t, os.WriteFile(path, []byte(src), 0o600))
			_, err := RenderBundleConfigFile(path)
			require.ErrorContains(t, err, "number exceeds render limits")
		})
	}
}

func TestRenderScientificNumbersPreservePrecision(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.uds.hcl")
	src := `variables = { values = [1e-500, 1.2345678901234567890123456789e100] }`
	require.NoError(t, os.WriteFile(path, []byte(src), 0o600))
	out, err := RenderBundleConfigFile(path)
	require.NoError(t, err)
	input, diags := hclsyntax.ParseConfig([]byte(src), path, hcl.InitialPos)
	require.False(t, diags.HasErrors(), diags.Error())
	output, diags := hclsyntax.ParseConfig(out, path, hcl.InitialPos)
	require.False(t, diags.HasErrors(), diags.Error())
	want, diags := input.Body.(*hclsyntax.Body).Attributes["variables"].Expr.Value(nil)
	require.False(t, diags.HasErrors(), diags.Error())
	got, diags := output.Body.(*hclsyntax.Body).Attributes["variables"].Expr.Value(nil)
	require.False(t, diags.HasErrors(), diags.Error())
	assert.True(t, want.RawEquals(got))
}

func TestRenderErrorsRedactEvaluatedData(t *testing.T) {
	for _, tc := range []struct {
		name, secret, src, category string
		wantErr                     error
	}{
		{"nested file path", "sensitive-file-path", `variables = { x = file(file("secret")) }`, "evaluate variables", ErrEvaluateVariables},
		{"concurrency", "87654321", `options { concurrency = file("secret") }`, "concurrency", ErrInvalidConcurrency},
		{"object key", "sensitive-object-key", `variables = { (file("secret")) = null }`, "invalid variables", ErrInvalidVariables},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "config.uds.hcl")
			require.NoError(t, os.WriteFile(filepath.Join(dir, "secret"), []byte(tc.secret), 0o600))
			require.NoError(t, os.WriteFile(path, []byte(tc.src), 0o600))
			_, err := RenderBundleConfigFile(path)
			require.ErrorIs(t, err, tc.wantErr)
			assert.NotContains(t, err.Error(), tc.secret)
			assert.Contains(t, err.Error(), tc.category)
			assert.Contains(t, err.Error(), "config.uds.hcl:1,")
		})
	}
}

func TestRenderValidationDoesNotDiscloseFileContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.uds.hcl")
	secret := "sensitive-render-test-value"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "secret"), []byte(secret), 0o600))
	for _, option := range []string{"log_level", "tmp_dir"} {
		t.Run(option, func(t *testing.T) {
			require.NoError(t, os.WriteFile(path, []byte("options { "+option+" = file(\"secret\") }"), 0o600))
			_, err := RenderBundleConfigFile(path)
			require.Error(t, err)
			assert.NotContains(t, err.Error(), secret)
		})
	}
}

func TestRenderStrictWithoutChangingParserCompatibility(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.uds.hcl")
	require.NoError(t, os.WriteFile(path, []byte("unknown = true"), 0o600))
	_, err := (&HCLParser{}).ParseBundleConfig(context.Background(), path)
	require.NoError(t, err)
	_, err = RenderBundleConfigFile(path)
	require.ErrorContains(t, err, "unknown")
}

func TestRenderBundleConfigRejectsMapVariables(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.uds.hcl")
	require.NoError(t, os.WriteFile(path, []byte(`variables = { x = true ? { a = 1 } : { b = 2 } }`), 0o600))
	_, err := (&HCLParser{}).ParseBundleConfig(context.Background(), path)
	require.ErrorIs(t, err, ErrUnsupportedVariableType)
	_, err = RenderBundleConfigFile(path)
	require.ErrorIs(t, err, ErrUnsupportedVariableType)
}

func TestRenderBundleConfigFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.uds.hcl")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "value.txt"), []byte("${literal} %{directive}\n"), 0o600))
	src := `variables = {
  content = file("value.txt")
  list = [for n in [1, 2] : n * 2]
  precise = 123456789012345678901234567890
}
options {
  plain_http = false
  concurrency = 0
  log_level = true ? "info" : "debug"
}
signature_verification {
  keyless {
    certificate_identity = "id"
    certificate_oidc_issuer = "issuer"
  }
}
`
	require.NoError(t, os.WriteFile(path, []byte(src), 0o600))
	rendered, err := RenderBundleConfigFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(rendered), "plain_http  = false")
	assert.Contains(t, string(rendered), "concurrency = 0")
	assert.NotContains(t, string(rendered), "file(")
	assert.NotContains(t, string(rendered), "for n")
	assert.Contains(t, string(rendered), "$${literal} %%{directive}")
	require.NoError(t, os.Remove(filepath.Join(dir, "value.txt")))
	file, diags := hclsyntax.ParseConfig(rendered, "rendered", hcl.InitialPos)
	require.False(t, diags.HasErrors(), diags.Error())
	body := file.Body.(*hclsyntax.Body)
	value, diags := body.Attributes["variables"].Expr.Value(nil)
	require.False(t, diags.HasErrors(), diags.Error())
	assert.Equal(t, "${literal} %{directive}\n", value.GetAttr("content").AsString())
	precise, err := cty.ParseNumberVal("123456789012345678901234567890")
	require.NoError(t, err)
	assert.True(t, value.GetAttr("precise").RawEquals(precise))
	_, err = parseBundleConfigContent(rendered, path)
	require.NoError(t, err)
}

func TestRenderBundleConfigValidation(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{name: "empty"},
		{name: "empty variables", src: "variables = {}"},
		{name: "empty options", src: "options {}"},
		{name: "zero concurrency", src: "options { concurrency = 0 }"},
		{name: "coerced concurrency", src: "options { concurrency = \"2\" }"},
		{name: "invalid concurrency", src: "options { concurrency = -1 }", want: "concurrency"},
		{name: "invalid log level", src: "options { log_level = \"invalid\" }", want: "level"},
		{name: "unknown attribute", src: "extra = true", want: "extra"},
		{name: "unknown block", src: "extra {}", want: "extra"},
		{name: "unknown option", src: "options { extra = true }", want: "decode config"},
		{name: "unknown keyless attribute", src: "signature_verification {\n keyless {\n extra = true\n }\n}", want: "decode config"},
		{name: "unknown nested block", src: "signature_verification {\n keyless {\n extra {}\n }\n}", want: "decode config"},
		{name: "invalid variables", src: "variables = { x = null }", want: "invalid variables"},
		{name: "invalid expression", src: "variables = { x = file(\"missing\") }", want: "evaluate variables"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.uds.hcl")
			require.NoError(t, os.WriteFile(path, []byte(tt.src), 0o600))
			out, err := RenderBundleConfigFile(path)
			if tt.want != "" {
				require.ErrorContains(t, err, tt.want)
				return
			}
			require.NoError(t, err)
			if tt.name == "empty" {
				assert.NotContains(t, string(out), "variables")
			}
			if tt.name == "empty variables" {
				assert.Contains(t, string(out), "variables = {}")
			}
			if tt.name == "empty options" {
				assert.NotContains(t, string(out), "concurrency")
			}
		})
	}
}
