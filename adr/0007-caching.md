# 7. Caching

Date: 17 June 2024

## Status
IN PROGRESS

## Context
UDS interacts with both local and remote bundles that are made up of both local and remote zarf packages. When dealing with remote artifacts, the process can be slow due to the time it takes to download the artifacts. We want to speed up that process by caching the layers of the artifacts. There are different ways to cache these artifacts, and we need to decide which one to use. We want to take into consideration the following: efficiency, ease of implementation, and code readability. We want to be able to leverage the same caching  implementation everywhere it makes sense. We also want the caching implementation to be resilient and able to recover from any corruption.

Currently we are implementing caching in two different ways: leveraging how the oras-go library copies data from remote sources and maintaining a separate cache directory that we control. We need to decide which approach to use and refactor the codebase to use that approach everywhere.

When pulling a UDS bundle from a remote source, we are currently using the oras-go library to copy the data from the remote source to the destination content-addressable storage (CAS). The destination CAS is a persistent directory that we use as a cache.

When creating a UDS bundle and fetching remote zarf packages, we are currently using a separate cache directory that we control, to populate the temporary destination CAS that the oras-go library uses for processing.

Both approaches have their pros and cons and are described in more detail below.

## Options

### ORAS Content-Addressable Storage (CAS) as Cache
By leveraging how the [oras-go](https://github.com/oras-project/oras-go/tree/main) library copies a rooted directed acyclic graph (DAG) from the remote source CAS to the destination CAS we can treat the destination CAS as a local storage cache if we make that destination CAS persistent. While traversing the graph, for each node, it checks if it already exists in the destination. If so, it skips the node. If the node doesn't exist in the destination, it finds the successors of the node. If the node has successors, it processes the successors first. After all successors are processed, it checks if the node exists in the in-memory cache. If so, it copies the node from the in-memory cache to the destination. If not, it copies the node from the remote source to the destination.

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
We currently utilize both options in different parts of the codebase. We will need to refactor the code to use the "Programmatically Handle Caching" option everywhere. This will allow us to have more control over the caching process and make it more resilient. We will need to update the documentation to reflect the changes.

In programmatically handling the caching ourselves, we will use the UDS_CACHE which by default is set to `~/.uds-cache` to only cache image layers in the `layers` directory since the image layers are the most expensive to download in terms of size and time. This will also allow us to reuse the same caching implementation for all of the parts of the codebase that need caching. Before we copy a layer from the remote source to the destination, we will check the cache directory for the layer. If the layer exists in the cache, we will copy it from the cache to the destination CAS, otherwise we will add it to the list of layers that will need to be copied down from the remote source.
