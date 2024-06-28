<script lang="ts">
  import { page } from '$app/stores';
  import { isSidebarExpanded } from '$lib/stores/layout';
  import {
    AdjustmentsVerticalSolid,
    AngleUpOutline,
    CogSolid,
    FileLinesSolid,
    QuestionCircleSolid
  } from 'flowbite-svelte-icons';
  import { routes } from './routes';
  import './Sidebar.postcss';

  let { pathname } = $page.url;

  const submenus: Record<string, boolean> = {};

  routes.forEach((route) => {
    submenus[route.path] = pathname.includes(route.path);
  });
</script>

<aside
  id="main-sidebar"
  class="fixed left-0 top-14 z-40 h-screen -translate-x-full transition-all duration-300 ease-in-out sm:translate-x-0 {$isSidebarExpanded
    ? 'w-64'
    : 'w-16'}"
  aria-label="Sidenav"
>
  <div
    class="h-full overflow-y-auto border-r border-gray-200 bg-white px-3 py-5 dark:border-gray-700 dark:bg-gray-800"
  >
    <ul class="space-y-2">
      {#each routes as route}
        <li>
          {#if route.children}
            <button
              type="button"
              class="group flex w-full items-center rounded-lg p-2 text-base font-normal text-gray-900 transition duration-300 hover:bg-gray-100 dark:text-white dark:hover:bg-gray-700"
              on:click={() => (submenus[route.path] = !submenus[route.path])}
            >
              <svelte:component this={route.icon} class="icon" />
              <span class="expanded-only ml-3 flex-1 whitespace-nowrap text-left">{route.name}</span
              >
              <AngleUpOutline
                class="expanded-only h-6 w-6 transition duration-300 {submenus[route.path]
                  ? 'rotate-180 transform'
                  : ''}"
              />
            </button>
            <ul class="expanded-only space-y-2 py-2 {submenus[route.path] ? '' : 'hidden'}">
              {#each route.children as child}
                <li>
                  <a
                    href={child.path}
                    class="group flex w-full items-center rounded-lg p-2 pl-11 text-base font-normal text-gray-900 transition duration-300 hover:bg-gray-100 dark:text-white dark:hover:bg-gray-700"
                    >{child.name}</a
                  >
                </li>
              {/each}
            </ul>
          {:else}
            <a
              href={route.path}
              class="group flex items-center rounded-lg p-2 text-base font-normal text-gray-900 hover:bg-gray-100 dark:text-white dark:hover:bg-gray-700"
            >
              <svelte:component this={route.icon} class="icon" />
              <span class="expanded-only ml-3">{route.name}</span>
            </a>
          {/if}
        </li>
      {/each}
    </ul>
    <ul class="mt-5 space-y-2 border-t border-gray-200 pt-5 dark:border-gray-700">
      <li>
        <a
          href="/docs"
          class="group flex items-center rounded-lg p-2 text-base font-normal text-gray-900 transition duration-300 hover:bg-gray-100 dark:text-white dark:hover:bg-gray-700"
        >
          <FileLinesSolid class="icon" />
          <span class="expanded-only ml-3">Docs</span>
        </a>
      </li>
      <li>
        <a
          href="/help"
          class="group flex items-center rounded-lg p-2 text-base font-normal text-gray-900 transition duration-300 hover:bg-gray-100 dark:text-white dark:hover:bg-gray-700"
        >
          <QuestionCircleSolid class="icon" />
          <span class="expanded-only ml-3">Help</span>
        </a>
      </li>
    </ul>
  </div>
  <div
    class="absolute bottom-16 left-0 z-20 flex hidden w-full justify-center border-r border-gray-200 bg-white p-4 lg:flex dark:border-gray-700 dark:bg-gray-800 {$isSidebarExpanded
      ? ''
      : 'flex-col'}"
  >
    <a
      href="/preferences"
      class="inline-flex cursor-pointer justify-center rounded p-2 text-gray-500 hover:bg-gray-100 hover:text-gray-900 dark:text-gray-400 dark:hover:bg-gray-600 dark:hover:text-white"
    >
      <AdjustmentsVerticalSolid class="h-6 w-6" />
    </a>
    <a
      href="/settings"
      class="inline-flex cursor-pointer justify-center rounded p-2 text-gray-500 hover:bg-gray-100 hover:text-gray-900 dark:text-gray-400 dark:hover:bg-gray-600 dark:hover:text-white"
    >
      <CogSolid class="h-6 w-6" />
    </a>
  </div>
</aside>
