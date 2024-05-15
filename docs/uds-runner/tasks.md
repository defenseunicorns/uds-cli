---
title: Tasks
type: docs
weight: 1
---

Tasks serve as the foundational components of the UDS Runner, defining the operations to be executed. Within the `tasks.yaml` file, the `tasks` key at the root level delineates a list of tasks scheduled for execution. The specific operations carried out by each task are detailed under the `actions` key:

```yaml
tasks:
  - name: all-the-tasks
    actions:
      - task: make-build-dir
      - task: install-deps
```

In the above example, the name of the task is "all-the-tasks", and it is composed of multiple sub-tasks to run. These sub-tasks would also be defined in the list of `tasks`:

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

```cli
uds run all-the-tasks   # runs all-the-tasks, which calls make-build-dir and install-deps
uds run make-build-dir  # only runs make-build-dir
```

### Default Tasks

In the provided example above, there is a special type of task known as the default task. This task is optional and can be used as the common entry point for your tasks. When attempting to execute the `default` task, you can exclude the task name from the run command:

```cli
uds run
```
