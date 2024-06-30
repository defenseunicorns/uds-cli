import type { V1Namespace } from '@kubernetes/client-node';
import {
  ResourceStore,
  type ColumnWrapper,
  type ResourceStoreInterface,
  type ResourceWithTable
} from './common';

interface Row {
  name: string;
  status: string;
  age: string;
}

export type Columns = ColumnWrapper<Row>;

/**
 * Create a new NamespaceStore for streaming namespaces
 *
 * @returns A new NamespaceStore instance
 */
export function createStore(): ResourceStoreInterface<V1Namespace, Row> {
  const store = new ResourceStore<V1Namespace, Row>('name');

  const start = () =>
    store.start(
      `http://localhost:8080/api/v1/resources/namespaces`,
      (resources) =>
        resources.map((r) => ({
          resource: r,
          table: {
            name: r.metadata?.name ?? '',
            status: r.status?.phase ?? '',
            age: r.metadata?.creationTimestamp ?? ''
          }
        })) as ResourceWithTable<V1Namespace, Row>[]
    );

  return {
    ...store,
    start,
    sortByKey: store.sortByKey.bind(store)
  };
}
