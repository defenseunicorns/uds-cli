// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present the Maru Authors

package variables

import (
	"fmt"
	"regexp"
)

// SetVariableMap represents a map of variable names to their set values
type SetVariableMap[T any] map[string]*SetVariable[T]

// GetSetVariable gets a variable set within a VariableConfig by its name
func (vc *VariableConfig[T]) GetSetVariable(name string) (variable *SetVariable[T], ok bool) {
	variable, ok = vc.setVariableMap[name]
	return variable, ok
}

// GetSetVariables gets the variables set within a VariableConfig
func (vc *VariableConfig[T]) GetSetVariables() SetVariableMap[T] {
	return vc.setVariableMap
}

// PopulateVariables handles setting the active variables within a VariableConfig's SetVariableMap
func (vc *VariableConfig[T]) PopulateVariables(variables []InteractiveVariable[T], presetVariables map[string]string) error {
	for name, value := range presetVariables {
		var extra T
		vc.SetVariable(name, value, "", extra)
	}

	for _, variable := range variables {
		_, present := vc.setVariableMap[variable.Name]

		// Variable is present, no need to continue checking
		if present {
			vc.setVariableMap[variable.Name].Pattern = variable.Pattern
			vc.setVariableMap[variable.Name].Extra = variable.Extra
			if err := vc.CheckVariablePattern(variable.Name); err != nil {
				return err
			}
			continue
		}

		// First set default (may be overridden by prompt)
		vc.SetVariable(variable.Name, variable.Default, variable.Pattern, variable.Extra)

		// Variable is set to prompt the user
		if variable.Prompt {
			// Prompt the user for the variable
			val, err := vc.prompt(variable)

			if err != nil {
				return err
			}

			vc.SetVariable(variable.Name, val, variable.Pattern, variable.Extra)
		}

		if err := vc.CheckVariablePattern(variable.Name); err != nil {
			return err
		}
	}

	return nil
}

// SetVariable sets a variable in a VariableConfig's SetVariableMap
func (vc *VariableConfig[T]) SetVariable(name, value, pattern string, extra T) {
	vc.setVariableMap[name] = &SetVariable[T]{
		Variable: Variable[T]{
			Name:    name,
			Pattern: pattern,
			Extra:   extra,
		},
		Value: value,
	}
}

// CheckVariablePattern checks to see if a current variable is set to a value that matches its pattern
func (vc *VariableConfig[T]) CheckVariablePattern(name string) error {
	if variable, ok := vc.setVariableMap[name]; ok {
		if regexp.MustCompile(variable.Pattern).MatchString(variable.Value) {
			return nil
		}

		return fmt.Errorf("provided value for variable %q does not match pattern %q", name, variable.Pattern)
	}

	return fmt.Errorf("variable %q was not found in the current variable map", name)
}
