// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package deploy contains the TUI logic for bundle deploys
package deploy

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/defenseunicorns/uds-cli/src/pkg/bundle/tui"
	"github.com/fatih/color"
	"github.com/goccy/go-yaml/lexer"
	"github.com/goccy/go-yaml/printer"
)

var (
	termWidth     int
	termHeight    int
	styledCheck   = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")).Render("✔")
	lightBlueText = lipgloss.NewStyle().Foreground(tui.LIGHTBLUE)
	lightGrayText = lipgloss.NewStyle().Foreground(tui.LIGHTGRAY)
	logMsg        = tui.IndentStyle.Render(fmt.Sprintf("\n%s %s",
		lightBlueText.Render("<l>"), lightGrayText.Render("Toggle logs")))
)

var (
	titleStyle = func() lipgloss.Style {
		b := lipgloss.RoundedBorder()
		b.Right = "├"
		return lipgloss.NewStyle().BorderStyle(b).Padding(0, 1)
	}()
)

func (m *Model) logView() string {
	headerMsg := fmt.Sprintf("%s %s", lightBlueText.Render(m.packages[m.pkgIdx].name), lightGrayText.Render("package logs"))
	return tui.IndentStyle.Render(
		fmt.Sprintf("%s\n%s\n%s\n\n", m.logHeaderView(headerMsg), m.logViewport.View(), m.logFooterView()),
	)
}

func (m *Model) yamlHeaderView() string {
	upArrow := "▲  "
	styledUpArrow := lipgloss.NewStyle().Foreground(tui.LIGHTGRAY).Render(upArrow)
	if !m.yamlViewport.AtTop() {
		styledUpArrow = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFF258")).Render(upArrow)
	}
	headerLine := strings.Repeat("─", max(0, m.logViewport.Width-lipgloss.Width(styledUpArrow)-1))
	return lipgloss.JoinHorizontal(lipgloss.Center, styledUpArrow, headerLine)
}

func (m *Model) yamlFooterView() string {
	downArrow := "▼ "
	styledDownArrow := lipgloss.NewStyle().Foreground(tui.LIGHTGRAY).Render(downArrow)
	if !m.yamlViewport.AtBottom() {
		styledDownArrow = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFF258")).Render(downArrow)

	}
	footerLine := strings.Repeat("─", max(0, m.logViewport.Width-lipgloss.Width(styledDownArrow)-1))
	return lipgloss.JoinHorizontal(lipgloss.Center, styledDownArrow, footerLine)
}

func (m *Model) logHeaderView(msg string) string {
	title := titleStyle.Render(msg)
	if msg == "" {
		title = ""
	}
	headerLine := strings.Repeat("─", max(0, m.logViewport.Width-lipgloss.Width(title)-1))
	return lipgloss.JoinHorizontal(lipgloss.Center, title, headerLine)
}

func (m *Model) logFooterView() string {
	footerLine := strings.Repeat("─", max(0, m.logViewport.Width)-1)
	return lipgloss.JoinHorizontal(lipgloss.Center, footerLine)
}

func (m *Model) deployView() string {
	view := ""
	for _, p := range m.packages {
		// count number of successful components
		numComponentsSuccess := 0
		if !p.resetProgress {
			for _, status := range p.componentStatuses {
				if status {
					numComponentsSuccess++
				}
			}
		}

		var text string
		if m.isRemoteBundle {
			text = genRemotePkgText(p, numComponentsSuccess)
		} else {
			text = genLocalPkgText(p, numComponentsSuccess)
		}

		if p.complete {
			text = tui.IndentStyle.
				Align(lipgloss.Left).
				Render(fmt.Sprintf("%s Package %s deployed", styledCheck, lightBlueText.Render(p.name)))
		}

		view = lipgloss.JoinVertical(lipgloss.Left, view, text+"\n")
	}

	return view
}

func genLocalPkgText(p pkgState, numComponentsSuccess int) string {
	text := ""
	styledName := lightBlueText.Render(p.name)
	styledComponents := lightGrayText.Render(fmt.Sprintf("(%d / %d components)", min(numComponentsSuccess+1, p.numComponents), p.numComponents))
	if p.numComponents > 0 {
		text = tui.IndentStyle.
			Align(lipgloss.Left).
			Render(fmt.Sprintf("%s Package %s deploying %s", p.deploySpinner.View(), styledName, styledComponents))
	} else {
		text = tui.IndentStyle.
			Align(lipgloss.Left).
			Render(fmt.Sprintf("%s Package %s deploying", p.deploySpinner.View(), styledName))
	}
	return text
}

