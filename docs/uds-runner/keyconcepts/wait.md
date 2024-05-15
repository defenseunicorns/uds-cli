---
title: Wait
type: docs
weight: 11
---

The `wait` key function serves to halt the execution, effectively pausing the program until a specified resource becomes available. This resource could include anything from network responses to Kubernetes operations:

```yaml
tasks:
  - name: network-response
    wait:
      network:
        protocol: https
        address: 1.1.1.1
        code: 200
  - name: configmap-creation
    wait:
      cluster:
        kind: configmap
        name: simple-configmap
        namespace: foo
```
