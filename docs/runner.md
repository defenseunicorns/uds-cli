# UDS Runner

UDS runner enables UDS Bundle developers to automate UDS builds and perform common shell tasks. It
uses [Zarf](https://zarf.dev/) under the hood to perform tasks and shares a syntax similar to `zarf.yaml` manifests.
Many [Zarf Actions features](https://docs.zarf.dev/docs/create-a-zarf-package/component-actions) are also available in
UDS runner.

## Table of Contents

- [UDS Runner](#uds-runner)
  - [Quickstart](#quickstart)
  - [Key Concepts](#key-concepts)
    - [Tasks](#tasks)
    - [Actions](#actions)
      - [Task](#task)
      - [Cmd](#cmd)
    - [Variables](#variables)
    - [Files](#files)
    - [Wait](#wait)
    - [Includes](#includes)

## Quickstart

Create a file called `tasks.yaml`

```yaml
variables:
  - name: FOO
    default: foo

tasks:
  - name: default
    actions:
      - cmd: echo "run default task"

  - name: example
    actions:
      - task: set-variable
      - task: echo-variable

  - name: set-variable
    actions:
      - cmd: echo "bar"
        setVariables:
          - name: FOO

  - name: echo-variable
    actions:
      - cmd: echo ${FOO}
```

From the same directory as the `tasks.yaml`, run the `example` task using:

```bash
uds run example
```

This will run the `example` tasks which in turn runs the `set-variable` and `echo-variable`. In this example, the text "
bar" should be printed to the screen twice.

Optionally, you can specify the location and name of your `tasks.yaml` using the `--file` or `-f` flag:

```bash
uds run example -f tmp/tasks.yaml
```

You can also view the tasks that are available to run using the `list` flag:

```bash
uds run -f tmp/tasks.yaml --list
```

## Key Concepts

### Tasks

Tasks are the fundamental building blocks of the UDS runner and they define operations to be performed. The `tasks` key
at the root of `tasks.yaml` define a list of tasks to be run. This underlying operations performed by a task are defined
under the `actions` key:

```yaml
tasks:
  - name: all-the-tasks
    actions:
      - task: make-build-dir
      - task: install-deps
```

In this example, the name of the task is "all-the-tasks", and it is composed of multiple sub-tasks to run. These sub-tasks
would also be defined in the list of `tasks`:

```yaml
tasks:
  - name: default
    actions:
      - cmd: echo "run default task"

  - name: all-the-tasks
    actions:
      - task: make-build-dir
      - task: install-deps

  - name: make-build-dir
    actions:
      - cmd: mkdir -p build

  - name: install-deps
    actions:
      - cmd: go mod tidy
```

Using the UDS CLI, these tasks can be run individually:

```bash
uds run all-the-tasks   # runs all-the-tasks, which calls make-build-dir and install-deps
uds run make-build-dir  # only runs make-build-dir
```

#### Default Tasks
In the above example, there is also a `default` task, which is special, optional, task that can be used for the most common entrypoint for your tasks. When trying to run the `default` task, you can omit the task name from the run command:

```bash
uds run
```

### Actions

Actions are the underlying operations that a task will perform. Each action under the `actions` key has a unique syntax.

#### Task

A task can reference a task, thus making tasks composable.

```yaml
tasks:
  - name: foo
    actions:
      - task: bar
  - name: bar
    actions:
      - task: baz
  - name: baz
    actions:
      - cmd: "echo task foo is composed of task bar which is composed of task baz!"
```

In this example, the task `foo` calls a task called `bar` which calls a task `baz` which prints some output to the
console.

#### Cmd

Actions can run arbitrary bash commands including in-line scripts, and the output of a command can be placed in a
variable using the `setVariables` key

```yaml
tasks:
  - name: foo
    actions:
      - cmd: echo -n 'dHdvIHdlZWtzIG5vIHByb2JsZW0=' | base64 -d
        setVariables:
          - name: FOO
```

This task will decode the base64 string and set the value as a variable named `FOO` that can be used in other tasks.

Command blocks can have several other properties including:

- `description`: description of the command
  - `mute`: boolean value to mute the output of a command
  - `dir`: the directory to run the command in
  - `env`: list of environment variables to run for this `cmd` block only

    ```yaml
    tasks:
      - name: foo
        actions:
          - cmd: echo ${BAR}
            env:
              - BAR=bar
    ```

  - `maxRetries`: number of times to retry the command
  - `maxTotalSeconds`: max number of seconds the command can run until it is killed; takes precendence
    over `maxRetries`

### Variables

Variables can be defined in 3 ways:

1. At the top of the `tasks.yaml`

   ```yaml
   variables:
     - name: FOO
       default: foo

   tasks: ...
   ```

1. As the output of a `cmd`

   ```yaml
   variables:
     - name: FOO
       default: foo
   tasks:
     - name: foo
       actions:
         - cmd: uname -m
           mute: true
           setVariables:
             - name: FOO
         - cmd: echo ${FOO}
   ```

1. Using the `--set` flag in the CLI : `uds run foo --set FOO=bar`

To use a variable, reference it using `${VAR_NAME}`

Note that variables also have the following attributes:

- `sensitive`: boolean value indicating if a variable should be visible in output
- `default`: default value of a variable

### Files

The `files` key is used to copy local or remote files to the current working directory

```yaml
tasks:
  - name: copy-local
    files:
      - source: /tmp/foo
        target: foo
  - name: copy-remote
    files:
      - source: https://cataas.com/cat
        target: cat.jpeg
```

Files blocks can also use the following attributes:

- `executable`: boolean value indicating if the file is executable
- `shasum`: SHA string to verify the integrity of the file
- `symlinks`: list of strings referring to symlink the file to

### Wait

The `wait`key is used to block execution while waiting for a resource, including network responses and K8s operations

```yaml
tasks:
  - name: network-response
    wait:
      network:
        protocol: https
        address: 1.1.1.1
        code: 200
  - name: configmap-creation
    wait:
      cluster:
        kind: configmap
        name: simple-configmap
        namespace: foo
```

### Includes

The `includes` key is used to import tasks from either local or remote task files. This is useful for sharing common tasks across multiple task files. When importing a task from a local task file, the path is relative to the file you are currently in.

```yaml
includes:
  - local: ./path/to/tasks-to-import.yaml
  - remote: https://raw.githubusercontent.com/defenseunicorns/uds-cli/main/src/test/tasks/remote-import-tasks.yaml

tasks:
  - name: import-local
    actions:
      - task: local:some-local-task
  - name: import-remote
    actions:
      - task: remote:echo-var
```

Note that included task files can also include other task files, with the following restriction:

- If a task file includes a remote task file, the included remote task file cannot include any local task files
