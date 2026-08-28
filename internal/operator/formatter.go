// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package operator contains UDS Core operator monitoring logic for UDS CLI Next.
package operator

import (
	"fmt"
	"io"
	"strings"
)

const (
	ansiReset  = "\x1b[0m"
	ansiRed    = "\x1b[31m"
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"
	ansiCyan   = "\x1b[36m"
)

// EventKind identifies the high-level Pepr event shown to users.
type EventKind string

const (
	// EventAllowed represents an allowed Pepr policy decision.
	EventAllowed EventKind = "ALLOWED"
	// EventDenied represents a denied Pepr policy decision.
	EventDenied EventKind = "DENIED"
	// EventMutated represents a Pepr mutation.
	EventMutated EventKind = "MUTATED"
	// EventOperator represents a UDS operator event from Pepr.
	EventOperator EventKind = "OPERATOR"
)

// PatchOperation describes one rendered JSON patch operation.
type PatchOperation struct {
	Kind  string
	Path  string
	Value string
}

// Event is a display-oriented Pepr event.
type Event struct {
	Kind     EventKind
	Resource string
	Repeated int
	Message  string
	Patch    []PatchOperation
}

// Formatter renders Pepr events as structured terminal text.
type Formatter struct {
	NoColor bool
}

// WriteEvents writes Pepr events in the proposed Next monitor format.
func (f Formatter) WriteEvents(w io.Writer, events []Event) error {
	for i, event := range events {
		if i > 0 {
			if _, err := fmt.Fprintln(w); err != nil {
				return err
			}
		}
		if err := f.WriteEvent(w, event); err != nil {
			return err
		}
	}

	return nil
}

// WriteEvent writes one Pepr event in the proposed Next monitor format.
func (f Formatter) WriteEvent(w io.Writer, event Event) error {
	if _, err := fmt.Fprintf(w, "%s%s resource=%s", f.token(event.Kind), tokenPadding(event.Kind), event.Resource); err != nil {
		return err
	}
	if event.Repeated > 0 {
		if _, err := fmt.Fprintf(w, " repeated=%d", event.Repeated); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}

	if event.Message != "" {
		if _, err := fmt.Fprintf(w, "  message=%q\n", event.Message); err != nil {
			return err
		}
	}

	for _, patch := range event.Patch {
		if patch.Value == "" {
			if _, err := fmt.Fprintf(w, "  %s path=%s\n", patch.Kind, patch.Path); err != nil {
				return err
			}
			continue
		}
		if _, err := fmt.Fprintf(w, "  %s path=%s value=%s\n", patch.Kind, patch.Path, patch.Value); err != nil {
			return err
		}
	}

	return nil
}

func (f Formatter) token(kind EventKind) string {
	if f.NoColor {
		return string(kind)
	}

	switch kind {
	case EventAllowed:
		return colorize(ansiGreen, string(kind))
	case EventDenied:
		return colorize(ansiRed, string(kind))
	case EventMutated:
		return colorize(ansiCyan, string(kind))
	case EventOperator:
		return colorize(ansiYellow, string(kind))
	default:
		return string(kind)
	}
}

func colorize(color, text string) string {
	return color + text + ansiReset
}

func tokenPadding(kind EventKind) string {
	padding := 8 - len(kind)
	if padding < 0 {
		return ""
	}
	return strings.Repeat(" ", padding)
}
