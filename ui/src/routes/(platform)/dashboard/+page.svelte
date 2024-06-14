<script lang="ts">
  import { Breadcrumb, BreadcrumbItem } from 'carbon-components-svelte';
</script>

<Breadcrumb noTrailingSlash>
  <BreadcrumbItem href="/Dashboard" isCurrentPage={true}>Dashboard</BreadcrumbItem>
</Breadcrumb>

<div style:padding-top="var(--cds-spacing-07)">
  <h1>Dashboard</h1>
</div>
