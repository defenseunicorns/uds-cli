import type { V1Namespace as Resource } from '@kubernetes/client-node';
import {
  ResourceStore,
  type ColumnWrapper,
  type ResourceStoreInterface,
  type ResourceWithTable
} from './common';

interface Row {
  name: string;
  status: string;
  age: Date | string;
}

export type Columns = ColumnWrapper<Row>;

/**
 * Create a new NamespaceStore for streaming namespaces
 *
 * @returns A new NamespaceStore instance
 */
export function createStore(): ResourceStoreInterface<Resource, Row> {
  const url = `http://localhost:8080/api/v1/resources/namespaces`;

  const transform = (resources: Resource[]) =>
    resources.map<ResourceWithTable<Resource, Row>>((r) => ({
      resource: r,
      table: {
        name: r.metadata?.name ?? '',
        status: r.status?.phase ?? '',
        age: r.metadata?.creationTimestamp ?? ''
      }
    }));

  const store = new ResourceStore<Resource, Row>('name');

  return {
    ...store,
    start: () => store.start(url, transform),
    sortByKey: store.sortByKey.bind(store)
  };
}
