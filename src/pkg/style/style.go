// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package style contains the styles for the UDS CLI and UDS Engine streaming output.
package style

import (
	"fmt"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/compat"
)

var (
	Bold      = lipgloss.NewStyle().Bold(true)
	Underline = lipgloss.NewStyle().Underline(true)

	// styles for gray-90 tags from https://carbondesignsystem.com/elements/color/tokens/
	Gray     = lipgloss.NewStyle().Foreground(compat.AdaptiveColor{Light: lipgloss.Color("#636363"), Dark: lipgloss.Color("#8d8d8d")})
	CoolGray = lipgloss.NewStyle().Foreground(compat.AdaptiveColor{Light: lipgloss.Color("#5d646a"), Dark: lipgloss.Color("#878d96")})
	WarmGray = lipgloss.NewStyle().Foreground(compat.AdaptiveColor{Light: lipgloss.Color("#565151"), Dark: lipgloss.Color("#8f8b8b")})
	Red      = lipgloss.NewStyle().Foreground(compat.AdaptiveColor{Light: lipgloss.Color("#c21e25"), Dark: lipgloss.Color("#fa4d56")})
	Orange   = lipgloss.NewStyle().Foreground(compat.AdaptiveColor{Light: lipgloss.Color("#d67c00"), Dark: lipgloss.Color("#fff3e1")})
	Yellow   = lipgloss.NewStyle().Foreground(compat.AdaptiveColor{Light: lipgloss.Color("#e3a21a"), Dark: lipgloss.Color("#fff9e7")})
	Green    = lipgloss.NewStyle().Foreground(compat.AdaptiveColor{Light: lipgloss.Color("#11742f"), Dark: lipgloss.Color("#24a148")})
	Teal     = lipgloss.NewStyle().Foreground(compat.AdaptiveColor{Light: lipgloss.Color("#007070"), Dark: lipgloss.Color("#009d9a")})
	Cyan     = lipgloss.NewStyle().Foreground(compat.AdaptiveColor{Light: lipgloss.Color("#00539a"), Dark: lipgloss.Color("#1192e8")})
	Blue     = lipgloss.NewStyle().Foreground(compat.AdaptiveColor{Light: lipgloss.Color("#0043ce"), Dark: lipgloss.Color("#4589ff")})
	Purple   = lipgloss.NewStyle().Foreground(compat.AdaptiveColor{Light: lipgloss.Color("#7c3dd6"), Dark: lipgloss.Color("#a56eff")})
	Magenta  = lipgloss.NewStyle().Foreground(compat.AdaptiveColor{Light: lipgloss.Color("#bf1d63"), Dark: lipgloss.Color("#ee5396")})
	Pink     = lipgloss.NewStyle().Foreground(compat.AdaptiveColor{Light: lipgloss.Color("#d60093"), Dark: lipgloss.Color("#ffe0f6")})
)

func RenderFmt(style lipgloss.Style, format string, a ...any) string {
	return style.Render(fmt.Sprintf(format, a...))
}
