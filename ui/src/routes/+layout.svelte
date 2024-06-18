<script lang="ts">
  $: innerWidth = 0;
</script>

<svelte:window bind:innerWidth />

<slot />
