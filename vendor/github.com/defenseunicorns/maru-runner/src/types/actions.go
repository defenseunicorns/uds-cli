// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present the Maru Authors

// Package types contains all the types used by the runner.
package types

import (
	"github.com/defenseunicorns/maru-runner/src/pkg/variables"
	"github.com/defenseunicorns/pkg/exec"
)

// ActionDefaults sets the default configs for actions and represents an interface shared with Zarf
type ActionDefaults struct {
	Env             []string             `json:"env,omitempty" jsonschema:"description=Additional environment variables for commands"`
	Mute            bool                 `json:"mute,omitempty" jsonschema:"description=Hide the output of commands during execution (default false)"`
	MaxTotalSeconds int                  `json:"maxTotalSeconds,omitempty" jsonschema:"description=Default timeout in seconds for commands (default to 0, no timeout)"`
	MaxRetries      int                  `json:"maxRetries,omitempty" jsonschema:"description=Retry commands given number of times if they fail (default 0)"`
	Dir             string               `json:"dir,omitempty" jsonschema:"description=Working directory for commands (default CWD)"`
	Shell           exec.ShellPreference `json:"shell,omitempty" jsonschema:"description=(cmd only) Indicates a preference for a shell for the provided cmd to be executed in on supported operating systems"`
}

// BaseAction represents a single action to run and represents an interface shared with Zarf
type BaseAction[T any] struct {
	Description     string                  `json:"description,omitempty" jsonschema:"description=Description of the action to be displayed during package execution instead of the command"`
	Cmd             string                  `json:"cmd,omitempty" jsonschema:"description=The command to run. Must specify either cmd or wait for the action to do anything."`
	Wait            *ActionWait             `json:"wait,omitempty" jsonschema:"description=Wait for a condition to be met before continuing. Must specify either cmd or wait for the action."`
	Env             []string                `json:"env,omitempty" jsonschema:"description=Additional environment variables to set for the command"`
	Mute            *bool                   `json:"mute,omitempty" jsonschema:"description=Hide the output of the command during package deployment (default false)"`
	MaxTotalSeconds *int                    `json:"maxTotalSeconds,omitempty" jsonschema:"description=Timeout in seconds for the command (default to 0, no timeout for cmd actions and 300, 5 minutes for wait actions)"`
	MaxRetries      *int                    `json:"maxRetries,omitempty" jsonschema:"description=Retry the command if it fails up to given number of times (default 0)"`
	Dir             *string                 `json:"dir,omitempty" jsonschema:"description=The working directory to run the command in (default is CWD)"`
	Shell           *exec.ShellPreference   `json:"shell,omitempty" jsonschema:"description=(cmd only) Indicates a preference for a shell for the provided cmd to be executed in on supported operating systems"`
	SetVariables    []variables.Variable[T] `json:"setVariables,omitempty" jsonschema:"description=(onDeploy/cmd only) An array of variables to update with the output of the command. These variables will be available to all remaining actions and components in the package."`
}

// ActionWait specifies a condition to wait for before continuing
type ActionWait struct {
	Cluster *ActionWaitCluster `json:"cluster,omitempty" jsonschema:"description=Wait for a condition to be met in the cluster before continuing. Only one of cluster or network can be specified."`
	Network *ActionWaitNetwork `json:"network,omitempty" jsonschema:"description=Wait for a condition to be met on the network before continuing. Only one of cluster or network can be specified."`
}

// ActionWaitCluster specifies a condition to wait for before continuing
type ActionWaitCluster struct {
	Kind       string `json:"kind" jsonschema:"description=The kind of resource to wait for,example=Pod,example=Deployment)"`
	Identifier string `json:"name" jsonschema:"description=The name of the resource or selector to wait for,example=podinfo,example=app&#61;podinfo"`
	Namespace  string `json:"namespace,omitempty" jsonschema:"description=The namespace of the resource to wait for"`
	Condition  string `json:"condition,omitempty" jsonschema:"description=The condition or jsonpath state to wait for; defaults to exist, a special condition that will wait for the resource to exist,example=Ready,example=Available,'{.status.availableReplicas}'=23"`
}

// ActionWaitNetwork specifies a condition to wait for before continuing
type ActionWaitNetwork struct {
	Protocol string `json:"protocol" jsonschema:"description=The protocol to wait for,enum=tcp,enum=http,enum=https"`
	Address  string `json:"address" jsonschema:"description=The address to wait for,example=localhost:8080,example=1.1.1.1"`
	Code     int    `json:"code,omitempty" jsonschema:"description=The HTTP status code to wait for if using http or https,example=200,example=404"`
}
