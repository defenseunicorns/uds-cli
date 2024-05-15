---
title: Variables
type: docs
weight: 9
---

Variables can be defined in several ways:

- At the top of the `tasks.yaml`:

```yaml
variables:
  - name: FOO
    default: foo

tasks: ...
```

- As the output of a `cmd`:

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

- As an environment variable, it is recommended to prefix them with `UDS_`. In the given example, creating an environment variable `UDS_FOO=bar` would result in the `FOO` variable being set to `bar`.
- Using the `--set` flag in the CLI:

```cli
uds run foo --set FOO=bar
```

To use a variable, reference it using `${VAR_NAME}`

Note that variables also have the following attributes when setting them with YAML:

- `sensitive`: boolean value indicating if a variable should be visible in output.
- `default`: default value of a variable.
  - In the provided example, if the variable `FOO` lacks a default value and you set the environment variable `UDS_FOO=bar`, the default value will be configured as `bar`.

### Environment Variable Files

To incorporate a file containing environment variables into a task and load them for use, utilize the `envPath` key within the task configuration. This key facilitates the loading of all environment variables from the specified file into both the task being invoked and its subsequent child tasks:

```yaml
tasks:
  - name: env
    actions:
      - cmd: echo $FOO
      - cmd: echo $UDS_ARCH
      - task: echo-env
  - name: echo-env
    envPath: ./path/to/.env
    actions:
      - cmd: echo different task $FOO
```

### Variable Precendence

Variable precedence is as follows, from least to most specific:

- Variable defaults set in YAML.
- Environment variables prefixed with `UDS_`.
- Variables set with the `--set` flag in the CLI.

{{% alert-note %}}
Variables established using the `--set` flag take precedence over variables from all other outlined sources.
{{% /alert-note %}}
