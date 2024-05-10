# Contributing to UDS CLI
Welcome :unicorn: to the UDS CLI! If you'd like to contribute, please reach out to one of the [CODEOWNERS](CODEOWNERS) and we'll be happy to get you started!

Below are some notes on our core software design philosophies that should help guide contributors.

## Table of Contents
1. [Code Quality and Standards](#code-quality-and-standards)
1. [How to Contribute](#how-to-contribute)
    - [Building the app](#building-the-app)
    - [Pre-Commit Hooks and Linting](#pre-commit-hooks-and-linting)
    - [Testing](#testing)

## Code Quality and Standards
Fundamentally, software engineering is a communication problem; we write code for each other, not a computer. When working on this project (or any project!) keep your fellow humans in mind and write clearly and concisely. Below are some general guidelines for code quality and standards that make UDS CLI :sparkles:

- **Write tests that give confidence**: Unless there is a technical blocker, every new feature and bug fix should be tested in the project's automated test suite. Although many of our tests are E2E, unit and integration-style tests are also welcomed. Note that unit tests can live in a `*_test.go` file alongside the source code, and E2E tests live in `src/test/e2e`


- **Prefer readability over being clever**: We have a strong preference for code readabilty in UDS CLI. Specifically, this means things like: naming variables appropriately, keeping functions to a reasonable size and avoiding complicated solutions when simple ones exist.


- **User experience is paramount**: UDS CLI doesn't have a pretty UI (yet), but the core user-centered design principles that apply when building a frontend also apply to this CLI tool. First and foremost, features in UDS CLI should enhance workflows and make life easier for end users; if a feature doesn't accomplish this, it will be dropped.

### Continuous Delivery
Continuous Delivery is core to our development philosophy. Check out [https://minimumcd.org](https://minimumcd.org/) for a good baseline agreement on what that means.

Specifically:

- We do trunk-based development (`main`) with short-lived feature branches that originate from the trunk, get merged into the trunk, and are deleted after the merge
- We don't merge code into `main` that isn't releasable
- We perform automated testing on all changes before they get merged to `main`
- We create immutable release artifacts

## How to Contribute
Please ensure there is a Gitub issue for your proposed change, this helps the UDS CLI team to understand the context of the change and to track the progress of the work. If there isn't an issue for your change, please create one before starting work. The recommended workflow for contributing is as follows:

1. **Fork this repo** and clone it locally
1. **Create a branch** for your changes
1. **Create, [test](#testing)** your changes
1. **Add docs** where appropriate
1. **Push your branch** to your fork
1. **Open a PR** against the `main` branch of this repo

### Building the app
We use UDS CLI's `run` feature (ie. vendored [Maru](https://github.com/defenseunicorns/maru-runner)) to build UDS CLI; this means that you'll need to have the `uds` binary [installed](./README.md#install) on your system in order to build the app.

To build the app, check out the [tasks](tasks.yaml) with `uds run --list-all`, find the appropriate build target for your system, and run it from the root of the repo (ex. `uds run build-cli-mac-apple`). This will create a binary in the `build` directory that you can use to test your changes (note that this binary is automatically used when running [E2E Tests](#running-tests).

### Pre-Commit Hooks and Linting
In this repo you can optionally use [pre-commit](https://pre-commit.com/) hooks for automated validation and linting, but if not CI will run these checks for you

### Testing

We strive to test all changes made to UDS CLI. If you're adding a new feature or fixing a bug, please add tests to cover the new functionality. Unit tests and E2E tests are both welcome, but we leave it up to the contributor to decide which is most appropriate for the change. Below are some guidelines for testing:

#### Unit Tests
Unit tests reside alongside the source code in a `*_test.go` file. These tests should be used to test individual functions or methods in isolation. Unit tests should be fast and focused on a single piece of functionality.

#### E2E Tests
E2E tests reside in the `src/test/e2e` directory. They use bundles located in the `src/test/e2e/bundles` which contain Zarf packages from the `src/test/e2e/packages` directory. Feel free to add new bundles and packages where appropriate. It's encouraged to write comments/metadata in any new bundles or packages to explain what they are testing.

#### Assertions
We prefer to use Testify's [require](https://github.com/stretchr/testify/tree/master/require) package for assertions in tests. This package provides a rich set of assertion functions that make tests more readable and easier to debug. See other tests in this repo for examples.

#### Running Tests
- **Unit Tests**: To run unit tests, run `uds run test:unit` from the root of the repo. This will run all unit tests in the `src` directory.


- **E2E Tests**: To run E2E tests, you'll need build UDS CLI locally, and re-build any time you make a change to the source code; this is because the binary in the `build` directory is used to drive the tests. To run the entire suite of E2E tests locally, run `uds run test:e2e-no-ghcr-write` (note that this intentionally skips the tests that involve writing to GHCR).
