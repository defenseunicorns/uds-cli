// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present the Maru Authors

package message

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/defenseunicorns/pkg/helpers/v2"
	"github.com/pterm/pterm"
)

var activeSpinner *Spinner

var sequence = []string{" ⬒ ", " ⬔ ", " ◨ ", " ◪ ", " ⬓ ", " ⬕ ", " ◧ ", " ◩ "}

// NoProgress sets whether the default spinners and progress bars should use fancy animations
var NoProgress bool

// Spinner is a wrapper around pterm.SpinnerPrinter.
type Spinner struct {
	spinner   *pterm.SpinnerPrinter
	termWidth int
}

// NewProgressSpinner creates a new progress spinner.
var NewProgressSpinner = func(format string, a ...any) helpers.ProgressWriter {
	if activeSpinner != nil {
		activeSpinner.Updatef(format, a...)
		debugPrinter(2, "Active spinner already exists")
		return activeSpinner
	}

	var spinner *pterm.SpinnerPrinter
	if NoProgress {
		infof(format, a...)
	} else {
		text := pterm.Sprintf(format, a...)
		spinner, _ = pterm.DefaultSpinner.
			WithRemoveWhenDone(false).
			// Src: https://github.com/gernest/wow/blob/master/spin/spinners.go#L335
			WithSequence(sequence...).
			Start(text)
	}

	activeSpinner = &Spinner{
		spinner:   spinner,
		termWidth: pterm.GetTerminalWidth(),
	}

	return activeSpinner
}

// Write the given text to the spinner.
func (p *Spinner) Write(raw []byte) (int, error) {
	size := len(raw)
	if NoProgress {
		os.Stderr.Write(raw)

		return size, nil
	}

	// Split the text into lines and update the spinner for each line.
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		text := pterm.Sprintf("     %s", scanner.Text())
		// Clear the current line with the ANSI escape code
		pterm.Fprinto(p.spinner.Writer, "\033[K")
		pterm.Fprintln(p.spinner.Writer, text)
	}

	return size, nil
}

// Updatef updates the spinner text.
func (p *Spinner) Updatef(format string, a ...any) {
	if NoProgress {
		debugPrinter(2, fmt.Sprintf(format, a...))
		return
	}

	pterm.Fprinto(p.spinner.Writer, strings.Repeat(" ", pterm.GetTerminalWidth()))
	text := pterm.Sprintf(format, a...)
	p.spinner.UpdateText(text)
}

// Close stops the spinner.
func (p *Spinner) Close() error {
	if p.spinner != nil && p.spinner.IsActive {
		return p.spinner.Stop()
	}
	activeSpinner = nil
	return nil
}

// Successf prints a success message with the spinner and stops it.
func (p *Spinner) Successf(format string, a ...any) {
	if p.spinner != nil {
		text := pterm.Sprintf(format, a...)
		p.spinner.Success(text)
	} else {
		successf(format, a...)
	}
	p.Close()
}

// Failf prints an error message with the spinner.
func (p *Spinner) Failf(format string, a ...any) {
	if p.spinner != nil {
		text := pterm.Sprintf(format, a...)
		p.spinner.Fail(text)
	} else {
		errorf(format, a...)
	}
	p.Close()
}
