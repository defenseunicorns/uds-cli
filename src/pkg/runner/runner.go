// SPDX-License-Identifier: Apache-2.0

// Package runner provides functions for running tasks in a run.yaml
package runner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
	"time"

	// used for compile time directives to pull functions from Zarf
	_ "unsafe"

	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/defenseunicorns/zarf/src/config/lang"
	"github.com/defenseunicorns/zarf/src/pkg/message"
	zarfUtils "github.com/defenseunicorns/zarf/src/pkg/utils"
	"github.com/defenseunicorns/zarf/src/pkg/utils/helpers"
	zarfTypes "github.com/defenseunicorns/zarf/src/types"
	goyaml "github.com/goccy/go-yaml"
	"github.com/mholt/archiver/v3"
)

// Runner holds the necessary data to run tasks from a tasks file
type Runner struct {
	TemplateMap map[string]*zarfUtils.TextTemplate
	TasksFile   types.TasksFile
	TaskNameMap map[string]bool
	EnvFilePath string
}

// Run runs a task from tasks file
func Run(tasksFile types.TasksFile, taskName string, setVariables map[string]string, withInputs map[string]string) error {
	runner := Runner{
		TemplateMap: map[string]*zarfUtils.TextTemplate{},
		TasksFile:   tasksFile,
		TaskNameMap: map[string]bool{},
	}

	tmpDir, err := zarfUtils.MakeTempDir("")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	runner.EnvFilePath = filepath.Join(tmpDir, "env")

	// Add the uds arch to the environment.
	if err := os.WriteFile(runner.EnvFilePath, []byte("UDS_ARCH="+config.GetArch()+"\n"), 0600); err != nil {
		return err
	}

	runner.populateTemplateMap(tasksFile.Variables, setVariables)

	task, err := runner.getTask(taskName)
	if err != nil {
		return err
	}

	err = runner.processIncludes(task, tasksFile, setVariables)
	if err != nil {
		return err
	}

	if err = runner.checkForTaskLoops(task, tasksFile, setVariables); err != nil {
		return err
	}

	// if withInputs is not nil, validate that the inputs are sufficient
	if withInputs != nil {
		withMap := make(map[string]string)
		withEnv := []string{}
		for k, v := range withInputs {
			withMap[k] = v
			withEnv = append(withEnv, formatEnvVar(k, v))
		}

		task.Actions, err = templateTaskActionsWithInputs(task, withMap)
		if err != nil {
			return err
		}

		if err := validateActionableTaskCall(task.Name, task.Inputs, withMap); err != nil {
			return err
		}

		for _, action := range task.Actions {
			action.Env = mergeEnv(withEnv, action.Env)
		}
	}

	err = runner.executeTask(task)
	return err
}

func (r *Runner) processIncludes(task types.Task, tasksFile types.TasksFile, setVariables map[string]string) error {
	for _, a := range task.Actions {
		if strings.Contains(a.TaskReference, ":") {
			taskReferenceName := strings.Split(a.TaskReference, ":")[0]
			for _, include := range tasksFile.Includes {
				if include[taskReferenceName] != "" {
					referencedIncludes := []map[string]string{include}
					err := r.importTasks(referencedIncludes, config.TaskFileLocation, setVariables)
					if err != nil {
						return err
					}
					break
				}
			}
		}
	}
	return nil
}

