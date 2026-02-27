# UDS-CLI

[![Latest Release](https://img.shields.io/github/v/release/defenseunicorns/uds-cli)](https://github.com/defenseunicorns/uds-cli/releases)
[![Go version](https://img.shields.io/github/go-mod/go-version/defenseunicorns/uds-cli?filename=go.mod)](https://go.dev/)
[![Build Status](https://img.shields.io/github/actions/workflow/status/defenseunicorns/uds-cli/release.yaml)](https://github.com/defenseunicorns/uds-cli/actions/workflows/release.yaml)
[![OpenSSF Scorecard](https://api.securityscorecards.dev/projects/github.com/defenseunicorns/uds-cli/badge)](https://api.securityscorecards.dev/projects/github.com/defenseunicorns/uds-cli)

## Install
Recommended installation method is with Brew:
```
brew install defenseunicorns/tap/uds
```
UDS CLI binaries are also included with each [Github Release](https://github.com/defenseunicorns/uds-cli/releases)

## Official Documentation
Official documentation is located at [uds.defenseunicorns.com/reference/cli/overview/](https://uds.defenseunicorns.com/reference/cli/overview/)

## Quickstart
UDS-CLI provides a mechanism to bundle and deploy multiple, independent Zarf packages. To create a `UDSBundle` of Zarf packages, create a `uds-bundle.yaml` file like so:

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
    timeout: 20m
    optionalComponents:
      - git-server
  - name: podinfo
    repository: ghcr.io/defenseunicorns/uds-cli/podinfo
    ref: 0.0.1
```
Running `uds create` in the same directory as the above `uds-bundle.yaml` will create a bundle tarball containing both the Zarf init package and podinfo. The bundle can be deployed with `uds deploy`.

## Contributing
Build instructions and contributing docs are located in [CONTRIBUTING.md](https://github.com/defenseunicorns/uds-cli/blob/main/CONTRIBUTING.md).
