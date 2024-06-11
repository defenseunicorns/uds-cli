<script lang="ts">
  import { onMount } from 'svelte';
  import type { Pod } from 'kubernetes-types/core/v1';

  export let data;
  let pods: Pod[] = [];
  let loading = true;
  let error: string | null = null;

  const getPods = async () => {
    try {
      const res = await data.repo.getPods();
      pods = res as Pod[];
    } catch (err) {
      error = 'Failed to fetch pods.';
      console.error(err);
    } finally {
      loading = false;
    }
  };

  onMount(() => {
    getPods();
  });
</script>

<h1>Pods</h1>

{#if loading}
  <p>Loading...</p>
{:else if error}
  <p>{error}</p>
{:else}
  <ul>
    {#each pods as pod}
      <li>{pod.metadata.name}</li>
    {/each}
  </ul>
{/if}
