<script>
    import {onDestroy, onMount} from 'svelte';
    import {writable} from 'svelte/store';

    const streamData = writable('');

    onMount(() => {
        const eventSource = new EventSource('http://localhost:8080/api/v1/policies');

        eventSource.onopen = () => {
            console.log('Connection to server opened.');
        };

        eventSource.onmessage = (event) => {
            streamData.update(current => current + event.data + '\n');
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

<style>
    .stream {
        white-space: pre-wrap;
        font-family: monospace;
    }
</style>

<h1>Policies</h1>
<div class="stream">
    <pre>{$streamData}</pre>
</div>
