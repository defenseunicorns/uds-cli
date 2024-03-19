// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

package deploy

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/zarf/src/pkg/cluster"
	"golang.org/x/term"
)

type deployTickMsg time.Time
type deployOp string
type packageOp string

const (
	doDeploy        deployOp  = "deploy"
	newPackage      packageOp = "newPackage"
	totalComponents packageOp = "totalComponents"
	totalPackages   packageOp = "totalPackages"
	complete        packageOp = "complete"
	verified        packageOp = "verified"
)

var (
	Program          *tea.Program
	c                *cluster.Cluster
	logVpWidthScale  = 0.9
	logVpHeightScale = 0.4
	lineWidthScale   = 0.75
)

// private interface to decouple tui pkg from bundle pkg
type bndlClientShim interface {
	Deploy() error
	ClearPaths()
}

// pkgState contains the state of the pkg as its deploying
type pkgState struct {
	name              string
	numComponents     int
	componentStatuses []bool
	spinner           spinner.Model
	complete          bool
	resetProgress     bool
	progress          progress.Model
}

type Model struct {
	bndlClient   bndlClientShim
	bundleYAML   string
	doneChan     chan int
	pkgIdx       int
	totalPkgs    int
	confirmed    bool
	done         bool
	packages     []pkgState
	deploying    bool
	inProgress   bool
	viewLogs     bool
	logViewport  viewport.Model
	isScrolling  bool
	errChan      chan error
	yamlViewport viewport.Model
}

func InitModel(client bndlClientShim, bundleYAML string) Model {
	var confirmed bool
	var inProgress bool
	if config.CommonOptions.Confirm {
		confirmed = true
		inProgress = true
	}

	// create cluster client for querying packages during deployment
	c, _ = cluster.NewCluster()

	// set termWidth and line length based on window size
	termWidth, termHeight, _ = term.GetSize(0)
	line = lipgloss.NewStyle().Padding(0, 3).Render(strings.Repeat("─", int(float64(termWidth)*lineWidthScale)))

	// set up logViewport for logs, adjust width and height of logViewport
	logViewport := viewport.New(int(float64(termWidth)*logVpWidthScale), int(float64(termHeight)*logVpHeightScale))
	logViewport.MouseWheelEnabled = true
	logViewport.MouseWheelDelta = 1

	// set up yamlViewport to ensure the preDeploy YAML is scrollable
	numYamlLines := 10
	yamlViewport := viewport.New(termWidth, numYamlLines)
	yamlViewport.MouseWheelEnabled = true
	yamlViewport.MouseWheelDelta = 1

	return Model{
		bndlClient:   client,
		doneChan:     make(chan int),
		errChan:      make(chan error),
		confirmed:    confirmed,
		bundleYAML:   bundleYAML,
		inProgress:   inProgress,
		logViewport:  logViewport,
		yamlViewport: yamlViewport,
	}
}

func (m *Model) Init() tea.Cmd {
	return func() tea.Msg {
		return doDeploy
	}
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	select {
	case err := <-m.errChan:
		cmd := m.handleDone(err)
		return m, cmd
	case <-m.doneChan:
		cmd := m.handleDone(nil)
		return m, cmd

	default:
		switch msg := msg.(type) {
		// FrameMsg is sent when the progress bar wants to animate itself
		case progress.FrameMsg:
			if len(m.packages) > m.pkgIdx {
				progressModel, cmd := m.packages[m.pkgIdx].progress.Update(msg)
				m.packages[m.pkgIdx].progress = progressModel.(progress.Model)
				return m, cmd
			}
		// handle changes in window size
		case tea.WindowSizeMsg:
			termWidth = msg.Width
			termHeight = msg.Height
			line = lipgloss.NewStyle().Padding(0, 3).Render(strings.Repeat("─", int(float64(termWidth)*lineWidthScale)))
			m.logViewport.Width = int(float64(termWidth) * logVpWidthScale)
			m.logViewport.Height = int(float64(termHeight) * logVpHeightScale)

		// handle mouse events
		case tea.MouseMsg:
			m.isScrolling = true
			m.logViewport, _ = m.logViewport.Update(msg)
			m.yamlViewport, _ = m.yamlViewport.Update(msg)

		// handle spinner
		case spinner.TickMsg:
			var cmd tea.Cmd
			m.packages[m.pkgIdx].spinner, cmd = m.packages[m.pkgIdx].spinner.Update(msg)
			return m, cmd

		// handle ticks
		case deployTickMsg:
			return m.handleDeployTick()

		// handle key presses
		case tea.KeyMsg:
			switch msg.String() {
			case "y", "Y":
				if !m.confirmed {
					m.confirmed = true
					m.inProgress = true
				}
				return m, func() tea.Msg {
					return doDeploy
				}

			case "n", "N":
				if !m.confirmed && !m.inProgress {
					m.done = true
					quitMsg := tea.Println("\n👋 Deployment cancelled")
					return m, tea.Sequence(quitMsg, tea.Println(), tea.Quit)
				}
			case "ctrl+c", "q":
				return m, tea.Quit

			case "l", "L":
				if m.inProgress && !m.viewLogs {
					m.viewLogs = true
					m.isScrolling = false
				} else if m.inProgress {
					m.viewLogs = false
				}
			}

		// handle deploy
		case deployOp:
			cmd := m.handleDeploy()
			return m, cmd

		// handle package updates
		case string:
			if strings.Contains(msg, ":") {
				switch packageOp(strings.Split(msg, ":")[0]) {
				case newPackage:
					pkgName := strings.Split(msg, ":")[1]
					pkgIdx, _ := strconv.Atoi(strings.Split(msg, ":")[2])
					cmd := m.handleNewPackage(pkgName, pkgIdx)
					return m, cmd
				case totalComponents:
					if totalComponents, err := strconv.Atoi(strings.Split(msg, ":")[1]); err == nil {
						m.packages[m.pkgIdx].numComponents = totalComponents
						m.packages[m.pkgIdx].componentStatuses = make([]bool, totalComponents)
					}
				case totalPackages:
					if totalPkgs, err := strconv.Atoi(strings.Split(msg, ":")[1]); err == nil {
						m.totalPkgs = totalPkgs
					}
				case verified:
					// update progress bar
					if perc, err := strconv.ParseFloat(strings.Split(msg, ":")[1], 64); err == nil {
						cmd := m.packages[m.pkgIdx].progress.SetPercent(perc)
						return m, cmd
					}
				case complete:
					m.packages[m.pkgIdx].complete = true
				}
			}
		}
	}

	return m, nil
}

func (m *Model) View() string {
	if m.done {
		// no errors, clear the controlled Program's output
		return ""
	} else if m.viewLogs {
		return fmt.Sprintf("%s\n\n%s\n", logMsg, m.logView())
	} else if m.confirmed {
		return fmt.Sprintf("%s\n%s\n", logMsg, m.deployView())
	} else {
		return fmt.Sprintf("%s\n", m.preDeployView())
	}
}
