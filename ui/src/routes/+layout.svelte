<script lang="ts">
  import { afterNavigate } from '$app/navigation';
  import 'flowbite';
  import { initFlowbite } from 'flowbite';
  import 'flowbite/dist/flowbite.css';
  import { onMount } from 'svelte';

  import Breadcrumb from '$lib/components/Navigation/Breadcrumb.svelte';
  import Navbar from '$lib/components/Navigation/Navbar.svelte';
  import Sidebar from '$lib/components/Navigation/Sidebar.svelte';
  import { isSidebarExpanded } from '$lib/stores/layout';
  import '../app.postcss';

  onMount(initFlowbite);
  afterNavigate(initFlowbite);
</script>

<Navbar />

<Sidebar />

<main
  class="flex h-screen flex-col pt-16 transition-all duration-300 ease-in-out {$isSidebarExpanded
    ? 'md:ml-64'
    : 'md:ml-16'}"
>
  <div class="p-5">
    <Breadcrumb />
  </div>
  <div class="flex-grow overflow-hidden p-4 pt-0">
    <slot />
  </div>
</main>
