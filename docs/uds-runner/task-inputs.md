---
title: Task Inputs and Reusable Tasks
type: docs
weight: 7
---

While the expectation is for tasks to be reusable, there are instances where you may need to create a task that accommodates various inputs for reuse. To create such a versatile task, include an `inputs` key featuring a mapping of inputs to the task:

```yaml
tasks:
  - name: echo-var
    inputs:
      hello-input:
        default: hello world
        description: This is an input to the echo-var task
      deprecated-input:
        default: foo
        description: this is a input from a previous version of this task
        deprecatedMessage: this input is deprecated, use hello-input instead
    actions:
      # to use the input, reference it using INPUT_<INPUT_NAME> in all caps
      - cmd: echo $INPUT_HELLO_INPUT

  - name: use-echo-var
    actions:
      - task: echo-var
        with:
          # hello-input is the name of the input in the echo-var task, hello-unicorn is the value we want to pass in
          hello-input: hello unicorn
```

In the provided example, the `echo-var` task is configured to receive an input named `hello-input` and display it on the console. It's worth noting that the `input` can be assigned a `default` value. Subsequently, the `use-echo-var` task invokes `echo-var` with an alternative input value, specified through the `with` key. In this example, the input `hello unicorn` is passed to the `hello-input` input.

It's important to highlight the presence of the `deprecated-input` input, which comes with a `deprecatedMessage` attribute. This attribute serves the purpose of signaling that the input is deprecated and should be avoided. Should a task be executed with a deprecated input, a warning message will be printed to the console.

### Templates

When creating a task with `inputs` you can use [Go templates](https://pkg.go.dev/text/template#hdr-Functions) in that task's `actions`. For example:

```yaml
tasks:
  - name: length-of-inputs
    inputs:
      hello-input:
        default: hello world
        description: This is an input to the echo-var task
      another-input:
        default: another world
    actions:
      # index and len are go template functions, while .inputs is map representing the inputs to the task
      - cmd: echo ${{ index .inputs "hello-input" | len }}
      - cmd: echo ${{ index .inputs "another-input" | len }}

  - name: len
    actions:
      - task: length-of-inputs
        with:
          hello-input: hello unicorn
```

Running `uds run len` will print the length of the inputs to `hello-input` and `another-input` to the console.
