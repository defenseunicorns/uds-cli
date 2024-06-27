# 2. Addition of UDS Runner

Date: 1 Feb 2024

## Status
[AMENDED](#amendment-1)

## Context

Due to frustration with current build tooling (ie. Makefiles) and the need for a more custom build system for UDS, we have decided to experiment with new a feature in UDS CLI called UDS Runner. This feature will allow users to define and run complex build-related workflows in a simple, declarative way, based on existing functionality in Zarf Actions.

### Alternatives

#### Make
Aside from concerns around syntax, maintainability and personal preference, we wanted a tool that we could use in all environments (dev, CI, prod, etc), and could support creating and deploying Zarf packages and UDS bundles. After becoming frustrated with several overly-large and complex Makefiles to perform these tasks, the team decided to explore additional tooling outside of Make.

#### Task
According to the [official docs](https://taskfile.dev/) "Task is a task runner / build tool that aims to be simpler and easier to use than, for example, GNU Make." This project was evaluated during a company Dash Days event and was found to be a good fit for our needs. However, due to the context of the larger UDS ecosystem, we are largely unable to bring in projects that have primarily non-US contributors.

## Decision

After quickly gaining adoption across the organization, we have decided to make the UDS Runner a first-class citizen in UDS CLI. It is important to note that although UDS Runner takes ideas from Task, Github Actions and Gitlab pipelines, it is not a direct copy of any of these tools, and implementing a particular pattern from one of these tools does not mean that all features from that tool should be implemented.


## Consequences

The UDS CLI team will own the UDS Runner functionality and is responsible for maintaining it. Furthermore, because the UDS Runner uses Zarf, the UDS CLI team will contribute to upstream Zarf Actions and common library functionality to support UDS Runner.

# Amendment 1

Date: 1 March 2024

## Status

Accepted

## Context and Decision
In an effort to reduce the scope of UDS CLI and experiment with a new standalone project, the UDS Runner functionality will be moved to a new project tentatively named [maru-runner](https://github.com/defenseunicorns/maru-runner). This project will be maintained by the UDS CLI team for a short time but ownership will be eventually be transferred to a different team. Furthermore, UDS CLI will vendor the runner such that no breaking changes will be introduced for UDS CLI users.
