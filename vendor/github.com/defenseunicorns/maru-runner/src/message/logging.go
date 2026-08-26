// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present the Maru Authors

// Package message contains functions to print messages to the screen
package message

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/pterm/pterm"
)

// LogLevel is the level of logging to display.
type LogLevel int

const (
	// Supported log levels. These are in order of increasing severity, and
	// match the constants in the log/slog package.

	// TraceLevel level. Effectively the same as Debug but with line numbers.
	//
	// NOTE: There currently is no Trace() function in the log/slog package. In
	// order to use this level, you must use message.SLog.Log() and specify the
	// level. Maru currently uses the Trace level specifically for adding line
	// numbers to logs from calls to message.SLog.Debug(). Because of this,
	// Trace is effectively the same as Debug but with line numbers.
	TraceLevel LogLevel = -8
	// DebugLevel level. Usually only enabled when debugging. Very verbose logging.
	DebugLevel LogLevel = -4
	// InfoLevel level. General operational entries about what's going on inside the
	// application.
	InfoLevel LogLevel = 0
	// WarnLevel level. Non-critical entries that deserve eyes.
	WarnLevel LogLevel = 4
	// ErrorLevel level. Errors only.
	ErrorLevel LogLevel = 8
)

// logLevel is the log level for the runner. When set, log messages with a level
// greater than or equal to this level will be logged. Log messages with a level
// lower than this level will be ignored.
var logLevel = InfoLevel

// logFile acts as a buffer for logFile generation
var logFile *os.File

// UseLogFile writes output to stderr and a logFile.
func UseLogFile(dir string) (io.Writer, error) {
	// Prepend the log filename with a timestamp.
	ts := time.Now().Format("2006-01-02-15-04-05")

	var err error
	logFile, err = os.CreateTemp(dir, fmt.Sprintf("maru-%s-*.log", ts))
	if err != nil {
		return nil, err
	}

	return logFile, nil
}

// LogFileLocation returns the location of the log file.
func LogFileLocation() string {
	if logFile == nil {
		return ""
	}
	return logFile.Name()
}

// SetLogLevel sets the log level.
func SetLogLevel(lvl LogLevel) {
	logLevel = lvl
	// Enable pterm debug messages if the log level is Trace or Debug
	if logLevel <= DebugLevel {
		pterm.EnableDebugMessages()
	}
}

// GetLogLevel returns the current log level.
func GetLogLevel() LogLevel {
	return logLevel
}
