<script lang="ts">
  import Header from '$lib/components/Header/Header.svelte';

  import 'carbon-components-svelte/css/all.css';

  $: innerWidth = 0;
</script>

<svelte:window bind:innerWidth />

<Header authenticated={true} />

<slot />

<style lang="scss">
  :global(.bx--content) {
    margin-left: auto;
    margin-right: auto;
  }

</style>
