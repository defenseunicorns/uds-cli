import type { V1Pod as Resource } from '@kubernetes/client-node';
import {
  ResourceStore,
  type ColumnWrapper,
  type ResourceStoreInterface,
  type ResourceWithTable
} from './common';

interface Row {
  name: string;
  namespace: string;
  containers: number;
  restarts: number;
  controller: string;
  node: string;
  age: Date | string;
  status: string;
}

export type Columns = ColumnWrapper<Row>;

/**
 * Create a new PodStore for streaming Pod resources
 *
 * @returns A new PodStore instance
 */
export function createStore(): ResourceStoreInterface<Resource, Row> {
  const url = `http://localhost:8080/api/v1/resources/pods`;

  const transform = (resources: Resource[]) =>
    resources.map<ResourceWithTable<Resource, Row>>((pod) => ({
      resource: pod,
      table: {
        name: pod.metadata?.name ?? '',
        namespace: pod.metadata?.namespace ?? '',
        containers: pod.spec?.containers.length ?? 0,
        restarts:
          pod.status?.containerStatuses?.reduce((acc, curr) => acc + curr.restartCount, 0) ?? 0,
        controller: pod.metadata?.ownerReferences?.at(0)?.kind ?? '',
        node: pod.spec?.nodeName ?? '',
        age: pod.metadata?.creationTimestamp ?? '',
        status: pod.status?.phase ?? ''
      }
    }));

  const store = new ResourceStore<Resource, Row>('name');

  return {
    ...store,
    start: () => store.start(url, transform),
    sortByKey: store.sortByKey.bind(store)
  };
}
