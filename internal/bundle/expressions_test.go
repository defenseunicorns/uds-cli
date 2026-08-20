// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/defenseunicorns/uds-cli/internal/filesystem"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty/function"
)

func parseExpressionTestHCL(t *testing.T, src string) *hcl.File {
	t.Helper()

	hclFile, diags := hclsyntax.ParseConfig([]byte(src), "test.hcl", hcl.Pos{Line: 1, Column: 1})
	require.False(t, diags.HasErrors(), diags.Error())

	return hclFile
}

func TestExtractLocalsMultipleLocalsBlocks(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("from a"), filesystem.PrivateFileMode))

	hclFile := parseExpressionTestHCL(t, `
locals {
  name = file(local.path)
}
locals {
  path = "a.txt"
}
`)

	locals, contexts, err := (&HCLParser{}).extractLocals(hclFile, map[string]function.Function{
		"file": newFileFunction(dir),
	})
	require.NoError(t, err)

	assert.Equal(t, "from a", locals["name"].AsString())
	assert.Equal(t, "a.txt", locals["path"].AsString())
	assert.NotEmpty(t, contexts)
}

func TestExtractLocalsSupportsMultiLevelDependencyChain(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("from a"), filesystem.PrivateFileMode))

	hclFile := parseExpressionTestHCL(t, `
locals {
  name = file(local.path)
  path = local.dir
  dir = "${local.filename}.txt"
  filename = "a"
}
`)

	locals, contexts, err := (&HCLParser{}).extractLocals(hclFile, map[string]function.Function{
		"file": newFileFunction(dir),
	})
	require.NoError(t, err)

	assert.Equal(t, "from a", locals["name"].AsString())
	assert.Equal(t, "a.txt", locals["path"].AsString())
	assert.Equal(t, "a.txt", locals["dir"].AsString())
	assert.Equal(t, "a", locals["filename"].AsString())
	assert.NotEmpty(t, contexts)
}

func TestExtractLocalsIndexStyleReference(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("from a"), filesystem.PrivateFileMode))

	hclFile := parseExpressionTestHCL(t, `
locals {
  name = file(local["path"])
}
locals {
  path = "a.txt"
}
`)

	locals, contexts, err := (&HCLParser{}).extractLocals(hclFile, map[string]function.Function{
		"file": newFileFunction(dir),
	})
	require.NoError(t, err)

	assert.Equal(t, "from a", locals["name"].AsString())
	assert.Equal(t, "a.txt", locals["path"].AsString())
	assert.NotEmpty(t, contexts)
}

func TestExtractLocalsCatchesCircularDependencyChain(t *testing.T) {
	hclFile := parseExpressionTestHCL(t, `
locals {
  name = "${local.path}"
  path = "${local.name}"
}
`)

	_, _, err := (&HCLParser{}).extractLocals(hclFile, nil)

	var cyclicErr *CyclicLocalDependencyError
	require.ErrorAs(t, err, &cyclicErr)
	assert.Equal(t, []string{"name", "path", "name"}, cyclicErr.Cycle)
}

func TestExtractLocalsCatchesUndefinedLocal(t *testing.T) {
	hclFile := parseExpressionTestHCL(t, `
locals {
  name = local.missing
}
`)

	_, _, err := (&HCLParser{}).extractLocals(hclFile, nil)
	var undefinedErr *UndefinedLocalDependencyError
	require.ErrorAs(t, err, &undefinedErr)
	assert.Equal(t, "missing", undefinedErr.Name)
}

func TestExtractLocalsCatchesDuplicateLocalDefinitions(t *testing.T) {
	hclFile := parseExpressionTestHCL(t, `
locals {
  name = "first"
}

locals {
  name = "second"
}
`)

	_, _, err := (&HCLParser{}).extractLocals(hclFile, nil)

	var duplicateErr *DuplicateLocalError
	require.ErrorAs(t, err, &duplicateErr)
	assert.Equal(t, "name", duplicateErr.Name)
	assert.NotEqual(t, duplicateErr.Existing, duplicateErr.Duplicate)
}

func TestExtractLocalsFailedEvaluate(t *testing.T) {
	dir := t.TempDir()

	hclFile := parseExpressionTestHCL(t, `
locals {
  name = file(local.path)
}
locals {
  path = "a.txt"
}
`)

	_, _, err := (&HCLParser{}).extractLocals(hclFile, map[string]function.Function{
		"file": newFileFunction(dir),
	})
	var evaluateErr *LocalEvaluationError
	require.ErrorAs(t, err, &evaluateErr)
	assert.Equal(t, "name", evaluateErr.Name)
}

// This behavior is intentional as it catches latent errors that may be reached with usage of reconfigure
func TestExtractLocalsCatchesUnevaluatedConditionalBranchError(t *testing.T) {
	hclFile := parseExpressionTestHCL(t, `
locals {
  enabled  = true
  value    = local.enabled ? "ok" : local.fallback
  fallback = local.value
}
`)

	_, _, err := (&HCLParser{}).extractLocals(hclFile, nil)

	var cyclicErr *CyclicLocalDependencyError
	require.ErrorAs(t, err, &cyclicErr)
	assert.Equal(t, []string{"value", "fallback", "value"}, cyclicErr.Cycle)
}
