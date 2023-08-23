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

### Bundle Create
Pulls the Zarf packages from the registry and bundles them into an OCI artifact.

There are 2 ways to create Bundles:
1. Inside an OCI registry: `uds bundle create <dir> --set INIT_VERSION=v0.28.3 --insecure -o localhost:5000`
1. Locally on your filesystem: `uds bundle create <dir> --set INIT_VERSION=v0.28.3 --insecure`

Noting that the `--insecure` flag will be necessary when running the registry from the Makefile.

### Bundle Deploy
Deploys the bundle

There are 2 ways to deploy Bundles:
1. From an OCI registry: `uds bundle deploy oci://localhost:5000/<name>:<tag> --insecure --confirm`
1. From your local filesystem: `uds bundle deploy uds-bundle-<name>.tar.zst --confirm`

## Variables
In addition to setting Bundle templates (`###BNDL_TMPL_###`) in the `uds-bundle.yaml`, you can also pass variables between Zarf packages
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

## Bundle Anatomy
A UDS Bundle is an OCI artifact with the following form:

![](docs/.images/uds-bundle.png)