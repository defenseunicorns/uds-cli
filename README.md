# UDS-CLI

[![Latest Release](https://img.shields.io/github/v/release/defenseunicorns/uds-cli)](https://github.com/defenseunicorns/uds-cli/releases)
[![Go version](https://img.shields.io/github/go-mod/go-version/defenseunicorns/uds-cli?filename=go.mod)](https://go.dev/)
[![Build Status](https://img.shields.io/github/actions/workflow/status/defenseunicorns/uds-cli/release.yaml)](https://github.com/defenseunicorns/uds-cli/actions/workflows/release.yaml)
[![OpenSSF Scorecard](https://api.securityscorecards.dev/projects/github.com/defenseunicorns/uds-cli/badge)](https://api.securityscorecards.dev/projects/github.com/defenseunicorns/uds-cli)

## Table of Contents

1. [Install](#install)
1. [Contributing](CONTRIBUTING.md)
1. [Quickstart](#quickstart)
    - [Create](#bundle-create)
    - [Deploy](#bundle-deploy)
    - [Inspect](#bundle-inspect)
    - [Publish](#bundle-publish)
    - [Remove](#bundle-remove)
    - [Logs](#logs)
1. [Bundle Architecture and Multi-Arch Support](#bundle-architecture-and-multi-arch-support)
1. [Configuration](#configuration)
1. [Sharing Variables](#sharing-variables)
1. [Duplicate Packages and Naming](#duplicate-packages-and-naming)
1. [Zarf Integration](#zarf-integration)
1. [Bundle Overrides](docs/overrides.md)
1. [Bundle Anatomy](docs/anatomy.md)
1. [Runner](docs/runner.md)
1. [Dev Mode](#dev-mode)

## Install
Recommended installation method is with Brew:
```
brew tap defenseunicorns/tap && brew install uds
```
UDS CLI Binaries are also included with each [Github Release](https://github.com/defenseunicorns/uds-cli/releases)

## Contributing
Build instructions and contributing docs are located in [CONTRIBUTING.md](CONTRIBUTING.md).

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
    ref: v0.33.0
    optionalComponents:
      - git-server
  - name: podinfo
    repository: ghcr.io/defenseunicorns/uds-cli/podinfo
    ref: 0.0.1
```
The above `UDSBundle` deploys the Zarf init package and podinfo.

The packages referenced in `packages` can exist either locally or in an OCI registry. See [here](src/test/packages/03-local-and-remote) for an example that deploys both local and remote Zarf packages. More `UDSBundle` examples can be found in the [src/test/bundles](src/test/bundles) folder.

#### Declarative Syntax
The syntax of a `uds-bundle.yaml` is entirely declarative. As a result, the UDS CLI will not prompt users to deploy optional components in a Zarf package. If you want to deploy an optional Zarf component, it must be specified in the `optionalComponents` key of a particular `package`.

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

#### Specifying Packages using `--packages`
By default all the packages in the bundle are deployed, but you can also deploy only certain packages in the bundle by using the `--packages` flag.

As an example: `uds deploy uds-bundle-<name>.tar.zst --packages init,nginx`

#### Resuming Bundle Deploys using `--resume`
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

### Logs

> [!NOTE]
> Only works with `uds deploy` for now, may work for other operations but isn't guaranteed.

The `uds logs` command can be used to view the most recent logs of a bundle operation. Note that depending on your OS temporary directory and file settings, recent logs are purged after a certain amount of time, so this command may return an error if the logs are no longer available.

## Bundle Architecture and Multi-Arch Support
There are several ways to specify the architecture of a bundle according to the following precedence:
1. Setting `--architecture` or `-a` flag during `uds ...` operations: `uds create <dir> --architecture arm64`
2. Setting a `UDS_ARCHITECTURE` environment variable
3. Setting the `options.architecture` key in a `uds-config.yaml`
4. Setting the `metadata.architecture` key in a `uds-bundle.yaml`

This means that setting the `--architecture` flag takes precedence over all other methods of specifying the architecture.

UDS CLI supports multi-arch bundles. This means you can push bundles with different architectures to the same remote OCI repository, at the same tag. For example, you can push both an `amd64` and `arm64` bundle to `ghcr.io/<org>/<bundle name>:0.0.1`.


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

Variables that you want to make available to other packages are in the `export` block of the Zarf package to export a variable from. By default, all exported variables are available to all of the packages in a bundle. To have another package ingest a specific exported variable, like in the case of variable name collisions, use the `imports` key to name both the `variable` and `package` that the variable is exported from, like in the example above.

In the example above, the `OUTPUT` variable is created as part of a Zarf Action in the [output-var](src/test/packages/zarf/no-cluster/output-var) package, and the [receive-var](src/test/packages/zarf/no-cluster/receive-var) package expects a variable called `OUTPUT`.

### Sharing Variables Across Multiple Packages
If a Zarf variable has the same name in multiple packages and you don't want to set it multiple times via the import/export syntax, you can set an environment variable prefixed with `UDS_` and it will be applied to all the Zarf packages in a bundle. For example, if multiple packages require a `DOMAIN` variable, you could set it once with a `UDS_DOMAIN` environment variable and it would be applied to all packages. Note that this can also be done with the `shared` key in the `uds-config.yaml` file.

On deploy, you can also set package variables by using the `--set` flag. If the package name isn't included in the key
(example: `--set super=true`) the variable will get applied to all of the packages. If the package name is included in the key (example: `--set cool-package.super=true`) the variable will only get applied to that package.

### Variable Precedence and Specificity
In a bundle, variables can come from 4 sources. Those sources and their precedence are shown below in order of least to most specificity:
- Variables declared in a Zarf pkg
- Variables `import`'ed from a bundle package's `export`
- Variables configured in the `shared` key in a `uds-config.yaml`
- Variables configured in the `variables` key in a `uds-config.yaml`
- Variables set with an environment variable prefixed with `UDS_` (ex. `UDS_OUTPUT`)
- Variables set using the `--set` flag when running the `uds deploy` command

That is to say, variables set using the `--set` flag take precedence over all other variable sources.


## Duplicate Packages And Naming

It is possible to deploy multiple instances of the same Zarf package in a bundle. For example, the following `uds-bundle.yaml` deploys 3 instances of the [helm-overrides](src/test/packages/helm/zarf.yaml) Zarf packags:
```yaml
kind: UDSBundle
metadata:
   name: duplicates
   description: testing a bundle with duplicate packages in specified namespaces
   version: 0.0.1

packages:
   - name: helm-overrides
     repository: localhost:5000/helm-overrides
     ref: 0.0.1
     overrides:
        podinfo-component:
           unicorn-podinfo: # name of Helm chart
              namespace: podinfo-ns

   # note the unique name and namespace
   - name: helm-overrides-duplicate
     repository: localhost:5000/helm-overrides
     ref: 0.0.1
     overrides:
        podinfo-component:
           unicorn-podinfo:
              namespace: another-podinfo-ns

   # note the unique name, namespace and the path to the Zarf package tarball
   - name: helm-overrides-local-duplicate
     path: src/test/packages/helm/zarf-package-helm-overrides-arm64-0.0.1.tar.zst
     ref: 0.0.1
     overrides:
        podinfo-component:
           unicorn-podinfo:
              namespace: yet-another-podinfo-ns
```

The naming conventions for deploying duplicate packages are as follows:
1. The `name` field of the package in the `uds-bundle.yaml` must be unique
1. The duplicate packages must be deployed in different namespaces
1. In order to deploy duplicates of local packages, the `path` field must point to a Zarf package tarball instead of to a folder.

> [!NOTE]  
> Today the duplicate packages feature is only supported for packages with Helm charts. This is because Helm charts' [namespaces can be overridden](docs/overrides.md#namespace) at deploy time.

## Zarf Integration
UDS CLI includes a vendored version of Zarf inside of its binary. To use Zarf, simply run `uds zarf <command>`. For example, to create a Zarf package, run `uds zarf create <dir>`, or to use the [airgap tooling](https://docs.zarf.dev/docs/the-zarf-cli/cli-commands/zarf_tools) that Zarf provides, run `uds zarf tools <cmd>`.

## Dev Mode

> [!NOTE]  
> Dev mode is a BETA feature

Dev mode facilitates faster dev cycles when developing and testing bundles

```
uds dev deploy <path-to-bundle-yaml-dir> | <oci-ref>
```

The `dev deploy` command performs the following operations

- If local bundle: Creates Zarf packages for all local packages in a bundle
  - Creates the Zarf tarball in the same directory as the `zarf.yaml`
  - Will only create the Zarf tarball if one does not already exist
  - Ignores any `kind: ZarfInitConfig` packages in the bundle
  - Creates a bundle from the newly created Zarf packages
- Deploys the bundle in [YOLO](https://docs.zarf.dev/faq/#what-is-yolo-mode-and-why-would-i-use-it) mode, eliminating the need to do a `zarf init`
