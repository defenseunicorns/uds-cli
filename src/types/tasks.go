// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package types contains all the types used by UDS.
package types

import (
	zarfVariables "github.com/defenseunicorns/zarf/src/pkg/variables"
	zarfTypes "github.com/defenseunicorns/zarf/src/types"
)

// TasksFile represents the contents of a tasks file
type TasksFile struct {
	Includes  []map[string]string                 `json:"includes,omitempty" jsonschema:"description=List of local task files to include"`
	Variables []zarfVariables.InteractiveVariable `json:"variables,omitempty" jsonschema:"description=Definitions and default values for variables used in run.yaml"`
	Tasks     []Task                              `json:"tasks" jsonschema:"description=The list of tasks that can be run"`
}

// Task represents a single task
type Task struct {
	Name        string                    `json:"name" jsonschema:"description=Name of the task"`
	Description string                    `json:"description,omitempty" jsonschema:"description=Description of the task"`
	Files       []zarfTypes.ZarfFile      `json:"files,omitempty" jsonschema:"description=Files or folders to download or copy"`
	Actions     []Action                  `json:"actions,omitempty" jsonschema:"description=Actions to take when running the task"`
	Inputs      map[string]InputParameter `json:"inputs,omitempty" jsonschema:"description=Input parameters for the task"`
	EnvPath     string                    `json:"envPath,omitempty" jsonschema:"description=Path to file containing environment variables"`
}

// InputParameter represents a single input parameter for a task, to be used w/ `with`
type InputParameter struct {
	Description       string `json:"description" jsonschema:"description=Description of the parameter,required"`
	DeprecatedMessage string `json:"deprecatedMessage,omitempty" jsonschema:"description=Message to display when the parameter is deprecated"`
	Required          bool   `json:"required,omitempty" jsonschema:"description=Whether the parameter is required,default=true"`
	Default           string `json:"default,omitempty" jsonschema:"description=Default value for the parameter"`
}

// TODO make schema complain if an action has more than one of cmd, task or wait

// Action is a Zarf action inside a Task
type Action struct {
	*zarfTypes.ZarfComponentAction `yaml:",inline"`
	TaskReference                  string            `json:"task,omitempty" jsonschema:"description=The task to run, mutually exclusive with cmd and wait"`
	With                           map[string]string `json:"with,omitempty" jsonschema:"description=Input parameters to pass to the task,type=object"`
}

// TaskReference references the name of a task
type TaskReference struct {
	Name string `json:"name" jsonschema:"description=Name of the task to run"`
}
