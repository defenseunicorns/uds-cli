---
title: UDS Runner
type: docs
weight: 4
---

The UDS Runner enables UDS Bundle developers to automate UDS builds and execute routine shell tasks. The UDS CLI contains vendors and configures the maru-runner build tool to support simple compiling and building of UDS Bundles.
Quickstart
Running a Task

To run a task from a tasks.yaml execute the following command:

uds run <task-name>

To run a task from a specific tasks file, execute the following command:

uds run -f <path/to/tasks.yaml> <task-name>

{{% alert-note %}} The maru documentation describes how to build the tasks.yaml files to configure the UDS Runner. {{% /alert-note %}}
Variables Set With Environment Variables

When running a tasks.yaml with uds run my-task variables can be set using environment prefixed with UDS_. For example, running UDS_FOO=bar uds run echo-foo on the following task will echo bar:

variables:
 - name: FOO
   default: foo
tasks:
 - name: echo-foo
   actions:
     - cmd: echo ${FOO}

No Zarf Dependency
Considering that the UDS CLI also vendors Zarf, there is no need to have Zarf installed on your system.
