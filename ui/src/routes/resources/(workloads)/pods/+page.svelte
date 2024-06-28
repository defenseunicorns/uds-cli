<script lang="ts">
  import {
    AngleDownOutline,
    AngleUpOutline,
    FilterSolid,
    SearchOutline
  } from 'flowbite-svelte-icons';

  import { createPodStore, SearchByType } from '$lib/stores/resources/pods';
  import { onMount } from 'svelte';

  const pods = createPodStore();
  const { search, searchBy, sortAsc, sortBy } = pods;

  onMount(() => {
    return pods.start();
  });

  const headers = [
    'name',
    'namespace',
    'containers',
    'restarts',
    'controller',
    'node',
    'age',
    'status'
  ];

  const searchTypes = Object.values(SearchByType);
</script>

<section class="table-section">
  <div class="table-container">
    <div class="table-content">
      <div class="table-filter-section">
        <div class="relative lg:w-96">
          <div class="pointer-events-none absolute inset-y-0 left-0 flex items-center pl-3">
            <SearchOutline class="h-5 w-5 text-gray-400" />
          </div>
          <input
            type="text"
            name="email"
            class="focus:ring-primary-500 focus:border-primary-500 dark:focus:ring-primary-500 dark:focus:border-primary-500 block w-full rounded-lg border border-gray-300 bg-gray-50 p-2.5 pl-9 text-gray-900 sm:text-sm dark:border-gray-600 dark:bg-gray-700 dark:text-white dark:placeholder-gray-400"
            placeholder="Search"
            bind:value={$search}
          />
        </div>
        <button
          id="filterDropdownButton"
          data-dropdown-toggle="filterDropdown"
          class="hover:text-primary-700 flex items-center justify-center rounded-lg border border-gray-200 bg-white px-4 py-2 text-sm font-medium text-gray-900 hover:bg-gray-100 focus:z-10 focus:outline-none focus:ring-4 focus:ring-gray-200 md:w-auto dark:border-gray-600 dark:bg-gray-800 dark:text-gray-400 dark:hover:bg-gray-700 dark:hover:text-white dark:focus:ring-gray-700"
          type="button"
        >
          <FilterSolid class="mr-2 h-4 w-4 text-gray-400" />
          {$searchBy}
          <AngleDownOutline class="ml-2 h-4 w-4 text-gray-400" />
        </button>
        <div
          id="filterDropdown"
          class="z-10 hidden w-48 rounded-lg bg-white p-3 shadow dark:bg-gray-700"
        >
          <h6 class="mb-3 text-sm font-medium text-gray-900 dark:text-white">Search By</h6>
          <ul class="space-y-2 text-sm" aria-labelledby="filterDropdownButton">
            {#each searchTypes as searchType}
              <li class="flex items-center">
                <input
                  id={searchType}
                  type="radio"
                  name="searchType"
                  value={searchType}
                  class="h-4 w-4 border-gray-300 focus:ring-2 focus:ring-blue-300 dark:border-gray-600 dark:bg-gray-700 dark:focus:bg-blue-600 dark:focus:ring-blue-600"
                  bind:group={$searchBy}
                />
                <label
                  for={searchType}
                  class="ms-2 block text-sm font-medium text-gray-900 dark:text-gray-300"
                >
                  {searchType}
                </label>
              </li>
            {/each}
          </ul>
        </div>
        <div class="flex-grow"></div>
      </div>
      <div class="table-scroll-container">
        <table>
          <thead>
            <tr>
              {#each headers as header}
                <th>
                  <button on:click={() => pods.sortByKey(header)}>
                    {header}
                    <AngleUpOutline
                      class="sort 
                              {$sortAsc ? 'rotate-180' : ''}
                              {$sortBy === header ? 'opacity-100' : 'opacity-0'}"
                    />
                  </button>
                </th>
              {/each}
            </tr>
          </thead>
          <tbody>
            {#each $pods as pod}
              <tr>
                <td class="emphasize">{pod.table.name}</td>
                <td>{pod.table.namespace}</td>
                <td>{pod.table.containers}</td>
                <td>{pod.table.restarts}</td>
                <td>{pod.table.controller}</td>
                <td>{pod.table.node}</td>
                <td>{pod.table.age}</td>
                <td>{pod.table.status}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    </div>
  </div>
</section>
