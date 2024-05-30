<script lang="ts">
  import { Button, HeaderGlobalAction, Search } from 'carbon-components-svelte';

  import ArrowRight from 'carbon-icons-svelte/lib/ArrowRight.svelte';
  import HelpFilled from 'carbon-icons-svelte/lib/HelpFilled.svelte';
  import NotificationFilled from 'carbon-icons-svelte/lib/NotificationFilled.svelte';
  import UserAvatarFilled from 'carbon-icons-svelte/lib/UserAvatarFilled.svelte';

  import HeaderSelect from '$lib/components/Header/HeaderSelect.svelte';
  import HeaderSeparator from '$lib/components/Header/HeaderSeparator.svelte';

  import { type HeaderSelectProps } from './types';

  let ref = null;
  let active = true;
  let value = '';
  let selectedResultIndex = 0;

  $: console.log('ref', ref);
  $: console.log('active', active);
  $: console.log('value', value);
  $: console.log('selectedResultIndex', selectedResultIndex);

  export let authenticated: boolean;

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

  <div class="bx--header__global">
    <HeaderGlobalAction iconDescription="Help" tooltipAlignment="start" icon={HelpFilled} />

    {#if authenticated}
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
