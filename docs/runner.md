# UDS Runner

UDS CLI contains vendors and configures the [maru-runner](https://github.com/defenseunicorns/maru-runner) build tool to make compiling and building UDS bundles simple.


## Quickstart

#### Running a Task
To run a task from a `tasks.yaml`:
```
uds run <task-name>
```

#### Running a Task from a specific tasks file
```
uds run -f <path/to/tasks.yaml> <task-name>
```


The Maru [docs](https://github.com/defenseunicorns/maru-runner) describe how to build `tasks.yaml` files to configure the runner. The functionality in UDS CLI is mostly identical with the following exceptions

### Variables Set with Environment Variables
When running a `tasks.yaml` with `uds run my-task` you can set variables using environment prefixed with `UDS_`

For example, running `UDS_FOO=bar uds run echo-foo` on the following task will echo `bar`.

```yaml
variables:
 - name: FOO
   default: foo
tasks:
 - name: echo-foo
   actions:
     - cmd: echo ${FOO}
```

### No Dependency on Zarf
Since UDS CLI also vendors [Zarf](https://github.com/defenseunicorns/zarf), there is no need to also have Zarf installed on your system.
