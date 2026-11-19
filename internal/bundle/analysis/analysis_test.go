package analysis

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const healthyBundle = `uds {
  bundle_api_version = "uds.dev/v1alpha1"
}
metadata {
  name = "lint-test"
}
package "app" {
  source = "oci://invalid.example.test/app:1.0.0"
  %s
}
`

func put(t *testing.T, root, name, text string) string {
	t.Helper()
	path := filepath.Join(root, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte(text), 0o600))
	return path
}

func fixture(t *testing.T, extra string) string {
	t.Helper()
	root := t.TempDir()
	put(t, root, "bundle.uds.hcl", fmt.Sprintf(healthyBundle, extra))
	return root
}

func TestLintValuesTemplates(t *testing.T) {
	for _, tt := range []struct {
		name     string
		template string
		skip     bool
		want     string
	}{
		{name: "plain values", template: "replicas: 2\n"},
		{name: "missing variables fail by default", template: "hostname: {{ .vars.domain }}", want: "--skip-template-evaluation"},
		{name: "skip execution", template: "hostname: {{ .vars.domain }}", skip: true},
		{name: "skip still checks syntax", template: "hostname: {{", skip: true, want: "invalid values template syntax"},
		{name: "syntax checked by default", template: "{{ if }}", want: "invalid values template syntax"},
		{name: "not YAML validation", template: "not: [yaml", skip: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := fixture(t, `values_files = ["values.yaml"]`)
			put(t, root, "values.yaml", tt.template)
			diagnostics, err := Lint(t.Context(), root, Options{SkipTemplateEvaluation: tt.skip})
			require.NoError(t, err)
			if tt.want == "" {
				assert.Empty(t, diagnostics)
				return
			}
			require.Len(t, diagnostics, 1)
			assert.Equal(t, "values.yaml", diagnostics[0].Path)
			assert.Equal(t, 1, diagnostics[0].Line)
			assert.Contains(t, diagnostics[0].Message, tt.want)
			assert.Contains(t, diagnostics[0].Message, `package "app"`)
		})
	}
}

func TestLintUsesDefaultsAndExplicitConfig(t *testing.T) {
	root := fixture(t, `values_files = ["values.yaml"]`)
	put(t, root, "defaults.uds.hcl", `variables = { domain = "default", nested = { retained = "yes", changed = "default" } }`)
	put(t, root, "config.uds.hcl", `this is deliberately not valid config`)
	put(t, root, "values.yaml", `{{ if ne .vars.domain "default" }}{{ .vars.wrong }}{{ end }}{{ .vars.nested.retained }}`)
	diagnostics, err := Lint(t.Context(), root, Options{})
	require.NoError(t, err)
	assert.Empty(t, diagnostics, "unselected adjacent config must be ignored")

	configDir := t.TempDir()
	put(t, configDir, "domain.txt", "config")
	config := put(t, configDir, "dev.hcl", `variables = { domain = file("domain.txt"), nested = { changed = "config" } }`)
	put(t, root, "values.yaml", `{{ if ne .vars.domain "config" }}{{ .vars.wrong }}{{ end }}{{ .vars.nested.retained }}`)
	diagnostics, err = Lint(t.Context(), filepath.Join(root, "bundle.uds.hcl"), Options{ConfigPath: config})
	require.NoError(t, err)
	assert.Empty(t, diagnostics)

	put(t, root, "values.yaml", `{{ if ne .vars.domain "cli" }}{{ .vars.wrong }}{{ end }}{{ .vars.nested.retained }}`)
	diagnostics, err = Lint(t.Context(), root, Options{ConfigPath: config, Variables: []string{"domain=cli"}})
	require.NoError(t, err)
	assert.Empty(t, diagnostics)

	prod := put(t, configDir, "prod.hcl", `variables = { domain = "prod" }`)
	diagnostics, err = Lint(t.Context(), root, Options{ConfigPath: prod})
	require.NoError(t, err)
	require.Len(t, diagnostics, 1, "separate environments must not share variables")
	assert.Contains(t, diagnostics[0].Message, "evaluation failed")
}

