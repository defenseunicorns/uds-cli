// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package tui contains logic for the TUI operations
package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	// LIGHTBLUE is the common light blue color used in the TUI
	LIGHTBLUE = lipgloss.Color("#4BFDEB")

	// LIGHTGRAY is the common light gray color used in the TUI
	LIGHTGRAY = lipgloss.Color("#7A7A78")
)

var (
	// IndentStyle is the style for indenting text
	IndentStyle = lipgloss.NewStyle().Padding(0, 4)
)

// Pause pauses the TUI for a short period of time
func Pause() tea.Cmd {
	return tea.Tick(time.Millisecond*500, func(_ time.Time) tea.Msg {
		return nil
	})
}
