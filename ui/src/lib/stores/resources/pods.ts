import type { V1Pod } from '@kubernetes/client-node';
import { ResourceStore, SearchByType, type ResourceWithTable, type TableRow } from './common';

interface PodTable extends TableRow {
  name: string;
  namespace: string;
  containers: number;
  restarts: number;
  controller: string;
  node: string;
  age: string;
  status: string;
}

/**
 * Create a new PodStore for streaming Pod resources
 *
 * @returns A new PodStore instance
 */
export function createPodStore() {
  const store = new ResourceStore<V1Pod, PodTable>('name');

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
        })) as ResourceWithTable<V1Pod, PodTable>[]
    );

  return {
    ...store,
    start,
    sortByKey: store.sortByKey.bind(store)
  };
}

export type PodStore = ReturnType<typeof createPodStore>;
export type PodResourceWithTable = ResourceWithTable<V1Pod, PodTable>;
export { SearchByType };
