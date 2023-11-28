package tui

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/defenseunicorns/zarf/src/config"
	"github.com/defenseunicorns/zarf/src/pkg/k8s"
	"github.com/defenseunicorns/zarf/src/pkg/message"
	"github.com/defenseunicorns/zarf/src/types"
	"github.com/pterm/pterm"
)

type tickMsg time.Time
type operation string

const (
	DeployOp operation = "deploy"
	tick     operation = "tick"
)

var (
	Program *tea.Program
)

func InitModel(content string, client bndlClientShim, buf *bytes.Buffer, op operation) model {
	return model{
		content:             content,
		bndlClient:          client,
		packageOutputBuffer: buf,
		op:                  op,
		quitChan:            make(chan int),
		componentChan:       make(chan int),
		progress:            progress.New(progress.WithDefaultGradient()),
		currentPkg:          "",
		currentComponent:    1,
	}
}

// private interface to decouple tui pkg from bundle pkg
type bndlClientShim interface {
	Deploy(*bytes.Buffer) error
	ClearPaths()
}

type model struct {
	content             string
	ready               bool
	packageOutputBuffer *bytes.Buffer
	bndlClient          bndlClientShim
	op                  operation
	quitChan            chan int
	progress            progress.Model
	currentPkg          string
	numComponents       int
	componentChan       chan int
	currentComponent    int
}

func (m model) Init() tea.Cmd {
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

func GetDeployedPackage(packageName string) (deployedPackage *types.DeployedPackage) {
	// Get the secret that describes the deployed package
	k8sClient, _ := k8s.New(message.Debugf, k8s.Labels{config.ZarfManagedByLabel: "zarf"})
	secret, err := k8sClient.GetSecret("zarf", config.ZarfPackagePrefix+packageName)
	if err != nil {
		return deployedPackage
	}

	err = json.Unmarshal(secret.Data["data"], &deployedPackage)
	if err != nil {
		panic(0)
	}
	return deployedPackage
}

func finalPause() tea.Cmd {
	return tea.Tick(time.Millisecond*750, func(_ time.Time) tea.Msg {
		return nil
	})
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		cmds []tea.Cmd
	)
	select {
	case <-m.quitChan:
		return m, tea.Sequence(tea.Quit)
	default:
		switch msg := msg.(type) {
		case progress.FrameMsg:
			progressModel, cmd := m.progress.Update(msg)
			m.progress = progressModel.(progress.Model)
			return m, cmd
		case tickMsg:
			var cmd tea.Cmd
			if m.numComponents > 0 {
				deployedPkg := GetDeployedPackage(m.currentPkg)
				if deployedPkg != nil {
					if m.currentComponent == m.numComponents {
						cmd = m.progress.SetPercent(100.0)
					} else {
						m.currentComponent++
						cmd = m.progress.IncrPercent(float64(m.currentComponent) / float64(m.numComponents))
					}
				}
			}
			return m, tea.Sequence(cmd, tickCmd())
		case operation:
			m.ready = true
			m.packageOutputBuffer.Reset()

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
					m.quitChan <- 1
				}()
				// use a ticker to update the TUI while the deploy runs
				return m, tickCmd()
			}
		case tea.KeyMsg:
			if k := msg.String(); k == "ctrl+c" || k == "q" || k == "esc" {
				return m, tea.Quit
			}
		case string:
			if strings.Split(msg, ":")[0] == "package" {
				pkgName := strings.Split(msg, ":")[1]
				m.currentPkg = pkgName
			} else if strings.Split(msg, ":")[0] == "numComponents" {
				if totalComponents, err := strconv.Atoi(strings.Split(msg, ":")[1]); err == nil {
					m.numComponents = totalComponents // else return err
				}

			}
		}
	}

	return m, tea.Batch(cmds...)
}

func (m model) View() string {
	if !m.ready {
		return "\n  Initializing..."
	}
	return fmt.Sprintf("%s", m.boxView())
}

func (m model) boxView() string {
	width := 100
	question := lipgloss.NewStyle().
		Width(50).
		Align(lipgloss.Left).
		Padding(0, 3).
		Render(fmt.Sprintf("📦 Deploying: %s", m.currentPkg))

	progressBar := lipgloss.NewStyle().
		Width(50).
		Align(lipgloss.Left).
		Padding(0, 3).
		MarginTop(1).
		Render(m.progress.View())

	ui := lipgloss.JoinVertical(lipgloss.Center, question, progressBar)

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#874BFD")).
		Padding(1, 0).
		BorderTop(true).
		BorderLeft(true).
		BorderRight(true).
		BorderBottom(true)
	subtle := lipgloss.AdaptiveColor{Light: "#D9DCCF", Dark: "#383838"}

	box := lipgloss.Place(width, 9,
		lipgloss.Left, lipgloss.Top,
		boxStyle.Render(ui),
		lipgloss.WithWhitespaceForeground(subtle),
	)

	return box
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
