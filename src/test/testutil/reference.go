// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package testutil

import (
	"crypto/rand"
	"encoding/hex"
	"regexp"
	"strings"
	"testing"
)

var invalidReferenceCharacters = regexp.MustCompile(`[^a-z0-9]+`)

// UniqueRepository returns a repository nested below base that is unique to the test run.
func UniqueRepository(t *testing.T, base string) string {
	t.Helper()

	return strings.TrimSuffix(strings.Trim(base, "/"), "/") + "/" + uniqueReferencePart(t, t.Name())
}

// UniqueTag returns a tag with a readable prefix and a random suffix.
func UniqueTag(t *testing.T, prefix string) string {
	t.Helper()

	return uniqueReferencePart(t, prefix+"-"+t.Name())
}

func uniqueReferencePart(t *testing.T, value string) string {
	t.Helper()

	value = strings.Trim(invalidReferenceCharacters.ReplaceAllString(strings.ToLower(value), "-"), "-")
	if value == "" {
		value = "test"
	}

	random := make([]byte, 6)
	if _, err := rand.Read(random); err != nil {
		t.Fatalf("generate unique registry reference: %v", err)
	}
	return value + "-" + hex.EncodeToString(random)
}
