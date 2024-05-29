<script lang="ts">
  import { Breadcrumb, BreadcrumbItem, Content, Theme } from 'carbon-components-svelte';

  import { type CarbonTheme } from 'carbon-components-svelte/src/Theme/Theme.svelte';

  import Header from '$lib/components/Header/Header.svelte';

  import 'carbon-components-svelte/css/all.css';

  $: innerWidth = 0;
  let theme: CarbonTheme = 'g90';
</script>

<Theme bind:theme />
<svelte:window bind:innerWidth />

<Header authenticated={true} />

<nav
  aria-hidden="false"
  class="bx--side-nav__navigation bx--side-nav bx--side-nav--ux bx--side-nav--expanded"
>
  <ul class="bx--side-nav__items">
    <li class="bx--side-nav__item">
      <div aria-expanded="true" class="bx--side-nav__submenu">
        <span class="bx--side-nav__submenu-title">Dashboard</span>
      </div>
    </li>

    <li role="separator" class="bx--side-nav__divider"></li>

    <li class="bx--side-nav__item">
      <div aria-expanded="true" class="bx--side-nav__submenu">
        <span class="bx--side-nav__submenu-title">Cluster</span>
      </div>

      <ul role="menu" class="bx--side-nav__menu">
        <li class="bx--side-nav__menu-item">
          <a href="/" class="bx--side-nav__link">
            <span class="bx--side-nav__link-text">Pods</span>
          </a>
        </li>
      </ul>
    </li>

    <li role="separator" class="bx--side-nav__divider"></li>

    <li class="bx--side-nav__item">
      <div aria-expanded="true" class="bx--side-nav__submenu">
        <span class="bx--side-nav__submenu-title">Deployment</span>
      </div>

      <ul role="menu" class="bx--side-nav__menu">
        <li class="bx--side-nav__menu-item">
          <a href="/" class="bx--side-nav__link">
            <span class="bx--side-nav__link-text">Bundles</span>
          </a>
        </li>
        <li class="bx--side-nav__menu-item">
          <a href="/" class="bx--side-nav__link">
            <span class="bx--side-nav__link-text">Packages</span>
          </a>
        </li>
        <li class="bx--side-nav__menu-item">
          <a href="/" class="bx--side-nav__link">
            <span class="bx--side-nav__link-text">Configuration</span>
          </a>
        </li>
      </ul>
    </li>

    <li role="separator" class="bx--side-nav__divider"></li>

    <li class="bx--side-nav__item">
      <div aria-expanded="true" class="bx--side-nav__submenu">
        <span class="bx--side-nav__submenu-title">Logs</span>
      </div>

      <ul role="menu" class="bx--side-nav__menu">
        <li class="bx--side-nav__menu-item">
          <a href="/" class="bx--side-nav__link" on:click={() => console.log('clicked')}>
            <span class="bx--side-nav__link-text">Operator</span>
          </a>
        </li>
        <li class="bx--side-nav__menu-item">
          <a href="/" class="bx--side-nav__link bx--side-nav__link--current">
            <span class="bx--side-nav__link-text">Policy Enforcement</span>
          </a>
        </li>
      </ul>
    </li>

    <li role="separator" class="bx--side-nav__divider"></li>

    <li class="bx--side-nav__item">
      <div aria-expanded="true" class="bx--side-nav__submenu">
        <span class="bx--side-nav__submenu-title">Security</span>
      </div>

      <ul role="menu" class="bx--side-nav__menu">
        <li class="bx--side-nav__menu-item">
          <a href="/" class="bx--side-nav__link" on:click={() => console.log('clicked')}>
            <span class="bx--side-nav__link-text">Vulnerabilities</span>
          </a>
        </li>
        <li class="bx--side-nav__menu-item">
          <a href="/" class="bx--side-nav__link">
            <span class="bx--side-nav__link-text">Compliance</span>
          </a>
        </li>
      </ul>
    </li>
  </ul>
</nav>

<Content>
  <slot />
</Content>

<style lang="scss">
  $side-nav__header-text: #aaa;

  :global(.bx--side-nav) {
    background-color: var(--cds-inverse-01);
  }

  /* Sidenav header color */
  :global(.bx--side-nav__submenu-title) {
    color: $side-nav__header-text;
  }

  /* Sidenav link color */
  :global(.bx--side-nav__link > .bx--side-nav__link-text) {
    color: $side-nav__header-text !important;
  }

  :global(.bx--side-nav__divider) {
    background-color: var(--cds-ui-01);
  }

  :global(.bx--side-nav__link):hover {
    background-color: var(--cds-ui-01) !important;
  }

  :global(.bx--side-nav__submenu) {
    cursor: default;
  }

  /* Remove hover highlight on hover for the Sidenav header */
  :global(.bx--side-nav__submenu:hover) {
    background-color: transparent;
  }

  /* Sidenav active/ selected link background color */
  :global(a.bx--side-nav__link--current) {
    background-color: var(--cds-ui-01) !important;
  }

  :global(a.bx--side-nav__link--current > .bx--side-nav__link-text) {
    color: var(--cds-interactive-03) !important;
  }
</style>
