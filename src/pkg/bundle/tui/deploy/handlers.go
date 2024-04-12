// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package deploy contains the TUI logic for bundle deploys
package deploy

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/bundle/tui"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils"
	"github.com/defenseunicorns/zarf/src/pkg/cluster"
	"github.com/defenseunicorns/zarf/src/pkg/message"
	zarfTypes "github.com/defenseunicorns/zarf/src/types"
)

func (m *Model) handleNewPackage(pkgName string, currentPkgIdx int) tea.Cmd {
	// check if pkg has already been deployed
	var deployedPkg *zarfTypes.DeployedPackage
	if c != nil {
		deployedPkg, _ = c.GetDeployedPackage(pkgName)
	} else {
		// keep checking for cluster connectivity
		c, _ = cluster.NewCluster()
	}
	newPkg := pkgState{
		name: pkgName,
	}

	// upgrade scenario, reset component progress
	if deployedPkg != nil {
		newPkg.resetProgress = true
	}

	// finish creating newPkg and start the spinners
	m.pkgIdx = currentPkgIdx

	// create spinner to track deployment progress
	deploySpinner := spinner.New()
	deploySpinner.Spinner = spinner.Dot
	deploySpinner.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	newPkg.deploySpinner = deploySpinner

	// for remote packages, create spinner to track verification and download progress
	verifySpinner := spinner.New()
	verifySpinner.Spinner = spinner.Dot
	verifySpinner.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	newPkg.verifySpinner = verifySpinner
	downloadSpinner := spinner.New()
	downloadSpinner.Spinner = spinner.Dot
	downloadSpinner.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	newPkg.downloadSpinner = downloadSpinner

	m.packages = append(m.packages, newPkg)
	return tea.Batch(m.packages[m.pkgIdx].deploySpinner.Tick,
		m.packages[m.pkgIdx].verifySpinner.Tick,
		m.packages[m.pkgIdx].downloadSpinner.Tick,
	)
}

func (m *Model) handlePreDeploy() tea.Cmd {
	cmd := func() tea.Msg {
		name, bundleYAML, source, err := m.bndlClient.PreDeployValidation()
		if err != nil {
			m.errChan <- err
		}
		m.validatingBundle = false
		m.bundleYAML = bundleYAML
		m.bundleName = name
		// check if the bundle is remote
		if strings.HasPrefix(source, "oci://") {
			m.isRemoteBundle = true
		}
		return doDeploy
	}

	return cmd
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
	cmds := []tea.Cmd{tea.Println(), tea.Println(m.udsTitle()), tea.Println()}
	m.done = true // remove the current view
	cmds = append(cmds, genSuccessCmds(m)...)
	if err != nil {
		hint := lightBlueText.Render("uds logs")
		message.Debug(err) // capture err in debug logs
		errMsg := tui.IndentStyle.Render(fmt.Sprintf("\n❌ Error deploying bundle: %s\n\nRun %s to view deployment logs", lightGrayText.Render(err.Error()), hint) + "\n")
		cmds = []tea.Cmd{tea.Println(errMsg), tui.Pause(), tea.Quit}
		return tea.Sequence(cmds...)
	}
	styledBundleName := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFF258")).Render(m.bundleName)
	successMsg := tea.Println(
		tui.IndentStyle.
			Render(fmt.Sprintf("\n✨ Bundle %s deployed successfully\n", styledBundleName)))
	cmds = append(cmds, successMsg, tui.Pause(), tea.Quit)
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

		var deployedPkg *zarfTypes.DeployedPackage
		if c != nil {
			deployedPkg, _ = c.GetDeployedPackage(p.name)
		} else {
			// keep checking for cluster connectivity
			c, _ = cluster.NewCluster()
		}

		// if deployedPkg is nil, the package hasn't been deployed yet
		if deployedPkg == nil {
			break
		}
		// handle upgrade scenario by resetting the component progress, otherwise increment it
		if p.resetProgress {
			// if upgraded len(deployedPkg.DeployedComponents) will be equal to the number of components in the package
			if deployedPkg != nil && len(deployedPkg.DeployedComponents) > 0 {
				m.packages[i].resetProgress = false
			}
			break
		}
		// check component progress
		for j := range deployedPkg.DeployedComponents {
			// check numComponents bc there is a slight delay between rendering the TUI and updating this value
			// also nil check the componentStatuses to avoid panic
			componentSucceeded := deployedPkg.DeployedComponents[j].Status == zarfTypes.ComponentStatusSucceeded
			if p.numComponents > 0 && len(p.componentStatuses) >= j && componentSucceeded {
				m.packages[i].componentStatuses[j] = true
			}
		}
	}

	// always update logViewport content with logs
	file, _ := os.ReadFile(config.LogFileName)
	m.logViewport.SetContent(string(file))
	m.logViewport.GotoBottom()

	return m, tickCmd()
}
