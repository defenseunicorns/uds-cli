// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present the Maru Authors
// SPDX-FileCopyrightText: 2023-Present the Maru Authors

// Package utils provides utility fns for maru
package utils

import (
	"regexp"
	"strings"
	"text/template"

	"github.com/defenseunicorns/maru-runner/src/config"
	"github.com/defenseunicorns/maru-runner/src/pkg/variables"
	"github.com/defenseunicorns/maru-runner/src/types"
	goyaml "github.com/goccy/go-yaml"
)

// TemplateTaskAction templates a task's actions with the given inputs and variables
func TemplateTaskAction[T any](action types.Action, withs map[string]string, inputs map[string]types.InputParameter, setVarMap variables.SetVariableMap[T]) (types.Action, error) {
	data := map[string]map[string]string{
		"inputs":    {},
		"variables": {},
	}

	// get inputs from "with" map
	for name := range withs {
		data["inputs"][name] = withs[name]
	}

	// get vars from "vms" map
	for name := range setVarMap {
		data["variables"][name] = setVarMap[name].Value
	}

	// use default if not populated in data
	for name := range inputs {
		if current, ok := data["inputs"][name]; !ok || current == "" {
			data["inputs"][name] = inputs[name].Default
		}
	}

	b, err := goyaml.Marshal(action)
	if err != nil {
		return action, err
	}

	t, err := template.New("template task actions").Option("missingkey=error").Delims("${{", "}}").Parse(string(b))
	if err != nil {
		return action, err
	}

	var templated strings.Builder

	if err := t.Execute(&templated, data); err != nil {
		return action, err
	}

	result := templated.String()

	var templatedAction types.Action
	if err := goyaml.Unmarshal([]byte(result), &templatedAction); err != nil {
		return action, err
	}

	return templatedAction, nil
}

// TemplateString replaces ${...} with the value from the template map
func TemplateString[T any](setVariableMap variables.SetVariableMap[T], s string) string {
	// Create a regular expression to match ${...}
	re := regexp.MustCompile(`\${(.*?)}`)

	// template string using values from the set variable map
	result := re.ReplaceAllStringFunc(s, func(matched string) string {
		varName := strings.TrimSuffix(strings.TrimPrefix(matched, "${"), "}")

		if value, ok := config.GetExtraEnv()[varName]; ok {
			return value
		}

		if value, ok := setVariableMap[varName]; ok {
			return value.Value
		}
		return matched // If the key is not found, keep the original substring
	})

	return result
}
