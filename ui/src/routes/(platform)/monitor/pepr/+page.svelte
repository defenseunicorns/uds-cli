<script lang="ts">
  import { onMount } from 'svelte';
  import AnsiDisplay from '$lib/components/AnsiDisplay.svelte';

  export let data;

  let addMessage: (message: string) => void;

  onMount(() => {
    const policyLogsRepo = new data.policyLogsRepo('http://localhost:8080/api/v1/policies');
    policyLogsRepo.onMessageHandler((message: string) => {
      addMessage(message);
    });
    policyLogsRepo.onErrorHandler((error: Event) => {
      console.error('EventSource failed:', error);
    });

    return () => {
      policyLogsRepo.close();
    };
  });
</script>

<div class="stream">
  <AnsiDisplay bind:addMessage />
</div>

<style>
  .stream {
    margin-top: var(--cds-spacing-07);
    white-space: pre-wrap;
    font-family: monospace;
    width: calc(100% - 18em);
    height: 100%;
    background-color: #001a30;
    margin-left: 18em;
  }
</style>
