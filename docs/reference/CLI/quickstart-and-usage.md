---
title: Quickstart and Usage
---

## Install

Recommended installation method is with Brew:

```bash
brew tap defenseunicorns/tap && brew install uds
```

UDS CLI Binaries are also included with each [Github Release](https://github.com/defenseunicorns/uds-cli/releases)

## Contributing

Build instructions and contributing docs are located in [CONTRIBUTING.md](https://github.com/defenseunicorns/uds-cli/blob/main/CONTRIBUTING.md).

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

The packages referenced in `packages` can exist either locally or in an OCI registry. See [here](https://github.com/defenseunicorns/uds-cli/tree/main/src/test/bundles/03-local-and-remote) for an example that deploys both local and remote Zarf packages. More `UDSBundle` examples can be found in the [src/test/bundles](https://github.com/defenseunicorns/uds-cli/tree/main/src/test/bundles) folder.

### Declarative Syntax

The syntax of a `uds-bundle.yaml` is entirely declarative. As a result, the UDS CLI will not prompt users to deploy optional components in a Zarf package. If you want to deploy an optional Zarf component, it must be specified in the `optionalComponents` key of a particular `package`.

### First-class UDS Support

When running `deploy`,`inspect`,`remove`, and `pull` commands, UDS CLI contains shorthand for interacting with the Defense Unicorns org on GHCR. Specifically, unless otherwise specified, paths will automatically be expanded to the Defense Unicorns org on GHCR. For example:

- `uds deploy unicorn-bundle:v0.1.0` is equivalent to `uds deploy ghcr.io/defenseunicorns/packages/uds/bundles/unicorn-bundle:v0.1.0`

The bundle matching and expansion is ordered as follows:

1. Local with a `tar.zst` extension
2. Remote path: `oci://ghcr.io/defenseunicorns/packages/uds/bundles/<path>`
3. Remote path: `oci://ghcr.io/defenseunicorns/packages/delivery/<path>`
4. Remote path: `oci://ghcr.io/defenseunicorns/packages/<path>`

That is to say, if the bundle is not local, UDS CLI will check path 2, path 3, etc for the remote bundle artifact. This behavior can be overridden by specifying the full path to the bundle artifact, for example `uds deploy ghcr.io/defenseunicorns/dev/path/dev-bundle:v0.1.0`.

### Bundle Create

Pulls the Zarf packages from the registry and bundles them into an OCI artifact.

There are 2 ways to create Bundles:

1. Inside an OCI registry: `uds create <dir> -o ghcr.io/defenseunicorns/dev`
1. Locally on your filesystem: `uds create <dir>`

:::note
The `--insecure` flag is necessary when interacting with a local registry, but not from secure, remote registries such as GHCR.
:::

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

### Pruning Unreferenced Packages

In the process of upgrading bundles, it's common to swap or remove packages from a `uds-bundle.yaml`. These packages can become `unreferenced`, meaning that they are still deployed to the cluster, but are no longer referenced by a bundle. To remove these packages from the cluster, you can use the `--prune` flag when deploying a bundle.

#### Pre-Deploy View

When `uds deploy` is executed, the bundle's metadata, along with a list of its packages and each package's overrides and Zarf variables, will be outputted to the terminal. Unlike [`inspect --list-variables`](#viewing-variables), this output will show the value set for each override or Zarf variable. Overrides and variables that have not been set will not be shown in the output.

:::note
To view this output more easily or for troubleshooting, run `deploy` without the `--confirm` flag.
:::

### Bundle Inspect

Inspect the `uds-bundle.yaml` of a bundle

1. From an OCI registry: `uds inspect oci://ghcr.io/defenseunicorns/dev/<name>:<tag>`
1. From your local filesystem: `uds inspect uds-bundle-<name>.tar.zst`

#### Viewing Images in a Bundle

It is possible to derive images from a `uds-bundle.yaml`, local UDS tarball artifacts, and remote OCI repos. This can be useful for situations where you need to know what images will be bundled before you actually create the bundle or what images will be deployed if using an already created bundle. This is accomplished with the `--list-images` flag. For example:

`uds inspect --list-images [BUNDLE_YAML_FILE|BUNDLE_TARBALL|OCI_REF]`

This command will return a list of images derived from the bundle's packages, taking into account optional and required package components.

The list of images will be grouped by package they are derived from and outputted in a YAML format.

e.g.
`uds inspect k3d-core-slim-dev:0.26.0 --list-images`

```yaml
core-slim-dev:
- docker.io/istio/pilot:1.22.3-distroless
- docker.io/istio/proxyv2:1.22.3-distroless
- ghcr.io/defenseunicorns/pepr/controller:v0.34.1
- quay.io/keycloak/keycloak:24.0.5
- ghcr.io/defenseunicorns/uds/identity-config:0.6.0
init:
- library/registry:2.8.3
- library/registry:2.8.3
- ghcr.io/zarf-dev/zarf/agent:v0.38.2
```

*To extract only the image names and de-dupe*:

`uds inspect k3d-core-slim-dev:0.26.0 --list-images | yq '.[] | .[]'` | sort | uniq
```yaml
docker.io/istio/pilot:1.22.3-distroless
docker.io/istio/proxyv2:1.22.3-distroless
ghcr.io/defenseunicorns/pepr/controller:v0.34.1
ghcr.io/defenseunicorns/uds/identity-config:0.6.0
ghcr.io/zarf-dev/zarf/agent:v0.38.2
library/registry:2.8.3
quay.io/keycloak/keycloak:24.0.5
```


#### Viewing SBOMs

There are 2 additional flags for the `uds inspect` command you can use to extract and view SBOMs:

- Output the SBOMs as a tar file: `uds inspect ... --sbom`
- Output SBOMs into a directory as files: `uds inspect ... --sbom --extract`

This functionality will use the `sboms.tar` of the underlying Zarf packages to create new a `bundle-sboms.tar` artifact containing all SBOMs from the Zarf packages in the bundle.

#### Viewing Variables

To view the configurable overrides and Zarf variables of a bundle's packages:

`uds inspect --list-variables [BUNDLE_YAML_FILE|BUNDLE_TARBALL|OCI_REF]`

### Bundle Publish

Local bundles can be published to an OCI registry like so:
`uds publish <bundle>.tar.zst oci://<registry>`

As an example: `uds publish uds-bundle-example-arm64-0.0.1.tar.zst oci://ghcr.io/github_user`

#### Tagging

Bundles, by default, are tagged based on the bundle version found in the metadata of the `uds-bundle.yaml` file. To override the default tag, you can use the `--version` flag like so:

`uds publish uds-bundle-example-arm64-0.0.1.tar.zst oci://ghcr.io/github_user --version <custom-tag>`

### Bundle Remove

Removes the bundle

There are 2 ways to remove Bundles:

1. From an OCI registry: `uds remove oci://ghcr.io/defenseunicorns/dev/<name>:<tag> --confirm`
1. From your local filesystem: `uds remove uds-bundle-<name>.tar.zst --confirm`

By default all the packages in the bundle are removed, but you can also remove only certain packages in the bundle by using the `--packages` flag.

As an example: `uds remove uds-bundle-<name>.tar.zst --packages init,nginx`

### Logs

:::note
Only works with `uds deploy` for now, may work for other operations but isn't guaranteed.
:::

The `uds logs` command can be used to view the most recent logs of a bundle operation. Note that depending on your OS temporary directory and file settings, recent logs are purged after a certain amount of time, so this command may return an error if the logs are no longer available.

## Bundle Architecture and Multi-Arch Support

There are several ways to specify the architecture of a bundle according to the following precedence:

1. Setting `--architecture` or `-a` flag during `uds ...` operations: `uds create <dir> --architecture arm64`
2. Setting a `UDS_ARCHITECTURE` environment variable
3. Setting the `options.architecture` key in a `uds-config.yaml`
4. Setting the `metadata.architecture` key in a `uds-bundle.yaml`

This means that setting the `--architecture` flag takes precedence over all other methods of specifying the architecture.

UDS CLI supports multi-arch bundles. This means you can push bundles with different architectures to the same remote OCI repository, at the same tag. For example, you can push both an `amd64` and `arm64` bundle to `ghcr.io/<org>/<bundle name>:0.0.1`.

### Architecture Validation

When deploying a local bundle, the bundle's architecture will be used for comparison against the cluster architecture to ensure compatibility. If deploying a remote bundle, by default the bundle is pulled based on system architecture, which is then checked against the cluster.

:::note
It is possible to override the bundle architecture used at validation time by using the `--architecture` / `-a` flag.
:::

If, for example, you have a multi-arch remote bundle that you want to deploy from an arm64 machine to an amd64 cluster, the validation with fail because the system arch does not match the cluster arch. However, you can pull the correct bundle version by specifying the arch with the command line architecture flag.

e.g.
`uds deploy -a amd64 <remote-multi-arch-bundle.tar.zst> --confirm`

## Variables and Configuration

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
  my-zarf-package: # name of Zarf package
    ui_color: green # key is not case sensitive and refers to name of Zarf variable
    UI_MSG: "Hello Unicorn"
    hosts: # variables can be complex types such as lists and maps
      - host: burning.boats
        paths:
          - path: "/"
            pathType: "Prefix"
```

The `options` key contains UDS CLI options that are not specific to a particular Zarf package. The `variables` key contains variables that are specific to a particular Zarf package. If you want to share insensitive variables across multiple Zarf packages, you can use the `shared` key, where the key is the variable name and the value is the variable value.

### Sharing Variables

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

In the example above, the `OUTPUT` variable is created as part of a Zarf Action in the [output-var](https://github.com/defenseunicorns/uds-cli/tree/main/src/test/packages/no-cluster/output-var) package, and the [receive-var](https://github.com/defenseunicorns/uds-cli/tree/main/src/test/packages/no-cluster/receive-var) package expects a variable called `OUTPUT`.

### Sharing Variables Across Multiple Packages

If a Zarf variable has the same name in multiple packages and you don't want to set it multiple times via the import/export syntax, you can set an environment variable prefixed with `UDS_` and it will be applied to all the Zarf packages in a bundle. For example, if multiple packages require a `DOMAIN` variable, you could set it once with a `UDS_DOMAIN` environment variable and it would be applied to all packages. Note that this can also be done with the `shared` key in the `uds-config.yaml` file.

On deploy, you can also set package variables by using the `--set` flag. If the package name isn't included in the key
(example: `--set super=true`) the variable will get applied to all of the packages. If the package name is included in the key (example: `--set cool-package.super=true`) the variable will only get applied to that package.

### Variable Precedence and Specificity

In a bundle, variables can come from 6 sources. Those sources and their precedence are shown below in order of least to most specificity:

- declared in a Zarf pkg
- `import`'ed from a bundle package's `export`
- configured in the `shared` key in a `uds-config.yaml`
- configured in the `variables` key in a `uds-config.yaml`
- set with an environment variable prefixed with `UDS_` (ex. `UDS_OUTPUT`)
- set using the `--set` flag when running the `uds deploy` command

That is to say, variables set using the `--set` flag take precedence over all other variable sources.

### Configuring Zarf Init Packages
Zarf init packages that are typically deployed using `zarf init` have a few special flags that are attached to that command. These options can be configured like any other variable: specified in a `uds-config.yaml`, as an environment variable prefixed with `UDS_` or via the `--set` flag.
```yaml
# uds-config.yaml
variables:
  zarf-init:
    INIT_REGISTRY_URL: "https://my-registry.io"
    INIT_REGISTRY_PUSH_USERNAME: "registry-user"
```

## Duplicate Packages And Naming

It is possible to deploy multiple instances of the same Zarf package in a bundle. For example, the following `uds-bundle.yaml` deploys 3 instances of the [helm-overrides](https://github.com/defenseunicorns/uds-cli/blob/main/src/test/packages/helm/zarf.yaml) Zarf packages:

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

:::note
Today the duplicate packages feature is only supported for packages with Helm charts. This is because Helm charts' [namespaces can be overridden](https://github.com/defenseunicorns/uds-cli/blob/main/docs/overrides.md) at deploy time.
:::

## Zarf Integration

UDS CLI includes a vendored version of Zarf inside of its binary. To use Zarf, simply run `uds zarf <command>`. For example, to create a Zarf package, run `uds zarf create <dir>`, or to use the [airgap tooling](https://docs.zarf.dev/docs/the-zarf-cli/cli-commands/zarf_tools) that Zarf provides, run `uds zarf tools <cmd>`.

## Dev Mode

:::note
Dev mode is a BETA feature
:::

Dev mode facilitates faster dev cycles when developing and testing bundles

```sh
uds dev deploy <path-to-bundle-yaml-dir> | <oci-ref>
```

The `dev deploy` command performs the following operations:

- Deploys the bundle in [YOLO](https://docs.zarf.dev/faq/#what-is-yolo-mode-and-why-would-i-use-it) mode, eliminating the need to do a `zarf init`
  - Any `kind: ZarfInitConfig` packages in the bundle will be ignored
- For local bundles:
  - For local packages:
    - Creates the Zarf tarball if one does not already exist or the `--force-create` flag can be used to force the creation of a new Zarf package
      - The Zarf tarball is created in the same directory as the `zarf.yaml`
      - The `--flavor` flag can be used to specify what flavor of a package you want to create (example: `--flavor podinfo=upstream` to specify the flavor for the `podinfo` package or `--flavor upstream` to specify the flavor for all the packages in the bundle)
  - For remote packages:
    - The `--ref` flag can be used to specify what package ref you want to deploy (example: `--ref podinfo=0.2.0`)
  - Creates a bundle from the newly created Zarf packages

## Monitor

UDS CLI provides a `uds monitor` command that can be used to monitor the status of a UDS cluster

### Monitor Pepr

To monitor the status of a UDS cluster's admission and operator controllers, run: `uds monitor pepr`

#### UDS Controllers

UDS clusters contain two Kubernetes controllers, both created using [Pepr](https://pepr.dev/):

1. **Admission Controller**: Corresponds to the `pepr-uds-core` pods in the cluster. This controller is responsible for validating and mutating resources in the cluster including the enforcement of [UDS Exemptions](https://uds.defenseunicorns.com/reference/configuration/uds-configure-policy-exemptions).

1. **Operator Controller**: Corresponds to the `pepr-uds-core-watcher` pods. This controller is responsible for managing the lifecyle of [UDS Package](https://uds.defenseunicorns.com/reference/configuration/uds-operator/) resources in the cluster.

#### Monitor Args

Aggregate all admission and operator logs into a single stream:

```bash
uds monitor pepr
```

Stream UDS Operator actions (UDS Package processing, status updates, and errors):

```bash
uds monitor pepr operator
```

Stream UDS Policy logs (Allow, Deny, Mutate):

```bash
uds monitor pepr policies
```

Stream UDS Policy allow logs:

```bash
uds monitor pepr allowed
```

Stream UDS Policy deny logs:

```bash
uds monitor pepr denied
```

Stream UDS Policy mutation logs:

```bash
uds monitor pepr mutated
```

Stream UDS Policy deny logs and UDS Operator error logs:
`uds monitor pepr failed`

#### Monitor Flags

`-f, --follow` Continuously stream Pepr logs

`--json` Return the raw JSON output of the logs
``
`--since duration Only return logs newer than a relative duration like 5s, 2m, or 3h. Defaults to all logs.

`-t, --timestamps` Show timestamps in Pepr log

## Scan

:::note
Trivy is a prerequisite for scanning container images and filesystem for vulnerabilities. You can find more information and installation instructions at [Trivy's official documentation](https://aquasecurity.github.io/trivy).
:::

The `scan` command is used to scan a Zarf package for vulnerabilities and generate a report. This command is currently in ALPHA.

### Usage

To use the `scan` command, run:

```sh
uds scan --org <organization> --package-name <package-name> --tag <tag> [options]
```

### Required Parameters

- `--org` or `-o`: Organization name (default: `defenseunicorns`)
- `--package-name` or `-n`: Name of the package (e.g., `packages/uds/gitlab-runner`)
- `--tag` or `-g`: Tag name (e.g., `16.10.0-uds.0-upstream`)
- `--output-file` or `-f`: Output file for CSV results

### Optional Parameters

- `--docker-username` or `-u`: Docker username for registry access, accepts CSV values
- `--docker-password` or `-p`: Docker password for registry access, accepts CSV values

### Example Usage

```sh
uds scan -o defenseunicorns -n packages/uds/gitlab-runner -g 16.10.0-uds.0-upstream -u docker-username -p docker-password -f gitlab-runner.csv
```
