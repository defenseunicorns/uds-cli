---
title: Bundle Anatomy
type: docs
weight: 5
---

## Bunble Anatomy
A UDS Bundle is an OCI artifact with the following form:

{{ $image := resources.GetRemote "https://github.com/defenseunicorns/uds-cli/blob/main/docs/.images/uds-bundle.png" }}

{{ $image := .Resources.GetMatch "sunset.jpg" }}
{{ with $image }}
  <img src="{{ .RelPermalink }}" width="{{ .Width }}" height="{{ .Height }}">
{{ end }}
