// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package style contains the styles for the UDS CLI and UDS Engine streaming output.
package style

import (
	"fmt"
	"image/color"
	"os"

	"charm.land/lipgloss/v2"
)

var lightDark = lipgloss.LightDark(lipgloss.HasDarkBackground(os.Stdin, os.Stdout))

func adaptiveColor(light, dark string) color.Color {
	return lightDark(lipgloss.Color(light), lipgloss.Color(dark))
}

var (
	Bold      = lipgloss.NewStyle().Bold(true)
	Underline = lipgloss.NewStyle().Underline(true)

	// styles for gray-90 tags from https://carbondesignsystem.com/elements/color/tokens/
	Gray     = lipgloss.NewStyle().Foreground(adaptiveColor("#636363", "#8d8d8d"))
	CoolGray = lipgloss.NewStyle().Foreground(adaptiveColor("#5d646a", "#878d96"))
	WarmGray = lipgloss.NewStyle().Foreground(adaptiveColor("#565151", "#8f8b8b"))
	Red      = lipgloss.NewStyle().Foreground(adaptiveColor("#c21e25", "#fa4d56"))
	Orange   = lipgloss.NewStyle().Foreground(adaptiveColor("#d67c00", "#fff3e1"))
	Yellow   = lipgloss.NewStyle().Foreground(adaptiveColor("#e3a21a", "#fff9e7"))
	Green    = lipgloss.NewStyle().Foreground(adaptiveColor("#11742f", "#24a148"))
	Teal     = lipgloss.NewStyle().Foreground(adaptiveColor("#007070", "#009d9a"))
	Cyan     = lipgloss.NewStyle().Foreground(adaptiveColor("#00539a", "#1192e8"))
	Blue     = lipgloss.NewStyle().Foreground(adaptiveColor("#0043ce", "#4589ff"))
	Purple   = lipgloss.NewStyle().Foreground(adaptiveColor("#7c3dd6", "#a56eff"))
	Magenta  = lipgloss.NewStyle().Foreground(adaptiveColor("#bf1d63", "#ee5396"))
	Pink     = lipgloss.NewStyle().Foreground(adaptiveColor("#d60093", "#ffe0f6"))
)

func RenderFmt(style lipgloss.Style, format string, a ...any) string {
	return style.Render(fmt.Sprintf(format, a...))
}
