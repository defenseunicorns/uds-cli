// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"bytes"
	"fmt"
	"strings"
)

// BufferString returns a human-readable summary of the bundle as a buffer.
func (b *UDSBundle) BufferString() (*bytes.Buffer, error) {
	var out bytes.Buffer

	if _, err := fmt.Fprint(&out, "BUNDLE METADATA\n"); err != nil {
		return nil, err
	}
	if _, err := fmt.Fprintf(&out, "  Name:        %s\n", b.Metadata.Name); err != nil {
		return nil, err
	}
	if b.Metadata.Description != "" {
		if _, err := fmt.Fprintf(&out, "  Description: %s\n", b.Metadata.Description); err != nil {
			return nil, err
		}
	}
	if b.Metadata.Version != "" {
		if _, err := fmt.Fprintf(&out, "  Version:     %s\n", b.Metadata.Version); err != nil {
			return nil, err
		}
	}

	if _, err := fmt.Fprintf(&out, "\nPACKAGES (%d)\n", len(b.Packages)); err != nil {
		return nil, err
	}
	for _, pkg := range b.Packages {
		if _, err := fmt.Fprintf(&out, "  %s\n", pkg.Name); err != nil {
			return nil, err
		}
		if _, err := fmt.Fprintf(&out, "    Source: %s\n", pkg.Source); err != nil {
			return nil, err
		}
		if pkg.Namespace != "" {
			if _, err := fmt.Fprintf(&out, "    Namespace: %s\n", pkg.Namespace); err != nil {
				return nil, err
			}
		}
		if len(pkg.DependsOn) > 0 {
			if _, err := fmt.Fprintf(&out, "    DependsOn: %s\n", strings.Join(pkg.DependsOn, ", ")); err != nil {
				return nil, err
			}
		}
		if len(pkg.ValueFiles) > 0 {
			if _, err := fmt.Fprintf(&out, "    Value Files: %s\n", strings.Join(pkg.ValueFiles, ", ")); err != nil {
				return nil, err
			}
		}
		if _, err := out.WriteString("\n"); err != nil {
			return nil, err
		}
	}

	return &out, nil
}
