// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

package deploy

import (
	"fmt"
	"os"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/bundle/tui"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils"
	zarfTypes "github.com/defenseunicorns/zarf/src/types"
)

func (m *Model) handleNewPackage(pkgName string, currentPkgIdx int) tea.Cmd {
	// see if pkg has already been deployed
	deployedPkg, _ := c.GetDeployedPackage(pkgName)
	newPkg := pkgState{
		name: pkgName,
	}

	// upgrade scenario, reset component progress
	if deployedPkg != nil {
		newPkg.resetProgress = true
	}

	// finish creating newPkg and start the spinner
	newPkg.progress = progress.New(progress.WithDefaultGradient())
	m.pkgIdx = currentPkgIdx
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	newPkg.spinner = s
	m.packages = append(m.packages, newPkg)
	return m.packages[m.pkgIdx].spinner.Tick
}

func (m *Model) handleDeploy() tea.Cmd {
	// ensure bundle deployment is confirmed and is only being deployed once
	if m.confirmed && !m.deploying {
		// run Deploy concurrently so we can update the TUI while it runs
		deployCmd := func() tea.Msg {
			// if something goes wrong in Deploy(), reset the terminal
			defer utils.GracefulPanic()

			if err := m.bndlClient.Deploy(); err != nil {
				m.bndlClient.ClearPaths()
				m.errChan <- err
			}
			return nil
		}
		m.deploying = true

		// use a ticker to update the TUI during deployment
		return tea.Batch(tickCmd(), deployCmd)
	}
	return nil
}

func (m *Model) handleDone(err error) tea.Cmd {
	var cmds []tea.Cmd
	m.done = true // remove the current view
	cmds = append(cmds, genSuccessOrFailCmds(m)...)
	if err != nil {
		hint := lightBlueText.Render("uds logs")
		errMsg := lipgloss.NewStyle().Padding(0, 3).Render(fmt.Sprintf("\n❌ Error deploying bundle: %s\n\nRun %s to view deployment logs", lightGrayText.Render(err.Error()), hint) + "\n")
		cmds = []tea.Cmd{tea.Println(errMsg)}
	}
	cmds = append(cmds, tui.Pause(), tea.Quit)
	return tea.Sequence(cmds...)
}

func (m *Model) handleDeployTick() (tea.Model, tea.Cmd) {
	// check if all pkgs are complete
	numComplete := 0
	if len(m.packages) == m.totalPkgs {
		for _, p := range m.packages {
			if !p.complete {
				break
			}
			numComplete++
		}
	}

	// check if last pkg is complete
	if numComplete == m.totalPkgs {
		return m, func() tea.Msg {
			m.doneChan <- 1
			return nil
		}
	}

	// update component progress
	for i, p := range m.packages {
		if p.complete {
			continue
		}
		deployedPkg, _ := c.GetDeployedPackage(p.name)
		// if deployedPkg is nil, the package hasn't been deployed yet
		if deployedPkg == nil {
			break
		}
		// handle upgrade scenario by resetting the progress bar, otherwise increment it
		if p.resetProgress {
			// if upgraded len(deployedPkg.DeployedComponents) will be equal to the number of components in the package
			if deployedPkg != nil && len(deployedPkg.DeployedComponents) == 1 {
				m.packages[i].resetProgress = false
			}
			break
		}
		// check component progress
		for j := range deployedPkg.DeployedComponents {
			// check numComponents bc there is a slight delay between rendering the TUI and updating this value
			if p.numComponents > 0 && deployedPkg.DeployedComponents[j].Status == zarfTypes.ComponentStatusSucceeded {
				m.packages[i].componentStatuses[j] = true
			}
		}
	}

	// always update logViewport content with logs
	file, _ := os.ReadFile(config.LogFileName)
	m.logViewport.SetContent(string(file))
	if !m.isScrolling {
		m.logViewport.GotoBottom()
	}

	return m, tickCmd()
}

// genSuccessOrFailCmds generates the success or failure messages for each package
func genSuccessOrFailCmds(m *Model) []tea.Cmd {
	cmds := []tea.Cmd{tea.Println(fmt.Sprintf("%s\n", logMsg))}
	for i := 0; i < len(m.packages); i++ {
		if m.packages[i].complete {
			successStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#32A852")).Padding(0, 3)
			successMsg := fmt.Sprintf("✅ Package %s deployed\n", m.packages[i].name)
			cmds = append(cmds, tea.Println(successStyle.Render(successMsg)))
		} else {
			failStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Padding(0, 3)
			failMsg := fmt.Sprintf("❌ Package %s failed to deploy\n", m.packages[i].name)
			cmds = append(cmds, tea.Println(failStyle.Render(failMsg)))
		}
	}
	return cmds
}
