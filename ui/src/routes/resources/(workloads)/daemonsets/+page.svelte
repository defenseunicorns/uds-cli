<script lang="ts">
  import Datatable from '$lib/components/data/Datatable.svelte';
  import { createStore, type Columns } from '$lib/stores/resources/daemonsets';

  const columns: Columns = [
    ['name', 'emphasize'],
    ['namespace'],
    ['desired'],
    ['current'],
    ['ready'],
    ['up_to_date'],
    ['available'],
    ['node_selector'],
    ['age']
  ];
</script>

<Datatable {columns} {createStore} />
