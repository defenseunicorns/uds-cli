<script lang="ts">
  import { type HeaderSelectProps } from './types';

  export let items: HeaderSelectProps[];
  export let title: string;
  export let withIcon: boolean = false;

  let expanded: boolean = false;
  let ref: any = null;
</script>

<svelte:window
  on:click={({ target }) => {
    if (!ref.contains(target)) {
      expanded = false;
    }
  }}
/>

<ul role="menubar" class="bx--header__menu-bar">
  <li role="none" class="bx--header__submenu" bind:this={ref}>
    <a
      on:click={() => (expanded = !expanded)}
      role="menuitem"
      tabindex="0"
      aria-haspopup="menu"
      aria-expanded={expanded}
      aria-label="Menu"
      href="/"
      class="bx--header__menu-item bx--header__menu-title"
      style="z-index: 1;"
      data-testid="header__select-menu-action-{title.toLowerCase()}-test-id"
    >
      {#if withIcon}
        <div class="header__select-icon" data-testid="header__select-icon--{title.toLowerCase()}-test-id">
          <slot name="account-icon" />
        </div>
      {/if}

      {title}
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

    <ul
      role="menu"
      aria-label="Menu"
      class="bx--header__menu"
      data-testid="bx--header__menu-test-id-{title.toLowerCase()}"
    >
      {#each items as item}
        <li role="none">
          <a role="menuitem" tabindex="0" href="/" class="bx--header__menu-item">
            <span class="bx--text-truncate--end">{item.title}</span>
          </a>
        </li>
      {/each}
    </ul>
  </li>
</ul>

<style lang="scss">
  /* override z-index so we can see menu options over the sidenav */
  :global(.bx--side-nav) {
    z-index: 0 !important;
  }

  .header__select-icon {
    margin-top: 6px;
    margin-right: 8px;
  }
</style>
