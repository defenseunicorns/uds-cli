<script lang="ts">
  import Datatable from '$lib/components/data/Datatable.svelte';
  import { createStore, type Columns } from '$lib/stores/resources/namespaces';

  export let columns: Columns = [['name', 'emphasize'], ['status'], ['age']];
</script>

<Datatable {columns} {createStore} />
