import type{Pod} from 'kubernetes-types/core/v1'
import type {PodRepository} from "$lib/repos/repository";

export const testData: Pod[] = [
    {
        apiVersion: "v1",
        kind: "Pod",
        metadata: {
            name: "pod1",
            namespace: "default"
        }
    },
    {
        apiVersion: "v1",
        kind: "Pod",
        metadata: {
            name: "pod2",
            namespace: "default"
        }
    },
]

export class FakePodRepository implements PodRepository{
    getPods(): Promise<Pod[]> {
        return Promise.resolve(testData)
    }
}
