// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present the Maru Authors

// Package config contains configuration strings for maru
package config

const (
	// TasksYAML is the string for the default tasks.yaml
	TasksYAML = "tasks.yaml"

	// EnvPrefix is the prefix for environment variables
	EnvPrefix = "MARU"

	// KeyringService is the name given to the service Maru uses in the Keyring
	KeyringService = "com.defenseunicorns.maru"
)

var (
	// CLIVersion track the version of the CLI
	CLIVersion = "unset"

	// CmdPrefix is used to prefix Zarf cmds (like wait-for), useful when vendoring both the runner and Zarf
	// if not set, the system Zarf will be used
	CmdPrefix string

	// TaskFileLocation is the location of the tasks file to run
	TaskFileLocation string

	// TempDirectory is the directory to store temporary files
	TempDirectory string

	// VendorPrefix is the prefix for environment variables that an application vendoring Maru wants to use
	VendorPrefix string

	// MaxStack is the maximum stack size for task references
	MaxStack = 2048

	extraEnv = map[string]string{"MARU": "true"}
)

// AddExtraEnv adds a new envirmentment variable to the extraEnv to make it available to actions
func AddExtraEnv(key string, value string) {
	extraEnv[key] = value
}

// GetExtraEnv returns the map of extra environment variables that have been set and made available to actions
func GetExtraEnv() map[string]string {
	return extraEnv
}

// ClearExtraEnv clears extraEnv back to empty map
func ClearExtraEnv() {
	extraEnv = make(map[string]string)
}
