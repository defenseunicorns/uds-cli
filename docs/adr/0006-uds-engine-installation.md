# 6. UDS Engine Installation

Date: 12 June 2024

## Status

ACCEPTED

## Context

The goal is to establish a method for installing the UDS Engine in a cluster. The chosen approach should be efficient, user-friendly, and compatible with the existing system architecture and workflows.

## Alternatives

### 1. Install UDS Engine as a UDS Core Capability
This approach involves packaging the UDS Engine in a Zarf package with a Helm chart and deploying it as part of UDS Core.

<b>Pros:</b>
- UDS Engine is installed as part of UDS Core, ensuring seamless integration.
- The initial Engine capability depends on Pepr, which is already installed as part of UDS Core.

<b>Cons:</b>
- UDS Core includes additional capabilities that may not be necessary or desired in all scenarios.

### 2. UDS command
This alternative proposes a UDS cli command that "bootstraps" a cluster with UDS Engine, similar to how Zarf initializes a cluster with the `zarf init` command.

<b>Pros:</b>
- This method is quick and lightweight, allowing users to install only UDS Engine in the cluster.
- It's convenient for rapid development work cycles.

<b>Cons:</b>
- Users would have to manually install/deploy any other dependencies.
- There's a potential for maintaining a `uds-cli` command that rarely gets used, once it's run and UDS Engine is installed.

### 3. UDS Core and UDS Dev command
This strategy proposes integrating UDS Engine as a feature within UDS Core. Additionally, we plan to develop a `uds dev` command to facilitate the Engine's installation. The `uds dev` command is primarily intended for platform engineers who are engaged in development and testing tasks. By designating the installation command as a subcommand of `uds dev`, we underscore that this command is not intended for production environments.

<b>Pros:</b>
- This method combines the advantages of alternatives 1 and 2 while potentially eliminating the disadvantages.

<b>Cons:</b>
- It would require more work to maintain two different ways of installing UDS Engine.

## Decision

Option 1 (Install UDS Engine as a UDS Core Capability).

In adding UDS Engine as a UDS Core application, we will treat it like any other UDS Core application. This approach allows us to leverage the existing UDS Core architecture and workflows. We should be able to do any UDS Engine dev work locally without needing to add any additional `uds dev` commands for installation.

## Consequences

By going with option 1, UDS Engine will be installed as part of UDS Core. This decision will require additional work to ensure that UDS Engine is properly integrated with UDS Core. However, this approach will provide a seamless installation experience for users and maintain compatibility with the existing system architecture and workflows.

```
