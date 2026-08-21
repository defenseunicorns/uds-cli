# 18. Explicit Package Signature Verification

Date: 2026-07-29

## Status

Accepted

## Context

Package signatures establish the authenticity and integrity of a package, but
only when bundle creation verifies every package against explicitly declared
trust material. Implicit verification defaults or global policy files make it
too easy to create an artifact whose package trust posture is unclear.

## Decision

Every `package` block used by `uds bundle create` must contain a
`signature_verification` block. It declares exactly one of these postures:

- `verify = false`, with no verification material; or
- verification enabled, with exactly one `public_key` or `keyless`
  configuration.

Keyless verification requires exactly one certificate identity constraint and
one OIDC issuer constraint. Verification material travels with the package
definition. `file()` materialization (ADR-0017) makes file-backed public keys
and trusted roots part of the created artifact.

`uds bundle create` verifies each enabled package through Zarf before its
contents enter the bundle. Zarf performs all cryptographic checks. Keyless
verification enables transparency-log inclusion and SCT validation unless the
bundle author explicitly selects the corresponding insecure setting. A package
with `verify = false` is allowed, but create emits a prominent warning naming
the unverified package. Remote references are resolved and pinned to one
manifest digest before retrieval, and both remote and local creation ingest the
exact private staged content that Zarf verified.

`uds bundle reconfigure` remains the defaults-only operation defined by
ADR-0008. It rewrites the bundle definition/defaults layers and does not fetch,
verify, rebuild, or modify package manifests and blobs. Since it does not affect
the integrity of the underlying packages, re-verification is not performed.

The signature-verification schema is enforced only at the create boundary.
Deploying a built bundle artifact uses Zarf `VerifyNever`; remove and inspect
do not verify packages.

Direct deployment from a bundle directory or `bundle.uds.hcl` is a development
workflow. It pulls each package once, asks Zarf to verify it with any valid
package verification settings, and continues after verification or policy
errors with a prominent warning that the package would fail bundle creation.
An explicit `verify = false` also warns that the development deployment is
using an unverified package. This advisory behavior gives bundle authors early
feedback without making direct deployment as strict as artifact creation.

## Consequences

- All bundle definitions used to create artifacts must explicitly declare a
  package verification posture per package. Unsigned packages require
  `verify = false` until they are signed.
- Bundle creation fails before package ingestion if an enabled package is
  unsigned, lacks matching trust material, or fails Zarf verification.
- Bundle signing and verification (future work) are required to provide a
  full integrity boundary for bundle consumption.
