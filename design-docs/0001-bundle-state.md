# Bundle State

## Context

The following 2 issues provide context driving the need for a UDS state tracking mechanism:

#### 1. What's in my cluster?

UDS CLI users have requested the ability to see the state of their bundles that are currently deployed in K8s cluster. This information is useful to know before applying upgrades, troubleshooting, removing bundles, etc. Currently, Zarf provides a `package list` command that lists all the packages in the cluster and users would like similar functionality for bundles.

#### 2. Unreferenced Zarf Packages

Today it's possible for packages that have been removed from a `uds-bundle.yaml` but previously deployed in the cluster to become unreferenced. For example, an engineer has created a UDS bundle consisting of 3 packages and deployed this bundle to a mission environment. Later, the engineer decides one of those packages is no longer needed, so they remove it from the bundle and deploy the bundle again. The package that was removed from the bundle is still present in the cluster and is now unreferenced, and the engineer must manually remove it.

## UDS State

In order to address the above issues, the team has decided to implement a state tracking mechanism that will store metadata about a bundle that has been deployed to the cluster.

#### Design Principles

- Keep state as simple as possible. Meaning that we should think of state as a record of an event, as opposed to a complex object that drives CLI behavior.
- No destructive action should be taken based on UDS state unless the user explicitly requests it.
- State should be backwards compatible and should not interfere with existing UDS CLI functionality.
  - For now, do not base any UDS CLI business logic on UDS state
  - On backwards compatibility: if a user attempts an action that is based on state but state does not exist, CLI should fail quickly and indicate to the user that state does not exist and provide instructions on how to create it (likely simply re-deploying the bundle)

## State Storage

The following options were considered for storing UDS state:

- K8s Secrets
  - Pros: limits access by namespace; Helm and Zarf's proven implementation
  - Cons: hacking a secret resource for something isn't technically a secret
- K8s ConfigMaps
  - Pros: Easy to use
  - Cons: Not as secure as secrets (any namespace can access)
- K8s Custom Resources
  - Pros: Custom resource designed to store bundle information
  - Cons: Heavy-handed approach; we don't want to secret data to be easily manipulated by users

### Decision

We will use K8s Secrets to store UDS state. This decision was made because it is the most secure option and aligns with the way Helm and Zarf store state information.

## State Contents and Location

Each UDS bundle deployed in the cluster will have its own state secret. The state secret will take the following form:

```go
type PkgStatus struct {
	Name        string    `json:"name"`
	Version     string    `json:"version"`
	Status      string    `json:"status"`
	DateUpdated time.Time `json:"date_updated"`
}

type BundleState struct {
	Name        string      `json:"name"`
	Version     string      `json:"version"`
	PkgStatuses []PkgStatus `json:"packages"`
	Status      string      `json:"status"`
	DateUpdated time.Time   `json:"date_updated"`
}
```

Note that the `BundleState.DateUpdated` field refers to the last time the bundle itself or any of the bundled packages were updated. The `BundleState` struct will be stored in the secret's `data` field as a base64 encoded JSON string.

#### Namespace

The UDS state secret will be stored in the `uds` namespace. If the `uds` namespace doesn't exist, the CLI will create it.

### Viewing State
For now, we will not introduce a dedicated UDS CLI command for users to view state. To view state, users can either used the vendored tools (`kubectl` and/or `k9s`) or view the state in UDS Runtime.

## State Implementation

Generally speaking, UDS CLI will update a bundle's state during deploy and remove operations. The bundle state will serve as a record of the bundle's deployment status and the status of each package in the bundle.

### Behaviors

- Bundle states will track the packages in the bundle and their statuses, if a package has been removed in future versions of a bundle, CLI will provide a mechanism to prune the cluster and state.
- The state secret will be created during the first deployment of a bundle. If the bundle is deployed again, the state secret will be updated.
- The state secret will be deleted when a bundle is removed.
- If using the `--packages` flag, the CLI will only update the state for the specified packages, but the bundle secret will indicate that a package in the bundle hasn't been deployed, or in the case of removal, that a package has been removed.

### API Design
UDS CLI will provide a bundle state API in the form of a Go pkg called `github.com/defenseunicorns/uds-cli/src/pkg/state`. This package will provide an API for creating and interacting with state during the bundle lifecycle. Proposed public methods include:

- `NewClient`: creates a new state client
- `InitBundleState`: creates the `uds` namespace if it doesn't exist; creates a new state secret or returns an existing state secret for a previously deployed bundle
- `GetBundleState`: retrieves the state for a given bundle
- `GetBundlePkgState`: retrieves the state for a given package inside a bundle
- `UpdateBundleState`: updates the state for a given bundle
- `UpdateBundlePkgState`: updates the state for a given package inside a bundle
- `RemoveBundleState`: deletes the state for a given bundle; note that this will remove the K8s secret containing the state

### Statuses

`BundleState` and corresponding `PkgStatus` will be limited to the following statuses:

```go
  Success      = "success" // deployed successfully
  Failed       = "failed"  // failed to deploy
  Deploying    = "deploying" // deployment in progress
  NotDeployed  = "not_deployed" // package is in the bundle but not deployed
  Removing     = "removing" // removal in progress
  Removed      = "removed" // package removed (does not apply to BundleState)
  FailedRemove = "failed_remove" // package failed to be removed (does not apply to BundleState)
  Unreferenced     = "Unreferenced" // package has been removed from the bundle but still exists in the cluster
```

We will intentionally keep the list of statuses small to reduce the need for more complex state management.

### Pruning

If a package is removed from a bundle, the CLI will provide a mechanism to prune the cluster of unreferenced packages and update state. The general algorithm for pruning is as follows:

1. An engineer deploys a bundle containing package `foo` and package `bar`
1. UDS state is updated to reflect the bundle and its 2 packages have deployed successfully
1. Later, the engineer decides they no longer need package `bar` and removes it from their `uds-bundle.yaml`
1. Upon the next deployment, the CLI will update the state to reflect that package `bar` is no longer in the bundle and will mark it as `Unreferenced`

#### Pruning Mechanisms

There are 2 mechanisms for removing `Unreferenced` packages from the cluster. Using the example above:
1. If the engineer runs `uds deploy ... --prune` then the CLI will remove package `bar` from the cluster and update the state to reflect that package `bar` has been removed, by removing `bar` from the bundle's state
1. If the engineer runs `uds prune <bundle tarball>` then the user will be shown a list of unreferenced packages and asked to confirm removal. The CLI will then remove the unreferenced packages from the cluster and update the state to reflect that the packages have been removed.
    - `uds prune` will also have a flag `--confirm` to automatically remove unreferenced packages without user confirmation
