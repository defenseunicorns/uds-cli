---
title: CLI Configuration
type: docs
weight: 2
---

## Configuration

The UDS CLI can be easily configured through a `uds-config.yaml` file, offering flexibility in placement either within the current working directory or via the specification of an environment variable named `UDS_CONFIG`. The fundamental structure of the `uds-config.yaml` file is outlined below:

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

The `options` key contains UDS CLI options that are not specific to a particular Zarf Package. The `variables` key contains variables that are specific to a particular Zarf Package. If there is a need to share insensitive variables across multiple Zarf Packages, the `shared` key can be used. In this case, the `shared` key represents the variable name, and the value is the variable's intended value.
