package tui

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/defenseunicorns/zarf/src/pkg/message"
	"github.com/pterm/pterm"

	"github.com/defenseunicorns/uds-cli/src/pkg/bundle"
)

type operation string

const (
	DeployOp operation = "deploy"
	tick     operation = "tick"
)

// You generally won't need this unless you're processing stuff with
// complicated ANSI escape sequences. Turn it on if you notice flickering.
//
// Also keep in mind that high performance rendering only works for programs
// that use the full size of the terminal. We're enabling that below with
// tea.EnterAltScreen().
const useHighPerformanceRenderer = false

var (
	titleStyle = func() lipgloss.Style {
		b := lipgloss.RoundedBorder()
		b.Right = "├"
		return lipgloss.NewStyle().BorderStyle(b).Padding(0, 1)
	}()

	infoStyle = func() lipgloss.Style {
		b := lipgloss.RoundedBorder()
		b.Left = "┤"
		return titleStyle.Copy().BorderStyle(b)
	}()
)

func InitModel(content string, client *bundle.Bundler, buf *bytes.Buffer, op operation) Model {
	return Model{
		content:             content,
		viewport:            viewport.Model{},
		bndlClient:          client,
		packageOutputBuffer: buf,
		op:                  op,
		quitChan:            make(chan int),
	}
}

type Model struct {
	content             string
	ready               bool
	viewport            viewport.Model
	packageOutputBuffer *bytes.Buffer
	bndlClient          *bundle.Bundler
	op                  operation
	quitChan            chan int
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
		cmd  tea.Cmd
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
			m.viewport.SetContent(m.content)
			m.ready = true
			m.packageOutputBuffer.Reset()
			// autoscroll the contents of the viewport
			//m.viewport.GotoBottom()

			switch msg {
			case DeployOp:
				// run Deploy concurrently so we can update the TUI while it runs
				go func() {
					// todo: don't actually put the buffer in the call to Deploy()
					if err := m.bndlClient.Deploy(m.packageOutputBuffer); err != nil {
						// use existing Zarf pterm things for errors
						pterm.EnableOutput()
						pterm.SetDefaultOutput(os.Stderr)
						m.bndlClient.ClearPaths()
						message.Fatalf(err, "Failed to deploy bundle: %s", err.Error())
						m.quitChan <- 1
					}
					//m.quitChan <- 1
				}()
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

		case tea.WindowSizeMsg:
			headerHeight := lipgloss.Height(m.headerView())
			footerHeight := lipgloss.Height(m.footerView())
			verticalMarginHeight := headerHeight + footerHeight

			if !m.ready {
				// Since this program is using the full size of the viewport we
				// need to wait until we've received the window dimensions before
				// we can initialize the viewport. The initial dimensions come in
				// quickly, though asynchronously, which is why we wait for them
				// here.
				m.viewport = viewport.New(msg.Width, msg.Height-verticalMarginHeight)
				m.viewport.YPosition = headerHeight
				m.viewport.HighPerformanceRendering = useHighPerformanceRenderer
				m.viewport.SetContent(m.content)
				m.ready = true

				// This is only necessary for high performance rendering, which in
				// most cases you won't need.
				//
				// Render the viewport one line below the header.
				m.viewport.YPosition = headerHeight + 1
			} else {
				m.viewport.Width = msg.Width
				m.viewport.Height = msg.Height - verticalMarginHeight
			}

			if useHighPerformanceRenderer {
				// Render (or re-render) the whole viewport. Necessary both to
				// initialize the viewport and when the window is resized.
				//
				// This is needed for high-performance rendering only.
				cmds = append(cmds, viewport.Sync(m.viewport))
			}
		}
	}

	// Handle keyboard and mouse events in the viewport
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m Model) View() string {
	if !m.ready {
		return "\n  Initializing..."
	}
	return fmt.Sprintf("%s\n%s\n%s", m.headerView(), m.viewport.View(), m.footerView())
}

func (m Model) headerView() string {
	title := titleStyle.Render("📦 Deploying: podinfo")
	line := strings.Repeat("─", max(0, m.viewport.Width-lipgloss.Width(title)))
	return lipgloss.JoinHorizontal(lipgloss.Center, title, line)
}

func (m Model) footerView() string {
	info := infoStyle.Render(fmt.Sprintf("%3.f%%", m.viewport.ScrollPercent()*100))
	line := strings.Repeat("─", max(0, m.viewport.Width-lipgloss.Width(info)))
	return lipgloss.JoinHorizontal(lipgloss.Center, line, info)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
