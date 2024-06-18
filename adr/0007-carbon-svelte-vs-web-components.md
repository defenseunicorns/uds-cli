# 7. Carbon Svelte vs Web Components

## Status

proposed

## Context

We had previously decided to use the IBM Carbon Design System for our Svelte applications using `carbon-components-svelte`, but we wanted to take a look at another option that was framework agnostic, so we starting looking at Web Components.

IBM's supports Web Components nas has a [package](https://github.com/carbon-design-system/carbon-for-ibm-dotcom/tree/main/packages/carbon-web-components) to help you create Custom Elements

What we wanted to do was take a look at the differences/ pros and cons of using the Svelte variation vs the Web Componnets variations of Carbon in order to decided which one provides the best solution for development but also for design. Some of the determining factors are ease of use, ability to customize as well as support in design tools such as Figma

What is the issue that we're seeing that is motivating this decision or change?

- why was it made

```JavaScript
import { css } from 'lit';
import { customElement } from 'lit/decorators/custom-element.js';
import SideNav from '@carbon/web-components/es/components/ui-shell/side-nav.js';

@customElement('my-side-nav')
export class MySideNav extends SideNav {
  static styles = css`
    ${SideNav.styles}
    .cds--side-nav__navigation {
      background-color: white;
    }
  `;
}

```

## Decision

We will continue to adopt the Carbon Design System using Svelte components over switching to Carbon with Web Components.

### Argument

Switching over to Web Components requires learning a new technology as well as it's underlining templating, component library [Lit](https://lit.dev/docs/v1/lit-html/introduction/)

In the meantime, if we do feel like we need to create web components to use across platforms, we can leverage Svelte itself, to create web components as documented [here](https://svelte.dev/docs/custom-elements-api), which does come with some [limitations](https://svelte.dev/docs/custom-elements-api#caveats-and-limitations)

Customization played a big role in the decision of which option to chose. Svelte allowed for easier customization because Svelte components are simply broken down into traditional html elements like `div, section, header` etc. This allowed us to look at the generated html from a component with all of the carbon classes. We could then add or subtract from the html structure to create either a simpler component or a more complex component. Here is an example below

This...

converts to this...

And we can then custommizt to add an icon to a componnet that didn't previously have the option to

### Related Decisions

- [Adopting Carbon Design System](https://coda.io/d/Product_dGmk3eNjmm8/Draft-ADR-Design-System-Carbon-Design_sutAh?loginToken=billy%40defenseunicorns.com#_luHN1)
- [UI Framework](https://coda.io/d/Product_dGmk3eNjmm8/Draft-ADR-UI-framework_suDXx#_luQvX)

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

## Consequences

What becomes easier or more difficult to do because of this change?
