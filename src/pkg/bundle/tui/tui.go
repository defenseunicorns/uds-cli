package tui

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils"
	"github.com/defenseunicorns/zarf/src/config"
	"github.com/defenseunicorns/zarf/src/pkg/k8s"
	"github.com/defenseunicorns/zarf/src/pkg/message"
	"github.com/defenseunicorns/zarf/src/types"
)

// todo: watch naming collisions, spinner also has a TickMsg
type tickMsg time.Time
type operation string

const (
	DeployOp operation = "deploy"
)

var (
	Program       *tea.Program
	resetProgress bool
)

// private interface to decouple tui pkg from bundle pkg
type bndlClientShim interface {
	Deploy() error
	ClearPaths()
}

type model struct {
	bndlClient      bndlClientShim
	completeChan    chan int
	fatalChan       chan error
	progress        progress.Model
	currentPkg      string
	totalComponents int
	componentChan   chan int
	spinner         spinner.Model
	confirmed       bool
	complete        bool
}

func InitModel(client bndlClientShim) model {
	// configure spinner
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	var confirmed bool
	if config.CommonOptions.Confirm {
		confirmed = true
	}

	return model{
		bndlClient:    client,
		completeChan:  make(chan int),
		fatalChan:     make(chan error),
		componentChan: make(chan int),
		progress:      progress.New(progress.WithDefaultGradient()),
		currentPkg:    "",
		spinner:       s,
		confirmed:     confirmed,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Sequence(func() tea.Msg {
		return DeployOp
	}, m.spinner.Tick)
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

// todo: I think Zarf has this...
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
	return tea.Tick(time.Second*1, func(_ time.Time) tea.Msg {
		return nil
	})
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		cmds []tea.Cmd
	)
	select {
	case <-m.completeChan:
		m.complete = true
		return m, tea.Sequence(tickCmd())
	case <-m.fatalChan:
		m.spinner.Spinner.Frames = []string{""}
		m.spinner.Style = lipgloss.NewStyle().SetString("❌")
		s, spinnerCmd := m.spinner.Update(spinner.TickMsg{})
		m.spinner = s
		return m, tea.Sequence(spinnerCmd, finalPause(), tea.Quit)
	default:
		switch msg := msg.(type) {
		case progress.FrameMsg:
			progressModel, cmd := m.progress.Update(msg)
			m.progress = progressModel.(progress.Model)
			return m, cmd
		case tickMsg:
			var progressCmd tea.Cmd

			if m.complete {
				progressCmd = m.progress.SetPercent(100)
				m.spinner.Spinner.Frames = []string{""}
				m.spinner.Style = lipgloss.NewStyle().SetString("✅")
				s, spinnerCmd := m.spinner.Update(spinner.TickMsg{})
				m.spinner = s
				return m, tea.Sequence(progressCmd, spinnerCmd, finalPause(), tea.Quit)
			}

			if m.totalComponents > 0 {
				deployedPkg := GetDeployedPackage(m.currentPkg)
				if deployedPkg != nil && !resetProgress {
					// todo: instead of going off of DeployedComponents, find a way to include deployedPkg.DeployedComponents[0].Status
					progressCmd = m.progress.SetPercent(float64(len(deployedPkg.DeployedComponents)) / float64(m.totalComponents))
					if m.progress.Percent() == 1 {
						// todo: instead of going off percentage, go off successful deployment of the pkg
						// stop the spinner and show success
						m.spinner.Spinner.Frames = []string{""}
						m.spinner.Style = lipgloss.NewStyle().SetString("✅")
					}
				} else {
					// handle upgrade scenario by resetting the progress bar until DeployedComponents is back to 1 (ie. the first component)
					progressCmd = m.progress.SetPercent(0)
					if deployedPkg != nil && len(deployedPkg.DeployedComponents) >= 1 {
						resetProgress = false
					}
				}
			}
			// must send a spinner.TickMsg to the spinner to keep it spinning
			s, spinnerCmd := m.spinner.Update(spinner.TickMsg{})
			m.spinner = s

			return m, tea.Sequence(progressCmd, spinnerCmd, tickCmd())

		case tea.KeyMsg:
			switch msg.String() {
			case "y", "Y":
				m.confirmed = true
				return m, func() tea.Msg {
					return DeployOp
				}
			case "n", "N":
				m.confirmed = false
			case "ctrl+c", "q":
				return m, tea.Quit
			}

		case operation:
			if m.confirmed {
				go func() {
					// if something goes wrong in Deploy(), reset the terminal
					defer utils.GracefulPanic()
					// run Deploy concurrently so we can update the TUI while it runs
					if err := m.bndlClient.Deploy(); err != nil {
						m.bndlClient.ClearPaths()
						m.fatalChan <- fmt.Errorf("failed to deploy bundle: %s", err.Error())
					} else {
						m.completeChan <- 1
					}
				}()
				// use a ticker to update the TUI during deployment
				return m, tickCmd()
			}

		case string:
			if strings.Split(msg, ":")[0] == "package" {
				pkgName := strings.Split(msg, ":")[1]
				m.currentPkg = pkgName
				// if pkg is already deployed, set resetProgress to true
				if deployedPkg := GetDeployedPackage(pkgName); deployedPkg != nil && len(deployedPkg.DeployedComponents) != 0 {
					resetProgress = true
				}
			} else if strings.Split(msg, ":")[0] == "totalComponents" {
				if totalComponents, err := strconv.Atoi(strings.Split(msg, ":")[1]); err == nil {
					m.totalComponents = totalComponents // else return err
				}
			}
		}
	}

	return m, tea.Batch(cmds...)
}

func (m model) View() string {
	if m.confirmed {
		return fmt.Sprintf("%s\n%s", m.preDeployView(), m.deployView())

	} else {
		return m.preDeployView()
	}
}

func (m model) deployView() string {
	width := 100 // todo: make dynamic?
	text := lipgloss.NewStyle().
		Width(50).
		Align(lipgloss.Left).
		Padding(0, 3).
		Render(fmt.Sprintf("%s Package %s deploying ...", m.spinner.View(), m.currentPkg))

	// render progress bar until deployment is complete
	progressBar := lipgloss.NewStyle().
		Width(50).
		Align(lipgloss.Left).
		Padding(0, 3).
		MarginTop(1).
		Render(m.progress.View())
	ui := lipgloss.JoinVertical(lipgloss.Center, text, progressBar)

	if m.complete {
		text = lipgloss.NewStyle().
			Width(50).
			Align(lipgloss.Left).
			Padding(0, 3).
			Render(fmt.Sprintf("%s Package %s deployed", m.spinner.View(), m.currentPkg))
		ui = lipgloss.JoinVertical(lipgloss.Center, text)
	}

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

func (m model) preDeployView() string {
	header := "🎁 BUNDLE DEFINITION"
	yaml := `
# Example YAML
name: my-bundle
version: 1.0.0
dependencies:
  - dependency1
  - dependency2
`

	prompt := "Deploy this bundle? (Y/N): "

	// Concatenate header, highlighted YAML, and prompt
	return fmt.Sprintf("%s\n%s\n%s", header, yaml, prompt)
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}
