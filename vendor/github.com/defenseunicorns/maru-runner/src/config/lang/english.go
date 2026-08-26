// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present the Maru Authors

// Package lang contains the language strings in english used by maru
package lang

import "errors"

// Common Error Messages
const (
	ErrDownloading = "failed to download %s: %w"
	ErrCreatingDir = "failed to create directory %s: %s"
	ErrWritingFile = "failed to write file %s: %s"
	ErrFileExtract = "failed to extract filename %s from archive %s: %s"
)

// Root
const (
	RootCmdShort              = "CLI for the maru runner"
	RootCmdFlagSkipLogFile    = "Disable log file creation"
	RootCmdFlagLogLevel       = "Log level for the runner. Valid options are: error, warn, info, debug, trace"
	RootCmdFlagNoProgress     = "Disable fancy UI progress bars, spinners, logos, etc"
	RootCmdErrInvalidLogLevel = "Invalid log level. Valid options are: error, warn, info, debug, trace."
	RootCmdFlagArch           = "Architecture for the runner"
	RootCmdFlagTempDir        = "Specify the temporary directory to use for intermediate files"
)

// Version
const (
	CmdVersionShort = "Shows the version of the running runner binary"
	CmdVersionLong  = "Displays the version of the runner release that the current binary was built from."
)

// Internal
const (
	CmdInternalShort             = "Internal cmds used by the runner"
	CmdInternalConfigSchemaShort = "Generates a JSON schema for the tasks.yaml configuration"
	CmdInternalConfigSchemaErr   = "Unable to generate the tasks.yaml schema"
)

// Viper
const (
	CmdViperErrLoadingConfigFile = "failed to load config file: %s"
	CmdViperInfoUsingConfigFile  = "Using config file %s"
)

// Run
const (
	CmdRunShort       = "Runs a specified task from a task file"
	CmdRunFlag        = "Name and location of task file to run"
	CmdRunSetVarFlag  = "Set a runner variable from the command line (KEY=value)"
	CmdRunWithVarFlag = "(experimental) Set the inputs for a task from the command line (KEY=value)"
	CmdRunList        = "List available tasks in a task file"
	CmdRunListAll     = "List all available tasks in a task file, including tasks from included files"
	CmdRunDryRun      = "Validate the task without actually running any commands"
)

// Auth
const (
	CmdAuthShort           = "[beta] Authentication commands for pulling private remote task files"
	CmdLoginShort          = "[beta] Adds a token for a given host to your keyring"
	CmdLoginTokenFlag      = "The personal access token (bearer) you would like to save"
	CmdLoginTokenStdInFlag = "Whether to pull the token from standard input"
	CmdLogoutShort         = "[beta] Removes a token for a given host from your keyring"
)

// Common Errors
var (
	ErrInterrupt = errors.New("execution cancelled due to an interrupt")
)
