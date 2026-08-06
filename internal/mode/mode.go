// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package mode selects the UDS CLI implementation.
package mode

import "fmt"

// Mode identifies a complete UDS CLI command tree.
type Mode string

const (
	// Legacy selects the current UDS CLI.
	Legacy Mode = "legacy"
	// Next selects UDS CLI Next.
	Next Mode = "next"
)

func (m Mode) String() string {
	return string(m)
}

func parse(value string) (Mode, error) {
	switch Mode(value) {
	case Legacy:
		return Legacy, nil
	case Next:
		return Next, nil
	default:
		return "", fmt.Errorf("invalid CLI mode %q, expected legacy or next", value)
	}
}
