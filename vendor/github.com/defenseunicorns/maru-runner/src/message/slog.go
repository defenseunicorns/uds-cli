// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present the Maru Authors

// Package message provides a rich set of functions for displaying messages to the user.
package message

import (
	"context"
	"log/slog"
)

var (
	// SLog sets the default structured log handler for messages
	SLog = slog.New(MaruHandler{})
)

// MaruHandler is a simple handler that implements the slog.Handler interface
type MaruHandler struct{}

// Enabled determines if the handler is enabled for the given level. This
// function is called for every log message and will compare the level of the
// message to the log level set (default is info). Log levels are defined in
// src/message/logging.go and match the levels used in the underlying log/slog
// package. Logs with a level below the set log level will be ignored.
//
// Examples:
//
//	SetLogLevel(TraceLevel) // show everything, with file names and line numbers
//	SetLogLevel(DebugLevel) // show everything
//	SetLogLevel(InfoLevel)  // show info and above (does not show debug logs)
//	SetLogLevel(WarnLevel)  // show warn and above (does not show debug/info logs)
//	SetLogLevel(ErrorLevel)  // show only errors (does not show debug/info/warn logs)
func (z MaruHandler) Enabled(_ context.Context, level slog.Level) bool {
	// only log if the log level is greater than or equal to the set log level
	return int(level) >= int(logLevel)
}

// WithAttrs is not suppported
func (z MaruHandler) WithAttrs(_ []slog.Attr) slog.Handler {
	return z
}

// WithGroup is not supported
func (z MaruHandler) WithGroup(_ string) slog.Handler {
	return z
}

// Handle prints the respective logging function in Maru
// This function ignores any key pairs passed through the record
func (z MaruHandler) Handle(_ context.Context, record slog.Record) error {
	level := record.Level
	message := record.Message

	switch level {
	case slog.LevelDebug:
		debugf("%s", message)
	case slog.LevelInfo:
		infof("%s", message)
	case slog.LevelWarn:
		warnf("%s", message)
	case slog.LevelError:
		errorf("%s", message)
	}
	return nil
}
