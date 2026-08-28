// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present the Maru Authors

// Package runner provides functions for running tasks in a tasks.yaml
package runner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/defenseunicorns/maru-runner/src/pkg/variables"
	"github.com/defenseunicorns/pkg/exec"
	"github.com/defenseunicorns/pkg/helpers/v2"

	"github.com/defenseunicorns/maru-runner/src/config"
	"github.com/defenseunicorns/maru-runner/src/message"
	"github.com/defenseunicorns/maru-runner/src/pkg/utils"
	"github.com/defenseunicorns/maru-runner/src/types"
)

func (r *Runner) performAction(action types.Action, withs map[string]string, inputs map[string]types.InputParameter) error {

	message.SLog.Debug(fmt.Sprintf("Evaluating action conditional %s", action.If))

	action, _ = utils.TemplateTaskAction(action, withs, inputs, r.variableConfig.GetSetVariables())
	if action.If == "false" && action.TaskReference != "" {
		message.SLog.Info(fmt.Sprintf("Skipping action %s", action.TaskReference))
		return nil
	} else if action.If == "false" && action.Description != "" {
		message.SLog.Info(fmt.Sprintf("Skipping action %s", action.Description))
		return nil
	} else if action.If == "false" && action.Cmd != "" {
		cmdEscaped := helpers.Truncate(action.Cmd, 60, false)
		message.SLog.Info(fmt.Sprintf("Skipping action %q", cmdEscaped))
		return nil
	}

	if action.TaskReference != "" {
		// todo: much of this logic is duplicated in Run, consider refactoring
		referencedTask, err := r.getTask(action.TaskReference)
		if err != nil {
			return err
		}
		for k, v := range action.With {
			action.With[k] = utils.TemplateString(r.variableConfig.GetSetVariables(), v)
		}
		withEnv := []string{}
		for name := range action.With {
			withEnv = append(withEnv, utils.FormatEnvVar(name, action.With[name]))
		}
		if err := validateActionableTaskCall(referencedTask.Name, referencedTask.Inputs, action.With); err != nil {
			return err
		}
		for _, a := range referencedTask.Actions {
			a.Env = utils.MergeEnv(withEnv, a.Env)
		}

		if err := r.executeTask(referencedTask, action.With); err != nil {
			return err
		}
	} else {
		err := RunAction(action.BaseAction, r.envFilePath, r.variableConfig, r.dryRun)
		if err != nil {
			return err
		}
	}
	return nil
}

// processAction checks if action needs to be processed for a given task
func (r *Runner) processAction(task types.Task, action types.Action) bool {

	taskReferenceName := strings.Split(task.Name, ":")[0]
	actionReferenceName := strings.Split(action.TaskReference, ":")[0]
	// don't need to process if the action.TaskReference is empty or if the task and action references are the same since
	// that indicates the task and task in the action are in the same file
	if action.TaskReference != "" && (taskReferenceName != actionReferenceName) {
		for _, task := range r.tasksFile.Tasks {
			// check if TasksFile.Tasks already includes tasks with given reference name, which indicates that the
			// reference has already been processed.
			if strings.Contains(task.Name, taskReferenceName+":") || strings.Contains(task.Name, actionReferenceName+":") {
				return false
			}
		}
		return true
	}
	return false
}

func getUniqueTaskActions(actions []types.Action) []types.Action {
	uniqueMap := make(map[string]bool)
	var uniqueArray []types.Action

	for _, action := range actions {
		if !uniqueMap[action.TaskReference] {
			uniqueMap[action.TaskReference] = true
			uniqueArray = append(uniqueArray, action)
		}
	}
	return uniqueArray
}

