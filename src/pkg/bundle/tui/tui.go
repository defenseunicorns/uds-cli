package tui

import (
	"encoding/json"
	"fmt"
	"regexp"
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
	"github.com/fatih/color"
	"github.com/goccy/go-yaml/lexer"
	"github.com/goccy/go-yaml/printer"
)

// todo: watch naming collisions, spinners also has a TickMsg
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
	bndlClient            bndlClientShim
	bundleYAML            string
	completeChan          chan int
	fatalChan             chan error
	progressBars          []progress.Model
	pkgNames              []string
	pkgIdx                int
	totalComponentsPerPkg []int
	totalPkgs             int
	spinners              []spinner.Model
	confirmed             bool
	complete              []bool
}

func InitModel(client bndlClientShim, bundleYAML string) model {
	var confirmed bool
	if config.CommonOptions.Confirm {
		confirmed = true
	}

	return model{
		bndlClient:   client,
		completeChan: make(chan int),
		fatalChan:    make(chan error),
		confirmed:    confirmed,
		bundleYAML:   bundleYAML,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Sequence(func() tea.Msg {
		return DeployOp
	})
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
	case <-m.fatalChan:
		m.spinners[m.pkgIdx].Spinner.Frames = []string{""}
		m.spinners[m.pkgIdx].Style = lipgloss.NewStyle().SetString("❌")
		s, spinnerCmd := m.spinners[m.pkgIdx].Update(spinner.TickMsg{})
		m.spinners[m.pkgIdx] = s
		return m, tea.Sequence(spinnerCmd, finalPause(), tea.Quit)
	default:
		switch msg := msg.(type) {
		case progress.FrameMsg:
			progressModel, cmd := m.progressBars[m.pkgIdx].Update(msg)
			m.progressBars[m.pkgIdx] = progressModel.(progress.Model)
			return m, cmd
		case tickMsg:
			var progressCmd tea.Cmd
			if len(m.complete) > m.pkgIdx && m.complete[m.pkgIdx] {
				progressCmd = m.progressBars[m.pkgIdx].SetPercent(100)
				m.spinners[m.pkgIdx].Spinner.Frames = []string{""}
				m.spinners[m.pkgIdx].Style = lipgloss.NewStyle().SetString("✅")
				s, spinnerCmd := m.spinners[m.pkgIdx].Update(spinner.TickMsg{})
				m.spinners[m.pkgIdx] = s

				// check if last pkg is complete
				if m.pkgIdx == m.totalPkgs-1 {
					return m, tea.Sequence(progressCmd, spinnerCmd, finalPause(), tea.Quit)
				}
				return m, tea.Sequence(progressCmd, spinnerCmd)
			}

			if len(m.totalComponentsPerPkg) > m.pkgIdx && m.totalComponentsPerPkg[m.pkgIdx] > 0 {
				deployedPkg := GetDeployedPackage(m.pkgNames[m.pkgIdx])
				if deployedPkg != nil && !resetProgress {
					// todo: instead of going off of DeployedComponents, find a way to include deployedPkg.DeployedComponents[0].Status
					progressCmd = m.progressBars[m.pkgIdx].SetPercent(
						float64(len(deployedPkg.DeployedComponents)) / float64(m.totalComponentsPerPkg[m.pkgIdx]),
					)
					if m.progressBars[m.pkgIdx].Percent() == 1 {
						// todo: instead of going off percentage, go off successful deployment of the pkg
						// stop the spinners and show success
						m.spinners[m.pkgIdx].Spinner.Frames = []string{""}
						m.spinners[m.pkgIdx].Style = lipgloss.NewStyle().SetString("✅")
					}
				} else {
					// handle upgrade scenario by resetting the progress bar until DeployedComponents is back to 1 (ie. the first component)
					progressCmd = m.progressBars[m.pkgIdx].SetPercent(0)
					if deployedPkg != nil && len(deployedPkg.DeployedComponents) >= 1 {
						resetProgress = false
					}
				}
			}
			// must send a spinners.TickMsg to the spinners to keep it spinning
			if len(m.spinners) > 0 {
				s, spinnerCmd := m.spinners[m.pkgIdx].Update(spinner.TickMsg{})
				m.spinners[m.pkgIdx] = s

				return m, tea.Sequence(progressCmd, spinnerCmd, tickCmd())
			}

			return m, tea.Sequence(progressCmd, tickCmd())

		case tea.KeyMsg:
			switch msg.String() {
			case "y", "Y":
				if !m.confirmed {
					m.confirmed = true
				}
				return m, func() tea.Msg {
					return DeployOp
				}
			case "n", "N":
				if !m.confirmed {
					m.confirmed = false
				}
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
					}
				}()
				// use a ticker to update the TUI during deployment
				return m, tickCmd()
			}

		case string:
			if strings.Split(msg, ":")[0] == "package" {
				pkgName := strings.Split(msg, ":")[1]
				m.pkgNames = append(m.pkgNames, pkgName)
				// if pkg is already deployed, set resetProgress to true
				if deployedPkg := GetDeployedPackage(pkgName); deployedPkg != nil && len(deployedPkg.DeployedComponents) != 0 {
					resetProgress = true
				}
			} else if strings.Split(msg, ":")[0] == "totalComponents" {
				if totalComponents, err := strconv.Atoi(strings.Split(msg, ":")[1]); err == nil {
					m.totalComponentsPerPkg = append(m.totalComponentsPerPkg, totalComponents)
				}
			} else if strings.Split(msg, ":")[0] == "totalPackages" {
				if totalPkgs, err := strconv.Atoi(strings.Split(msg, ":")[1]); err == nil {
					m.totalPkgs = totalPkgs
				}
			} else if strings.Split(msg, ":")[0] == "idx" {
				if currentPkgIdx, err := strconv.Atoi(strings.Split(msg, ":")[1]); err == nil {
					m.pkgIdx = currentPkgIdx
					m.progressBars = append(m.progressBars, progress.New(progress.WithDefaultGradient()))
					s := spinner.New()
					s.Spinner = spinner.Dot
					s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
					m.spinners = append(m.spinners, s)
				}
			} else if strings.Split(msg, ":")[0] == "complete" {
				m.complete = append(m.complete, true)
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
	view := ""
	for i := range m.progressBars {
		width := 50 // todo: make dynamic?
		text := lipgloss.NewStyle().
			Width(50).
			Align(lipgloss.Left).
			Padding(0, 3).
			Render(fmt.Sprintf("%s Package %s deploying ...", m.spinners[i].View(), m.pkgNames[i]))

		// render progress bar until deployment is complete
		bar := lipgloss.NewStyle().
			Width(50).
			Align(lipgloss.Left).
			Padding(0, 3).
			MarginTop(1).
			Render(m.progressBars[i].View())

		ui := lipgloss.JoinVertical(lipgloss.Center, text, bar)

		//if len(m.complete) > i && m.complete[i] {
		//	text = lipgloss.NewStyle().
		//		Width(50).
		//		Align(lipgloss.Left).
		//		Padding(0, 3).
		//		Render(fmt.Sprintf("%s Package %s deployed", m.spinners[i].View(), m.pkgNames[i]))
		//	ui = lipgloss.JoinVertical(lipgloss.Center, text)
		//}

		boxStyle := lipgloss.NewStyle().Width(width).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#874BFD")).
			//Padding(1, 0).
			BorderTop(true).
			BorderLeft(true).
			BorderRight(true).
			BorderBottom(true)
		subtle := lipgloss.AdaptiveColor{Light: "#D9DCCF", Dark: "#383838"}
		box := lipgloss.Place(0, 6,
			lipgloss.Left, lipgloss.Top,
			boxStyle.Render(ui),
			lipgloss.WithWhitespaceForeground(subtle),
		)

		//if len(m.complete) > i && m.complete[i] {
		//	boxStyle = lipgloss.NewStyle().
		//		Border(lipgloss.RoundedBorder()).
		//		BorderForeground(lipgloss.Color("#874BFD")).
		//		BorderTop(true).
		//		BorderLeft(true).
		//		BorderRight(true).
		//		BorderBottom(true)
		//	box = lipgloss.Place(width, 6,
		//		lipgloss.Left, lipgloss.Top,
		//		boxStyle.Render(ui),
		//		lipgloss.WithWhitespaceForeground(subtle),
		//	)
		//}
		//view = lipgloss.JoinVertical(lipgloss.Center, view, box)
		view += fmt.Sprintf("%s\n", box)
	}

	return view
}

func (m model) preDeployView() string {
	header := "🎁 BUNDLE DEFINITION"

	prompt := "Deploy this bundle? (Y/N): "

	prettyYAML := colorPrintYAML(m.bundleYAML)

	// Concatenate header, highlighted YAML, and prompt
	return fmt.Sprintf("\n%s\n\n%s\n\n%s\n", header, prettyYAML, prompt)
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
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

	if config.NoColor {
		// If no color is specified strip any color codes from the output - https://regex101.com/r/YFyIwC/2
		ansiRegex := regexp.MustCompile(`\x1b\[(.*?)m`)
		outputYAML = ansiRegex.ReplaceAllString(outputYAML, "")
	}
	return outputYAML
}

func yamlFormat(attr color.Attribute) string {
	const yamlEscape = "\x1b"
	return fmt.Sprintf("%s[%dm", yamlEscape, attr)
}
