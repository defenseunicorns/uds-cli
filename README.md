# UDS-CLI
**Warning**: UDS-CLI is in early alpha, expect changes to the schema and workflow

## Quickstart
The UDS-CLI's flagship feature is deploying multiple, independent Zarf packages. To create a `UDSBundle` of Zarf packages, create a `uds-bundle.yaml` file like so:

```yaml
kind: UDSBundle
metadata:
  name: example
  description: an example UDS bundle
  version: 0.0.1

zarf-packages:
  - name: init 
    repository: localhost:5000/init
    ref: "###BNDL_TMPL_INIT_VERSION###"
    optional-components:
      - git-server
  - name: podinfo
    repository: localhost:5000/podinfo
    ref: 0.0.1
```
The above `UDSBundle` deploys the Zarf init package and podinfo.

The packages referenced in `zarf-packages` can exist either locally or in an OCI registry. See [here](src/test/packages/03-local-and-remote) for an example that deploys both local and remote Zarf packages. More `UDSBundle` examples can be found in the [src/test/packages](src/test/packages) folder. 

#### Declarative Syntax
The syntax of a `uds-bundle.yaml` is entirely declarative. As a result, the UDS CLI will not prompt users to deploy optional components in a Zarf package. If you want to deploy an optional Zarf component, it must be specified in the `optional-components` key of a particular `zarf-package`.

### Bundle Create
Pulls the Zarf packages from the registry and bundles them into an OCI artifact.

There are 2 ways to create Bundles:
1. Inside an OCI registry: `uds create <dir> --set INIT_VERSION=v0.29.1 --insecure -o localhost:5000`
1. Locally on your filesystem: `uds create <dir> --set INIT_VERSION=v0.29.1 --insecure`

Noting that the `--insecure` flag will be necessary when running the registry from the Makefile.

### Bundle Deploy
Deploys the bundle

There are 2 ways to deploy Bundles:
1. From an OCI registry: `uds deploy oci://localhost:5000/<name>:<tag> --insecure`
1. From your local filesystem: `uds deploy uds-bundle-<name>.tar.zst`

### Bundle Inspect
Inspect the `uds-bundle.yaml` of a bundle
1. From an OCI registry: `uds inspect oci://localhost:5000/<name>:<tag> --insecure`
1. From your local filesystem: `uds inspect uds-bundle-<name>.tar.zst`

#### Viewing SBOMs
There are 2 additional flags for the `uds bundle inspect` command you can use to extract and view SBOMs:
- Output the SBOMs as a tar file: `uds inspect ... --sbom`
- Output SBOMs into a directory as files: `uds inspect ... --sbom --extract`

This functionality will use the `sboms.tar` of the  underlying Zarf packages to create new a `bundle-sboms.tar` artifact containing all SBOMs from the Zarf packages in the bundle.

### Bundle Publish
Local bundles can be published to an OCI registry like so:
`uds publish <bundle>.tar.zst oci://<registry> `

As an example: `uds publish uds-bundle-example-arm64-0.0.1.tar.zst oci://ghcr.io/github_user`

## Variables
In addition to setting Bundle templates (`###BNDL_TMPL_###`) in the `uds-bundle.yaml`, you can also pass variables between Zarf packages.
```yaml
kind: UDSBundle
metadata:
  name: simple-vars
  description: show how vars work
  version: 0.0.1

zarf-packages:
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

### Variable Precedence and Specificity
In a bundle, variables can come from 3 sources. Those sources and their precedence are shown below in order of least to most specificity:
- Variables declared in a Zarf pkg
- Variables `import`'ed from a bundle package's `export`
- Variables declared in `uds-config.yaml`

That is to say, deploy-time variables declared in `uds-config.yaml` take precedence over all other variable sources.

## Bundle Anatomy
A UDS Bundle is an OCI artifact with the following form:

![](docs/.images/uds-bundle.png)