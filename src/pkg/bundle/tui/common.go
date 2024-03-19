// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func Pause() tea.Cmd {
	return tea.Tick(time.Millisecond*500, func(_ time.Time) tea.Msg {
		return nil
	})
}
