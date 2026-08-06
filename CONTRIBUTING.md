# Contributing to UDS CLI
Welcome :unicorn: to the UDS CLI! If you'd like to contribute, please reach out to one of the [CODEOWNERS](CODEOWNERS) and we'll be happy to get you started!

Below are some notes on our core software design philosophies that should help guide contributors.

## Table of Contents
1. [Code Quality and Standards](#code-quality-and-standards)
1. [How to Contribute](#how-to-contribute)
    - [Set up the development environment](#set-up-the-development-environment)
    - [Building the app](#building-the-app)
    - [Hooks and linting](#hooks-and-linting)
    - [Testing](#testing)

## Code Quality and Standards
Fundamentally, software engineering is a communication problem; we write code for each other, not a computer. When working on this project (or any project!) keep your fellow humans in mind and write clearly and concisely. Below are some general guidelines for code quality and standards that make UDS CLI :sparkles:

- **Write tests that give confidence**: Unless there is a technical blocker, every new feature and bug fix should be tested in the project's automated test suite. Although many of our tests are E2E, unit and integration-style tests are also welcomed. Note that unit tests can live in a `*_test.go` file alongside the source code, and E2E tests live in `src/test/e2e`


- **Prefer readability over being clever**: We have a strong preference for code readability in UDS CLI. Specifically, this means things like: naming variables appropriately, keeping functions to a reasonable size and avoiding complicated solutions when simple ones exist.


- **User experience is paramount**: UDS CLI doesn't have a pretty UI (yet), but the core user-centered design principles that apply when building a frontend also apply to this CLI tool. First and foremost, features in UDS CLI should enhance workflows and make life easier for end users; if a feature doesn't accomplish this, it will be dropped.


- **Design Decision**: We use [Architectural Decision Records](https://adr.github.io/) to document the design decisions that we make. These records live in the `adr` directory. We highly recommend reading through the existing ADRs to understand the context and decisions that have been made in the past, and to inform current development.

### Continuous Delivery
Continuous Delivery is core to our development philosophy. Check out [https://minimumcd.org](https://minimumcd.org/) for a good baseline agreement on what that means.

Specifically:

- We do trunk-based development (`main`) with short-lived feature branches that originate from the trunk, get merged into the trunk, and are deleted after the merge
- We don't merge code into `main` that isn't releasable
- We perform automated testing on all changes before they get merged to `main`
- We create immutable release artifacts

## How to Contribute
Please ensure there is a GitHub issue for your proposed change, this helps the UDS CLI team to understand the context of the change and to track the progress of the work. If there isn't an issue for your change, please create one before starting work. The recommended workflow for contributing is as follows:


*Before starting development, we highly recommend reading through the UDS CLI [documentation](https://docs.defenseunicorns.com/cli/) and our [ADRs](./adr).

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

### Testing

We strive to test all changes made to UDS CLI. If you're adding a new feature or fixing a bug, please add tests to cover the new functionality. Unit tests and E2E tests are both welcome, but we leave it up to the contributor to decide which is most appropriate for the change. Below are some guidelines for testing:

#### Unit Tests
Unit tests reside alongside the source code in a `*_test.go` file. These tests should be used to test individual functions or methods in isolation. Unit tests should be fast and focused on a single piece of functionality.

#### E2E Tests
E2E tests reside in the `src/test/e2e` directory. They use bundles located in the `src/test/e2e/bundles` which contain Zarf packages from the `src/test/e2e/packages` directory. Feel free to add new bundles and packages where appropriate. It's encouraged to write comments/metadata in any new bundles or packages to explain what they are testing.

#### Assertions
We prefer to use Testify's [require](https://github.com/stretchr/testify/tree/master/require) package for assertions in tests. This package provides a rich set of assertion functions that make tests more readable and easier to debug. See other tests in this repo for examples.

#### Running Tests
- **Unit Tests**: Run `uds run test` from the repository root. This runs all unit tests in `src`.


- **E2E Tests**: Build UDS CLI with `uds run build` before running E2E tasks; rebuild after source changes because the tests use `build/uds`. Run the focused E2E tasks listed by `uds run --list-all` (for example, `uds run test:bundle`). The `test:e2e-ghcr` task writes to GHCR and is intended for CI only.
