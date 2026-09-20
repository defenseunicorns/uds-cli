# Contributing to UDS CLI
Welcome :unicorn: to the UDS CLI! If you'd like to contribute, please reach out to one of the [CODEOWNERS](CODEOWNERS) and we'll be happy to get you started!

Below are some notes on our core software design philosophies that should help guide contributors.

## Table of Contents
1. [Code Quality and Standards](#code-quality-and-standards)
1. [Snapshot releases](#snapshot-releases)
1. [How to Contribute](#how-to-contribute)
    - [Set up the development environment](#set-up-the-development-environment)
    - [Building the app](#building-the-app)
    - [Hooks and linting](#hooks-and-linting)
    - [CLI modes](#cli-modes)
    - [Testing](#testing)

## Code Quality and Standards
Fundamentally, software engineering is a communication problem; we write code for each other, not a computer. When working on this project (or any project!) keep your fellow humans in mind and write clearly and concisely. Below are some general guidelines for code quality and standards that make UDS CLI :sparkles:

- **Write tests that give confidence**: Unless there is a technical blocker, every new feature and bug fix should be tested in the project's automated test suite. Although many of our tests are E2E, unit and integration-style tests are also welcomed. Unit tests can live in a `*_test.go` file alongside the source code. Legacy E2E tests live in `tests/legacy/e2e`, and Next tests live under `tests/cli`, `tests/library`, and `tests/cluster`.


- **Prefer readability over being clever**: We have a strong preference for code readability in UDS CLI. Specifically, this means things like: naming variables appropriately, keeping functions to a reasonable size and avoiding complicated solutions when simple ones exist.


- **User experience is paramount**: UDS CLI doesn't have a pretty UI (yet), but the core user-centered design principles that apply when building a frontend also apply to this CLI tool. First and foremost, features in UDS CLI should enhance workflows and make life easier for end users; if a feature doesn't accomplish this, it will be dropped.


- **Design Decision**: We use [Architectural Decision Records](https://adr.github.io/) to document the design decisions that we make. Next ADRs live in `docs/adr`, and Legacy ADRs live in `docs/adr/legacy`. We highly recommend reading through the existing ADRs to understand the context and decisions that have been made in the past, and to inform current development.

### Continuous Delivery
Continuous Delivery is core to our development philosophy. Check out [https://minimumcd.org](https://minimumcd.org/) for a good baseline agreement on what that means.

Specifically:

- We do trunk-based development (`main`) with short-lived feature branches that originate from the trunk, get merged into the trunk, and are deleted after the merge
- We don't merge code into `main` that isn't releasable
- We perform automated testing on all changes before they get merged to `main`
- We create unique release artifacts

## Snapshot releases

The `Snapshot Release` workflow runs daily at 03:00 UTC against `main`. Manual dispatches also build only `main`. Tags use `vX.Y.Z-snapshot+YYYYMMDDHHMMSS-XXXXXXXX`, containing the latest stable version prefix, UTC timestamp, and exact source commit. They are never overwritten.

Before tagging, the workflow rejects tag collisions and runs the reusable release test suite against `main`. That suite includes authenticated GHCR-write tests and UDS Core smoke tests. After publishing, the workflow verifies the Linux amd64 checksum and confirms both CLI modes report the release tag. The stable release workflow excludes snapshot tags.

Do not dispatch this workflow without maintainer approval. Its tests and publishing jobs use repository and package write permissions, and successful runs create remote tags, GitHub prereleases, and package state through the `release-snapshot` environment. Scheduled cleanup retains three snapshot prereleases and preserves their tags.

## How to Contribute
Please ensure there is a GitHub issue for your proposed change, this helps the UDS CLI team to understand the context of the change and to track the progress of the work. If there isn't an issue for your change, please create one before starting work. The recommended workflow for contributing is as follows:


Before starting development, we highly recommend reading through the UDS CLI [documentation](https://docs.defenseunicorns.com/cli/) and our [ADRs](./docs/adr).

1. **Fork this repo** and clone it locally
1. **Create a branch** for your changes
1. **Create, [test](#testing)** your changes
1. **Add docs** where appropriate
1. **Push your branch** to your fork
1. **Open a PR** against the `main` branch of this repo

### Set up the development environment

Install [mise](https://mise.jdx.dev/getting-started.html) and activate it in your shell by following mise's [shell activation instructions](https://mise.jdx.dev/dev-tools/#activate). The repository pins Go, UDS CLI, hk, golangci-lint, k3d, and Node.js in [mise.toml](mise.toml), so no separate tool installation is needed.

From the repository root, run:

```console
mise trust mise.toml
mise install
hk install
```

`hk install` enables the repository's pre-commit checks. Run `hk check --all` at any time to run the same checks without committing, or `hk fix --all` to apply supported fixes.

### Building the app

Use the repository's UDS Runner tasks to build and validate UDS CLI. After mise is activated, the pinned bootstrap UDS CLI is available on your `PATH`.

To build a local binary, run `uds run build`. This creates `build/uds`. Use `uds run --list-all` for platform-specific and release builds; CI uses `uds run build-cli-linux-amd`.

### Hooks and linting

Commits run hk automatically. To run the complete repository check suite manually, use `uds run lint` (or `hk check --all`). CI runs this same hk configuration.

### CLI modes

UDS CLI currently contains both Legacy and Next implementations.

- Legacy mode is the default during alpha and preserves existing behavior.
- Next mode is available as an alpha preview with `--features=NextMode=true` or `CLI_FEATURES=NextMode=true`.
- Next mode is expected to become the default in beta.
- Legacy mode will be removed after the beta migration window.

New feature work should target the canonical Next packages unless a maintainer explicitly scopes the change to Legacy. Use the following package boundaries:

- Legacy implementation: `internal/legacy/...`
- Legacy public packages: `pkg/legacy/...`
- Legacy E2E tests and fixtures: `tests/legacy/e2e` and `testdata/legacy`
- Next command wiring and implementation: canonical `internal/...` packages that are not under `internal/legacy`
- Next public packages: `pkg/bundle/...` and `pkg/iostreams`
- Next tests: `tests/cli`, `tests/library`, and `tests/cluster`

### Testing

We strive to test all changes made to UDS CLI. If you're adding a new feature or fixing a bug, please add tests to cover the new functionality. Unit tests, integration tests, cluster tests, and E2E tests are all welcome, but we leave it up to the contributor to decide which is most appropriate for the change. Below are some guidelines for testing:

#### Unit Tests
Unit tests reside alongside the source code in a `*_test.go` file. These tests should be used to test individual functions or methods in isolation. Unit tests should be fast and focused on a single piece of functionality.

#### Legacy E2E Tests
Legacy E2E tests reside in the `tests/legacy/e2e` directory. They use bundles and packages under `testdata/legacy`. Feel free to add new fixtures where appropriate. It's encouraged to write comments or metadata in new fixtures to explain what they are testing.

#### Next tests

Use the lowest layer that proves the behavior. Library tests and their helpers
exercise UDS only through `pkg/bundle`, `pkg/bundle/spec`, and `pkg/iostreams`.
CLI tests execute Cobra directly. A minimal primary-binary check covers process
startup and routing. Core E2E belongs to the CLI layer.

| Layer | Starting condition | Task |
| --- | --- | --- |
| Unit | No cluster | `test:next-unit` |
| Library / non-cluster | Local fixtures | `test:next-library-non-cluster` |
| Library / non-cluster | Creates an isolated cluster | `test:next-library-non-cluster-lifecycle` |
| Library / in-cluster | Supplied Zarf-initialized cluster | `test:next-library-in-cluster` |
| CLI / non-cluster | Local fixtures | `test:next-cli-non-cluster` |
| CLI / non-cluster | Creates an isolated cluster | `test:next-cli-non-cluster-lifecycle` |
| CLI / in-cluster | Supplied Zarf-initialized cluster | `test:next-cli-in-cluster` |

Run a task with `uds run <task>`. Build `build/uds` with `uds run build` before
the minimal binary checks in `test:next-cli-non-cluster`. Other CLI tasks and all
library tests need no built UDS binary. Owned-cluster and Core
tasks use `mise exec` to install and activate Zarf matching `go.mod` for package
action callbacks.
The remote-package verification subset is
`test:next-library-non-cluster-package-verification`.
`test:next-library-non-cluster-keyless` requires GitHub Actions OIDC and fails
without it.

In-cluster tasks require `KUBECONFIG` and `UDS_TEST_KUBECONFIG` to name the same
explicit kubeconfig file. They never create or delete the supplied cluster.
CI uses `hack/test-cluster.sh setup <isolated-directory>` before these tasks and
`cleanup <isolated-directory>` afterward. The helper uses the pinned Zarf
tool to prepare infrastructure. Public API bootstrap is
covered separately by the library lifecycle test. Owned-cluster tests use unique
names and clean up only their own clusters.

#### Assertions
We prefer to use Testify's [require](https://github.com/stretchr/testify/tree/master/require) package for assertions in tests. This package provides a rich set of assertion functions that make tests more readable and easier to debug. See other tests in this repo for examples.

#### Running Tests
- **Unit Tests**: Run `uds run test` from the repository root. This runs the preparation test suite.


- **Legacy E2E Tests**: Build UDS CLI with `uds run build` before running E2E tasks; rebuild after source changes because the tests use `build/uds`. Run the focused E2E tasks listed by `uds run --list-all` (for example, `uds run test:bundle`). The `test:e2e-ghcr` task writes to GHCR and is intended for CI only.


- **Next Tests**: Build UDS CLI with `uds run build` before binary-driven tests, then run the focused Next task for the tier you need. For example, run `uds run test:next-cli-non-cluster` for local command coverage or `uds run test:next-cli-non-cluster-lifecycle` for an isolated cluster lifecycle.
