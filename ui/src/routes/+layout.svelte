<script lang="ts">
  import { Content, Theme } from 'carbon-components-svelte';

  import { type CarbonTheme } from 'carbon-components-svelte/src/Theme/Theme.svelte';

  import Header from '$lib/components/Header/Header.svelte';

  import 'carbon-components-svelte/css/all.css';

  $: innerWidth = 0;
  let theme: CarbonTheme = 'g100';
</script>

<Theme bind:theme />
<svelte:window bind:innerWidth />

<Header authenticated={true} />

<Content>
  <slot />
</Content>

<style lang="scss">
  :global(.bx--content) {
    max-width: 924px;
    margin-left: auto;
    margin-right: auto;
  }
</style>
