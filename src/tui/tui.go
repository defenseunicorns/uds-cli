package tui

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/defenseunicorns/uds-cli/src/pkg/bundle"
)

type operation string

const (
	DeployOp operation = "deploy"
	tick     operation = "tick"
)

func InitModel(content string, client *bundle.Bundler, buf *bytes.Buffer, op operation) Model {
	return Model{
		content:             content,
		bndlClient:          client,
		packageOutputBuffer: buf,
		op:                  op,
		quitChan:            make(chan int),
		progress:            progress.New(progress.WithDefaultGradient()),
	}
}

type Model struct {
	content             string
	ready               bool
	packageOutputBuffer *bytes.Buffer
	bndlClient          *bundle.Bundler
	op                  operation
	quitChan            chan int
	progress            progress.Model
}

func (m Model) Init() tea.Cmd {
	return func() tea.Msg {
		return m.op
	}
}

// allows us to get way more Zarf output
// adopted from:
// https://stackoverflow.com/questions/74375547/how-to-deal-with-log-output-which-contains-progress-bar
func cleanFlushInfo(bytesBuffer *bytes.Buffer) string {
	scanner := bufio.NewScanner(bytesBuffer)
	finalString := ""

	for scanner.Scan() {
		line := scanner.Text()
		chunks := strings.Split(line, "\r")
		lastChunk := chunks[len(chunks)-1] // fetch the last update of the line
		finalString += lastChunk + "\n"
	}
	return finalString
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		//cmd  tea.Cmd
		cmds []tea.Cmd
	)
	select {
	case <-m.quitChan:
		return m, tea.Quit
	default:
		switch msg := msg.(type) {
		case operation:
			// todo: this should probably go somewhere else
			m.content += cleanFlushInfo(m.packageOutputBuffer)
			//m.viewport.SetContent(m.content)
			m.ready = true
			m.packageOutputBuffer.Reset()

			switch msg {
			case DeployOp:
				// run Deploy concurrently so we can update the TUI while it runs

				// use a ticker to update the TUI while the deploy runs
				return m, tea.Tick(time.Millisecond, func(time.Time) tea.Msg {
					return tick
				})
			case tick:
				return m, tea.Tick(time.Millisecond, func(time.Time) tea.Msg {
					return tick
				})
			}

		case tea.KeyMsg:
			if k := msg.String(); k == "ctrl+c" || k == "q" || k == "esc" {
				return m, tea.Quit
			}
		}
	}

	return m, tea.Batch(cmds...)
}

func (m Model) View() string {
	if !m.ready {
		return "\n  Initializing..."
	}
	return fmt.Sprintf("%s", m.boxView())
}

func textInBox(text string) string {
	return lipgloss.NewStyle().MarginLeft(1).Width(100).Background(lipgloss.Color("#E81E27")).Render(text)
}

func (m Model) progressBar() string {
	m.progress.EmptyColor = "#98e695"
	return lipgloss.NewStyle().Render(m.progress.View())

}

func (m Model) boxView() string {
	boxStyle := func() lipgloss.Style {
		b := lipgloss.RoundedBorder()
		return lipgloss.NewStyle().BorderStyle(b).Width(100).BorderForeground(lipgloss.Color("#874BFD"))
	}()

	box := boxStyle.Render(lipgloss.JoinHorizontal(0.2, textInBox("📦 Deploying: podinfo"), m.progressBar()))
	return box
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