func TestLintArchitecturePrecedence(t *testing.T) {
	root := fixture(t, `values_files = ["${sys.arch}.yaml"]`)
	config := put(t, root, "environment.hcl", "options {\n architecture = \"arm64\"\n}\n")
	put(t, root, "arm64.yaml", "valid: true")
	put(t, root, "amd64.yaml", "{{")
	diagnostics, err := Lint(t.Context(), root, Options{ConfigPath: config})
	require.NoError(t, err)
	assert.Empty(t, diagnostics)
	diagnostics, err = Lint(t.Context(), root, Options{ConfigPath: config, Architecture: "amd64"})
	require.NoError(t, err)
	require.Len(t, diagnostics, 1)
	assert.Equal(t, "amd64.yaml", diagnostics[0].Path)
}

func TestLintInvalidVariablesSuppressOnlyExecution(t *testing.T) {
	root := fixture(t, `values_files = ["missing-variable.yaml", "broken.yaml"]`)
	put(t, root, "defaults.uds.hcl", `variables = {`)
	put(t, root, "missing-variable.yaml", `{{ .vars.missing }}`)
	put(t, root, "broken.yaml", `{{`)
	diagnostics, err := Lint(t.Context(), root, Options{})
	require.NoError(t, err)
	var paths []string
	for _, d := range diagnostics {
		paths = append(paths, d.Path)
		assert.NotContains(t, d.Message, "evaluation failed")
	}
	assert.Contains(t, paths, "defaults.uds.hcl")
	assert.Contains(t, paths, "broken.yaml")
	assert.NotContains(t, paths, "missing-variable.yaml")
}

func TestLintMissingFilesAndIndependentFailures(t *testing.T) {
	root := fixture(t, `values_files = ["missing.yaml", "bad.yaml", "other.yaml"]`)
	put(t, root, "bad.yaml", `{{`)
	put(t, root, "other.yaml", `{{`)
	diagnostics, err := Lint(t.Context(), root, Options{SkipTemplateEvaluation: true})
	require.NoError(t, err)
	require.Len(t, diagnostics, 3)
	assert.Equal(t, "bad.yaml", diagnostics[0].Path)
	assert.Equal(t, "bundle.uds.hcl", diagnostics[1].Path)
	assert.Positive(t, diagnostics[1].Line)
	assert.Positive(t, diagnostics[1].Column)
	assert.Contains(t, diagnostics[1].Message, "missing.yaml")
	assert.Equal(t, "other.yaml", diagnostics[2].Path)
	again, err := Lint(t.Context(), root, Options{SkipTemplateEvaluation: true})
	require.NoError(t, err)
	assert.Equal(t, diagnostics, again)
}

func TestLintTrustedExternalFiles(t *testing.T) {
	workspace := t.TempDir()
	root := filepath.Join(workspace, "bundle")
	put(t, workspace, "name.txt", "external-name")
	put(t, workspace, "shared.yaml", `{{ .vars.missing }}`)
	put(t, root, "bundle.uds.hcl", strings.Replace(fmt.Sprintf(healthyBundle, `values_files = ["../shared.yaml"]`), `name = "lint-test"`, `name = file("../name.txt")`, 1))
	link := filepath.Join(root, "linked.yaml")
	require.NoError(t, os.Symlink(filepath.Join(workspace, "shared.yaml"), link))
	diagnostics, err := Lint(t.Context(), root, Options{})
	require.NoError(t, err)
	require.Len(t, diagnostics, 1)
	assert.Equal(t, "../shared.yaml", diagnostics[0].Path)
	put(t, root, "bundle.uds.hcl", fmt.Sprintf(healthyBundle, `values_files = ["linked.yaml"]`))
	diagnostics, err = Lint(t.Context(), root, Options{SkipTemplateEvaluation: true})
	require.NoError(t, err)
	assert.Empty(t, diagnostics)
}

