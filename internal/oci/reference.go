// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package oci

import (
	"fmt"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
	"oras.land/oras-go/v2/registry"
)

// TrimScheme removes the scheme from a reference name
// (e.g., "oci://ghcr.io/org/repo:tag" -> "ghcr.io/org/repo:tag")
func TrimScheme(refName string) string {
	if idx := strings.Index(refName, "://"); idx >= 0 {
		return refName[idx+3:]
	}
	return refName
}

// IsOCIReference checks if a string looks like an OCI registry reference
// (e.g., "oci://ghcr.io/org/repo:tag", "ghcr.io/org/repo:tag", or "registry.example.com/image").
func IsOCIReference(s string) bool {
	if strings.HasPrefix(s, "oci://") {
		return true
	}
	if strings.Contains(s, "://") {
		return false
	}
	if strings.HasPrefix(s, "/") || strings.HasPrefix(s, "./") || strings.HasPrefix(s, "../") || strings.Contains(s, "\\") {
		return false
	}
	if strings.Contains(s, " ") {
		return false
	}
	if strings.HasPrefix(s, "localhost/") || strings.HasPrefix(s, "localhost:") {
		return strings.Contains(s, "/")
	}
	if strings.Contains(s, ".") && strings.Contains(s, "/") && (strings.Contains(s, ":") || strings.Contains(s, "@")) {
		return true
	}
	if strings.HasSuffix(s, ".hcl") || strings.Contains(s, ".tar") || strings.HasSuffix(s, ".yaml") || strings.HasSuffix(s, ".yml") {
		return false
	}
	return strings.Contains(s, ".") && strings.Contains(s, "/")
}

// ReferenceIdentifier returns the tag/digest portion of an OCI reference.
func ReferenceIdentifier(ref string) (string, error) {
	trimmed := TrimScheme(ref)
	if _, err := registry.ParseReference(trimmed); err != nil {
		return "", fmt.Errorf("%w %q: %w", ErrParseReference, ref, err)
	}
	r, err := name.ParseReference(trimmed)
	if err != nil {
		return "", fmt.Errorf("%w %q: %w", ErrParseReference, ref, err)
	}
	return r.Identifier(), nil
}
