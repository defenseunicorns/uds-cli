import type { V1Pod } from '@kubernetes/client-node';
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
  age: string;
  status: string;
}

export type Columns = ColumnWrapper<Row>;

/**
 * Create a new PodStore for streaming Pod resources
 *
 * @returns A new PodStore instance
 */
export function createStore(): ResourceStoreInterface<V1Pod, Row> {
  const store = new ResourceStore<V1Pod, Row>('name');

  const start = () =>
    store.start(
      `http://localhost:8080/api/v1/resources/pods`,
      (pods: V1Pod[]) =>
        pods.map((pod) => ({
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
        })) as ResourceWithTable<V1Pod, Row>[]
    );

  return {
    ...store,
    start,
    sortByKey: store.sortByKey.bind(store)
  };
}
