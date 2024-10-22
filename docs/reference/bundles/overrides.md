---
title: Bundle Overrides
---

Bundle overrides provide a mechanism to customize Helm charts inside of Zarf packages.

## Quickstart

Consider the following `zarf.yaml` and `values.yaml` which deploys [podinfo](https://github.com/stefanprodan/podinfo)
with a couple of custom values:

```yaml
# zarf.yaml
kind: ZarfPackageConfig
metadata:
  name: helm-overrides-package
  version: 0.0.1

components:
  - name: helm-overrides-component
    required: true
    charts:
      - name: podinfo
        version: 6.4.0
        namespace: podinfo
        url: https://github.com/stefanprodan/podinfo.git
        gitPath: charts/podinfo
        valuesFiles:
          - values.yaml
    images:
      - ghcr.io/stefanprodan/podinfo:6.4.0
---
# values.yaml
replicaCount: 1
ui:
  color: blue
```

The bundle overrides feature allows users to override the values specified in Zarf packages. For example:

```yaml
kind: UDSBundle
metadata:
  name: helm-overrides
  description: testing a bundle with Helm overrides
  version: 0.0.1

packages:
  - name: helm-overrides-package
    path: "path/to/pkg"
    ref: 0.0.1

    overrides:
      helm-overrides-component:
        podinfo:
          valuesFiles:
            - values.yaml
          values:
            - path: "replicaCount"
              value: 2
          variables:
            - name: UI_COLOR
              path: "ui.color"
              description: "Set the color for podinfo's UI"
              default: "purple"
```

```yaml
#values.yaml
podAnnotations:
  customAnnotation: "customValue"
```

This bundle will deploy the `helm-overrides-package` Zarf package and override the `replicaCount`, `ui.color`, and `podAnnotations` values in the `podinfo` chart. The `values` can't be modified after the bundle has been created. However, at deploy time, users can override the `UI_COLOR` and other `variables` using a environment variable called `UDS_UI_COLOR` or by specifying it in a `uds-config.yaml` like so:

```yaml
variables:
  helm-overrides-package:
    UI_COLOR: green
```

## Overrides

### Syntax

Consider the following bundle `overrides`:

```yaml
packages:
  - name: helm-overrides-package
    path: "path/to/pkg"
    ref: 0.0.1

    overrides:
      helm-overrides-component: # component name inside of the helm-overrides-package Zarf pkg
        podinfo: # chart name from the helm-overrides-component component
          valuesFiles:
            - values.yaml
          values:
            - path: "replicaCount"
              value: 2
          variables:
            - name: UI_COLOR
              path: "ui.color"
              description: "Set the color for podinfo's UI"
              default: "purple"
```

```yaml
#values.yaml
podAnnotations:
  customAnnotation: "customValue"
```

In this example, the `helm-overrides-package` Zarf package has a component called `helm-overrides-component` which contains a Helm chart called `podinfo`; note how these names are keys in the `overrides` block. The `podinfo` chart has a `replicaCount` value that is overridden to `2`, a `podAnnotations` value that is overridden to include `customAnnotation: "customValue"` and a variable called `UI_COLOR` that is overridden to `purple`.

### Values Files

The `valuesFiles` in an `overrides` block are a list of `file`'s. It allows users to override multiple values in a Zarf package component's underlying Helm chart, by providing a file with those values instead of having to include them all individually in the `overrides` block.

### Values

The `values` in an `overrides` block are a list of `path` and `value` pairs. They allow users to override values in a Zarf package component's underlying Helm chart. Note that values are specified by bundle authors and **cannot be modified** after the bundle has been created.

#### Path

The `path` uses dot notation to specify the location of a value to override in the underlying Helm chart. For example, the `replicaCount` path in the `podinfo` chart is located at the top-level of the [podinfo values.yaml](https://github.com/stefanprodan/podinfo/blob/master/charts/podinfo/values.yaml), so the path is simply `replicaCount`, while the `ui.color` path is located under the `ui` key, so the path is `ui.color`.

#### Value

The `value` is the value to set at the `path`. Values can be simple values such as numbers and strings, as well as, complex lists and objects, for example:

```yaml
---
overrides:
  helm-overrides-component:
    podinfo:
      values:
        - path: "podinfo.tolerations"
          value:
            - key: "unicorn"
              operator: "Equal"
              value: "defense"
              effect: "NoSchedule"
        - path: podinfo.podAnnotations
          value:
            customAnnotation: "customValue"
```

#### Bundle Variables as Values

Bundle and Zarf variables can be used to set override values by using the syntax `${...}`. For example:

```yaml
# uds-config.yaml
variables:
  helm-overrides-package:
    replica_count: 2
```

```yaml
kind: UDSBundle
metadata:
  name: example-bundle
  description: Example for using an imported variable to set an overrides value
  version: 0.0.1

packages:
  - name: output-var
    repository: localhost:888/output-var
    ref: 0.0.1
    exports:
      - name: COLOR

  - name: helm-overrides-package
    path: "../../packages/helm"
    ref: 0.0.1

    overrides:
      podinfo-component:
        unicorn-podinfo:
          values:
            - path: "podinfo.replicaCount"
              value: ${REPLICA_COUNT}
            - path: "podinfo.ui.color"
              value: ${COLOR}
```

In the example above `${REPLICA_COUNT}` is set in the `uds-config.yaml` file and `${COLOR}` is set as an export from the `output-var` package. Note that you could also set these values with the `shared` key in a `uds-config.yaml`, environment variables prefixed with `UDS_` or with the `--set` flag during deployment.

#### Value Precedence

Value precedence is as follows:

1. The `values` in an `overrides` block
1. `values` set in the last `valuesFile` (if more than one specified)
1. `values` set in the previous `valuesFile` (if more than one specified)

### Variables

Variables are similar to [values](#values) in that they allow users to override values in a Zarf package component's underlying Helm chart; they also share a similar syntax. However, unlike `values`, `variables` can be overridden at deploy time. For example, consider the `variables` key in the following `uds-bundle.yaml`:

```yaml
kind: UDSBundle
metadata:
  name: example-bundle
  version: 0.0.1

packages:
  - name: helm-overrides-package
    path: "../../packages/helm"
    ref: 0.0.1"
    overrides:
      podinfo-component:
        unicorn-podinfo:
          variables:
            - name: UI_COLOR
              path: "ui.color"
              description: "Set the color for podinfo's UI"
              default: "purple"
```

There are 3 ways to override the `UI_COLOR` variable:

1. **UDS config**: you can create a `uds-config.yaml` file in the same directory as the bundle and specify the variable to override. For example, to override the `UI_COLOR` variable, you can create a `uds-config.yaml`:

   ```yaml
   variables:
     helm-overrides-package:
       ui_color: green # Note that the variable for `UI_COLOR` can be upper or lowercase
   ```

1. **Environment variables**: you can create an environment variable prefixed with `UDS_` and the name of the variable. For example, to override the `UI_COLOR` variable, you can create an environment variable called `UDS_UI_COLOR` and set it to the desired value. Note that environment variables take precedence over `uds-config.yaml` variables.

1. **--set Flag**: you can also override the variable using the CLI's `--set` flag. For example, to override the `UI_COLOR` variable, you can run one of the following commands:

   ```bash
   # by default ui_color will apply to all packages in the bundle
   uds deploy example-bundle --set ui_color=green

   # to specify a specific package that the variable should apply to you can prepend th package name to the variable
   uds deploy example-bundle --set helm-overrides-package.ui_color=green
   ```

   > **:warning: Warning**: Because Helm override variables and Zarf variables share the same --set syntax, be careful with variable names to avoid conflicts.

:::note
A variable that is not overridden by any of the methods above and has no default will be ignored.
:::

#### Variable Precedence

Variable precedence is as follows:

1. The `--set` flag
1. Environment variables
1. `uds-config.yaml` variables
1. Variables `default` in the`uds-bundle.yaml`

#### Variable Types

Variables can be of either type `raw` or `file`. The type will default to raw if not set explicitly.

:::caution
If a variable is set to accept a file as its value, but is missing the `file` type, then the file will not be processed.
:::

```yaml
kind: UDSBundle
metadata:
   name: example-bundle
   version: 0.0.1

packages:
   - name: helm-overrides-package
     path: "../../packages/helm"
     ref: 0.0.1
     overrides:
        podinfo-component:
          unicorn-podinfo:
           variables:
           - name: UI_COLOR
             path: "ui.color"
             description: "variable UI_COLOR accepts a raw value (e.g. a string, int, map) like "purple", which is passed to the ui.color helm path"
             type: raw
           - name: test_secret
             path: "testSecret"
             description: "variable TEST_SECRET will resolve to the contents of a file (e.g. test.cert), which gets passed to the testSecret helm path"
             type: file
```

**File Paths**

If a file path is not absolute, it will be set as relative to the `uds-config.yaml` directory.

e.g. the following `uds-config.yaml` is in [`src/test/bundles/07-helm-overrides/variable-files/`](https://github.com/defenseunicorns/uds-cli/blob/main/src/test/bundles/07-helm-overrides/uds-config.yaml)

```yaml
variables:
  helm-overrides:
    test_secret: test.cert
```

This means when `test.cert` is evaluated it will first be appended to the config path like so `src/test/bundles/07-helm-overrides/variable-files/test.cert`.

If the file path is already set to the same relative path as the config, then no merging will take place.

:::note
UDS CLI does not encrypt or base64 encode any file contents before passing said data to Zarf or Helm.

For example, if the file contains a key to be used in a Kubernetes secret, it must be base64 encoded before being ingested by UDS CLI.
:::

### Sensitive

Variables can be specified as sensitive, which means their values, regardless of how they're set, will be masked in output.

```yaml
kind: UDSBundle
metadata:
   name: example-bundle
   version: 0.0.1

packages:
   - name: helm-overrides-package
     path: "../../packages/helm"
     ref: 0.0.1
     overrides:
        podinfo-component:
          unicorn-podinfo:
           variables:
            - name: SECRET_VAL
                path: "testSecret"
                description: "should be masked in output"
                sensitive: true
```

### Namespace

It's also possible to specify a namespace for a packaged Helm chart to be installed in. For example, to deploy the a chart in the `custom-podinfo` namespace, you can specify the `namespace` in the `overrides` block:

```yaml
kind: UDSBundle
metadata:
  name: example-bundle
  version: 0.0.1

packages:
  - name: helm-overrides-package
    path: "../../packages/helm"
    ref: 0.0.1
    overrides:
      podinfo-component:
        unicorn-podinfo:
          namespace: custom-podinfo # custom namespace!
          values:
            - path: "podinfo.replicaCount"
              value: 1
```

### View All Variables

When working with a local or remote bundle you can view all overrides and zarf variables by running `uds inspect --list-variables BUNDLE_TARBALL|OCI_REF]`
