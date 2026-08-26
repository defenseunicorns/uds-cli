// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2024-Present Defense Unicorns

package exec

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync"

	"github.com/defenseunicorns/pkg/helpers/v2"
)

var supportedCmdMutations = []string{"zarf", "uds", "kubectl"}
var registeredCmdMutations = sync.Map{}

// GetFinalExecutablePath returns the absolute path to the current executable, following any symlinks along the way.
func GetFinalExecutablePath() (string, error) {
	binaryPath, err := os.Executable()
	if err != nil {
		return "", err
	}

	// In case the binary is symlinked somewhere else, get the final destination
	return filepath.EvalSymlinks(binaryPath)
}

// RegisterCmdMutation registers local ./ commands that should change to the specified cmdLocation
func RegisterCmdMutation(cmdKey string, cmdLocation string) error {
	if slices.Contains(supportedCmdMutations, cmdKey) {
		registeredCmdMutations.Store(cmdKey, cmdLocation)
		return nil
	}
	return fmt.Errorf("%s is not a supported command key", cmdKey)
}

// GetCmdMutation returns the cmdLocation for a given cmdKey and whether that key exists
func GetCmdMutation(cmdKey string) (string, bool) {
	if !slices.Contains(supportedCmdMutations, cmdKey) {
		return "", false
	}

	cmdValue, ok := registeredCmdMutations.Load(cmdKey)
	if !ok {
		return "", false
	}

	cmdLocation, ok := cmdValue.(string)

	return cmdLocation, ok
}

// MutateCommand performs some basic string mutations to make commands more useful.
func MutateCommand(cmd string, shellPref ShellPreference) string {
	return mutateCommandForOS(cmd, shellPref, runtime.GOOS)
}

func mutateCommandForOS(cmd string, shellPref ShellPreference, operatingSystem string) string {
	for _, cmdKey := range supportedCmdMutations {
		// Pull the command location from the map and if it doesn't exist default to $PATH
		cmdLocation, ok := registeredCmdMutations.Load(cmdKey)
		if !ok {
			cmdLocation = cmdKey
		}
		cmd = strings.ReplaceAll(cmd, fmt.Sprintf("./%s ", cmdKey), fmt.Sprintf("%s ", cmdLocation))
	}

	// Make commands 'more' compatible with Windows OS PowerShell
	if operatingSystem == "windows" && (IsPowerShell(shellPref.Windows) || shellPref.Windows == "") {
		// Replace "touch" with "New-Item" on Windows as it's a common command, but not POSIX so not aliased by M$.
		// See https://mathieubuisson.github.io/powershell-linux-bash/ &
		// http://web.cs.ucla.edu/~miryung/teaching/EE461L-Spring2012/labs/posix.html for more details.
		cmd = regexp.MustCompile(`^touch `).ReplaceAllString(cmd, `New-Item `)

		// Convert any ${ENV_*} or $ENV_* to ${Env:ENV_*} or $Env:ENV_* respectively.
		// https://regex101.com/r/bBDfW2/1
		envVarRegex := regexp.MustCompile(`(?P<envIndicator>\${?(?P<varName>([^E{]|E[^n]|En[^v]|Env[^:\s])([a-zA-Z0-9_-])+)}?)`)
		get, err := helpers.MatchRegex(envVarRegex, cmd)
		if err == nil {
			newCmd := strings.ReplaceAll(cmd, get("envIndicator"), fmt.Sprintf("$Env:%s", get("varName")))
			cmd = newCmd
		}
	}

	return cmd
}
