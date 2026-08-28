// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2024-Present Defense Unicorns

package variables

// VariableType represents a type of a variable
type VariableType string

const (
	// RawVariableType is the default type for a variable
	RawVariableType VariableType = "raw"
	// FileVariableType is a type for a variable that loads its contents from a file
	FileVariableType VariableType = "file"
)

// Variable represents a variable that has a value set programmatically
type Variable[T any] struct {
	Name    string `json:"name" jsonschema:"description=The name to be used for the variable,pattern=^[A-Z0-9_]+$"`
	Pattern string `json:"pattern,omitempty" jsonschema:"description=An optional regex pattern that a variable value must match before a package deployment can continue."`
	Extra   T      `json:",omitempty,inline"`
}

// InteractiveVariable is a variable that can be used to prompt a user for more information
type InteractiveVariable[T any] struct {
	Variable[T] `json:",inline"`
	Description string `json:"description,omitempty" jsonschema:"description=A description of the variable to be used when prompting the user a value"`
	Default     string `json:"default,omitempty" jsonschema:"description=The default value to use for the variable"`
	Prompt      bool   `json:"prompt,omitempty" jsonschema:"description=Whether to prompt the user for input for this variable"`
}

// SetVariable tracks internal variables that have been set during this execution run
type SetVariable[T any] struct {
	Variable[T] `json:",inline"`
	Value       string `json:"value" jsonschema:"description=The value the variable is currently set with"`
}

// ExtraVariableInfo carries any additional information that may be desired through variables passed and set by actions (available to library users).
type ExtraVariableInfo struct {
}
