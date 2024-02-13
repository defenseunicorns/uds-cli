# Bundle Overrides

Bundle overrides provide a mechanism to customize Helm charts inside of Zarf packages.

## Table of Contents

1. [Quickstart](#quickstart)
1. [Overrides](#variables)
    - [Syntax](#syntax)
    - [Values](#values)
    - [Variables](#variables)

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
          values:
            - path: "replicaCount"
              value: 2
          variables:
            - name: UI_COLOR
              path: "ui.color"
              description: "Set the color for podinfo's UI"
              default: "purple"
```

This bundle will deploy the `helm-overrides-package` Zarf package and override the `replicaCount` and `ui.color` values in the `podinfo` chart. The `values` can't be modified after the bundle has been created. However, at deploy time, users can override the `UI_COLOR` and other `variables` using a environment variable called `UDS_UI_COLOR` or by specifying it in a `uds-config.yaml` like so:

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
        podinfo:                # chart name from the helm-overrides-component component
          values:
            - path: "replicaCount"
              value: 2
          variables:
            - name: UI_COLOR
              path: "ui.color"
              description: "Set the color for podinfo's UI"
              default: "purple"
```

In this example, the `helm-overrides-package` Zarf package has a component called `helm-overrides-component` which contains a Helm chart called `podinfo`; note how these names are keys in the `overrides` block. The `podinfo` chart has a `replicaCount` value that is overridden to `2` and a variable called `UI_COLOR` that is overridden to `purple`.

### Values

The `values` in an `overrides` block are a list of `path` and `value` pairs. They allow users to override values in a Zarf package component's underlying Helm chart. Note that values are specified by bundle authors and **cannot be modified** after the bundle has been created.

#### Path

The `path` uses dot notation to specify the location of a value to override in the underlying Helm chart. For example, the `replicaCount` path in the `podinfo` chart is located at the top-level of the [podinfo values.yaml](https://github.com/stefanprodan/podinfo/blob/master/charts/podinfo/values.yaml), so the path is simply `replicaCount`, while the `ui.color` path is located under the `ui` key, so the path is `ui.color`.

#### Value

The `value` is the value to set at the `path`. Values can be simple values such as numbers and strings, as well as, complex lists and objects, for example:
```yaml
...
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
If [importing](../README.md#importingexporting-variables) a variable from another package, that variable can also be used to set a value, using the template syntax `${...}`

### Variables
Variables are similar to [values](#values) in that they allow users to override values in a Zarf package component's underlying Helm chart; they also share a similar syntax. However, unlike `values`, `variables` can be overridden at deploy time. For example, consider the following `variables`:

```yaml
...
    overrides:
      helm-overrides-component:
        podinfo:
          variables:
           - name: UI_COLOR
             path: "ui.color"
             description: "Set the color for podinfo's UI"
             default: "purple"
```

There are 2 ways to override the `UI_COLOR` variable:

1. **UDS config**: you can create a `uds-config.yaml` file in the same directory as the bundle and specify the variable to override. For example, to override the `UI_COLOR` variable, you can create a `uds-config.yaml`:

    ```yaml
      variables:
        helm-overrides-package:
          ui_color: green
    ```
Note that the variable for `UI_COLOR` can be upper or lowercase.  

1. **Environment variables**: you can create an environment variable prefixed with `UDS_` and the name of the variable. For example, to override the `UI_COLOR` variable, you can create an environment variable called `UDS_UI_COLOR` and set it to the desired value. Note that environment variables take precedence over `uds-config.yaml` variables.  

#### Variable Precedence
Variable precedence is as follows:
1. Environment variables
2. `uds-config.yaml` variables
3. Variables `default` in the`uds-bundle.yaml`