// RunAction executes a specific action command, either wait or cmd. It handles variable loading environment variables and manages retries and timeouts
func RunAction[T any](action *types.BaseAction[T], envFilePath string, variableConfig *variables.VariableConfig[T], dryRun bool) error {
	var (
		ctx        context.Context
		cancel     context.CancelFunc
		cmdEscaped string
		out        string
		err        error

		cmd = action.Cmd
	)

	// If the action is a wait, convert it to a command.
	if action.Wait != nil {
		// If the wait has no timeout, set a default of 5 minutes.
		if action.MaxTotalSeconds == nil {
			fiveMin := 300
			action.MaxTotalSeconds = &fiveMin
		}

		// Convert the wait to a command.
		if cmd, err = convertWaitToCmd(*action.Wait, action.MaxTotalSeconds); err != nil {
			return err
		}

		// Mute the output because it will be noisy.
		t := true
		action.Mute = &t

		// Set the max retries to 0.
		z := 0
		action.MaxRetries = &z

		// Not used for wait actions.
		d := ""
		action.Dir = &d
		action.Env = []string{}
		action.SetVariables = []variables.Variable[T]{}
	}

	if action.Description != "" {
		cmdEscaped = action.Description
	} else {
		cmdEscaped = helpers.Truncate(cmd, 60, false)
	}

	// if this is a dry run, print the command that would run and return
	if dryRun {
		message.SLog.Info(fmt.Sprintf("Dry-running %q", cmdEscaped))
		fmt.Println(cmd)
		return nil
	}

	// load the contents of the env file into the Action + the MARU_ARCH
	if envFilePath != "" {
		envFilePath := filepath.Join(filepath.Dir(config.TaskFileLocation), envFilePath)
		envFileContents, err := os.ReadFile(envFilePath)
		if err != nil {
			return err
		}
		action.Env = append(action.Env, strings.Split(string(envFileContents), "\n")...)
	}

	spinner := message.NewProgressSpinner("Running %q", cmdEscaped)

	cfg := GetBaseActionCfg(types.ActionDefaults{}, *action, variableConfig.GetSetVariables())

	if cmd = exec.MutateCommand(cmd, cfg.Shell); err != nil {
		message.SLog.Debug(err.Error())
		spinner.Failf("Error mutating command: %q", cmdEscaped)
	}

	// Template dir string
	cfg.Dir = utils.TemplateString(variableConfig.GetSetVariables(), cfg.Dir)

	// Template env strings
	for idx := range cfg.Env {
		cfg.Env[idx] = utils.TemplateString(variableConfig.GetSetVariables(), cfg.Env[idx])
	}

	duration := time.Duration(cfg.MaxTotalSeconds) * time.Second
	timeout := time.After(duration)

	// Keep trying until the max retries is reached.
retryLoop:
	for remaining := cfg.MaxRetries + 1; remaining > 0; remaining-- {

		// Perform the action run.
		tryCmd := func(ctx context.Context) error {
			// Try running the command and continue the retry loop if it fails.
			if out, err = ExecAction(ctx, cfg, cmd, cfg.Shell, spinner); err != nil {
				return err
			}

			out = strings.TrimSpace(out)

			// If an output variable is defined, set it.
			for _, v := range action.SetVariables {
				variableConfig.SetVariable(v.Name, out, v.Pattern, v.Extra)
				if err = variableConfig.CheckVariablePattern(v.Name); err != nil {
					message.SLog.Debug(err.Error())
					message.SLog.Warn(err.Error())
					return err
				}
			}

			// If the action has a wait, change the spinner message to reflect that on success.
			if action.Wait != nil {
				spinner.Successf("Wait for %q succeeded", cmdEscaped)
			} else {
				spinner.Successf("Completed %q", cmdEscaped)
			}

			// If the command ran successfully, continue to the next action.
			return nil
		}

		// If no timeout is set, run the command and return or continue retrying.
		if cfg.MaxTotalSeconds < 1 {
			spinner.Updatef("Waiting for \"%s\" (no timeout)", cmdEscaped)
			if err := tryCmd(context.TODO()); err != nil {
				continue
			}

			return nil
		}

		// Run the command on repeat until success or timeout.
		spinner.Updatef("Waiting for \"%s\" (timeout: %ds)", cmdEscaped, cfg.MaxTotalSeconds)
		select {
		// On timeout break the loop to abort.
		case <-timeout:
			break retryLoop

		// Otherwise, try running the command.
		default:
			ctx, cancel = context.WithTimeout(context.Background(), duration)
			if err := tryCmd(ctx); err != nil {
				cancel() // Directly cancel the context after an unsuccessful command attempt.
				continue
			}
			cancel() // Also cancel the context after a successful command attempt.
			return nil
		}
	}

	select {
	case <-timeout:
		// If we reached this point, the timeout was reached.
		return fmt.Errorf("command \"%s\" timed out after %d seconds", cmdEscaped, cfg.MaxTotalSeconds)

	default:
		// If we reached this point, the retry limit was reached.
		return fmt.Errorf("command \"%s\" failed after %d retries", cmdEscaped, cfg.MaxRetries)
	}
}

// GetBaseActionCfg merges the ActionDefaults with the BaseAction's configuration
func GetBaseActionCfg[T any](cfg types.ActionDefaults, a types.BaseAction[T], vars variables.SetVariableMap[T]) types.ActionDefaults {
	if a.Mute != nil {
		cfg.Mute = *a.Mute
	}

	// Default is no timeout, but add a timeout if one is provided.
	if a.MaxTotalSeconds != nil {
		cfg.MaxTotalSeconds = *a.MaxTotalSeconds
	}

	if a.MaxRetries != nil {
		cfg.MaxRetries = *a.MaxRetries
	}

	if a.Dir != nil {
		cfg.Dir = *a.Dir
	}

	if len(a.Env) > 0 {
		cfg.Env = append(cfg.Env, a.Env...)
	}

	if a.Shell != nil {
		cfg.Shell = *a.Shell
	}

	// Add variables to the environment.
	for k, v := range vars {
		cfg.Env = append(cfg.Env, fmt.Sprintf("%s=%s", k, v.Value))
	}

	for k, v := range config.GetExtraEnv() {
		cfg.Env = append(cfg.Env, fmt.Sprintf("%s=%s", k, v))
	}

	return cfg
}

