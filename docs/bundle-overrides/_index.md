---
title: Bundle Overrides Quickstart
type: docs
weight: 5
---

Consider the following `zarf.yaml` and `values.yaml` which deploys [`podinfo`](https://github.com/stefanprodan/podinfo) with a set of custom values:

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

The bundle overrides feature allows users to override the values specified in Zarf Packages:

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

This bundle facilitates the deployment of the `helm-overrides-package` Zarf Package while offering the ability to override specific values, such as `replicaCount` and `ui.color`, within the `podinfo` chart. Once the bundle has been created, these values remain immutable. However, during deployment, users have the flexibility to override variables such as `UI_COLOR` by either utilizing an environment variable named `UDS_UI_COLOR` or by explicitly specifying the override in a `uds-config.yaml` file, as demonstrated below:

```yaml
variables:
 helm-overrides-package:
   UI_COLOR: green
```
