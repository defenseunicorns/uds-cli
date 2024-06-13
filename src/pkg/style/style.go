// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package style contains the styles for the UDS CLI and UDS Engine streaming output.
package style

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

var (
	Bold      = lipgloss.NewStyle().Bold(true)
	Underline = lipgloss.NewStyle().Underline(true)

	// styles for gray-90 tags from https://carbondesignsystem.com/elements/color/tokens/
	Gray     = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#636363", Dark: "#8d8d8d"})
	CoolGray = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#5d646a", Dark: "#878d96"})
	WarmGray = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#565151", Dark: "#8f8b8b"})
	Red      = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#c21e25", Dark: "#fa4d56"})
	Orange   = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#d67c00", Dark: "#fff3e1"})
	Yellow   = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#e3a21a", Dark: "#fff9e7"})
	Green    = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#11742f", Dark: "#24a148"})
	Teal     = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#007070", Dark: "#009d9a"})
	Cyan     = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#00539a", Dark: "#1192e8"})
	Blue     = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#0043ce", Dark: "#4589ff"})
	Purple   = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#7c3dd6", Dark: "#a56eff"})
	Magenta  = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#bf1d63", Dark: "#ee5396"})
	Pink     = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#d60093", Dark: "#ffe0f6"})
)

func RenderFmt(style lipgloss.Style, format string, a ...any) string {
	return style.Render(fmt.Sprintf(format, a...))
}
