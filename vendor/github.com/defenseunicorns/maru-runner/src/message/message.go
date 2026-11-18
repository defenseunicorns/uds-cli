// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present the Maru Authors

package message

import (
	"fmt"
	"os"
	"runtime/debug"
	"time"

	"github.com/pterm/pterm"
)

const (
	// termWidth sets the width of full width elements like progress bars and headers
	termWidth = 100
)

// Fatalf prints a fatal error message and exits with a 1 with a given format.
func Fatalf(err any, format string, a ...any) {
	message := paragraph(format, a...)
	debugPrinter(2, err)
	errorPrinter(2).Println(message)
	debugPrinter(2, string(debug.Stack()))
	os.Exit(1)
}

// Debugf prints a debug message with a given format.
func debugf(format string, a ...any) {
	message := fmt.Sprintf(format, a...)
	debugPrinter(2, message)
}

// Warnf prints a warning message with a given format.
func warnf(format string, a ...any) {
	pterm.Println()
	message := paragraph(format, a...)
	pterm.Warning.Println(message)
}

// Successf prints a success message with a given format.
func successf(format string, a ...any) {
	pterm.Println()
	message := paragraph(format, a...)
	pterm.Success.Println(message)
}

// Failf prints a fail message with a given format.
func errorf(format string, a ...any) {
	pterm.Println()
	message := paragraph(format, a...)
	pterm.Error.Println(message)
}

// Successf prints a success message with a given format.
func infof(format string, a ...any) {
	pterm.Println()
	message := paragraph(format, a...)
	pterm.Info.Println(message)
}

// paragraph formats text into a paragraph matching the TermWidth
func paragraph(format string, a ...any) string {
	return pterm.DefaultParagraph.WithMaxWidth(termWidth).Sprintf(format, a...)
}

func debugPrinter(offset int, a ...any) {
	printer := pterm.Debug.WithShowLineNumber(logLevel <= TraceLevel).WithLineNumberOffset(offset)
	now := time.Now().Format(time.RFC3339)
	// prepend to a
	a = append([]any{now, " - "}, a...)

	printer.Println(a...)

	// Always write to the log file
	if logFile != nil {
		pterm.Debug.
			WithShowLineNumber(true).
			WithLineNumberOffset(offset).
			WithDebugger(false).
			WithWriter(logFile).
			Println(a...)
	}
}

func errorPrinter(offset int) *pterm.PrefixPrinter {
	return pterm.Error.WithShowLineNumber(logLevel <= TraceLevel).WithLineNumberOffset(offset)
}
