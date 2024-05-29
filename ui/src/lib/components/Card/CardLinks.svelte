<script lang="ts">
  import { Link } from 'carbon-components-svelte';

  import { type LinkType } from './types';

  export let links: LinkType[];
</script>

<slot name="card-links">
  <section class="card__links">
    {#each links as link}
      <Link style="margin-bottom: var(--cds-spacing-02)" href={link.url} size="sm" inline>
        {link.name}
      </Link>
    {/each}
  </section>
</slot>

<style lang="scss">
  .card__links {
    display: flex;
    flex-direction: column;
  }
</style>
