# 5. Dev Mode

Date: 05 April 2024

## Status
In Progress

## Context

Zarf currently has a dev command that helps speed up the zarf package development cycle, we want to provide similar capabilities to help speed up the UDS bundle development cycle.

The current bundle development lifecycle is:

1. Create a local zarf package or reference a remote zarf package
2. Create a `uds-bundle.yaml` and add packages
3. Create a bundle with `uds create <dir>`
4. Start up a cluster if one does not already exist
5. Run `uds zarf init` to initialize th cluster
6. Deploy the bundle with `uds deploy BUNDLE_TARBALL|OCI_REF]`

Currently the [Local Artifacts](#bundle-create) option has been implemented to provide an MVP (minimal viable product). This ADR is intended to determine potential future implementation.

## Alternatives
Regardless of implementation, the plan is to introduce `uds dev deploy` which allows you to deploy a UDS bundle in dev mode. When deploying in dev mode, any `kind: ZarfInitConfig` packages in the bundle will be ignored. If a **local** zarf package is missing, this command will create that zarf package for you assuming that your `zarf.yaml` file and zarf package are expected in the same directory. It will then create your bundle and deploy your zarf packages in [YOLO](https://docs.zarf.dev/docs/faq#what-is-yolo-mode-and-why-would-i-use-it) mode, eliminating the need to do a `uds zarf init`.

### Local Artifacts
 In order to maximize code reuse and leverage existing logic we will be creating local artifacts (both bundle and zarf package artifacts) in the same way that UDS bundles are currently created and deployed. This solution also minimizes the amount of code needed to process a zarf package regardless if it is a remote or local package.

 + code readibility/reuse regardless if `dev` or not and if processing local or remote packages
 - less efficient

### In Memory
The current `create` and `deploy` functionality does additional work creating local artifacts that potentially isn't necessary. We can speed up the dev cycle even more by doing the package and bundling processing in memory. Zarf currently has a `dev deploy` command that allows you to deploy zarf packages in memory in YOLO mode. This code can be leveraged, but only works for local packages. Additional work would be needed to handle remote packages. We will also need to handle the passing of variables and overrides between zarf packages within a bundle.

+ more efficient
- more new code, less reuse between `dev` and non-dev and local and remote zarf packages.

## Decision


## Consequences
Commands under `dev` are meant to be used in **development** environments, and are **not** meant to be used in **production** environments. There is still the possibility that a user will use `uds dev deploy` in a production environment, but the command name and documentation will make it clear that this is not the intended use case.

## Related Decisions
 - [Zarf `dev` command](https://github.com/defenseunicorns/zarf/blob/main/adr/0022-dev-cmd.md)
