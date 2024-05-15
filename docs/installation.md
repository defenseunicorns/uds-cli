---
title: Quickstart Guide
type: docs
weight: 1
---

## Install UDS CLI

Homebrew is the recommended installation method:

```git
brew tap defenseunicorns/tap && brew install uds
```

{{% alert-note %}}
Please note that UDS CLI Binaries are also included with each [GitHub release.](https://github.com/defenseunicorns/uds-cli/releases)
{{% /alert-note %}}

## Quickstart Guide

The UDS-CLI's primary feature is the ability to deploy multiple, independent Zarf Packages. To create a `UDSBundle` containing Zarf Packages, you can use the following `uds-bundle.yaml` file as a template:

```yaml
kind: UDSBundle
metadata:
  name: example
  description: an example UDS bundle
  version: 0.0.1

packages:
  - name: init
    repository: ghcr.io/defenseunicorns/packages/init
    ref: v0.33.0
    optionalComponents:
      - git-server
  - name: podinfo
    repository: ghcr.io/defenseunicorns/uds-cli/podinfo
    ref: 0.0.1
```

In this example, the `UDSBundle` named "example" deploys the Zarf Init Package and Podinfo. The packages listed under the `packages` section can be sourced either locally or from an OCI registry. Please see this [comprehensive example](https://github.com/defenseunicorns/uds-cli/blob/main/src/test/bundles/03-local-and-remote/uds-bundle.yaml) deploying both local and remote Zarf Packages. Additional `UDSBundle` examples can be explored in the [`src/test/bundles`](https://github.com/defenseunicorns/uds-cli/tree/main/src/test/bundles) folder.

### Declarative Syntax

The syntax of a `uds-bundle.yaml` file is entirely declarative. As a result, the UDS CLI does not prompt users to deploy optional components included in a Zarf Package. If there is a desire to deploy an optional Zarf component, it must be explicitly specified within the `optionalComponents` key of a specific `package`.

### UDS Support

When utilizing the UDS CLI for `deploy`, `inspect`, `remove`, and `pull` commands, the tool incorporates shorthand functionality for streamlined interaction with the Defense Unicorns organization on the GitHub Container Registry (GHCR). By default, paths are automatically expanded to the Defense Unicorns organization on GHCR, unless otherwise specified. For example:

`uds deploy unicorn-bundle:v0.1.0` is functionally equivalent to `uds deploy ghcr.io/defenseunicorns/packages/uds/bundles/unicorn-bundle:v0.1.0`

The matching and expansion of bundles is ordered as follows:

1. Local with a `tar.zst` extension.
2. Remote path: `oci://ghcr.io/defenseunicorns/packages/uds/bundles/<path>`.
3. Remote path: `oci://ghcr.io/defenseunicorns/packages/delivery/<path>`.
4. Remote path: `oci://ghcr.io/defenseunicorns/packages/<path>`.

If the bundle is not local, the UDS CLI will sequentially check path 2, path 3, and so forth, for the remote bundle artifact. This behavior can be overridden by explicitly specifying the full path to the bundle artifact, as demonstrated in the example: `uds deploy ghcr.io/defenseunicorns/dev/path/dev-bundle:v0.1.0`.

### Bundle Create

The `uds create` command pulls the necessary Zarf Packages from the registry and bundles them into an OCI artifact.

There are two ways to create UDS Bundles:

- Inside an OCI registry: `uds create <dir> -o ghcr.io/defenseunicorns/dev`.
- Locally on your filesystem: `uds create <dir>`.

{{% alert-note %}}
The `--insecure` flag is necessary when engaging with a local registry; however, it is not required when operating with secure, remote registries such as GHCR.
{{% /alert-note %}}

### Bundle Deploy

The `uds deploy` command deploys the UDS Bundle.

There are two ways to deploy UDS Bundles:

- From an OCI registry: `uds deploy ghcr.io/defenseunicorns/dev/<name>:<tag>`.
- From your local filesystem: `uds deploy uds-bundle-<name>.tar.zst`.

#### Specifying Packages Using `--packages`

By default, the deployment process includes all packages within the specified bundle. However, you have the option to selectively deploy specific packages from the bundle using the `--packages` flag.

For example: `uds deploy uds-bundle-<name>.tar.zst --packages init,nginx`.

In this example, only the `init` and `nginx` packages from the specified bundle will be deployed.

#### Resuming Bundle Deploys Using `--resume`

By default, the deployment process includes all packages within the bundle, irrespective of whether they have been previously deployed. However, users have the option to selectively deploy only those packages that have not been deployed before, achieved by utilizing the `--resume` flag.

For example: `uds deploy uds-bundle-<name>.tar.zst --resume`.

This command signifies a deployment operation where only the packages within the specified bundle that have not been deployed previously will be processed.

### Bundle Inspect

The `uds inspect` command inspects the `uds-bundle.yaml` of a bundle.

There are two ways to inspect UDS Bundles:

- From an OCI registry: `uds inspect oci://ghcr.io/defenseunicorns/dev/<name>:<tag>`.
- From your local filesystem: `uds inspect uds-bundle-<name>.tar.zst`.

#### Viewing Software Bill of Materials (SBOM)

The `uds inspect` command offers two additional flags for extracting and viewing the SBOM:

- To output the SBOM as a tar file, use: `uds inspect ... --sbom`.
- To output the SBOM into a directory as individual files, use: `uds inspect ... --sbom --extract`.

This functionality utilizes the `sboms.tar` within the underlying Zarf Packages to generate a new artifact named `bundle-sboms.tar`. This artifact consolidates all SBOMs from the Zarf Packages within the bundle.

### Bundle Publish

To publish local bundles to an OCI registry, utilize the following command format: `uds publish <bundle>.tar.zst oci://<registry>`.

For example: `uds publish uds-bundle-example-arm64-0.0.1.tar.zst oci://ghcr.io/github_user`.

### Bundle Remove

The `uds remove` command will remove the UDS Bundle.

There are two ways to remove UDS Bundles:

- From an OCI registry: `uds remove oci://ghcr.io/defenseunicorns/dev/<name>:<tag> --confirm`.
- From your local filesystem: `uds remove uds-bundle-<name>.tar.zst --confirm`.

By default, the removal operation targets all packages within the bundle. However, users have the flexibility to selectively remove specific packages from the bundle by employing the `--packages` flag.

For instance, the following command removes only the `init` and `nginx` packages from the bundle: `uds remove uds-bundle-<name>.tar.zst --packages init,nginx`.

### Logs

The `uds logs` command can be used to view the most recent logs of a bundle operation.

{{% alert-note %}}
Depending on your OS temporary directory and file settings, recent logs are purged after a certain amount of time, so this command may return an error if the logs are no longer available. Currently, `uds logs` is compatible only with `uds deploy`. While it may function with other operations, compatibility is not guaranteed.
{{% /alert-note %}}

## Zarf Integration

The UDS CLI incorporates a vendored version of Zarf within its binary distribution. To leverage the Zarf functionality, execute the command `uds zarf <command>`. For instance, to generate a Zarf Package, run the command `uds zarf package create <dir>`. Similarly, for utilizing the [air gap toolset](https://docs.zarf.dev/commands/zarf_tools/) provided by Zarf, execute `uds zarf tools <cmd>`.

### Development Mode

In development mode, you can accelerate development and testing cycles for UDS Bundles. The `uds dev deploy` command facilitates the deployment of a UDS Bundle in development mode. If a local Zarf Package is missing, this command will generate the required Zarf Package for you, assuming that both the `zarf.yaml` file and the Zarf Package are located in the same directory. Additionally, it will create your bundle and deploy the Zarf Packages in [`YOLO`](https://docs.zarf.dev/faq#what-is-yolo-mode-and-why-would-i-use-it) mode, eliminating the need to execute `zarf init`.

{{% alert-note %}}
Currently, development mode only works with local bundles.
{{% /alert-note %}}

## Bundle Architecture and Multi-Arch Support

There are several ways to specify the architecture of a bundle:

- Setting `--architecture` or `-a` flag during `uds ...` operations: `uds create <dir> --architecture arm64`.
- Setting the `metadata.architecture` key in a `uds-bundle.yaml`.
- Setting a `UDS_ARCHITECTURE` environment variable.
- Setting the `options.architecture` key in a `uds-config.yaml`.

{{% alert-note %}}
Setting the `--architecture` flag takes precedence over all other methods of specifying the architecture.
{{% /alert-note %}}

UDS CLI supports multi-arch bundles. This means you can push bundles with different architectures to the same remote OCI repository, at the same tag. For example, you can push both an `amd64` and `arm64` bundle to `ghcr.io/<org>/<bundle name>:0.0.1`.
