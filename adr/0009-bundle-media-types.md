# 9. Bundle OCI Media Types

Date: 1 July 2024 [[1]](#footnotes)

## Status

Accepted

## Context



The structure of a bundle OCI artifact is as follows:

![Bundle OCI Artifact Structure](../docs/.images/uds-bundle.png)


Note that both the bundle root manifest and the Zarf package root manifest are both OCI image manifests. The Zarf image manifest is taken directly from Zarf OCI packages, and the bundle root manifest is a new OCI image manifest that contains the Zarf image manifests as layers. This structure effectively creates a "double pointer" from the `index.json` to the layers in the bundle image manifest, and then to the Zarf image manifest.

This "double pointer" is problematic because, while the [OCI spec](https://github.com/opencontainers/image-spec/blob/main/manifest.md) for image manifests doesn't necessary prohibit image manifests from pointing to image manifests, this leads to undesirable behavior in our [oras-go](https://github.com/oras-project/oras-go) client which breaks compatability across Docker's registry/v2 and GHCR OCI registries.

The primary issue is multi-faceted but here are symptoms that have been observed:
- Docker's registry/v2 and GHCR store OCI artifacts differently. In the case of registry/v2, OCI layers can be pulled from either the `/blobs` or `/manifests` endpoint, effectively resulting in a flat file structure under the hood. However, GHCR follows the OCI spec more closely and specifically stores layers with the [image manifest](https://github.com/opencontainers/image-spec/blob/main/manifest.md#image-manifest) media type under `/manifests` and other layers under `/blobs`.
- The `oras-go` client is unable to handle the "double pointer" structure of the bundle OCI artifact. ORAS encounters the bundle root manifest, stores its layers (including the Zarf image manifests). Then, when the Zarf image manifests are encountered, because the root is already present in the OCI store, the Zarf layers are skipped. This issue can be gotten around by forcing ORAS to push the Zarf image manifests to the `/blobs` endpoint, registry/v2 will still find it whether or not it's under `/blobs` or `/manifests`, and skip the rest of the layers.

## Alternatives

The following solutions were attempted but did not work out:

- Skip Zarf image manifests during ORAS operations and push them manually. Them doesn't work because when ORAS pushes the bundle root manifest, it fails with `MANIFEST_BLOB_UNKNOWN` because the layers in the bundle root manifest do not exist (because we tried to push them after calling `oras.Copy`)
- Change the media type of the bundle root manifest itself. This results in `MANIFEST_INVALID` because both GHCR and Docker's registry/v2 expect the `index.json` to point to image manifests.

Other potential solutions include:
1. Force the media type of the layers in the bundle root manifest to be Zarf blobs layers,even though they actually point to Zarf image manifests. This is historically the way that UDS CLI has handled bundles and is verified to work. However, the cons of this approach include not being able to reuse existing Zarf OCI code and introducing custom OCI logic that can be difficult to grok.
    - **pros**: proven solution, works with existing UDS CLI code, code has been significantly refactored and commented for easier readability and maintainability, still reuses a lot of `zoci` and  `pkg/oci` code
    - **cons**: could potentially reuse even more `zoci` and `pkg/oci` code to make the solution more robust and easier to maintain, can still be difficult to grok

1. Use more abstracted `zoci` and `defenseunicorns/pkg/oci` functions to push portions of the bundle to OCI registries; then layer the custom OCI logic on the top.
    - **pros**: sharing more code across products, potentially easier to maintain and grok
    - **cons**: shared code means products are more tightly coupled and more susceptible to breaking changes, unproven solution (not sure if these even works) and would require significant refactoring of existing code

## Decision

Option 1, keep the historical media types. This decision is based on the following considerations:
- We'd like to keep the bundle's OCI format stable and not introduce breaking changes unless there is a benefit to end users.
- This is a proven solution that works with existing UDS CLI code.
- While Option 2 could be valuable, there isn't much benefit to existing users the code improvement may only be marginal.

## Consequences

All UDS CLI contributors need to become intimately familiar with the OCI spec and the `oras-go` client to understand the nuances of the bundle OCI artifact. This is a complex area of the codebase that will require careful maintenance and documentation to ensure that future contributors can understand the code and make changes without breaking existing functionality.

## Footnotes

1. This ADR is largely historical and serves as developer documentation for why bundle OCI artifacts are stuctured the way they are. This has been the case since UDS CLI's inception in August 2023.
