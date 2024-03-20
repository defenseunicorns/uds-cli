// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

package deploy

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/fatih/color"
	"github.com/goccy/go-yaml/lexer"
	"github.com/goccy/go-yaml/printer"
)

const (
	LIGHTBLUE = lipgloss.Color("#4BFDEB")
	LIGHTGRAY = lipgloss.Color("#7A7A78")
)

var (
	line          string
	termWidth     int
	termHeight    int
	lightBlueText = lipgloss.NewStyle().Foreground(LIGHTBLUE)
	lightGrayText = lipgloss.NewStyle().Foreground(LIGHTGRAY)
	logMsg        = lipgloss.NewStyle().Padding(0, 3).Render(fmt.Sprintf("\n🔍  %s %s",
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
	headerMsg := fmt.Sprintf("Package %s deploy logs", m.packages[m.pkgIdx].name)
	return lipgloss.NewStyle().Padding(0, 3).Render(
		fmt.Sprintf("%s\n%s\n%s\n\n", m.logHeaderView(headerMsg), m.logViewport.View(), m.logFooterView()),
	)
}

func (m *Model) logHeaderView(msg string) string {
	title := titleStyle.Render(msg)
	headerLine := strings.Repeat("─", max(0, m.logViewport.Width-lipgloss.Width(title)))
	return lipgloss.JoinHorizontal(lipgloss.Center, title, headerLine)
}

func (m *Model) logFooterView() string {
	footerLine := strings.Repeat("─", max(0, m.logViewport.Width))
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
		if p.percLayersVerified > 0 {
			perc := lightGrayText.Render(fmt.Sprintf("%d%%", int32(p.percLayersVerified)))
			text = lipgloss.NewStyle().
				Align(lipgloss.Left).
				Padding(0, 3).
				Render(fmt.Sprintf("%s Verifying pkg %s (%s)", p.verifySpinner.View(), p.name, perc))
		}
		if p.numComponents != 0 {
			// todo: sometimes this says it's deploying 0/0 components, fix this
			text = lipgloss.NewStyle().
				Align(lipgloss.Left).
				Padding(0, 3).
				Render(fmt.Sprintf("%s Package %s deploying (%d / %d components)", p.deploySpinner.View(), p.name, min(numComponentsSuccess+1, p.numComponents), p.numComponents))
		}
		if p.complete {
			text = lipgloss.NewStyle().
				Align(lipgloss.Left).
				Padding(0, 3).
				Render(fmt.Sprintf("✅ Package %s deployed", p.name))
		}

		view = lipgloss.JoinVertical(lipgloss.Left, view, text+"\n")
	}

	return view
}

func (m *Model) preDeployView() string {
	paddingStyle := lipgloss.NewStyle().Padding(0, 3)
	header := paddingStyle.Render("🎁 BUNDLE DEFINITION")
	prompt := paddingStyle.Render("❓ Deploy this bundle? (y/n)")
	prettyYAML := paddingStyle.Render(colorPrintYAML(m.bundleYAML))
	m.yamlViewport.SetContent(prettyYAML)

	headerMsg := "Use mouse wheel to scroll"
	//return lipgloss.NewStyle().Padding(0, 3).Render(
	//	fmt.Sprintf("%s\n%s\n%s\n\n", m.logHeaderView(headerMsg), m.logViewport.View(), m.logFooterView()),
	//)

	// Concatenate header, highlighted YAML, and prompt
	return fmt.Sprintf("\n%s\n\n%s\n%s\n%s\n\n%s",
		header,
		lipgloss.NewStyle().Padding(0, 3).Render(m.logHeaderView(headerMsg)),
		lipgloss.NewStyle().Padding(0, 3).Render(m.yamlViewport.View()),
		lipgloss.NewStyle().Padding(0, 3).Render(m.logFooterView()),
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
