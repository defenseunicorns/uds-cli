# UDS-CLI

[![Latest Release](https://img.shields.io/github/v/release/defenseunicorns/uds-cli)](https://github.com/defenseunicorns/uds-cli/releases)
[![Go version](https://img.shields.io/github/go-mod/go-version/defenseunicorns/uds-cli?filename=go.mod)](https://go.dev/)
[![Build Status](https://img.shields.io/github/actions/workflow/status/defenseunicorns/uds-cli/release.yaml)](https://github.com/defenseunicorns/uds-cli/actions/workflows/release.yaml)
[![OpenSSF Scorecard](https://api.securityscorecards.dev/projects/github.com/defenseunicorns/uds-cli/badge)](https://api.securityscorecards.dev/projects/github.com/defenseunicorns/uds-cli)

**:warning: Warning**:  UDS-CLI is in early alpha, expect changes to the schema and workflow

## Table of Contents

1. [Install](#install)
1. [Quickstart](#quickstart)
    - [Create](#bundle-create)
    - [Deploy](#bundle-deploy)
    - [Inspect](#bundle-inspect)
    - [Publish](#bundle-publish)
    - [Remove](#bundle-remove)
1. [Configuration](#configuration)
1. [Sharing Variables](#sharing-variables)
1. [Bundle Overrides](docs/overrides.md)
1. [Bundle Anatomy](docs/anatomy.md)
1. [UDS Runner](docs/runner.md)

## Install
Recommended installation method is with Brew:
```
brew tap defenseunicorns/tap && brew install uds
```
UDS CLI Binaries are also included with each [Github Release](https://github.com/defenseunicorns/uds-cli/releases)


## Quickstart
The UDS-CLI's flagship feature is deploying multiple, independent Zarf packages. To create a `UDSBundle` of Zarf packages, create a `uds-bundle.yaml` file like so:

```yaml
kind: UDSBundle
metadata:
  name: example
  description: an example UDS bundle
  version: 0.0.1

packages:
  - name: init
    repository: ghcr.io/defenseunicorns/packages/init
    ref: v0.31.4
    optional-components:
      - git-server
  - name: podinfo
    repository: localhost:5000/podinfo
    ref: 0.0.1
```
The above `UDSBundle` deploys the Zarf init package and podinfo.

The packages referenced in `packages` can exist either locally or in an OCI registry. See [here](src/test/packages/03-local-and-remote) for an example that deploys both local and remote Zarf packages. More `UDSBundle` examples can be found in the [src/test/packages](src/test/packages) folder.

#### Declarative Syntax
The syntax of a `uds-bundle.yaml` is entirely declarative. As a result, the UDS CLI will not prompt users to deploy optional components in a Zarf package. If you want to deploy an optional Zarf component, it must be specified in the `optional-components` key of a particular `package`.

#### First-class UDS Support
When running `deploy`,`inspect`,`remove`, and `pull` commands, UDS CLI contains shorthand for interacting with the Defense Unicorns org on GHCR. Specifically, unless otherwise specified, paths will automatically be expanded to the Defense Unicorns org on GHCR. For example:
- `uds deploy unicorn-bundle:v0.1.0` is equivalent to `uds deploy ghcr.io/defenseunicorns/packages/uds/bundles/unicorn-bundle:v0.1.0`

The bundle matching and expansion is ordered as follows:
1. Local with a `tar.zst` extension
2. Remote path: `oci://ghcr.io/defenseunicorns/packages/uds/bundles/<path>`
3. Remote path: `oci://ghcr.io/defenseunicorns/packages/delivery/<path>`
4. Remote path: `oci://ghcr.io/defenseunicorns/packages/<path>`

That is to say, if the bundle is not local, UDS CLI will check path 2, path 3, etc for the remote bundle artifact. This behavior can be overriden by specifying the full path to the bundle artifact, for example `uds deploy ghcr.io/defenseunicorns/dev/path/dev-bundle:v0.1.0`.

### Bundle Create
Pulls the Zarf packages from the registry and bundles them into an OCI artifact.

There are 2 ways to create Bundles:
1. Inside an OCI registry: `uds create <dir> -o ghcr.io/defenseunicorns/dev`
1. Locally on your filesystem: `uds create <dir>`

> [!NOTE]  
> The `--insecure` flag is necessary when interacting with a local registry, but not from secure, remote registries such as GHCR.

### Bundle Deploy
Deploys the bundle

There are 2 ways to deploy Bundles:
1. From an OCI registry: `uds deploy ghcr.io/defenseunicorns/dev/<name>:<tag>`
1. From your local filesystem: `uds deploy uds-bundle-<name>.tar.zst`

#### `--packages`
By default all the packages in the bundle are deployed, but you can also deploy only certain packages in the bundle by using the `--packages` flag.

As an example: `uds deploy uds-bundle-<name>.tar.zst --packages init,nginx`

#### `--resume`
By default all the packages in the bundle are deployed, regardless of if they have already been deployed, but you can also choose to only deploy packages that have not already been deployed by using the `--resume` flag

As an example: `uds deploy uds-bundle-<name>.tar.zst --resume`

### Bundle Inspect
Inspect the `uds-bundle.yaml` of a bundle
1. From an OCI registry: `uds inspect oci://ghcr.io/defenseunicorns/dev/<name>:<tag>`
1. From your local filesystem: `uds inspect uds-bundle-<name>.tar.zst`

#### Viewing SBOMs
There are 2 additional flags for the `uds inspect` command you can use to extract and view SBOMs:
- Output the SBOMs as a tar file: `uds inspect ... --sbom`
- Output SBOMs into a directory as files: `uds inspect ... --sbom --extract`

This functionality will use the `sboms.tar` of the  underlying Zarf packages to create new a `bundle-sboms.tar` artifact containing all SBOMs from the Zarf packages in the bundle.

### Bundle Publish
Local bundles can be published to an OCI registry like so:
`uds publish <bundle>.tar.zst oci://<registry> `

As an example: `uds publish uds-bundle-example-arm64-0.0.1.tar.zst oci://ghcr.io/github_user`

### Bundle Remove
Removes the bundle

There are 2 ways to remove Bundles:
1. From an OCI registry: `uds remove oci://ghcr.io/defenseunicorns/dev/<name>:<tag> --confirm`
1. From your local filesystem: `uds remove uds-bundle-<name>.tar.zst --confirm`

By default all the packages in the bundle are removed, but you can also remove only certain packages in the bundle by using the `--packages` flag.

As an example: `uds remove uds-bundle-<name>.tar.zst --packages init,nginx`

## Configuration
The UDS CLI can be configured with a `uds-config.yaml` file. This file can be placed in the current working directory or specified with an environment variable called `UDS_CONFIG`. The basic structure of the `uds-config.yaml` is as follows:
```yaml
options:
   log_level: debug
   architecture: arm64
   no_log_file: false
   no_progress: false
   uds_cache: /tmp/uds-cache
   tmp_dir: /tmp/tmp_dir
   insecure: false
   oci_concurrency: 3

shared:
   domain: uds.dev # shared across all packages in a bundle

variables:
  my-zarf-package:  # name of Zarf package
    ui_color: green # key is not case sensitive and refers to name of Zarf variable
    UI_MSG: "Hello Unicorn"
    hosts:          # variables can be complex types such as lists and maps
       - host: burning.boats
         paths:
            - path: "/"
              pathType: "Prefix"
```
The `options` key contains UDS CLI options that are not specific to a particular Zarf package. The `variables` key contains variables that are specific to a particular Zarf package. If you want to share insensitive variables across multiple Zarf packages, you can use the `shared` key, where the key is the variable name and the value is the variable value.

## Sharing Variables
### Importing/Exporting Variables
Zarf package variables can be passed between Zarf packages:
```yaml
kind: UDSBundle
metadata:
  name: simple-vars
  description: show how vars work
  version: 0.0.1

packages:
  - name: output-var
    repository: localhost:888/output-var
    ref: 0.0.1
    exports:
      - name: OUTPUT
  - name: receive-var
    repository: localhost:888/receive-var
    ref: 0.0.1
    imports:
      - name: OUTPUT
        package: output-var
```

Variables that you want to make available to other package are in the `export` block of the Zarf package to export a variable from. To have another package ingest an exported variable, use the `imports` key to name both the `variable` and `package` that the variable is exported from.

In the example above, the `OUTPUT` variable is created as part of a Zarf Action in the [output-var](src/test/packages/zarf/no-cluster/output-var) package, and the [receive-var](src/test/packages/zarf/no-cluster/receive-var) package expects a variable called `OUTPUT`.

### Sharing Variables Across Multiple Packages
If a Zarf variable has the same name in multiple packages and you don't want to set it multiple times via the import/export syntax, you can set an environment variable prefixed with `UDS_` and it will be applied to all the Zarf packages in a bundle. For example, if multiple packages require a `DOMAIN` variable, you could set it once with a `UDS_DOMAIN` environment variable and it would be applied to all packages. Note that this can also be done with the `shared` key in the `uds-config.yaml` file.

### Variable Precedence and Specificity
In a bundle, variables can come from 4 sources. Those sources and their precedence are shown below in order of least to most specificity:
- Variables declared in a Zarf pkg
- Variables `import`'ed from a bundle package's `export`
- Variables configured in the `shared` key in a `uds-config.yaml`
- Variables configured in the `variables` key in a `uds-config.yaml`
- Variables set with an environment variable prefixed with `UDS_` (ex. `UDS_OUTPUT`)

That is to say, variables set as environment variables take precedence over all other variable sources.