// ExecAction executes the given action configuration with the provided context
func ExecAction(ctx context.Context, cfg types.ActionDefaults, cmd string, shellPref exec.ShellPreference, spinner helpers.ProgressWriter) (string, error) {
	shell, shellArgs := exec.GetOSShell(shellPref)

	message.SLog.Debug(fmt.Sprintf("Running command in %s: %s", shell, cmd))

	execCfg := exec.Config{
		Env: cfg.Env,
		Dir: cfg.Dir,
	}

	if !cfg.Mute {
		execCfg.Stdout = spinner
		execCfg.Stderr = spinner
	}

	out, errOut, err := exec.CmdWithContext(ctx, execCfg, shell, append(shellArgs, cmd)...)
	// Dump final complete output (respect mute to prevent sensitive values from hitting the logs).
	if !cfg.Mute {
		message.SLog.Debug(fmt.Sprintf("%s %s %s", cmd, out, errOut))
	}

	return out, err
}

// TODO: (@WSTARR) - this is broken in Maru right now - this should not shell to Kubectl and instead should internally talk to a cluster
// convertWaitToCmd will return the wait command if it exists, otherwise it will return the original command.
func convertWaitToCmd(wait types.ActionWait, timeout *int) (string, error) {
	// Build the timeout string.
	timeoutString := fmt.Sprintf("--timeout %ds", *timeout)

	// If the action has a wait, build a cmd from that instead.
	cluster := wait.Cluster
	if cluster != nil {
		ns := cluster.Namespace
		if ns != "" {
			ns = fmt.Sprintf("-n %s", ns)
		}

		// Build a call to the zarf wait-for command (uses system Zarf)
		cmd := fmt.Sprintf("zarf tools wait-for %s %s %s %s %s",
			cluster.Kind, cluster.Identifier, cluster.Condition, ns, timeoutString)

		// config.CmdPrefix is set when vendoring both the runner and Zarf
		if config.CmdPrefix != "" {
			cmd = fmt.Sprintf("./%s %s", config.CmdPrefix, cmd)
		}
		return cmd, nil
	}

	network := wait.Network
	if network != nil {
		// Make sure the protocol is lower case.
		network.Protocol = strings.ToLower(network.Protocol)

		// If the protocol is http and no code is set, default to 200.
		if strings.HasPrefix(network.Protocol, "http") && network.Code == 0 {
			network.Code = 200
		}

		// Build a call to the zarf wait-for command (uses system Zarf)
		cmd := fmt.Sprintf("zarf tools wait-for %s %s %d %s",
			network.Protocol, network.Address, network.Code, timeoutString)

		// config.CmdPrefix is set when vendoring both the runner and Zarf
		if config.CmdPrefix != "" {
			cmd = fmt.Sprintf("./%s %s", config.CmdPrefix, cmd)
		}
		return cmd, nil
	}

	return "", fmt.Errorf("wait action is missing a cluster or network")
}

// validateActionableTaskCall validates a tasks "withs" and inputs
func validateActionableTaskCall(inputTaskName string, inputs map[string]types.InputParameter, withs map[string]string) error {
	missing := []string{}
	for inputKey, input := range inputs {
		// skip inputs that are not required or have a default value
		if !input.Required || input.Default != "" {
			continue
		}
		checked := false
		for withKey, withVal := range withs {
			// verify that the input is in the with map and the "with" has a value
			if inputKey == withKey && withVal != "" {
				checked = true
				break
			}
		}
		if !checked {
			missing = append(missing, inputKey)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("task %s is missing required inputs: %s", inputTaskName, strings.Join(missing, ", "))
	}
	for withKey := range withs {
		matched := false
		for inputKey, input := range inputs {
			if withKey == inputKey {
				if input.DeprecatedMessage != "" {
					message.SLog.Warn(fmt.Sprintf("This input has been marked deprecated: %s", input.DeprecatedMessage))
				}
				matched = true
				break
			}
		}
		if !matched {
			message.SLog.Warn(fmt.Sprintf("Task %s does not have an input named %s", inputTaskName, withKey))
		}
	}
	return nil
}
