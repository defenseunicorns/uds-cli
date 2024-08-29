# Bundle State

## Context

The following 2 issues provide context driving the need for a UDS state tracking mechanism:

#### 1. What's in my cluster?

UDS CLI users have requested the ability to see the state of their bundles that are currently deployed in K8s cluster. This information is useful to know before applying upgrades, troubleshooting, removing bundles, etc. Currently, Zarf provides a `package list` command that lists all the packages in the cluster and users would like similar functionality for bundles.

#### 2. Orphaned Zarf Packages
Today it's possible for packages that have been removed from a `uds-bundle.yaml` but previously deployed in the cluster to become orphaned. For example, an engineer has created a UDS bundle consisting of 3 packages and deployed this bundle to a mission environment. Later, the engineer decides one of those packages is no longer needed, so they remove it from the bundle and deploy the bundle again. The package that was removed from the bundle is still present in the cluster and is now orphaned, and the engineer must manually remove it.

## UDS State

In order to address the above issues, the team has decided to implement a state tracking mechanism that will store metadata about a bundle that has been deployed to the cluster.

#### Design Principles

- Keep state as simple as possible. Meaning that we should think of state as a record of an event, as opposed to a complex object that drives CLI behavior.
- No destructive action should be taken based on UDS state unless the user explicitly requests it.
- State should be backwards compatible and should not interfere with existing UDS CLI functionality.
  - On backards compatibility: if a user attempts an action that is based on state but state does not exist, CLI should fail quickly and indicate to the user that state does not exist and provide instructions on how to create it (likely simply re-deploying the bundle)

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
	Name    string `json:"name"`
	Version string `json:"version"`
	Status  string `json:"status"`
}

type BundleState struct {
	Name        string      `json:"name"`
	Version     string      `json:"version"`
	PkgStatuses []PkgStatus `json:"packages"`
	Status      string      `json:"status"`
}
```

The `BundleState` struct will be stored in the secret's `data` field as a base64 encoded JSON string.

#### Namespace

The UDS state secret will be stored in the `uds` namespace. If the `uds` namespace doesn't exist, the CLI will create it.

## State Implementation

### API Design
UDS CLI will provide a bundle state API in the form of a Go pkg called `github.com/defenseunicorns/uds-cli/src/pkg/state`. This package will provide an API for creating and interacting with state during the bundle lifecycle. Proposed public methods include:

- `NewClient`: creates a new state client
- `InitBundleState`: creates the `uds` namespace if it doesn't exist, and creates a new state secret or returns an existing state secret for a previously deployed bundle
- `GetBundleState`: retrieves the state for a given bundle
- `GetBundlePkg`: retrieves the state for a given package inside a bundle
- `UpdateBundleState`: updates the state for a given bundle
- `UpdateBundlePkgStat`: updates the state for a given package inside a bundle
- `RemoveBundleState`: deletes the state for a given bundle; note that this will remove the K8s secret containing the state

#### Statuses

`BundleState` and corresponding `PkgStatus` will be limited to the following statuses:

```go
  Success      = "success" // deployed successfully
  Failed       = "failed"  // failed to deploy
  Deploying    = "deploying" // deployment in progress
  NotDeployed  = "not_deployed" // package is in the bundle but not deployed
  Removing     = "removing" // removal in progress
  Removed      = "removed" // package removed (does not apply to BundleState)
  FailedRemove = "failed_remove" // package failed to be removed (does not apply to BundleState)
```
