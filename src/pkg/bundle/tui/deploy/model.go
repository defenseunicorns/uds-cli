// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package deploy contains the TUI logic for bundle deploys
package deploy

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/bundle/tui"
	"github.com/defenseunicorns/zarf/src/pkg/cluster"
	"golang.org/x/term"
)

type deployTickMsg time.Time
type deployOp string
type packageOp string

const (
	doDeploy        deployOp  = "deploy"
	doPreDeploy     deployOp  = "preDeploy"
	newPackage      packageOp = "newPackage"
	totalComponents packageOp = "totalComponents"
	totalPackages   packageOp = "totalPackages"
	complete        packageOp = "complete"
	verifying       packageOp = "verifying"
	downloading     packageOp = "downloading"
)

var (
	// Program is the Bubbletea TUI main program
	Program          *tea.Program
	c                *cluster.Cluster
	logVpWidthScale  = 0.9
	logVpHeightScale = 0.4
)

// private interface to decouple tui pkg from bundle pkg
type bndlClientShim interface {
	Deploy() error
	PreDeployValidation() (string, string, string, error)
	ClearPaths()
}

// pkgState contains the state of the pkg as its deploying
type pkgState struct {
	name               string
	numComponents      int
	percLayersVerified int
	componentStatuses  []bool
	deploySpinner      spinner.Model
	downloadSpinner    spinner.Model
	verifySpinner      spinner.Model
	complete           bool
	resetProgress      bool
	percDownloaded     int
	downloaded         bool
	verified           bool
}

// Model contains the state of the TUI
type Model struct {
	bndlClient              bndlClientShim
	bundleYAML              string
	doneChan                chan int
	pkgIdx                  int
	totalPkgs               int
	confirmed               bool
	done                    bool
	packages                []pkgState
	deploying               bool
	inProgress              bool
	viewLogs                bool
	logViewport             viewport.Model
	errChan                 chan error
	yamlViewport            viewport.Model
	isRemoteBundle          bool
	bundleName              string
	validatingBundle        bool
	validatingBundleSpinner spinner.Model
}

// InitModel initializes the model for the TUI
func InitModel(client bndlClientShim) Model {
	var confirmed bool
	var inProgress bool
	var isRemoteBundle bool
	if config.CommonOptions.Confirm {
		confirmed = true
		inProgress = true
	}

	// create spinner to track bundle validation
	validatingBundleSpinner := spinner.New()
	validatingBundleSpinner.Spinner = spinner.Ellipsis
	validatingBundleSpinner.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	// create cluster client for querying packages during deployment
	c, _ = cluster.NewCluster()

	// set termWidth and line length based on window size
	termWidth, termHeight, _ = term.GetSize(0)

	// make log viewport scale dynamic based on termHeight to prevent weird artifacts
	if termHeight < 30 {
		logVpHeightScale = 0.3
	} else {
		logVpHeightScale = 0.4
	}

	// set up logViewport for logs, adjust width and height of logViewport
	logViewport := viewport.New(int(float64(termWidth)*logVpWidthScale), int(float64(termHeight)*logVpHeightScale))

	// set up yamlViewport to ensure the preDeploy YAML is scrollable
	numYAMLLines := 10
	yamlViewport := viewport.New(termWidth, numYAMLLines)

	return Model{
		bndlClient:              client,
		doneChan:                make(chan int),
		errChan:                 make(chan error),
		confirmed:               confirmed,
		inProgress:              inProgress,
		logViewport:             logViewport,
		yamlViewport:            yamlViewport,
		isRemoteBundle:          isRemoteBundle,
		validatingBundleSpinner: validatingBundleSpinner,
		validatingBundle:        true,
	}
}

// Init performs some action when BubbleTea starts up
func (m *Model) Init() tea.Cmd {
	return func() tea.Msg {
		return doPreDeploy
	}
}

