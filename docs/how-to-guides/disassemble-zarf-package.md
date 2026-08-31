---
title: Disassemble a Zarf package
description: Convert a Zarf package into local source that can be rebuilt without registry or internet access.
---
## Disassemble a Zarf package

Use package disassembly to recover the assets and `zarf.yaml` from a complete Zarf package. The generated source replaces remote charts, manifests, files, images, and repositories with local content from the package.

Legacy mode uses:

```bash
uds dev disassemble <source> <output-dir>
```

Next mode uses:

```bash
uds --features=NextMode=true bundle dev disassemble <source> <output-dir>
```

`<source>` can be a Zarf package directory, a `.tar` or `.tar.zst` package archive, or an OCI package reference. Prefer the explicit `oci://` scheme for remote packages. `<output-dir>` must not exist or must be empty.

For example:

```bash
uds dev disassemble \
  zarf-package-app-amd64-1.0.0.tar.zst \
  ./app-source
```

Legacy mode reports successful completion as human-readable CLI output. Next mode writes its result as text by default and supports `--output json` or `--output yaml` for structured output.

### Rebuild offline

After disassembly, rebuild the package from the generated directory:

```bash
uds zarf package create ./app-source --confirm --output ./build
```

The generated package version ends in `-disassembled` so it does not collide with the original package version. The rebuild uses the local assets emitted under `app-source` and does not pull images or other declared package resources from remote sources.

### Repository limitation

Zarf currently uses one repository URL for both source acquisition and repository identity. Disassembly therefore emits absolute `file://` repository URLs so repository-bearing source can rebuild in its original output location without network access.

Do not move repository-bearing output before rebuilding it. Portable repository source requires upstream Zarf support for a package-relative local source that preserves the original repository URL and ref.

### Unsupported packages

Differential and skeleton Zarf packages do not contain complete source and cannot be disassembled. Bundle disassembly is not yet supported.
