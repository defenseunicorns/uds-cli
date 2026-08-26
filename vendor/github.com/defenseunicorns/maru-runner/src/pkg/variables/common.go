// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2024-Present Defense Unicorns

// Package variables contains functions for interacting with variables
package variables

import (
	"log/slog"
)

// VariableConfig represents a value to be templated into a text file.
type VariableConfig[T any] struct {
	setVariableMap SetVariableMap[T]

	prompt func(variable InteractiveVariable[T]) (value string, err error)
	logger *slog.Logger
}

// New creates a new VariableConfig
func New[T any](prompt func(variable InteractiveVariable[T]) (value string, err error), logger *slog.Logger) *VariableConfig[T] {
	return &VariableConfig[T]{
		setVariableMap: make(SetVariableMap[T]),
		prompt:         prompt,
		logger:         logger,
	}
}