func TestLintSourceAndDomainErrors(t *testing.T) {
	for _, tt := range []struct{ name, content, want string }{
		{"syntax", `package "app" {`, "bundle.uds.hcl"},
		{"empty", "", "bundle.uds.hcl"},
		{"domain", strings.Replace(fmt.Sprintf(healthyBundle, ""), `name = "lint-test"`, `name = ""`, 1), "metadata.name is required"},
		{"undefined local", strings.Replace(fmt.Sprintf(healthyBundle, ""), `name = "lint-test"`, `name = local.missing`, 1), "Unsupported attribute"},
		{"unknown dependency", fmt.Sprintf(healthyBundle, `depends_on = [package.missing]`), "unknown package"},
		{"cycle", fmt.Sprintf(healthyBundle, `depends_on = [package.other]`) + "\npackage \"other\" {\n source = \"oci://invalid.example.test/other:1\"\n depends_on = [package.app]\n}\n", "cycle"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			put(t, root, "bundle.uds.hcl", tt.content)
			diagnostics, err := Lint(t.Context(), root, Options{})
			require.NoError(t, err)
			require.NotEmpty(t, diagnostics)
			assert.Contains(t, fmt.Sprint(diagnostics), tt.want)
		})
	}
}

func TestLintNoRecursionAndInputFailures(t *testing.T) {
	root := t.TempDir()
	put(t, root, "nested/bundle.uds.hcl", fmt.Sprintf(healthyBundle, ""))
	_, err := Lint(t.Context(), root, Options{})
	require.ErrorIs(t, err, os.ErrNotExist)
	put(t, root, "bundle.uds.hcl", fmt.Sprintf(healthyBundle, ""))
	put(t, root, "nested/bundle.uds.hcl", "broken")
	diagnostics, err := Lint(t.Context(), root, Options{})
	require.NoError(t, err)
	assert.Empty(t, diagnostics)
	for _, path := range []string{"", " ", put(t, root, "other.hcl", "")} {
		_, err := Lint(t.Context(), path, Options{})
		require.ErrorIs(t, err, ErrInvalidInput)
	}
	_, err = Lint(t.Context(), root, Options{ConfigPath: filepath.Join(root, "absent.hcl")})
	require.ErrorIs(t, err, os.ErrNotExist)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	diagnostics, err = Lint(ctx, root, Options{})
	require.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, diagnostics)
}

func TestLintDoesNotLeakEvaluatedValues(t *testing.T) {
	secret := strings.Repeat("test-sensitive-value", 3)
	root := fixture(t, `values_files = ["values.yaml"]`)
	put(t, root, "values.yaml", `{{ call .vars.secret }}`)
	put(t, root, "defaults.uds.hcl", `variables = { secret = "`+secret+`" }`)
	diagnostics, err := Lint(t.Context(), root, Options{})
	require.NoError(t, err)
	require.NotEmpty(t, diagnostics)
	assert.NotContains(t, fmt.Sprint(diagnostics), secret)

	config := put(t, root, "selected.hcl", `options { concurrency = file("secret.txt") }`)
	put(t, root, "secret.txt", secret)
	diagnostics, err = Lint(t.Context(), root, Options{ConfigPath: config})
	require.NoError(t, err)
	require.NotEmpty(t, diagnostics)
	assert.NotContains(t, fmt.Sprint(diagnostics), secret)
	_, err = Lint(t.Context(), root, Options{Variables: []string{secret}})
	require.Error(t, err)
	assert.NotContains(t, err.Error(), secret)
}

func TestDiagnosticUnicodePositions(t *testing.T) {
	text := "first\r\n\"é😀é\" + invalid"
	doc := document{path: "bundle.uds.hcl", text: []byte(text)}
	d := doc.at(&hcl.Pos{Byte: strings.Index(text, "invalid")}, "test")
	assert.Equal(t, 2, d.Line)
	assert.Equal(t, 10, d.Column)
	assert.Zero(t, doc.at(nil, "test").Line)
}
