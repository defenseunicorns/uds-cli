# 10. In-cluster bundle state

Date: 17 July 2024

## Status
Accepted

## Context
As a user of the UDS-CLI, I want to be able to see the state of my bundles in the cluster so that I can understand what is currently deployed. This information is useful to know before applying upgrades, troubleshooting, removing bundles, etc.

Currently Zarf provides a `package list` command that lists all the packages in the cluster and users would like similar functionality for bundles.

## Options

### Zarf It
Save metadata about a package that has been deployed to the cluster in a secret. Saved data includes name, package kind, metadata, build data, components, constants, variables, and installed charts among other data. (Could also use a configmap)

pros:
- already have template of implementation in zarf
- (main reason zarf took this approach) this is how helm does it, it uses secrets as a default storage driver

cons:
- hacking a secret resource for something other than a secret

### Leverage package metadata
Add bundle information to the package metadata.
- There isn't a specific field we could use, but we could "hijack" an existing field like `description` and add relevant bundle information there.

Pros:
- implementation would be pretty straightforward, we already do package manipulation in dev mode.
- we would be able to leverage the `zarf package list` functionality to pull and filter on the bundle information that gets added.

Cons:
- we would be overloading the package metadata with bundle information, which could be confusing.
- we compromise the integrity of the package by modifying it.

### Custom Resource
We create a Bundle custom resource that stores all the bundle information.
https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/

Pros:
- we can create a custom resource that is specifically designed to store bundle information, that we can easily push to and retrieve from the cluster.

Cons:
- might be overkill if we are just using it for storage.

<b>From K8s docs:</b>

Should I use a ConfigMap or a custom resource?
Use a ConfigMap if any of the following apply:

- There is an existing, well-documented configuration file format, such as a mysql.cnf or pom.xml.
- You want to put the entire configuration into one key of a ConfigMap.
- The main use of the configuration file is for a program running in a Pod on your cluster to consume the file to configure itself.
- Consumers of the file prefer to consume via file in a Pod or environment variable in a pod, rather than the Kubernetes API.
- You want to perform rolling updates via Deployment, etc., when the file is updated.

Note:
Use a Secret for sensitive data, which is similar to a ConfigMap but more secure.

Use a custom resource (CRD or Aggregated API) if most of the following apply:

- You want to use Kubernetes client libraries and CLIs to create and update the new resource.
- You want top-level support from kubectl; for example, kubectl get my-object object-name.
- You want to build new automation that watches for updates on the new object, and then CRUD other objects, or vice versa.
- You want to write automation that handles updates to the object.
- You want to use Kubernetes API conventions like .spec, .status, and .metadata.
- You want the object to be an abstraction over a collection of controlled resources, or a summarization of other resources.