// Update updates the model based on the message received
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

		// handle changes in window size
		case tea.WindowSizeMsg:
			termWidth = msg.Width
			termHeight = msg.Height

			// make log viewport scale dynamic based on termHeight to prevent weird artifacts
			if termHeight < 30 {
				logVpHeightScale = 0.3
			} else {
				logVpHeightScale = 0.4
			}
			m.logViewport.Width = int(float64(termWidth) * logVpWidthScale)
			m.logViewport.Height = int(float64(termHeight) * logVpHeightScale)

		// spin the spinners
		case spinner.TickMsg:
			var spinDeploy, spinVerify, spinDownload, spinValidateBundle tea.Cmd
			if len(m.packages) > m.pkgIdx {
				m.packages[m.pkgIdx].deploySpinner, spinDeploy = m.packages[m.pkgIdx].deploySpinner.Update(msg)
				m.packages[m.pkgIdx].verifySpinner, spinVerify = m.packages[m.pkgIdx].verifySpinner.Update(msg)
				m.packages[m.pkgIdx].downloadSpinner, spinDownload = m.packages[m.pkgIdx].downloadSpinner.Update(msg)
			} else {
				m.validatingBundleSpinner, spinValidateBundle = m.validatingBundleSpinner.Update(msg)
			}
			return m, tea.Batch(spinDeploy, spinVerify, spinDownload, spinValidateBundle)

		// handle ticks
		case deployTickMsg:
			return m.handleDeployTick()

		// handle key presses
		case tea.KeyMsg:
			switch msg.String() {
			case "y", "Y":
				if !m.validatingBundle && !m.confirmed {
					m.confirmed = true
					m.inProgress = true
				}
				return m, func() tea.Msg {
					return doDeploy
				}

			case "n", "N":
				if !m.validatingBundle && !m.confirmed && !m.inProgress {
					m.done = true
					quitMsg := tea.Println(tui.IndentStyle.Render("\n👋 Deployment cancelled"))
					return m, tea.Sequence(quitMsg, tea.Println(), tea.Quit)
				}
			case "ctrl+c", "q":
				return m, tea.Sequence(tea.Quit)

			case "up":
				if !m.confirmed {
					m.yamlViewport.LineUp(1)
				}
			case "down":
				if !m.confirmed {
					m.yamlViewport.LineDown(1)
				}

			case "l", "L":
				if m.inProgress && !m.viewLogs {
					m.viewLogs = true
				} else if m.inProgress {
					m.viewLogs = false
				}
			}

		// handle deploy
		case deployOp:
			switch msg {
			case doDeploy:
				cmd := m.handleDeploy()
				return m, cmd
			case doPreDeploy:
				cmd := m.handlePreDeploy()
				return m, tea.Sequence(m.validatingBundleSpinner.Tick, cmd)
			}

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
					if tc, err := strconv.Atoi(strings.Split(msg, ":")[1]); err == nil {
						m.packages[m.pkgIdx].numComponents = tc
						m.packages[m.pkgIdx].componentStatuses = make([]bool, tc)
						if m.isRemoteBundle {
							m.packages[m.pkgIdx].downloaded = true
						}
					}
				case totalPackages:
					if totalPkgs, err := strconv.Atoi(strings.Split(msg, ":")[1]); err == nil {
						m.totalPkgs = totalPkgs
					}
				case verifying:
					if perc, err := strconv.Atoi(strings.Split(msg, ":")[1]); err == nil {
						m.packages[m.pkgIdx].percLayersVerified = perc
						if perc == 100 {
							m.packages[m.pkgIdx].verified = true
						}
					}
				case downloading:
					if perc, err := strconv.Atoi(strings.Split(msg, ":")[1]); err == nil {
						m.packages[m.pkgIdx].percDownloaded = perc
						if perc == 100 {
							m.packages[m.pkgIdx].downloaded = true
						}
					}
				case complete:
					m.packages[m.pkgIdx].complete = true
				}
			}
		}
	}

	return m, nil
}

// View returns the view for the TUI
func (m *Model) View() string {
	if m.done {
		// no errors, clear the controlled Program's output
		return ""
	} else if m.validatingBundle {
		validatingBundleMsg := lightGrayText.Render("Validating bundle")
		return tui.IndentStyle.Render(fmt.Sprintf("\n%s %s", validatingBundleMsg, m.validatingBundleSpinner.View()))
	} else if m.viewLogs {
		return fmt.Sprintf("\n%s\n\n%s\n%s\n\n%s\n", m.udsTitle(), m.bundleDeployProgress(), logMsg, m.logView())
	} else if m.confirmed {
		return fmt.Sprintf("\n%s\n\n%s\n%s\n%s\n", m.udsTitle(), m.bundleDeployProgress(), logMsg, m.deployView())
	}
	return fmt.Sprintf("%s\n", m.preDeployView())
}
