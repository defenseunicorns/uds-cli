// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2024-Present Defense Unicorns

package exec

import "runtime"

// ShellPreference represents the desired shell to use for a given command
type ShellPreference struct {
	Windows string `json:"windows,omitempty" jsonschema:"description=(default 'powershell') Indicates a preference for the shell to use on Windows systems (note that choosing 'cmd' will turn off migrations like touch -> New-Item),example=powershell,example=cmd,example=pwsh,example=sh,example=bash,example=gsh"`
	Linux   string `json:"linux,omitempty" jsonschema:"description=(default 'sh') Indicates a preference for the shell to use on Linux systems,example=sh,example=bash,example=fish,example=zsh,example=pwsh"`
	Darwin  string `json:"darwin,omitempty" jsonschema:"description=(default 'sh') Indicates a preference for the shell to use on macOS systems,example=sh,example=bash,example=fish,example=zsh,example=pwsh"`
}

// IsPowerShell returns whether a shell name is PowerShell
func IsPowerShell(shellName string) bool {
	return shellName == "powershell" || shellName == "pwsh"
}

// GetOSShell returns the shell and shellArgs based on the current OS
func GetOSShell(shellPref ShellPreference) (string, []string) {
	return getOSShellForOS(shellPref, runtime.GOOS)
}

func getOSShellForOS(shellPref ShellPreference, operatingSystem string) (string, []string) {
	var shell string
	var shellArgs []string
	powershellShellArgs := []string{"-Command", "$ErrorActionPreference = 'Stop';"}
	shShellArgs := []string{"-e", "-c"}

	switch operatingSystem {
	case "windows":
		shell = "powershell"
		if shellPref.Windows != "" {
			shell = shellPref.Windows
		}

		shellArgs = powershellShellArgs
		if shell == "cmd" {
			// Change shellArgs to /c if cmd is chosen
			shellArgs = []string{"/c"}
		} else if !IsPowerShell(shell) {
			// Change shellArgs to -c if a real shell is chosen
			shellArgs = shShellArgs
		}
	case "darwin":
		shell = "sh"
		if shellPref.Darwin != "" {
			shell = shellPref.Darwin
		}

		shellArgs = shShellArgs
		if IsPowerShell(shell) {
			// Change shellArgs to -Command if pwsh is chosen
			shellArgs = powershellShellArgs
		}
	case "linux":
		shell = "sh"
		if shellPref.Linux != "" {
			shell = shellPref.Linux
		}

		shellArgs = shShellArgs
		if IsPowerShell(shell) {
			// Change shellArgs to -Command if pwsh is chosen
			shellArgs = powershellShellArgs
		}
	default:
		shell = "sh"
		shellArgs = shShellArgs
	}

	return shell, shellArgs
}
