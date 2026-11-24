// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package redact

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
)

// Error masks every visible cause. Sentinels retain their errors.Is identity through masking.
func (s *Scope) Error(err error) error {
	if err == nil {
		return nil
	}
	mask := s.replacer()
	if mask == nil {
		return err
	}
	_, result := maskError(err, mask)
	return result
}

// Errorf formats and masks an error, preserving causes supplied with %w.
func (s *Scope) Errorf(format string, args ...any) error {
	return s.Error(fmt.Errorf(format, args...))
}

// MaskedError preserves the identities and structured data of a masked diagnostic.
type MaskedError struct {
	message  string
	causes   []error
	original error
}

func (e *MaskedError) Is(target error) bool { return errors.Is(e.original, target) }
func (e *MaskedError) Error() string        { return e.message }
func (e *MaskedError) Unwrap() []error      { return e.causes }

// As matches only this node. The errors package continues through its masked
// children, including separate branches of a joined error.
func (e *MaskedError) As(target any) bool {
	match := e.original
	if typed, ok := match.(interface{ WithMaskedMessage(string) error }); ok {
		match = typed.WithMaskedMessage(e.message)
	}
	value := reflect.ValueOf(match)
	dest := reflect.ValueOf(target).Elem()
	if !value.IsValid() || !value.Type().AssignableTo(dest.Type()) {
		return false
	}
	dest.Set(value)
	return true
}

func maskError(err error, mask *strings.Replacer) (bool, error) {
	text := err.Error()
	message := mask.Replace(text)
	changed := message != text
	var children []error
	switch e := err.(type) { //nolint:errorlint // Visit direct children exactly once, including joined causes.
	case interface{ Unwrap() []error }:
		children = e.Unwrap()
	case interface{ Unwrap() error }:
		if cause := e.Unwrap(); cause != nil {
			children = []error{cause}
		}
	}
	var causes []error
	for _, child := range children {
		if child != nil {
			masked, clean := maskError(child, mask)
			causes = append(causes, clean)
			changed = changed || masked
		}
	}
	if !changed {
		return false, err
	}
	// A nested operation may already have masked this node before this scope
	// discovered more values. Keep its original type and its latest safe text.
	original := err
	if previous, ok := err.(*MaskedError); ok { //nolint:errorlint // Match only this node.
		original = previous.original
	}
	return true, &MaskedError{message: message, causes: causes, original: original}
}
