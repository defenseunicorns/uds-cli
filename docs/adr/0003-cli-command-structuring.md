# 3. CLI Command Structuring

Date: 2026-02-04

## Status

Accepted

## Context

As UDS CLI Next is being designed as "the CLI entrypoint to the Defense Unicorns ecosystem" rather than a single-purpose tool, decisions must be made about how commands are structured.

## Decision

Based on the past discussion (see the [Appendix: Summary of the past discussions](#appendix-summary-of-the-past-discussions)) and the analysis of popular CLI tools (see the [Appendix: Analysis of Popular CLI Tools](#appendix-analysis-of-popular-cli-tools)), it has been decided to use the resource-first pattern (`uds <component> <resource> <action>`), where:
- **component** is the Defense Unicorns ecosystem component such as `core` for the UDS Core or `registry` for UDS Registry.
- **resource** is the type of resource being operated on, such as `bundle` in the context of `core` or `entitlements` in the context of `registry`.
- **action** is the operation being performed on the resource, such as `create`, `get`, `delete`, etc.

Here are a few examples:
```bash
uds registry entitlements get
uds registry entitlements delete
uds core pepr monitor
```

Both the **component** and the **action** can be optional. In this case, they are called the **default component** and the **default action**. This allows for shorter commands when the context is clear:
```bash
uds bundle create  # Uses the default component
uds core logs      # Uses the default action
```

## Consequences

### Positive

- **Ecosystem Scalability**: The component-resource-action pattern allows seamless addition of new components (e.g., `uds tofu`, `uds core`) without restructuring existing commands
- **Future-Proof Design**: Avoids the need to revisit command structure as new functionality (tofu, registry, core interactions) is added
- **Clarity for Users**: Explicit resource specification eliminates ambiguity about what the command operates on, especially as UDS CLI Next grows beyond bundles
- **Discoverability**: Users can explore available resources within a component via `uds <component> --help`
- **Consistency with Industry Standards**: Aligns with patterns used by kubectl, docker, and gh - tools familiar to our target audience

### Negative

- **More Typing**: Commands like `uds core monitor pepr` are longer than `uds monitor pepr`
- **Migration Effort**: Users familiar with UDS CLI v0 patterns will need to adapt to the new structure
- **Potential Redundancy**: The `uds core` pattern for core bundle operations may feel redundant initially
- **Restructuring Existing Commands**: When moving commands from, for example, the **default component** into a new **component**, this is always a breaking change with a large impact on Mission Heros (as the CLI is very likely to be used in scripting).

### Neutral

- **Shorthand Aliases**: May consider implementing aliases (e.g., `b` for `bundle`) to reduce typing while maintaining clarity
- **Documentation Requirements**: More comprehensive documentation needed to explain the command hierarchy
- **Tab Completion**: Good shell completion becomes more important to assist with longer command paths

---

## Appendix: Summary of the past discussions

1. **[PR #9 Discussion (uds-cli-next)](https://github.com/defenseunicorns/uds-cli-next/pull/9#discussion_r2741964623)**: The team discussed that including a resource noun (like `bundle`) helps clarify what the CLI operates on. The concern was raised that the resource-less `uds create` pattern may have contributed to UDS CLI v0 being perceived primarily as "the bundle tool" rather than an ecosystem entrypoint. UDS CLI Next is envisioned to handle bundles, tofu operations, registry interactions, UDS Core management, and developer tooling.

2. **[Issue #73 (uds-cli)](https://github.com/defenseunicorns/uds-cli/issues/73)**: A historical debate from 2023 about whether to use `uds bundle create` vs `uds create`. Key arguments:
    - **For resource noun**: Provides clarity when CLI will have multiple purposes; avoids confusion as new functionality is added; more intuitive for new users ("you're creating a bundle, not creating a uds").
    - **Against resource noun**: Less typing; follows patterns of single-purpose tools like `helm install`; if the primary purpose is bundles, the noun is implied.
    - **Resolution at the time**: The team decided to keep `uds <action>` without `bundle` keyword, but acknowledged this may need revisiting if scope expands.

## Appendix: Analysis of Popular CLI Tools

### 1. kubectl (Kubernetes CLI)

**Approach**:
- Resource-first hierarchy with a verb-noun pattern (`kubectl <action> <resource>`).
- Consistent verb usage across all resources (get, create, delete, apply, describe)

**Examples**:
```bash
kubectl get pods
kubectl create deployment nginx --image=nginx
kubectl delete service my-service
```

### 2. oc (OpenShift CLI)

**Approach**:
- Extends kubectl patterns with OpenShift-specific resources and developer workflows.
- Adds OpenShift-specific commands at root-level for common workflows

**Examples**:
```bash
oc get routes
oc start-build my-build
oc login https://cluster.example
```

### 3. cmctl (cert-manager CLI)

**Approach**:
- Resource-centric with domain-specific operations.
- Action-resource pattern for clarity

**Examples**:
```bash
cmctl status certificate my-cert
cmctl renew my-cert
cmctl approve my-csr
```

### 4. helm (Kubernetes Package Manager)

**Approach**:
- Primary resource (chart) is implied in most commands
- Verb-first pattern since there's mainly one thing you operate on

**Examples**:
```bash
helm install my-release bitnami/nginx
helm upgrade my-release bitnami/ngin
helm search repo nginx
```

### 5. docker (Container Runtime CLI)

**Approach**:
- Resource-based grouping with action verbs
- Commands grouped by resource type (container, image, network, volume)

**Examples**:
```bash
docker container run nginx
docker image pull nginx
docker run nginx
```

### 6. gh (GitHub CLI)

**Approach**:
- Commands organized by GitHub resource (repo, pr, issue, workflow)
- Resource-based with nested subcommands.

**Examples**:
```bash
gh repo clone owner/repo
gh pr create --title "My PR"
gh issue list
```

### 7. terraform/tofu (Infrastructure as Code)

**Approach**:
- Commands represent workflow stages (init, plan, apply, destroy)
- Workflow-based with implied resource (configuration).

**Examples**:
```bash
terraform plan
terraform apply
terraform workspace new dev
```

---

## Summary of Patterns

| CLI Tool | Pattern | Resource Explicit? | Primary Use Case |
|----------|---------|-------------------|------------------|
| kubectl | verb-resource | Yes | Multi-resource orchestration |
| oc | verb-resource + shortcuts | Yes | Multi-resource + dev workflows |
| cmctl | action-resource | Yes | Domain-specific resources |
| helm | verb (implied resource) | No | Single primary resource |
| docker | resource-verb | Yes (with shortcuts) | Multi-resource management |
| gh | resource-verb | Yes | Multi-resource (GitHub entities) |
| terraform | workflow-stage | No | Single configuration context |

**Summary**: CLIs that manage multiple resource types (kubectl, docker, gh) tend to require explicit resource specification. Single-purpose CLIs (helm, terraform) often imply the resource and might use a workflow-based pattern.
