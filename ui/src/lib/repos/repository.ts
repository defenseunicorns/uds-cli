import type{Pod} from 'kubernetes-types/core/v1';


export interface PodRepository{
    getPods(): Promise<Pod[]>;
}
