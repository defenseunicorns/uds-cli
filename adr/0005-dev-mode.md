# 5. Dev Mode

Date: 21 May 2024

## Status
Accepted

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


## Dev Mode Overview
Regardless of implementation, the plan is to introduce `uds dev deploy` which allows you to deploy a UDS bundle in dev mode. Dev mode will perform the following actions:
- Create Zarf packages for all local packages in a bundle
  - The Zarf tarball will be created in the same directory as the `zarf.yaml`
  - Dev mode will only create the Zarf tarball if one does not already exist
  - Ignore any `kind: ZarfInitConfig` packages in the bundle
- Create a bundle from the newly created Zarf packages
- Deploy the bundle in [YOLO](https://docs.zarf.dev/faq/#what-is-yolo-mode-and-why-would-i-use-it) mode, eliminating the need to do a `zarf init`


## Planned Features
 - Add a `--ref` flag to `uds dev deploy` to enable setting the `ref` field for a package at `dev deploy` time
 - Add a `--flavor` flag to `uds dev deploy` to enable setting the flavor of a ref at `dev deploy` time

## Handling Artifacts
The following options are being considered for handling the creation of Zarf packages and bundles in dev mode:

### Local Artifacts
In order to maximize code reuse and leverage existing logic we could create local artifacts (both bundle and Zarf package artifacts) in the same way that UDS bundles are currently created and deployed. This solution also minimizes the amount of code needed to process a zarf package regardless if it is a remote or local package.

 + code readibility/reuse regardless if `dev` or not and if processing local or remote packages
 - less efficient

### No Tarball
The current `create` and `deploy` functionality does additional work creating local artifacts and a tarball that potentially isn't necessary. We can speed up the dev cycle even more by doing the package and bundling processing without needing the intermediary bundle tarball. Zarf currently has a `dev deploy` command that allows you to deploy zarf packages in YOLO mode. This code can be leveraged, but only works for local packages. Additional work would be needed to handle remote packages. We will also need to handle the passing of variables and overrides between zarf packages within a bundle.

+ more efficient
- more new code, less reuse between `dev` and non-dev and local and remote zarf packages.

## Decision
By using the existing local artifacts solution, we can leverage current functionality and accelerate the development cycle. Understanding that this approach may be revisited based on future user feedback.

## Consequences
Commands under `dev` are meant to be used in **development** environments, and are **not** meant to be used in **production** environments. There is still the possibility that a user will use `uds dev deploy` in a production environment, but the command name and documentation will make it clear that this is not the intended use case.

## Related Decisions
 - [Zarf `dev` command](https://github.com/defenseunicorns/zarf/blob/main/adr/0022-dev-cmd.md)
