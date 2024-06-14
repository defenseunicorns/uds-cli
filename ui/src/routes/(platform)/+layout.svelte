<script lang="ts">
  import {Content, SideNavDivider, SideNavMenu} from 'carbon-components-svelte';

  import {
    Button,
    HeaderGlobalAction,
    SideNavMenuItem,
    SideNav,
    SideNavItems,
    Search
  } from 'carbon-components-svelte';

  import ArrowRight from 'carbon-icons-svelte/lib/ArrowRight.svelte';
  import HelpFilled from 'carbon-icons-svelte/lib/HelpFilled.svelte';
  import NotificationFilled from 'carbon-icons-svelte/lib/NotificationFilled.svelte';
  import UserAvatarFilled from 'carbon-icons-svelte/lib/UserAvatarFilled.svelte';

  import HeaderSelect from '$lib/components/Header/HeaderSelect.svelte';
  import HeaderSeparator from '$lib/components/Header/HeaderSeparator.svelte';

  import { type HeaderSelectProps } from '$lib/components/Header/types';

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

<SideNav isOpen>
  <SideNavItems>
    <SideNavMenu text="Dashboard" />

    <SideNavDivider />

    <SideNavMenu expanded text="Cluster">
      <SideNavMenuItem text="Pods" href="#" />
    </SideNavMenu>

    <SideNavDivider />

    <SideNavMenu expanded text="Deployment">
      <SideNavMenuItem text="Bundles" href="#" />
      <SideNavMenuItem text="Packages" href="#" />
      <SideNavMenuItem text="Configuration" href="#" />
    </SideNavMenu>

    <SideNavDivider />

    <SideNavMenu expanded text="Logs">
      <SideNavMenuItem text="Policy Enforcement" href="/logs/policies" isSelected />
    </SideNavMenu>

    <SideNavDivider />

    <SideNavMenu expanded text="Security">
      <SideNavMenuItem text="Vulnerabilities" href="#" />
      <SideNavMenuItem text="Compliance" href="#" />
    </SideNavMenu>
  </SideNavItems>
</SideNav>

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

  :global(.bx--side-nav) {
    background-color: var(--cds-inverse-01) !important;
    transition: none
  }

  /* Sidenav header color */
  :global(.bx--side-nav__submenu-title) {
    color: $side-nav__header-text !important;
  }

  /* Sidenav link color */
  :global(.bx--side-nav__link > .bx--side-nav__link-text) {
    color: $side-nav__header-text !important;
  }

  :global(.bx--side-nav__divider) {
    background-color: var(--cds-ui-01) !important;
  }

  /* Link item hover color */
  :global(.bx--side-nav__link):hover {
    background-color: var(--cds-ui-01) !important;
  }

  :global(.bx--side-nav__submenu) {
    cursor: default !important;
  }

  /* Remove hover highlight on hover for the Sidenav header */
  :global(.bx--side-nav__submenu:hover) {
    background-color: transparent !important;
  }

  /* Sidenav active/ selected link background color */
  :global(a.bx--side-nav__link[aria-current='page']) {
    background-color: var(--cds-ui-01) !important;
  }

  :global(a.bx--side-nav__link[aria-current='page']:focus) {
    border: none !important;
  }

  /* Removes outline/ border around a sidenav header that is clicked/ focused */
  :global(.bx--side-nav__submenu:focus) {
    outline: none !important;
  }

  :global(a.bx--side-nav__link[aria-current='page'] > .bx--side-nav__link-text) {
    color: var(--cds-interactive-03) !important;
  }

  :global(.bx--side-nav__icon) {
    display: none !important;
  }
</style>
