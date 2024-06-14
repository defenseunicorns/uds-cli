# 7. Carbon Svelte vs Web Components

## Status

proposed

## Context

What is the issue that we're seeing that is motivating this decision or change?

- when was the decicion made
- why was it made
- what was the context around it

## Decision

We will continue to adopt the Carbon Design System using Svelte components over switching to Carbon with Web Components.

## Pros and Cons of the Options

### IBM Carbon Svelte Components

**Pros**:

- Performance: Svelte compiles components to highly efficient vanilla JavaScript, leading to faster runtime performance.
- Developer Experience: Svelte's syntax is simple and intuitive, making it easy to learn and use, especially for developers familiar with modern JavaScript frameworks.
- Reactive Programming: Svelte has built-in reactivity, which simplifies state management and makes it easier to create dynamic user interfaces.
- Bundling and Tree Shaking: Svelte efficiently tree-shakes unused code, reducing bundle sizes and improving load times.
- Community and Ecosystem: Growing community support and increasing number of third-party components and tools for Svelte.

**Cons**:

- Maturity: Svelte is relatively new compared to more established frameworks, which may result in fewer resources and examples for troubleshooting.
- Integration: If your project involves other frameworks or libraries, integrating Svelte components might require additional setup.
- Tooling: While improving, Svelte's ecosystem of development tools and extensions is not as extensive as some older frameworks.
  IBM Carbon Web Components

### IBM Carbon Web Components

**Pros**:

- Framework Agnostic: Web components are based on standard web technologies (HTML, CSS, JavaScript) and can be used in any framework or even in plain HTML.
- Interoperability: They can be seamlessly integrated into projects that use different JavaScript frameworks, enhancing flexibility.
- Maturity and Stability: Web components are a W3C standard and have broad browser support, making them a stable choice for production applications.
- Isolation: Web components encapsulate their styles and functionality, reducing the risk of CSS and JavaScript conflicts.

**Cons**:

- Performance: Web components can sometimes be less performant compared to framework-specific solutions like Svelte due to their broader compatibility requirements.
- Complexity: The API for creating and managing web components can be more complex and verbose than framework-specific solutions.
- Reactivity: Web components do not have built-in reactivity, requiring additional effort to manage state changes and updates efficiently.
- Tooling: While supported by modern browsers, development tools and extensions specific to web components may be less mature than those for frameworks like React or Vue.

## Section here for code examples

### Argument

Switching over to Web Components requires learning a new technology as well as it's underlining templating, component library [Lit](https://lit.dev/docs/v1/lit-html/introduction/)

In the meantime, if we do feel like we need to create web components to use across platforms, we can leverage Svelte itself, to create web components as documented [here](https://svelte.dev/docs/custom-elements-api), which does come with some [limitations](https://svelte.dev/docs/custom-elements-api#caveats-and-limitations)

### Related Decisions

- [Adopting Carbon Design System](https://coda.io/d/Product_dGmk3eNjmm8/Draft-ADR-Design-System-Carbon-Design_sutAh?loginToken=billy%40defenseunicorns.com#_luHN1)
- [UI Framework](https://coda.io/d/Product_dGmk3eNjmm8/Draft-ADR-UI-framework_suDXx#_luQvX)

## Consequences

What becomes easier or more difficult to do because of this change?
