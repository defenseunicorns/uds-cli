# 5. Dev Mode

Date: 05 April 2024

## Status
In Progress

## Context

Zarf currently has a dev command that helps speed up the zarf package development cycle, we want to provide similar capabilities to help speed up the UDS bundle development cycle.

The current bundle development lifecycle is:

1. Create a local zarf package or reference a remote zarf package
2. Create a `bundle.yaml` and add packages
3. Create a bundle with `uds create <dir>`
4. Start up a cluster if one does not already exist
5. Run `uds zarf init` to initialize th cluster
6. Deploy the bundle with `uds deploy BUNDLE_TARBALL|OCI_REF]`

## Decision
 Introduce `uds dev deploy` which allows you to deploy a UDS bundle in dev mode. If a local zarf package is missing, this command will create that zarf package for you assuming that your `zarf.yaml` file and zarf package are expected in the same directory. It will then create your bundle and deploy your zarf packages in [YOLO](https://docs.zarf.dev/docs/faq#what-is-yolo-mode-and-why-would-i-use-it) mode, eliminating the need to do a `uds zarf init`

## Consequences
Commands under `dev` are meant to be used in **development** environments, and are **not** meant to be used in **production** environments. There is still the possibility that a user will use `uds dev deploy` in a production environment, but the command name and documentation will make it clear that this is not the intended use case.
