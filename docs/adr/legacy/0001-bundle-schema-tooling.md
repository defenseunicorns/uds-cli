# 1. Bundle Schema Tooling

Date: 17 August 2023

## Status
Accepted

## Context
The first feature of UDS-CLI is to support deploying independent Zarf packages. We'll need a schema to define and validate structured data representing these Zarf packages.

While many tools in the platform engineering landscape (Helm, K8s, etc) use YAML to structure data, we wanted to investigate [CUE-lang](https://github.com/cue-lang/cue) as it has also seen some adoption by tooling such as [Acorn](https://github.com/acorn-io/runtime).


### Shortcoming of YAML
- Using whitespace as a delimiter means engineers spend too much time counting spaces, and are one invisible character away from drastically altering the desired configuration.
- Limited validation; often the data schema itself is not easily discernible and you can't test the validation until runtime.


### Pros of CUE
- Theoretically provides a better user experience than YAML.
- Can generate CUE types from Go types and import them into CUE modules.
- Can perform static type checking for the underlying schema.

### Cons of CUE
- Cannot generate Go types from CUE types (only the other way around), necessitating handling Go struct tags separately.
- Incomplete functionality around CUE modules.
- Lack of comprehensive dependency management; inspired by Go modules but exhibits quirks.
- May introduce a significant learning curve for platform engineers, considering the trade-off for a relatively small amount of configuration.

<br>After experimenting with CUE for a few days it was clear that the complexity of configuring a CUE module, types, etc was likely going to be too great to recommend over simpler tech, especially given the limited configuration of the UDS-CLI at this point. Furthermore, we experimented with CUE plugins for both Goland and VSCode, and neither provided the static type checking in an expected way.

## Decision
We have decided to use YAML for schema tooling, as opposed to CUE, based on its level of adoption in the community and the current lack of maturity in CUE.

The decision to choose YAML over CUE is influenced by the following considerations:

1. **Simplicity and Familiarity:** YAML's ease of use and widespread adoption make it a familiar choice for most developers, potentially reducing the learning curve and enabling faster development.

1. **Complexity Trade-off:** While CUE offers advanced features, the complexity it introduces may outweigh the benefits for this project's relatively small amount of current configuration.

## Consequences
The UDS Bundle schema will be based on YAML for now, but, with the correct interfaces in code, can be swapped for other schema and validation tech in the future.
