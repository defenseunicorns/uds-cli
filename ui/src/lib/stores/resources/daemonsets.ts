import type { V1DaemonSet as Resource } from '@kubernetes/client-node';
import {
  ResourceStore,
  type ColumnWrapper,
  type ResourceStoreInterface,
  type ResourceWithTable
} from './common';

interface Row {
  name: string;
  namespace: string;
  desired: number;
  current: number;
  ready: number;
  up_to_date: number;
  available: number;
  node_selector: string;
  age: Date | string;
}

export type Columns = ColumnWrapper<Row>;

/**
 * Create a new DaemonsetStore for streaming deployment resources
 *
 * @returns A new DaemonsetStore instance
 */
export function createStore(): ResourceStoreInterface<Resource, Row> {
  const url = `http://localhost:8080/api/v1/resources/daemonsets`;

  const transform = (resources: Resource[]) =>
    resources.map<ResourceWithTable<Resource, Row>>((r) => ({
      resource: r,
      table: {
        name: r.metadata?.name ?? '',
        namespace: r.metadata?.namespace ?? '',
        desired: r.status?.desiredNumberScheduled ?? 0,
        current: r.status?.currentNumberScheduled ?? 0,
        ready: r.status?.numberReady ?? 0,
        up_to_date: r.status?.updatedNumberScheduled ?? 0,
        available: r.status?.conditions?.filter((c) => c.type === 'Available').length ?? 0,
        node_selector: r.spec?.template.spec?.nodeSelector
          ? Object.entries(r.spec?.template.spec?.nodeSelector ?? {})
              .map(([key, value]) => `${key}: ${value}`)
              .join(', ')
          : '-',
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
