<script lang="ts">
  import { onMount } from 'svelte';
  import AnsiDisplay from '$lib/components/AnsiDisplay.svelte';
  import { Breadcrumb, BreadcrumbItem } from 'carbon-components-svelte';

  let addMessage: (message: string) => void;

  onMount(() => {
    const eventSource = new EventSource('http://localhost:8080/api/v1/policies');

    eventSource.onmessage = (e) => {
      addMessage(e.data);
    };

    eventSource.onerror = (error) => {
      console.error('EventSource failed:', error);
    };
    return () => {
      eventSource.close();
      console.log('Connection to server closed.');
    };
  });
</script>

<Breadcrumb noTrailingSlash>
  <BreadcrumbItem>Logs</BreadcrumbItem>
  <BreadcrumbItem>Policy Enforcement</BreadcrumbItem>
</Breadcrumb>
<div class="stream">
  <AnsiDisplay bind:addMessage />
</div>

<style>
  .stream {
    margin-top: 2rem;
    white-space: pre-wrap;
    font-family: monospace;
    width: 100%;
    height: 90%;
    background-color: #001a30;
  }
</style>
