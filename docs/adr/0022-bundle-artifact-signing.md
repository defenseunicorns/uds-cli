# 22. Bundle Artifact Signing

Date: 2026-08-11

## Status

Accepted

## Context

ADR-0018 verifies an individual package when it enters a bundle. That check establishes the package author's identity and protects the package bytes, but it does not authenticate the bundle author or bind the package to the bundle's defaults, values files, dependency ordering, and other deployment configuration.

## Decision

UDS CLI signs the exact bytes of a created bundle's canonical child `oci/index.json` using Cosign-compatible Sigstore evidence. The index commits to the bundle definition, defaults, values files, package manifests, and package blobs through their OCI digests. Bundle verification authenticates the person or workflow that approved that complete deployable composition; it does not replace the recorded package-signature posture.

Bundle creation and reconfiguration require key-based or keyless signing unless the caller explicitly selects `--unsigned`. Consumers supply their own trust policy and verify bundle evidence before consuming an artifact. Bundle inspect does not require verification of the artifact currently since it only logs out metadata, but does present a warning if verification does not happen (and will still error if verification fails). Keyless verification constrains both certificate identity and OIDC issuer, validates Rekor inclusion and SCT evidence, and uses Zarf's embedded public Sigstore TrustedRoot unless the consumer supplies a custom root.

Local archives store evidence as `uds.bundle.sig` outside `oci/`. OCI registries store evidence as a subject-bound referrer artifact, with the OCI referrers-tag fallback when a registry lacks the Referrers API. A bundle has exactly one signature evidence artifact; missing or duplicate evidence fails verification.

Support for Sigstore bundle format and keyless options largely follow the approach taken by Zarf, see below [references](#References) section for the related ZEP, issue, and docs.

## Consequences

Changing any deployable bundle content changes the index or a referenced digest and invalidates the signature. Reconfigured bundles require a new signature. Consumers may bypass verification only through an explicit insecure flag that emits a warning.

## References

- [Zarf ZEP-0053: Bundle Format](https://github.com/zarf-dev/proposals/tree/main/0053-bundle-format): related Zarf bundle-format proposal.
- [Zarf Package Signing Enhancements](https://github.com/zarf-dev/zarf/issues/4289): Zarf’s signing and verification work tracked outside the ZEP repository.
- [Zarf Package Signing](https://docs.zarf.dev/ref/package-signing/): current Zarf documentation for Cosign-compatible Sigstore bundle signing and verification.
