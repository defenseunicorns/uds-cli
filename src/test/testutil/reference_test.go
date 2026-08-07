// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package testutil

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUniqueRepository(t *testing.T) {
	first := UniqueRepository(t, "packages/uds-cli/test")
	second := UniqueRepository(t, "packages/uds-cli/test")

	require.True(t, strings.HasPrefix(first, "packages/uds-cli/test/testuniquerepository-"))
	require.NotEqual(t, first, second)
	require.Regexp(t, regexp.MustCompile(`^[a-z0-9][a-z0-9/-]*$`), first)
}

func TestUniqueTag(t *testing.T) {
	tag := UniqueTag(t, "Bundle Test")

	require.True(t, strings.HasPrefix(tag, "bundle-test-testuniquetag-"))
	require.Regexp(t, regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`), tag)
}
