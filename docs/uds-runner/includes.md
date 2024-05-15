---
title: Includes
type: docs
weight: 6
---

The `includes` key serves the purpose of importing tasks from either local or remote task files. This functionality proves beneficial for sharing common tasks among various task files. When importing a task from a local task file, the path is relative to the current file. During task execution, both the tasks within the file and the `includes` tasks undergo processing to prevent any potential infinite loop references. This ensures a seamless and efficient task execution process:

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

It's important to be aware that task files included in a project can also include additional task files. However, there is a specific constraint to consider:

- If a task file includes a remote task file, it is crucial to note that the included remote task file cannot, in turn, include any local task files.
