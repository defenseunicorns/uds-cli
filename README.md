# UDS-CLI

[![Latest Release](https://img.shields.io/github/v/release/defenseunicorns/uds-cli)](https://github.com/defenseunicorns/uds-cli/releases)
[![Go version](https://img.shields.io/github/go-mod/go-version/defenseunicorns/uds-cli?filename=go.mod)](https://go.dev/)
[![Build Status](https://img.shields.io/github/actions/workflow/status/defenseunicorns/uds-cli/release.yaml)](https://github.com/defenseunicorns/uds-cli/actions/workflows/release.yaml)
[![OpenSSF Scorecard](https://api.securityscorecards.dev/projects/github.com/defenseunicorns/uds-cli/badge)](https://api.securityscorecards.dev/projects/github.com/defenseunicorns/uds-cli)

UDS CLI is the command-line DevX for interacting with the [UDS ecosystem](https://docs.defenseunicorns.com/). It is the primary way to interact with UDS workflows from a terminal, including bundle authoring, artifact creation, deployment, publishing, cluster operations, task automation, and access to the UDS-adjacent tooling needed to operate the platform.

> UDS CLI currently supports Legacy and Next modes. Legacy remains the default while Next matures as an alpha preview. See [CLI Modes](#cli-modes) for details.

## Install
Recommended installation method is with Brew:
```
brew trust --formula defenseunicorns/tap/uds && brew tap defenseunicorns/tap && brew install uds
```
UDS CLI binaries are also included with each [Github Release](https://github.com/defenseunicorns/uds-cli/releases)

### Snapshot prereleases

The `Snapshot Release` workflow runs daily at 03:00 UTC against `main`; manual dispatches also build only `main`. Successful runs publish a GitHub prerelease tagged `vX.Y.Z-snapshot+YYYYMMDDHHMMSS-XXXXXXXX`, where `vX.Y.Z` is the latest stable tag and `XXXXXXXX` is the source commit's eight-character SHA. Each prerelease contains Linux and macOS binaries for amd64 and arm64, Linux DEB/RPM packages, SHA-256 checksums, and per-binary SBOMs.

Snapshots do not update Homebrew or become the latest release. Scheduled cleanup retains the newest three snapshot prereleases and their tags.

## Official Documentation
Official documentation is located at [docs.defenseunicorns.com/cli/](https://docs.defenseunicorns.com/cli/)

## Quickstart

UDS CLI Next uses `bundle.uds.hcl` bundle definitions. This example creates a local k3d cluster with the UDS k3d package, initializes Zarf, and deploys podinfo.

Prerequisites: Docker and k3d must be installed and running.

Create `bundle.uds.hcl`:

```hcl
uds {
  bundle_api_version = "uds.dev/v1alpha1"
}

metadata {
  name        = "next-quickstart"
  description = "Next mode quickstart bundle"
  version     = "0.1.0"
}

package "uds_k3d_dev" {
  source = "oci://ghcr.io/defenseunicorns/packages/uds-k3d:0.20.2"
  signature_verification { verify = false }
}

package "init" {
  source = "oci://ghcr.io/zarf-dev/packages/init:v0.83.0"
  signature_verification {
    keyless {
      certificate_identity_regexp = "https://github\\.com/zarf-dev/zarf/\\.github/workflows/release\\.yml@refs/tags/v\\d+\\.\\d+\\.\\d+"
      certificate_oidc_issuer      = "https://token.actions.githubusercontent.com"
    }
  }
  depends_on = [package.uds_k3d_dev]
}

package "podinfo" {
  source = "oci://ghcr.io/defenseunicorns/uds-cli/podinfo:0.0.2"
  signature_verification { verify = false }
  depends_on = [package.init]
}
```

Deploy directly from the bundle definition during development:

```bash
CLI_FEATURES=NextMode=true uds bundle dev deploy .
```

Or create an unsigned bundle artifact and deploy it:

```bash
CLI_FEATURES=NextMode=true uds bundle create --unsigned .
CLI_FEATURES=NextMode=true uds bundle deploy ./uds-bundle-next-quickstart-<ARCH>-0.1.0.tar.zst --skip-signature-verification
```

Replace `ARCH` with the bundle architecture, such as `amd64` or `arm64`.

The `--unsigned` flag is required here because this quickstart does not configure bundle signing. Deploying the unsigned artifact requires `--skip-signature-verification`.

## CLI Modes
UDS CLI currently supports two modes while the next-generation CLI matures.

Legacy mode is the default today and preserves existing behavior. Next mode is available as an alpha preview behind the `NextMode` feature flag.

```bash
uds version
uds --features=NextMode=true version
CLI_FEATURES=NextMode=true uds version
```

Next mode will continue to mature during alpha and is planned to become the default in beta. Legacy mode will be removed after the beta migration window.

## Legacy Mode Quickstart
UDS-CLI provides a mechanism to bundle and deploy multiple, independent Zarf packages. To create a `UDSBundle` of Zarf packages, create a `uds-bundle.yaml` file like so:

```yaml
kind: UDSBundle
metadata:
  name: example
  description: an example UDS bundle
  version: 0.0.1

packages:
  - name: init
    repository: ghcr.io/zarf-dev/packages/init
    ref: v0.84.0
    keylessVerification:
      certificateIdentityRegexp: https://github\.com/zarf-dev/zarf/\.github/workflows/release\.yml@refs/tags/v\d+\.\d+\.\d+
      certificateOIDCIssuer: https://token.actions.githubusercontent.com
  - name: podinfo
    repository: ghcr.io/defenseunicorns/uds-cli/podinfo
    ref: 0.0.2
```
Running `uds create` in the same directory as the above `uds-bundle.yaml` will create a bundle tarball containing both the Zarf init package and podinfo. The bundle can be deployed with `uds deploy`.

## Contributing
Build instructions and contributing docs are located in [CONTRIBUTING.md](https://github.com/defenseunicorns/uds-cli/blob/main/CONTRIBUTING.md).
