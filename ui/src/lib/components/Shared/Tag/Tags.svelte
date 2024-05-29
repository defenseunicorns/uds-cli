<script lang="ts">
  import { type TagType } from './types';
  import { Tag } from 'carbon-components-svelte';

  export let tags: TagType[];
</script>

<slot name="tags">
  <section class="tags">
    {#each tags as tag}
      <Tag size="sm" type={tag.type}>{tag.name}</Tag>
    {/each}
  </section>
</slot>
