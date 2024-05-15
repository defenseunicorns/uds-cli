---
title: Actions
type: docs
weight: 8
---

Actions are the underlying operations that a task will perform. Each action under the `actions` key has a unique syntax.

### Task

A `task` can reference a task, thus making tasks composable:

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

In the example given above, the task named `foo` invokes a task labeled `bar`, which in turn triggers a task named `baz`. This `baz` task is responsible for outputting information to the console.

### CMD

Actions have the capability to execute arbitrary Bash commands, including in-line scripts. Additionally, the output of a command can be stored in a variable by utilizing the `setVariables` key:

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

- `description`: description of the command.
- `mute`: boolean value to mute the output of a command.
- `dir`: the directory to run the command in.
- `env`: list of environment variables to run for this `cmd` block only:

```yaml
tasks:
  - name: foo
    actions:
      - cmd: echo ${BAR}
        env:
          - BAR=bar
```

- `maxRetries`: number of times to retry the command.
- `maxTotalSeconds`: max number of seconds the command can run until it is killed; takes precendence over `maxRetries`.
