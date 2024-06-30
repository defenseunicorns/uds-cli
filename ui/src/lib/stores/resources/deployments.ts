import type { V1Deployment } from '@kubernetes/client-node';
import {
  ResourceStore,
  type ColumnWrapper,
  type ResourceStoreInterface,
  type ResourceWithTable
} from './common';

interface Row {
  name: string;
  namespace: string;
  ready: string;
  up_to_date: number;
  available: number;
  age: string;
}

export type Columns = ColumnWrapper<Row>;

/**
 * Create a new DeploymentStore for streaming deployment resources
 *
 * @returns A new DeploymentStore instance
 */
export function createStore(): ResourceStoreInterface<V1Deployment, Row> {
  const store = new ResourceStore<V1Deployment, Row>('name');

  const start = () =>
    store.start(
      `http://localhost:8080/api/v1/resources/deployments`,
      (resources) =>
        resources.map((r) => ({
          resource: r,
          table: {
            name: r.metadata?.name ?? '',
            namespace: r.metadata?.namespace ?? '',
            ready: `${r.status?.readyReplicas ?? 0} / ${r.status?.replicas ?? 0}`,
            up_to_date: r.status?.updatedReplicas ?? 0,
            available: r.status?.conditions?.filter((c) => c.type === 'Available').length ?? 0,
            age: r.metadata?.creationTimestamp ?? ''
          }
        })) as ResourceWithTable<V1Deployment, Row>[]
    );

  return {
    ...store,
    start,
    sortByKey: store.sortByKey.bind(store)
  };
}
