# 10. Legacy and Next Coexistence

Date: 13 August 2026

## Status

Accepted

## Context

UDS CLI Legacy and UDS CLI Next must ship from one Go module and one `uds` binary. This change prepares the repository without copying or exposing Next commands. The future Next integration remains blocked until this preparation is merged.

Legacy is the Alpha default. Its command implementation is private under `internal/legacy`, and its exported library is public under `pkg/legacy`. The canonical root `internal` and `pkg` paths are reserved for Next. Neutral composition packages remain at the root.

## Decision

Use this target structure:

```text
cmd/uds
internal/mode
internal/version
internal/legacy/cli
internal/legacy/testutil
internal/{artifact,bundle,cli,logger,oci,printer,testutil,zarf}
pkg/legacy/{config,types,bundle,bundler,cache,engine,interactive,message,sources,style,utils}
pkg/{bundle,iostreams}
tests/legacy/e2e
tests/{integration,cluster,library,smoke,test_data,testutil}
testdata/legacy/{bundles,packages,tasks}
```

The canonical Next package is `internal/bundle`. This reflects the final Next source even though the earlier design used `internal/bundlehcl`.

Move Legacy sources with `git mv` according to this contract:

| Source | Destination |
| --- | --- |
| `main.go` | `cmd/uds/main.go` |
| `src/cmd/**` | `internal/legacy/cli/**` |
| `src/config/**` | `pkg/legacy/config/**` |
| `src/types/**` | `pkg/legacy/types/**` |
| `src/pkg/**` | `pkg/legacy/**` |
| `src/test/common.go` | `internal/legacy/testutil/testutil.go` |
| `src/test/e2e/**` | `tests/legacy/e2e/**` |
| `src/test/{bundles,packages,tasks}/**` | `testdata/legacy/{bundles,packages,tasks}/**` |

Copy Next from commit `939b73b8051ce95dcb56944cae57ab3133718e6e` with this allowlist:

| Source | Destination |
| --- | --- |
| `internal/{artifact,bundle,cli,logger,oci,printer,testutil,version,zarf}/**` | `internal/{same package}/**` |
| `pkg/{bundle,iostreams}/**` | `pkg/{same package}/**` |
| `tests/{integration,cluster,library,smoke,test_data,testutil}/**` | `tests/{same area}/**` |

Do not copy the Next entrypoint, documentation, vendor tree, module files, tasks, workflows, release files, lint configuration, Renovate configuration, policy files, or other repository control files. Rewrite the Next module prefix to `github.com/defenseunicorns/uds-cli`. Adapt its composition into `cmd/uds`.

The root lint configuration retains the existing Legacy linters. The pinned Next source is validated with its canonical lint set, so `gosec`, `perfsprint`, and `revive` are excluded only for the allowlisted Next import roots. New composition code remains covered by the complete root lint set.

The preparation owns `internal/mode` and the single `internal/version` metadata seam. The integration must merge, not overwrite, these files. Legacy cannot import canonical Next implementation packages. The integration may compose Legacy command constructors at the root.

Use the existing module with Go 1.26.5. The direct dependency versions shared by both repositories already agree. Helm v3 and Helm v4 coexist through distinct major module paths. Merge Next requirements, run one root `go mod tidy`, and keep one dependency, vulnerability, license, and release graph.

Keep the current workflows, task runner, GoReleaser configuration, and release destinations canonical. The future Next integration adds the `test:next:{unit,library,integration,cluster,uds-core}` namespaces when their allowlisted source paths arrive. Build metadata must set Legacy version, canonical version, commit, build date, Zarf metadata, Helm v3 capability metadata, and Helm v4 metadata in the same artifact.

Pin GoReleaser 2.15.3 to preserve the existing Homebrew formula contract. GoReleaser 2.16 and later reject that contract; migrating the tap from formulas to casks requires separate release coordination before the pin can advance.

Legacy generators run through `go run ./cmd/uds --features=NextMode=false`. Generated schemas and command documentation retain their current destinations. Next has no additional nonvendor generators. Imported Next documentation and ADRs are deferred.

The integration must address Next command handlers that call `os.Exit` and Zarf code that inspects `os.Args` during package initialization. The bootstrap removes feature arguments before dependent command packages initialize, then resolves the saved arguments before Cobra construction. The integration must prove all Zarf tools and callbacks with both feature forms before the source import can merge.

## Consequences

Existing CLI invocations continue to select the complete Legacy tree. Library callers mechanically replace `github.com/defenseunicorns/uds-cli/src/...` with `github.com/defenseunicorns/uds-cli/pkg/legacy/...`; package names and exported symbols remain unchanged.

Confirming downstream consumer surfaces remains assigned to the future integration work before release.

The future Next integration can perform an allowlisted import without an intermediate broken commit. It remains blocked until the preparation build, tests, generation, lint, task graph, and release snapshot are green.