func (r *Runner) importTasks(includes []map[string]string, dir string, setVariables map[string]string) error {
	// iterate through includes, open the file, and unmarshal it into a Task
	var includeFilenameKey string
	var includeFilename string
	dir = filepath.Dir(dir)
	for _, include := range includes {
		if len(include) > 1 {
			return fmt.Errorf("included item %s must have only one key", include)
		}
		// grab first and only value from include map
		for k, v := range include {
			includeFilenameKey = k
			includeFilename = v
			break
		}

		includeFilename = r.templateString(includeFilename)

		var tasksFile types.TasksFile
		var includePath string
		// check if included file is a url
		if helpers.IsURL(includeFilename) {
			// If file is a url download it to a tmp directory
			tmpDir, err := zarfUtils.MakeTempDir("")
			defer os.RemoveAll(tmpDir)
			if err != nil {
				return err
			}
			includePath = filepath.Join(tmpDir, filepath.Base(includeFilename))
			if err := zarfUtils.DownloadToFile(includeFilename, includePath, ""); err != nil {
				return fmt.Errorf(lang.ErrDownloading, includeFilename, err.Error())
			}
		} else {
			includePath = filepath.Join(dir, includeFilename)
		}

		if err := zarfUtils.ReadYaml(includePath, &tasksFile); err != nil {
			return fmt.Errorf("unable to read included file %s: %w", includePath, err)
		}

		// prefix task names and actions with the includes key
		for i, t := range tasksFile.Tasks {
			tasksFile.Tasks[i].Name = includeFilenameKey + ":" + t.Name
			if len(tasksFile.Tasks[i].Actions) > 0 {
				for j, a := range tasksFile.Tasks[i].Actions {
					if a.TaskReference != "" && !strings.Contains(a.TaskReference, ":") {
						tasksFile.Tasks[i].Actions[j].TaskReference = includeFilenameKey + ":" + a.TaskReference
					}
				}
			}
		}
		r.TasksFile.Tasks = append(r.TasksFile.Tasks, tasksFile.Tasks...)

		// grab variables from included file
		for _, v := range tasksFile.Variables {
			r.TemplateMap["${"+v.Name+"}"] = &zarfUtils.TextTemplate{
				Sensitive:  v.Sensitive,
				AutoIndent: v.AutoIndent,
				Type:       v.Type,
				Value:      v.Default,
			}
		}

		// merge variables with setVariables
		setVariablesTemplateMap := make(map[string]*zarfUtils.TextTemplate)
		for name, value := range setVariables {
			setVariablesTemplateMap[fmt.Sprintf("${%s}", name)] = &zarfUtils.TextTemplate{
				Value: value,
			}
		}

		r.TemplateMap = helpers.MergeMap[*zarfUtils.TextTemplate](r.TemplateMap, setVariablesTemplateMap)

		// recursively import tasks from included files
		if tasksFile.Includes != nil {
			if err := r.importTasks(tasksFile.Includes, includePath, setVariables); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *Runner) getTask(taskName string) (types.Task, error) {
	for _, task := range r.TasksFile.Tasks {
		if task.Name == taskName {
			return task, nil
		}
	}
	return types.Task{}, fmt.Errorf("task name %s not found", taskName)
}

// mergeEnv merges two environment variable arrays,
// replacing variables found in env2 with variables from env1
// otherwise appending the variable from env1 to env2
func mergeEnv(env1, env2 []string) []string {
	for _, s1 := range env1 {
		replaced := false
		for j, s2 := range env2 {
			if strings.Split(s1, "=")[0] == strings.Split(s2, "=")[0] {
				env2[j] = s1
				replaced = true
			}
		}
		if !replaced {
			env2 = append(env2, s1)
		}
	}
	return env2
}

func formatEnvVar(name, value string) string {
	// replace all non-alphanumeric characters with underscores
	name = regexp.MustCompile(`[^a-zA-Z0-9]+`).ReplaceAllString(name, "_")
	name = strings.ToUpper(name)
	// prefix with INPUT_ (same as GitHub Actions)
	return fmt.Sprintf("INPUT_%s=%s", name, value)
}

func (r *Runner) executeTask(task types.Task) error {
	if len(task.Files) > 0 {
		if err := r.placeFiles(task.Files); err != nil {
			return err
		}
	}

	defaultEnv := []string{}
	for name, inputParam := range task.Inputs {
		d := inputParam.Default
		if d == "" {
			continue
		}
		defaultEnv = append(defaultEnv, formatEnvVar(name, d))
	}

	for _, action := range task.Actions {
		action.Env = mergeEnv(action.Env, defaultEnv)

		if err := r.performAction(action); err != nil {
			return err
		}
	}
	return nil
}

func (r *Runner) populateTemplateMap(zarfVariables []zarfTypes.ZarfPackageVariable, setVariables map[string]string) {
	for _, variable := range zarfVariables {
		r.TemplateMap[fmt.Sprintf("${%s}", variable.Name)] = &zarfUtils.TextTemplate{
			Sensitive:  variable.Sensitive,
			AutoIndent: variable.AutoIndent,
			Type:       variable.Type,
			Value:      variable.Default,
		}
	}

	setVariablesTemplateMap := make(map[string]*zarfUtils.TextTemplate)
	for name, value := range setVariables {
		setVariablesTemplateMap[fmt.Sprintf("${%s}", name)] = &zarfUtils.TextTemplate{
			Value: value,
		}
	}

	r.TemplateMap = helpers.MergeMap[*zarfUtils.TextTemplate](r.TemplateMap, setVariablesTemplateMap)
}

func (r *Runner) placeFiles(files []zarfTypes.ZarfFile) error {
	for _, file := range files {
		// template file.Source and file.Target
		srcFile := r.templateString(file.Source)
		targetFile := r.templateString(file.Target)

		// get current directory
		workingDir, err := os.Getwd()
		if err != nil {
			return err
		}
		dest := filepath.Join(workingDir, targetFile)
		destDir := filepath.Dir(dest)

		if helpers.IsURL(srcFile) {

			// If file is a url download it
			if err := zarfUtils.DownloadToFile(srcFile, dest, ""); err != nil {
				return fmt.Errorf(lang.ErrDownloading, srcFile, err.Error())
			}
		} else {
			// If file is not a url copy it
			if err := zarfUtils.CreatePathAndCopy(srcFile, dest); err != nil {
				return fmt.Errorf("unable to copy file %s: %w", srcFile, err)
			}

		}
		// If file has extract path extract it
		if file.ExtractPath != "" {
			_ = os.RemoveAll(file.ExtractPath)
			err = archiver.Extract(dest, file.ExtractPath, destDir)
			if err != nil {
				return fmt.Errorf(lang.ErrFileExtract, file.ExtractPath, srcFile, err.Error())
			}
		}

		// if shasum is specified check it
		if file.Shasum != "" {
			if file.ExtractPath != "" {
				if err := zarfUtils.SHAsMatch(file.ExtractPath, file.Shasum); err != nil {
					return err
				}
			} else {
				if err := zarfUtils.SHAsMatch(dest, file.Shasum); err != nil {
					return err
				}
			}
		}

		// template any text files with variables
		fileList := []string{}
		if zarfUtils.IsDir(dest) {
			files, _ := zarfUtils.RecursiveFileList(dest, nil, false)
			fileList = append(fileList, files...)
		} else {
			fileList = append(fileList, dest)
		}
		for _, subFile := range fileList {
			// Check if the file looks like a text file
			isText, err := zarfUtils.IsTextFile(subFile)
			if err != nil {
				fmt.Printf("unable to determine if file %s is a text file: %s", subFile, err)
			}

			// If the file is a text file, template it
			if isText {
				if err := zarfUtils.ReplaceTextTemplate(subFile, r.TemplateMap, nil, `\$\{[A-Z0-9_]+\}`); err != nil {
					return fmt.Errorf("unable to template file %s: %w", subFile, err)
				}
			}
		}

		// if executable make file executable
		if file.Executable || zarfUtils.IsDir(dest) {
			_ = os.Chmod(dest, 0700)
		} else {
			_ = os.Chmod(dest, 0600)
		}

		// if symlinks create them
		for _, link := range file.Symlinks {
			// Try to remove the filepath if it exists
			_ = os.RemoveAll(link)
			// Make sure the parent directory exists
			_ = zarfUtils.CreateParentDirectory(link)
			// Create the symlink
			err := os.Symlink(targetFile, link)
			if err != nil {
				return fmt.Errorf("unable to create symlink %s->%s: %w", link, targetFile, err)
			}
		}
	}
	return nil
}

func (r *Runner) performAction(action types.Action) error {
	if action.TaskReference != "" {
		referencedTask, err := r.getTask(action.TaskReference)
		if err != nil {
			return err
		}

		referencedTask.Actions, err = templateTaskActionsWithInputs(referencedTask, action.With)
		if err != nil {
			return err
		}

		withEnv := []string{}
		for name := range action.With {
			withEnv = append(withEnv, formatEnvVar(name, action.With[name]))
		}
		if err := validateActionableTaskCall(referencedTask.Name, referencedTask.Inputs, action.With); err != nil {
			return err
		}
		for _, a := range referencedTask.Actions {
			a.Env = mergeEnv(withEnv, a.Env)
		}
		if err := r.executeTask(referencedTask); err != nil {
			return err
		}
	} else {
		err := r.performZarfAction(action.ZarfComponentAction)
		if err != nil {
			return err
		}
	}
	return nil
}

func templateTaskActionsWithInputs(task types.Task, withs map[string]string) ([]types.Action, error) {
	data := map[string]map[string]string{
		"inputs": {},
	}

	// get inputs from "with" map
	for name := range withs {
		data["inputs"][name] = withs[name]
	}

	// use default if not populated in data
	for name := range task.Inputs {
		if current, ok := data["inputs"][name]; !ok || current == "" {
			data["inputs"][name] = task.Inputs[name].Default
		}
	}

	b, err := goyaml.Marshal(task.Actions)
	if err != nil {
		return nil, err
	}

	t, err := template.New("template task actions").Option("missingkey=error").Delims("${{", "}}").Parse(string(b))
	if err != nil {
		return nil, err
	}

	var templated strings.Builder

	if err := t.Execute(&templated, data); err != nil {
		return nil, err
	}

	result := templated.String()

	var templatedActions []types.Action

	return templatedActions, goyaml.Unmarshal([]byte(result), &templatedActions)
}

func (r *Runner) checkForTaskLoops(task types.Task, tasksFile types.TasksFile, setVariables map[string]string) error {
	// Filtering unique task actions allows for rerunning tasks in the same execution
	uniqueTaskActions := getUniqueTaskActions(task.Actions)
	for _, action := range uniqueTaskActions {
		if action.TaskReference != "" {
			exists := r.TaskNameMap[action.TaskReference]
			if exists {
				return fmt.Errorf("task loop detected")
			}
			r.TaskNameMap[action.TaskReference] = true
			newTask, err := r.getTask(action.TaskReference)
			if err != nil {
				return err
			}
			// check new task includes
			r.processIncludes(newTask, tasksFile, setVariables)
			if err = r.checkForTaskLoops(newTask, tasksFile, setVariables); err != nil {
				return err
			}
		}
		// Clear map once we get to a task that doesn't call another task
		clear(r.TaskNameMap)
	}
	return nil
}

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
					message.Warnf("This input has been marked deprecated: %s", input.DeprecatedMessage)
				}
				matched = true
				break
			}
		}
		if !matched {
			message.Warnf("Task %s does not have an input named %s", inputTaskName, withKey)
		}
	}
	return nil
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

