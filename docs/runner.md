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

### Running UDS and Zarf Commands
To run `uds` commands from within a task, you can invoke your system `uds` binary using the `./uds` syntax. Similarly, UDS CLI vendors Zarf, and tasks can run vendored Zarf commands using `./zarf`. For example:
```yaml
tasks:
- name: default
  actions:
    - cmd: ./uds inspect k3d-core-istio-dev:0.16.1 # uses system uds
    - cmd: ./zarf tools kubectl get pods -A        # uses vendored Zarf
```

### Architecture Environment Variable
When running tasks with `uds run`, there is a special `UDS_ARCH` environment variable accessible within tasks that is automatically set to your system architecture, but is also configurable with a `UDS_ARCHITECTURE` environmental variable. For example:
```
tasks:
- name: print-arch
  actions:
    - cmd: echo ${UDS_ARCH}
```
- Running `uds run print-arch` will echo your local system architecture
- Running `UDS_ARCHITECTURE=amd64 uds run print-arch` will echo "amd64"

### No Dependency on Zarf
Since UDS CLI also vendors [Zarf](https://github.com/defenseunicorns/zarf), there is no need to also have Zarf installed on your system.
