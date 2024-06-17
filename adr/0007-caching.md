# 7. Caching

Date: 17 June 2024

## Status
IN PROGRESS

## Context
UDS interacts with both local and remote bundles that are made up of both local and remote zarf packages. When dealing with remote artifacts, the process can be slow due to the time it takes to download the artifacts. We want to speed up that process by caching the artifacts. There are different ways to cache these artifacts, and we need to decide which one to use. We want to take into consideration the following: efficiency, ease of implementation, and code readability. We want to be able to leverage the same caching  implementation everywhere it makes sense. We also want the caching implementation to be resilient and able to recover from any corruption.

## Options

### ORAS Content-Addressable Storage (CAS) as Cache
By leveraging how the oras-go library copies a rooted directed acyclic graph (DAG) from the source CAS to the destination CAS we can treat the destination CAS as a local storage cache if we make that destination CAS persistent. While traversing the graph, for each node, it checks if it already exists in the destination. If so, it skips the node. If the node doesn't exist in the destination, it finds the successors of the node. If the node has successors, it processes the successors first. After all successors are processed, it checks if the node exists in the in-memory cache. If so, it copies the node from the in-memory cache to the destination. If not, it copies the node from the source to the destination.

The issue with using the destination CAS as a persistent cache in oras is that when it saves a node to the store, it saves all of the successors first before saving the node, that way it knows if a node is present then all of the successors are present. The issue is that if a successor is removed or corrupted in the store, oras will still only check the parent node and if it exists in the destination CAS it assumes that the successors are also present.

#### PROS
 - oras-go library handles the caching internally for us

#### CONS
 - We can't control what gets cached
 - We can't control how the cache is checked during processing

### Programmatically Handle Caching
We can still use the oras-go library for copying and storing oci artifacts, but instead of using the destination CAS as a cache, we can create a separate cache directory that we can control. We can check the cache directory for the existence of the data before copying it from the source. If the data exists in the cache, we can skip it. If the node doesn't exist in the cache, we can copy it from the source. With this approach, the destination CAS would be a tmp directory that we can delete after processing is done. This lets us control what gets cached and how the cache is checked during processing.

#### PROS
 - We can control what gets cached
 - We can control how the cache is checked during processing

#### CONS
 - We write and maintain the caching logic ourselves

## Decision
Option 2: Programmatically Handle Caching

## Consequences
We currently utilize both options in different parts of the codebase. We will need to refactor the code to use the programmatically handle caching option everywhere. This will allow us to have more control over the caching process and make it more resilient. We will need to update the documentation to reflect the changes.