func genRemotePkgText(p pkgState, numComponentsSuccess int) string {
	text := ""
	styledName := lightBlueText.Render(p.name)
	styledComponents := lightGrayText.Render(fmt.Sprintf("(%d / %d components)", min(numComponentsSuccess+1, p.numComponents), p.numComponents))
	if !p.verified {
		perc := lightGrayText.Render(fmt.Sprintf("(%d%%)", p.percLayersVerified))
		text = tui.IndentStyle.
			Align(lipgloss.Left).
			Render(fmt.Sprintf("%sVerifying %s package %s", p.verifySpinner.View(), styledName, perc))
	} else if p.verified && !p.downloaded {
		perc := lightGrayText.Render(fmt.Sprintf("(%d%%)", p.percDownloaded))
		text = tui.IndentStyle.
			Align(lipgloss.Left).
			Render(fmt.Sprintf("%sDownloading %s package %s", p.downloadSpinner.View(), styledName, perc))
	} else if p.downloaded && p.verified && p.numComponents > 0 {
		text = tui.IndentStyle.
			Align(lipgloss.Left).
			Render(fmt.Sprintf("%sDeploying %s package %s", p.deploySpinner.View(), styledName, styledComponents))
	} else {
		text = tui.IndentStyle.
			Align(lipgloss.Left).
			Render(fmt.Sprintf("%sDeploying %s package", p.deploySpinner.View(), styledName))
	}

	return text
}

func (m *Model) preDeployView() string {
	header := tui.IndentStyle.Render("📦 Bundle Definition (▲ / ▼)")
	prompt := tui.IndentStyle.Render("❓ Deploy this bundle? (y/n)")
	prettyYAML := tui.IndentStyle.Render(colorPrintYAML(m.bundleYAML))
	m.yamlViewport.SetContent(prettyYAML)

	// Concatenate header, highlighted YAML, and prompt
	return fmt.Sprintf("\n%s\n\n%s\n\n%s\n%s\n%s\n\n%s",
		m.udsTitle(),
		header,
		tui.IndentStyle.Render(m.yamlHeaderView()),
		tui.IndentStyle.Render(m.yamlViewport.View()),
		tui.IndentStyle.Render(m.yamlFooterView()),
		prompt,
	)
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Millisecond*250, func(t time.Time) tea.Msg {
		return deployTickMsg(t)
	})
}

// colorPrintYAML makes a pretty-print YAML string with color
func colorPrintYAML(yaml string) string {
	tokens := lexer.Tokenize(yaml)

	var p printer.Printer
	p.Bool = func() *printer.Property {
		return &printer.Property{
			Prefix: yamlFormat(color.FgHiWhite),
			Suffix: yamlFormat(color.Reset),
		}
	}
	p.Number = func() *printer.Property {
		return &printer.Property{
			Prefix: yamlFormat(color.FgHiWhite),
			Suffix: yamlFormat(color.Reset),
		}
	}
	p.MapKey = func() *printer.Property {
		return &printer.Property{
			Prefix: yamlFormat(color.FgHiCyan),
			Suffix: yamlFormat(color.Reset),
		}
	}
	p.Anchor = func() *printer.Property {
		return &printer.Property{
			Prefix: yamlFormat(color.FgHiYellow),
			Suffix: yamlFormat(color.Reset),
		}
	}
	p.Alias = func() *printer.Property {
		return &printer.Property{
			Prefix: yamlFormat(color.FgHiYellow),
			Suffix: yamlFormat(color.Reset),
		}
	}
	p.String = func() *printer.Property {
		return &printer.Property{
			Prefix: yamlFormat(color.FgHiMagenta),
			Suffix: yamlFormat(color.Reset),
		}
	}

	outputYAML := p.PrintTokens(tokens)
	return outputYAML
}

func yamlFormat(attr color.Attribute) string {
	const yamlEscape = "\x1b"
	return fmt.Sprintf("%s[%dm", yamlEscape, attr)
}

// udsTitle returns the title header for the UDS bundle
func (m *Model) udsTitle() string {
	styledBundleName := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFF258")).Render(m.bundleName + " ")
	title := " UDS Bundle: "
	styledTitle := lipgloss.NewStyle().Margin(0, 3).
		Padding(1, 0).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#6233f2")).
		Render(fmt.Sprintf("%s%s", title, styledBundleName))
	return styledTitle
}

// genSuccessCmds generates the success or failure messages for each package
func genSuccessCmds(m *Model) []tea.Cmd {
	var cmds []tea.Cmd
	for i := 0; i < len(m.packages); i++ {
		successMsg := fmt.Sprintf("%s Package %s deployed\n", styledCheck, lightBlueText.Render(m.packages[i].name))
		cmds = append(cmds, tea.Println(tui.IndentStyle.Render(successMsg)))
	}
	return cmds
}

func (m *Model) bundleDeployProgress() string {
	styledText := lightGrayText.Render("📦 Deploying bundle package")
	styledPkgCounter := lightGrayText.Render(fmt.Sprintf("(%d / %d)", m.pkgIdx+1, m.totalPkgs))
	msg := fmt.Sprintf("%s %s", styledText, styledPkgCounter)
	return tui.IndentStyle.Render(msg)
}
