<script lang="ts">
  import '@carbon/web-components/es/components/ui-shell/index.js';
</script>

<cds-header aria-label="UDS" role="banner">
  <img
    alt="Defense Unicorns Logo"
    src="https://www.defenseunicorns.com/images/svg/doug.svg"
    style:height="32px"
    style:width="32px"
    style:padding-left="16px"
  />

  <cds-header-name href="/" prefix="">UDS</cds-header-name>

  <cds-header-nav menu-bar-label="UDS" role="navigation">
    <cds-header-menu menu-label="Services" role="listitem" trigger-content="Services">
      <cds-header-menu-item href="javascript:void 0" tabindex="-1" role="listitem">
        Deployment
      </cds-header-menu-item>

      <cds-header-menu-item href="javascript:void 0" tabindex="-1" role="listitem">
        Security
      </cds-header-menu-item>

      <cds-header-menu-item href="javascript:void 0" tabindex="-1" role="listitem">
        IDAM
      </cds-header-menu-item>

      <cds-header-menu-item href="javascript:void 0" tabindex="-1" role="listitem">
        AI/ML
      </cds-header-menu-item>

      <cds-header-menu-item href="javascript:void 0" tabindex="-1" role="listitem">
        App Dashboard
      </cds-header-menu-item>
    </cds-header-menu>
  </cds-header-nav>

  <div class="cds--header__global">
    <cds-header-menu menu-label="prod.uds.is" role="listitem" trigger-content="prod.uds.is">
      <cds-header-menu-item href="javascript:void 0" tabindex="-1" role="listitem">
        uds.us/staging
      </cds-header-menu-item>

      <cds-header-menu-item href="javascript:void 0" tabindex="-1" role="listitem">
        prod.uds.us
      </cds-header-menu-item>

      <cds-header-menu-item href="javascript:void 0" tabindex="-1" role="listitem">
        prod.uds.is
      </cds-header-menu-item>

      <cds-header-menu-item href="javascript:void 0" tabindex="-1" role="listitem">
        Uds.is/staging
      </cds-header-menu-item>

      <cds-header-menu-item href="javascript:void 0" tabindex="-1" role="listitem">
        spaceforce.swf.gov
      </cds-header-menu-item>
    </cds-header-menu>

    <div part="divider" class="cds-ce--header__divider" />

    <cds-header-menu menu-label="username" role="listitem" trigger-content="username">
      <cds-header-menu-item href="javascript:void 0" tabindex="-1" role="listitem">
        Link 1
      </cds-header-menu-item>

      <cds-header-menu-item href="javascript:void 0" tabindex="-1" role="listitem">
        Link 2
      </cds-header-menu-item>

      <cds-header-menu-item href="javascript:void 0" tabindex="-1" role="listitem">
        Link 3
      </cds-header-menu-item>
    </cds-header-menu>
  </div>

  <cds-side-nav
    aria-label="Side navigation"
    collapse-mode="fixed"
    role="navigation"
    expanded
    style:margin-top="3px"
  >
    <cds-side-nav-items role="list">
      <cds-side-nav-menu title="Dashboard" role="listitem" />

      <cds-side-nav-divider />

      <cds-side-nav-menu title="Monitor" role="listitem" expanded>
        <cds-side-nav-menu-item href="/monitor/pepr" role="button" tabindex="-1">
          Pepr
        </cds-side-nav-menu-item>
      </cds-side-nav-menu>

      <cds-side-nav-divider />

      <cds-side-nav-menu title="Application Portal" role="listitem" />
    </cds-side-nav-items>
  </cds-side-nav>
</cds-header>

<div style:margin-left="2em" style:margin-top="4em">
  <slot />
</div>

<style lang="scss">
  @use '@carbon/styles/scss/reset';
  @use '@carbon/styles/scss/theme';
  @use '@carbon/styles/scss/themes';

  :root {
    @include theme.theme(themes.$g100);
    background-color: var(--cds-background);
    color: var(--cds-text-primary);
    font-family: 'IBM Plex Sans', 'Helvetica Neue', Arial, sans-serif;
  }

  .cds-ce--header__divider {
    margin-top: 25px;
    background-color: var(--cds-border-subtle);
    block-size: 1.5rem;
    inline-size: 0.0625rem;
    inset-block-start: 50%;
    inset-inline-start: 0;
    transform: translateY(-50%);
  }

  cds-side-nav-menu::part(expando-icon-container) {
    pointer-events: none;
    display: none;
  }
</style>
