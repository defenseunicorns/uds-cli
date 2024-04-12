package deploy

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"
)

func TestDeploy(t *testing.T) {
	testPkgs := []pkgState{
		{
			name:              "test-pkg",
			numComponents:     1,
			componentStatuses: []bool{true},
		}, {
			name:              "test-pkg-2",
			numComponents:     1,
			componentStatuses: []bool{true},
		},
	}
	initTestModel := func() *Model {

		m := InitModel(nil)
		m.validatingBundle = false
		m.totalPkgs = 2
		m.bundleName = "test-bundle"
		m.logViewport.Width = 50
		m.logViewport.Height = 50
		m.bundleYAML = "fake bundle YAML"
		return &m
	}

	t.Run("test deploy", func(t *testing.T) {
		m := initTestModel()

		// check pre-deploy view
		view := m.View()
		require.Contains(t, view, m.bundleYAML)
		require.Contains(t, view, "Deploy this bundle? (y/n)")

		// simulate pressing 'y' key to confirm deployment
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []int32{121}})
		view = m.View()
		require.Contains(t, view, "UDS Bundle: test-bundle")

		// deploy first pkg in bundle with simulated components
		m.Update("newPackage:test-pkg:0")
		m.packages[m.pkgIdx].numComponents = 1
		m.packages[m.pkgIdx].componentStatuses = []bool{true}
		view = m.View()
		require.Contains(t, view, "Deploying bundle package (1 / 2)")
		require.Contains(t, view, "Package test-pkg deploying (1 / 1 components)")

		// simulate package deployment completion
		m.Update("complete:test-pkg")
		//m.Update(deployTickMsg(time.Time{}))
		view = m.View()
		require.Contains(t, view, "Package test-pkg deployed")
		require.NotContains(t, view, "Package test-pkg deploying")

		// deploy second pkg in bundle with simulated components
		m.Update("newPackage:test-pkg-2:1")
		m.packages[m.pkgIdx].numComponents = 1
		m.packages[m.pkgIdx].componentStatuses = []bool{true}
		view = m.View()
		require.Contains(t, view, "Deploying bundle package (2 / 2)")
		require.Contains(t, view, "Package test-pkg-2 deploying (1 / 1 components)")

		// simulate package deployment completion
		m.Update("complete:test-pkg-2")
		view = m.View()
		require.Contains(t, view, "Package test-pkg-2 deployed")
		require.NotContains(t, view, "Package test-pkg-2 deploying")
	})

	t.Run("test toggle log view", func(t *testing.T) {
		m := initTestModel()

		// simulate passing --confirm
		m.inProgress = true
		m.confirmed = true
		m.packages = testPkgs

		view := m.View()
		require.Contains(t, view, "Package test-pkg deploying (1 / 1 components)")

		// simulate pressing 'l' key to toggle logs
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []int32{108}})

		view = m.View()
		require.Contains(t, view, "test-pkg package logs")

		// simulate pressing 'l' key to toggle logs
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []int32{108}})

		view = m.View()
		require.NotContains(t, view, "test-pkg package logs")
	})

	t.Run("test deploy cancel", func(t *testing.T) {
		m := initTestModel()
		view := m.View()
		require.Contains(t, view, "Deploy this bundle? (y/n)")

		// simulate pressing 'n' key to cancel deployment
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []int32{110}})
		view = m.View()

		// model's view is cleared after canceling deployment
		require.Equal(t, view, "")
	})

}
