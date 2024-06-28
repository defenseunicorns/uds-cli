<script lang="ts">
  import { page } from '$app/stores';
  import { AngleRightOutline, ChartPieSolid } from 'flowbite-svelte-icons';
  import { routes } from './routes';

  const flatRoutes = routes.flatMap((route) => {
    if (route.children) {
      return [route, ...route.children];
    }

    return route;
  });

  let matchParent: { name: string; path: string; icon?: ConstructorOfATypedSvelteComponent };
  let matchChild: { name: string; path: string };

  // Subscribe to the page store to get the current URL (regular $ doesnt seem to work here)
  page.subscribe((value) => {
    const { pathname } = value.url;

    // Find the current route
    [matchParent, matchChild] = flatRoutes.filter((route) => {
      if (pathname === '/') {
        return false;
      }

      if (route.path === '/' && pathname !== '/') {
        return false;
      }

      return pathname.includes(route.path);
    });
  });
</script>

<nav class="flex" aria-label="Breadcrumb">
  <ol class="inline-flex items-center space-x-1 md:space-x-2 rtl:space-x-reverse">
    <li class="inline-flex items-center">
      <a
        href="/"
        class="inline-flex items-center text-sm font-medium text-gray-700 hover:text-blue-600 dark:text-gray-400 dark:hover:text-white"
      >
        <ChartPieSolid class="mr-2" />
        Overview
      </a>
    </li>
    {#if matchParent}
      <li><AngleRightOutline class="w-5 h-5 text-gray-400 dark:text-gray-400" /></li>
      <li>
        <div class="flex items-center">
          <a
            href={matchParent.path}
            class="inline-flex items-center text-sm font-medium text-gray-700 hover:text-blue-600 dark:text-gray-400 dark:hover:text-white"
          >
            {matchParent?.name}
          </a>
        </div>
      </li>
    {/if}
    {#if matchChild}
      <li><AngleRightOutline class="w-5 h-5 text-gray-400 dark:text-gray-400" /></li>
      <li>
        <div class="flex items-center">
          <a
            href={matchChild.path}
            class="inline-flex items-center text-sm font-medium text-gray-700 hover:text-blue-600 dark:text-gray-400 dark:hover:text-white"
          >
            {matchChild.name}
          </a>
        </div>
      </li>
    {/if}
  </ol>
</nav>
