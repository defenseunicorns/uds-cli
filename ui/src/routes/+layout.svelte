<script lang="ts">
  import { Content } from 'carbon-components-svelte';
  import Header from '$lib/components/Header/Header.svelte';

  import 'carbon-components-svelte/css/all.css';

  $: innerWidth = 0;
</script>

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