func (r *Runner) performZarfAction(action *zarfTypes.ZarfComponentAction) error {
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

		// Mute the output becuase it will be noisy.
		t := true
		action.Mute = &t

		// Set the max retries to 0.
		z := 0
		action.MaxRetries = &z

		// Not used for wait actions.
		d := ""
		action.Dir = &d
		action.Env = []string{}
		action.SetVariables = []zarfTypes.ZarfComponentActionSetVariable{}
	}

	currentEnvFileContents, err := os.ReadFile(r.EnvFilePath)
	if err != nil {
		return err
	}

	action.Env = append(action.Env, strings.Split(string(currentEnvFileContents), "\n")...)
	action.Env = append(action.Env, "UDS_ENV="+r.EnvFilePath)

	if action.Description != "" {
		cmdEscaped = action.Description
	} else {
		cmdEscaped = message.Truncate(cmd, 60, false)
	}

	spinner := message.NewProgressSpinner("Running \"%s\"", cmdEscaped)
	// Persist the spinner output so it doesn't get overwritten by the command output.
	spinner.EnablePreserveWrites()

	// If the value template is not nil, get the variables for the action.
	// No special variables or deprecations will be used in the action.
	// Reload the variables each time in case they have been changed by a previous action.
	// if valueTemplate != nil {
	// 	vars, _ = valueTemplate.GetVariables(zarfTypes.ZarfComponent{})
	// }

	cfg := actionGetCfg(zarfTypes.ZarfComponentActionDefaults{}, *action, r.TemplateMap)

	if cmd, err = actionCmdMutation(cmd); err != nil {
		spinner.Errorf(err, "Error mutating command: %s", cmdEscaped)
	}

	// template cmd string
	cmd = r.templateString(cmd)

	duration := time.Duration(cfg.MaxTotalSeconds) * time.Second
	timeout := time.After(duration)

	// Keep trying until the max retries is reached.
	for remaining := cfg.MaxRetries + 1; remaining > 0; remaining-- {

		// Perform the action run.
		tryCmd := func(ctx context.Context) error {
			// Try running the command and continue the retry loop if it fails.
			if out, err = actionRun(ctx, cfg, cmd, cfg.Shell, spinner); err != nil {
				return err
			}

			out = strings.TrimSpace(out)

			// If an output variable is defined, set it.
			for _, v := range action.SetVariables {
				// include ${...} syntax in template map for uniformity and to satisfy zarfUtils.ReplaceTextTemplate
				nameInTemplatemap := "${" + v.Name + "}"
				r.TemplateMap[nameInTemplatemap] = &zarfUtils.TextTemplate{
					Sensitive:  v.Sensitive,
					AutoIndent: v.AutoIndent,
					Type:       v.Type,
					Value:      out,
				}
				if regexp.MustCompile(v.Pattern).MatchString(r.TemplateMap[nameInTemplatemap].Value); err != nil {
					message.WarnErr(err, err.Error())
					return err
				}
			}

			// If the action has a wait, change the spinner message to reflect that on success.
			if action.Wait != nil {
				spinner.Successf("Wait for \"%s\" succeeded", cmdEscaped)
			} else {
				spinner.Successf("Completed \"%s\"", cmdEscaped)
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
			break

		// Otherwise, try running the command.
		default:
			ctx, cancel = context.WithTimeout(context.Background(), duration)
			defer cancel()
			if err := tryCmd(ctx); err != nil {
				continue
			}

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

func (r *Runner) templateString(s string) string {
	// Create a regular expression to match ${...}
	re := regexp.MustCompile(`\${(.*?)}`)

	// template string using values from the template map
	result := re.ReplaceAllStringFunc(s, func(matched string) string {
		if value, ok := r.TemplateMap[matched]; ok {
			return value.Value
		}
		return matched // If the key is not found, keep the original substring
	})
	return result
}

// Perform some basic string mutations to make commands more useful.
func actionCmdMutation(cmd string) (string, error) {
	runCmd, err := zarfUtils.GetFinalExecutablePath()
	if err != nil {
		return cmd, err
	}

	// Try to patch the binary path in case the name isn't exactly "./uds".
	cmd = strings.ReplaceAll(cmd, "./uds ", runCmd+" ")

	return cmd, nil
}

// convertWaitToCmd will return the wait command if it exists, otherwise it will return the original command.
func convertWaitToCmd(wait zarfTypes.ZarfComponentActionWait, timeout *int) (string, error) {
	// Build the timeout string.
	timeoutString := fmt.Sprintf("--timeout %ds", *timeout)

	// If the action has a wait, build a cmd from that instead.
	cluster := wait.Cluster
	if cluster != nil {
		ns := cluster.Namespace
		if ns != "" {
			ns = fmt.Sprintf("-n %s", ns)
		}

		// Build a call to the uds tools wait-for command.
		return fmt.Sprintf("./uds zarf tools wait-for %s %s %s %s %s",
			cluster.Kind, cluster.Identifier, cluster.Condition, ns, timeoutString), nil
	}

	network := wait.Network
	if network != nil {
		// Make sure the protocol is lower case.
		network.Protocol = strings.ToLower(network.Protocol)

		// If the protocol is http and no code is set, default to 200.
		if strings.HasPrefix(network.Protocol, "http") && network.Code == 0 {
			network.Code = 200
		}

		// Build a call to the uds tools wait-for command.
		return fmt.Sprintf("./uds zarf tools wait-for %s %s %d %s",
			network.Protocol, network.Address, network.Code, timeoutString), nil
	}

	return "", fmt.Errorf("wait action is missing a cluster or network")
}

//go:linkname actionGetCfg github.com/defenseunicorns/zarf/src/pkg/packager.actionGetCfg
func actionGetCfg(cfg zarfTypes.ZarfComponentActionDefaults, a zarfTypes.ZarfComponentAction, vars map[string]*zarfUtils.TextTemplate) zarfTypes.ZarfComponentActionDefaults

//go:linkname actionRun github.com/defenseunicorns/zarf/src/pkg/packager.actionRun
func actionRun(ctx context.Context, cfg zarfTypes.ZarfComponentActionDefaults, cmd string, shellPref zarfTypes.ZarfComponentActionShell, spinner *message.Spinner) (string, error)
