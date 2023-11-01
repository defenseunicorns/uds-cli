// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package types contains all the types used by UDS.
package types

import (
	zarfTypes "github.com/defenseunicorns/zarf/src/types"
)

type TasksFile struct {
	Variables []zarfTypes.ZarfPackageVariable `json:"variables,omitempty" jsonschema:"description=Definitions and default values for variables used in run.yaml"`
	Tasks     []Task                          `json:"tasks" jsonschema:"description=The list of tasks that can be run"`
}

type Task struct {
	Name        string               `json:"name" jsonschema:"description=Name of the task"`
	Description string               `json:"description,omitempty" jsonschema:"description=Description of the task"`
	Files       []zarfTypes.ZarfFile `json:"files,omitempty" jsonschema:"description=Files or folders to download or copy"`
	Actions     []Action             `json:"actions,omitempty" jsonschema:"description=Actions to take when running the task"`
}

// TODO make schema complain if an action has more than one of cmd, task or wait

type Action struct {
	*zarfTypes.ZarfComponentAction `yaml:",inline"`
	TaskReference                  *TaskReference `json:"task,omitempty" jsonschema:"description=The task to run, mutually exclusive with cmd and wait"`
}

type TaskReference struct {
	Name string `json:"name" jsonschema:"description=Name of the task to run"`
}
