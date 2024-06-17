<script lang="ts">
    import {onMount} from 'svelte';
    import AnsiDisplay from '$lib/components/AnsiDisplay.svelte';
    import {Breadcrumb, BreadcrumbItem} from 'carbon-components-svelte';
    import {PolicyLogsRepo} from "$lib/repos/PolicyLogsRepo";

    export let data

    let addMessage: (message: string) => void;

    onMount(() => {
        const policyLogsRepo = data.policyLogsRepo;
        // const policyLogsRepo = new PolicyLogsRepo('http://localhost:8080/api/v1/policies')
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

<Breadcrumb noTrailingSlash>
    <BreadcrumbItem>Logs</BreadcrumbItem>
    <BreadcrumbItem>Policy Enforcement</BreadcrumbItem>
</Breadcrumb>

<div class="stream">
    <AnsiDisplay bind:addMessage/>
</div>

<style>
    .stream {
        margin-top: var(--cds-spacing-07);
        white-space: pre-wrap;
        font-family: monospace;
        width: 100%;
        height: 100%;
        background-color: #001a30;
    }
</style>
