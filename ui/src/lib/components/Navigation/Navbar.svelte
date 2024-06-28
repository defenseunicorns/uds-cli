<script lang="ts">
  import doug from '$lib/assets/doug.svg';
  import { isSidebarExpanded } from '$lib/stores/layout';
  import { BellSolid, SearchOutline, SearchSolid } from 'flowbite-svelte-icons';
</script>

<div class="bg-gray-50 antialiased dark:bg-gray-900">
  <nav
    class="fixed left-0 right-0 top-0 z-50 border-b border-gray-200 bg-white px-4 py-2.5 dark:border-gray-700 dark:bg-gray-800"
  >
    <div class="flex flex-wrap items-center justify-between">
      <div class="flex items-center justify-start">
        <button
          id="toggleSidebar"
          aria-expanded="true"
          aria-controls="sidebar"
          on:click={() => isSidebarExpanded.update((v) => !v)}
          class="mr-3 hidden cursor-pointer rounded p-2 text-gray-600 hover:bg-gray-100 hover:text-gray-900 lg:inline dark:text-gray-400 dark:hover:bg-gray-700 dark:hover:text-white"
        >
          <svg
            class="h-5 w-5"
            aria-hidden="true"
            xmlns="http://www.w3.org/2000/svg"
            fill="none"
            viewBox="0 0 16 12"
          >
            <path
              stroke="currentColor"
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M1 1h14M1 6h14M1 11h7"
            />
          </svg>
        </button>
        <button
          aria-expanded="true"
          aria-controls="sidebar"
          class="mr-2 cursor-pointer rounded-lg p-2 text-gray-600 hover:bg-gray-100 hover:text-gray-900 focus:bg-gray-100 focus:ring-2 focus:ring-gray-100 lg:hidden dark:text-gray-400 dark:hover:bg-gray-700 dark:hover:text-white dark:focus:bg-gray-700 dark:focus:ring-gray-700"
        >
          <svg
            class="h-[18px] w-[18px]"
            aria-hidden="true"
            xmlns="http://www.w3.org/2000/svg"
            fill="none"
            viewBox="0 0 17 14"
            ><path
              stroke="currentColor"
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M1 1h15M1 7h15M1 13h15"
            /></svg
          >
          <span class="sr-only">Toggle sidebar</span>
        </button>
        <a href="/" class="mr-4 flex">
          <img src={doug} class="mr-3 h-8" alt="FlowBite Logo" />
          <span class="self-center whitespace-nowrap text-2xl font-semibold dark:text-white"
            >UDS Platform</span
          >
        </a>
      </div>
      <div class="flex items-center lg:order-2">
        <button
          id="toggleSidebarMobileSearch"
          type="button"
          class="rounded-lg p-2 text-gray-500 hover:bg-gray-100 hover:text-gray-900 lg:hidden dark:text-gray-400 dark:hover:bg-gray-700 dark:hover:text-white"
        >
          <span class="sr-only">Search</span>
          <!-- Search icon -->

          <SearchSolid class="h-6 w-6" />
        </button>
        <!-- Notifications -->
        <button
          type="button"
          data-dropdown-toggle="notification-dropdown"
          class="mr-1 rounded-lg p-2 text-gray-500 hover:bg-gray-100 hover:text-gray-900 focus:ring-4 focus:ring-gray-300 dark:text-gray-400 dark:hover:bg-gray-700 dark:hover:text-white dark:focus:ring-gray-600"
        >
          <span class="sr-only">View notifications</span>
          <BellSolid class="h-6 w-6" />
        </button>
        <!-- Dropdown menu -->
        <div
          class="z-50 my-4 hidden max-w-sm list-none divide-y divide-gray-100 overflow-hidden rounded bg-white text-base shadow-lg dark:divide-gray-600 dark:bg-gray-700"
          id="notification-dropdown"
        >
          <div
            class="block bg-gray-50 px-4 py-2 text-center text-base font-medium text-gray-700 dark:bg-gray-700 dark:text-gray-400"
          >
            Notifications
          </div>
        </div>
      </div>
    </div>
  </nav>
</div>
