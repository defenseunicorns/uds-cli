<script lang="ts">
  import { Content } from 'carbon-components-svelte';

  import { Button, HeaderGlobalAction, Search } from 'carbon-components-svelte';

  import ArrowRight from 'carbon-icons-svelte/lib/ArrowRight.svelte';
  import HelpFilled from 'carbon-icons-svelte/lib/HelpFilled.svelte';
  import NotificationFilled from 'carbon-icons-svelte/lib/NotificationFilled.svelte';
  import UserAvatarFilled from 'carbon-icons-svelte/lib/UserAvatarFilled.svelte';

  import HeaderSelect from '$lib/components/Header/HeaderSelect.svelte';
  import HeaderSeparator from '$lib/components/Header/HeaderSeparator.svelte';

  import { type HeaderSelectProps } from '$lib/components/Header/types';

  let ref = null;
  let active = true;
  let value = '';
  let selectedResultIndex = 0;

  $: console.log('ref', ref);
  $: console.log('active', active);
  $: console.log('value', value);
  $: console.log('selectedResultIndex', selectedResultIndex);

  export let authenticated: boolean = true;

  let leftMenuLinks: HeaderSelectProps[] = [
    {
      title: 'Deployment',
      path: '/deployment'
    },
    {
      title: 'Security',
      path: '/security'
    },
    {
      title: 'IDAM',
      path: '/idam'
    },
    {
      title: 'AI/ML',
      path: '/ai_ml'
    },
    {
      title: 'App Dashboard',
      path: '/dashboard'
    }
  ];

  let prodMenuLinks: HeaderSelectProps[] = [
    {
      title: 'uds.us/staging',
      path: '/uds-us-staging'
    },
    {
      title: 'prod.uds.us',
      path: '/prod-us'
    },
    {
      title: 'prod.uds.is',
      path: '/prod-is'
    },
    {
      title: 'Uds.is/staging',
      path: '/uds-is-staging'
    },
    {
      title: 'spaceforce.swf.gov',
      path: '/spaceforce'
    }
  ];

  let lastMenuLinks: HeaderSelectProps[] = [
    {
      title: 'Link 1',
      path: '/link-1'
    },
    {
      title: 'Link 2',
      path: '/link-2'
    },
    {
      title: 'Link 3',
      path: '/link-3'
    }
  ];
</script>

<header class="bx--header">
  <button class="bx--header__action bx--header__menu-trigger bx--header__menu-toggle">
    <img
      alt="Defense Unicorns Logo"
      src="https://www.defenseunicorns.com/images/svg/doug.svg"
      style="width: 32px; height: 32px"
    />
  </button>
  <a href="#main-content" tabindex="0" class="bx--skip-to-content">Skip to main content</a>
  <a class="bx--header__name" href="#main-content">
    <span class="bx--header__name--prefix">UDS&nbsp;</span>
    <span>Platform</span>
  </a>

  <HeaderSeparator spaceLeft={0} spaceRight={20} />

  <HeaderSelect title="Services" items={leftMenuLinks} />

  <div class="bx--header__global">
    <Search size="sm" closeButtonLabelText="Hello" />

    <HeaderSeparator spaceLeft={0} spaceRight={6} />
    <HeaderGlobalAction iconDescription="Help" tooltipAlignment="start" icon={HelpFilled} />

    {#if authenticated}
      <HeaderGlobalAction iconDescription="Notification" icon={NotificationFilled} />
      <HeaderSeparator spaceLeft={0} />
      <HeaderSelect title="prod.uds.is" items={prodMenuLinks} />
      <HeaderSeparator spaceLeft={0} />
      <HeaderSelect title="username" items={lastMenuLinks} withIcon={true}>
        <div slot="account-icon">
          <UserAvatarFilled size={16} />
        </div>
      </HeaderSelect>
    {:else}
      <HeaderSeparator spaceLeft={6} spaceRight={4} />
      <Button size="small" icon={ArrowRight}>Sign in</Button>
    {/if}
  </div>
</header>

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
  :global(.bx--search) {
    display: flex !important;
    align-items: center !important;
    justify-content: center !important;
    width: 40% !important;
    left: -15% !important;
  }

  :global(.bx--search-close) {
    top: 8px !important;
  }

  @media (max-width: 1160px) {
    :global(.bx--search) {
      // width: 33% !important;
      left: -7% !important;
    }
  }

  $side-nav__header-text: #aaa;

  :global(.bx--content) {
    max-width: 924px;
  }

  :global(.bx--side-nav) {
    background-color: var(--cds-inverse-01) !important;
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
