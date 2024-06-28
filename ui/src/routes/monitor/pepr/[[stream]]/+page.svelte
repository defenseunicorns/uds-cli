<script lang="ts">
  import { UploadSolid } from 'flowbite-svelte-icons';
  import { writable } from 'svelte/store';

  import { afterNavigate, goto } from '$app/navigation';
  import { page } from '$app/stores';
  import './page.postcss';

  interface PeprEvent {
    _name: string;
    count: number;
    event: string;
    header: string;
    repeated?: number;
    ts?: string;
    epoch: number;
  }

  let streamFilter = '';

  const peprStream = writable<PeprEvent[]>([]);

  afterNavigate(() => {
    // Flush the peprStream on navigation
    peprStream.set([]);
    streamFilter = $page.params.stream || '';

    const eventSource = new EventSource(
      `http://localhost:8080/api/v1/monitor/pepr/${streamFilter}`
    );

    eventSource.onmessage = (e) => {
      try {
        const payload: PeprEvent = JSON.parse(e.data);

        // The event type is the first word in the header
        payload.event = payload.header.split(' ')[0];

        // If this is a repeated event, update the count
        if (payload.repeated) {
          // Find the first item in the peprStream that matches the header
          peprStream.update((collection) => {
            const idx = collection.findIndex((item) => item.header === payload.header);
            if (idx !== -1) {
              collection[idx].count = payload.repeated!;
              collection[idx].ts = payload.ts;
            }
            return collection;
          });
        } else {
          // Otherwise, add the new event to the peprStream
          peprStream.update((collection) => [payload, ...collection]);
        }
      } catch (error) {
        console.error('Error updating peprStream:', error);
      }
    };

    eventSource.onerror = (error) => {
      console.error('EventSource failed:', error);
    };
  });
</script>

<section class="table-section">
  <div class="table-container">
    <div class="table-content">
      <div class="table-filter-section">
        <div class="grid w-full grid-cols-1 md:grid-cols-4 md:gap-4 lg:w-2/3">
          <div class="w-full">
            <label for="stream" class="sr-only">Filter</label>
            <select
              id="stream"
              bind:value={streamFilter}
              on:change={(val) => {
                goto(`/monitor/pepr/${val.target.value}`);
              }}
            >
              <option value="">All Data</option>
              <hr />
              <option value="policies">UDS Policies</option>
              <option value="allowed">UDS Policies: Allowed</option>
              <option value="denied">UDS Policies: Denied</option>
              <option value="mutated">UDS Policies: Mutated</option>
              <hr />
              <option value="operator">UDS Operator</option>
              <option value="failed">Errors and Denials</option>
            </select>
          </div>
        </div>
        <div
          class="flex flex-shrink-0 flex-col space-y-3 md:flex-row md:items-center md:space-x-3 md:space-y-0 lg:justify-end"
        >
          <button type="button">
            <UploadSolid class="mr-2" />
            Export
          </button>
        </div>
      </div>
      <div class="table-scroll-container">
        <table>
          <thead>
            <tr>
              <th>Event</th>
              <th>Resource</th>
              <th>Count</th>
              <th>Timestamp</th>
            </tr>
          </thead>
          <tbody>
            {#each $peprStream as item}
              <tr>
                <td>
                  <span class="pepr-event {item.event}">{item.event}</span>
                </td>
                <td>{item._name}</td>
                <td>{item.count || 1}</td>
                <td>{item.ts}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    </div>
  </div>
</section>
