import type { KubernetesObject } from '@kubernetes/client-node';
import { derived, writable, type Writable } from 'svelte/store';

export type ColumnWrapper<T> = [name: keyof T, styles?: string][];

export interface ResourceWithTable<T extends KubernetesObject, U> {
  resource: T;
  table: U;
}

enum SearchByType {
  ANYWHERE = 'Anywhere',
  METADATA = 'Metadata',
  NAME = 'Name'
}

export interface ResourceStoreInterface<T extends KubernetesObject, U> {
  // Start the EventSource and update the resources
  start: () => () => void;
  // Sort the table by the key
  sortByKey: (key: keyof U) => void;
  // Store for search text
  search: Writable<string>;
  // Store for search by type
  searchBy: Writable<SearchByType>;
  // Store for sortBy key
  sortBy: Writable<keyof U>;
  // Store for sort direction
  sortAsc: Writable<boolean>;
  // The list of search types
  searchTypes: SearchByType[];
  // Subscribe to the filtered and sorted resources
  subscribe: (run: (value: ResourceWithTable<T, U>[]) => void) => () => void;
}

export class ResourceStore<T extends KubernetesObject, U> {
  // Keep an internal store for the resources
  private resources: Writable<ResourceWithTable<T, U>[]>;

  // Keep an internal reference to the EventSource and the table
  private eventSource: EventSource | null = null;
  private table: ResourceWithTable<T, U>[] = [];

  // Public stores for the search text, search by type, and sorting options
  public search: Writable<string>;
  public searchBy: Writable<SearchByType>;
  public sortBy: Writable<keyof U>;
  public sortAsc: Writable<boolean>;

  // The list of search types
  public searchTypes = Object.values(SearchByType);
  /**
   * Create a new ResourceStore instance
   *
   * @param initialSortBy The initial key to sort the table by
   */
  constructor(initialSortBy: keyof U) {
    // Initialize the internal store
    this.resources = writable<ResourceWithTable<T, U>[]>([]);

    // Initialize the public stores
    this.search = writable<string>('');
    this.searchBy = writable<SearchByType>(SearchByType.ANYWHERE);
    this.sortBy = writable<keyof U>(initialSortBy);
    this.sortAsc = writable<boolean>(true);

    // Create a derived store that combines all the filtering and sorting logic
    const filteredAndSortedResources = derived(
      [this.resources, this.search, this.searchBy, this.sortBy, this.sortAsc],
      ([$resources, $search, $searchBy, $sortBy, $sortAsc]) => {
        let filtered = $resources;

        // If there is a search term, filter the resources
        if ($search) {
          filtered = filtered.filter((item) => {
            let searchContents = '';

            // Determine what to search by
            switch ($searchBy) {
              case SearchByType.METADATA:
                searchContents = JSON.stringify(item.resource.metadata);
                break;
              case SearchByType.NAME:
                searchContents = item.resource.metadata?.name ?? '';
                break;
              // Default to anywhere (the entire resource)
              default:
                searchContents = JSON.stringify(item);
            }

            // Perform a case-insensitive search
            return searchContents.toLowerCase().includes($search.toLowerCase());
          });
        }

        // Sort the resources by the sortBy key
        return filtered.sort((a, b) => {
          const valueA = a.table[$sortBy];
          const valueB = b.table[$sortBy];
          if (valueA < valueB) return $sortAsc ? -1 : 1;
          if (valueA > valueB) return $sortAsc ? 1 : -1;
          return 0;
        });
      }
    );

    // Replace the subscribe method to use the derived store
    this.subscribe = filteredAndSortedResources.subscribe;
  }

  /**
   * Update the searchBy key
   *
   * @param key The key to search by
   */
  sortByKey(key: keyof U) {
    this.sortBy.update((currentSortBy) => {
      // If the key is the same as the current sortBy key, toggle the sort direction
      if (key === currentSortBy) {
        this.sortAsc.update((asc) => !asc);
        return currentSortBy;
      }

      // Otherwise, update the sortBy key
      return key;
    });
  }

  /**
   * Start the EventSource and update the resources
   *
   * @param url The URL to the EventSource
   * @param createTableCallback The callback to create the table from the resources
   *
   * @returns A function to stop the EventSource
   */
  start(url: string, createTableCallback: (data: T[]) => ResourceWithTable<T, U>[]) {
    this.eventSource = new EventSource(url);

    this.eventSource.onmessage = ({ data }) => {
      try {
        this.table = createTableCallback(JSON.parse(data));
        this.resources.set(this.table);
      } catch (err) {
        console.error('Error updating resources:', err);
      }
    };

    this.eventSource.onerror = (err) => {
      console.error('EventSource failed:', err);
    };

    return () => this.stop();
  }

  stop() {
    if (this.eventSource) {
      this.eventSource.close();
      this.eventSource = null;
    }
  }

  subscribe: (run: (value: ResourceWithTable<T, U>[]) => void) => () => void;
}

// Factory function to create a ResourceStore instance
export function createResourceStore<T extends KubernetesObject, U>(initialSortBy: keyof U) {
  return new ResourceStore<T, U>(initialSortBy);
}
