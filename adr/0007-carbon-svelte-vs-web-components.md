# 7. Carbon Svelte vs Web Components

## Status

proposed

## Context

We previously decided to use the IBM Carbon Design System for our Svelte applications using `carbon-components-svelte`. Recently but we wanted to take a look at another option that was framework agnostic, so we starting looking at Web Components.

IBM's supports Web Components nas has a [package](https://github.com/carbon-design-system/carbon-for-ibm-dotcom/tree/main/packages/carbon-web-components) to help you create Custom Elements

What we wanted to do was take a look at the differences/ pros and cons of using the Svelte variation vs the Web Componnets variations of Carbon in order to decided which one provides the best solution for development but also for design. Some of the determining factors are ease of use, ability to customize as well as support in design tools such as Figma

## Decision

We will continue to adopt the Carbon Design System using Svelte components over switching to Carbon with Web Components.

### Argument

Switching over to Web Components requires learning a new technology as well as it's underlining templating, component library [Lit](https://lit.dev/docs/v1/lit-html/introduction/)

In the meantime, if we do feel like we need to create web components to use across platforms, we can leverage Svelte itself, to create web components as documented [here](https://svelte.dev/docs/custom-elements-api), which does come with some [limitations](https://svelte.dev/docs/custom-elements-api#caveats-and-limitations)

Customization played a big role in the decision of which option to chose. Svelte allowed for easier customization because Svelte components are simply broken down into traditional html elements like `div, section, header` etc. This allowed us to look at the generated html from a component with all of the carbon classes. We could then add or subtract from the html structure to create either a simpler component or a more complex component. Here is an example below

This...

```Svelte
  <HeaderNav>
    <HeaderNavMenu text="Menu">
      <HeaderNavItem href="/" text="Link 1" />
      <HeaderNavItem href="/" text="Link 2" />
      <HeaderNavItem href="/" text="Link 3" />
    </HeaderNavMenu>
    <HeaderNavItem href="/" text="Link 4" />
  </HeaderNav>
```

converts to this...

```Svelte
  <nav class="bx--header__nav">
    <ul role="menubar" class="bx--header__menu-bar">
      <li role="none" class="bx--header__submenu">
        <a
          role="menuitem"
          tabindex="0"
          aria-haspopup="menu"
          aria-expanded="false"
          aria-label="Menu"
          href="/"
          class="bx--header__menu-item bx--header__menu-title"
          style="z-index: 1;"
        >
          Menu

          <svg
            xmlns="http://www.w3.org/2000/svg"
            viewBox="0 0 32 32"
            fill="currentColor"
            preserveAspectRatio="xMidYMid meet"
            width="16"
            height="16"
            aria-hidden="true"
            class="bx--header__menu-arrow"
          >
            <path d="M16 22L6 12 7.4 10.6 16 19.2 24.6 10.6 26 12z"></path>
          </svg>
        </a>

        <ul role="menu" aria-label="Menu" class="bx--header__menu">
          <li role="none">
            <a role="menuitem" tabindex="0" href="/" class="bx--header__menu-item">
              <span class="bx--text-truncate--end">Link 1</span>
            </a>
          </li>
          <li role="none">
            <a role="menuitem" tabindex="0" href="/" class="bx--header__menu-item">
              <span class="bx--text-truncate--end">Link 2</span>
            </a>
          </li>
          <li role="none">
            <a role="menuitem" tabindex="0" href="/" class="bx--header__menu-item">
              <span class="bx--text-truncate--end">Link 3</span>
            </a>
          </li>
        </ul>
      </li>
      <li role="none">
        <a role="menuitem" tabindex="0" href="/" class="bx--header__menu-item">
          <span class="bx--text-truncate--end">Link 4</span>
        </a>
      </li>
    </ul>
  </nav>
```

and then we can customize it and add the `withIcon` attribute as a slot to allow the ability to have an icon and be able to chose a what icon

```Svelte
  <nav class="bx--header__nav">
    <ul role="menubar" class="bx--header__menu-bar">
      <li role="none" class="bx--header__submenu">
        <a
          role="menuitem"
          tabindex="0"
          aria-haspopup="menu"
          aria-expanded="false"
          aria-label="Menu"
          href="/"
          class="bx--header__menu-item bx--header__menu-title"
          style="z-index: 1;"
        >
          {#if withIcon}
            <div
              class="header__select-icon"
              data-testid="header__select-icon--{title.toLowerCase()}-test-id"
            >
              <slot name="account-icon" />
            </div>
          {/if}

          Menu

          <svg
            xmlns="http://www.w3.org/2000/svg"
            viewBox="0 0 32 32"
            fill="currentColor"
            preserveAspectRatio="xMidYMid meet"
            width="16"
            height="16"
            aria-hidden="true"
            class="bx--header__menu-arrow"
          >
            <path d="M16 22L6 12 7.4 10.6 16 19.2 24.6 10.6 26 12z"></path>
          </svg>
        </a>

        <ul role="menu" aria-label="Menu" class="bx--header__menu">
          ...
        </ul>
      </li>
      <li role="none">
        ...
      </li>
    </ul>
  </nav>
```

When trying to customize a Web Component, we have to leverage the Lit library to be able to override a component...

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

The isue here is that, we can only seem to be able to override styles and if we try an approach similar to what we did with Svelte, it will not be as straight forward because Web Components are actual HTML elements and so they do not "break down" further to show the structure generated from the component. Trying to chase down the shadow elements created by the component tree, including it's slots and css parts can become a tedeous job that might not lead to the results desired.

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
