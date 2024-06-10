import { FakePodRepository } from '../../../tests/fakerepo';

const repo = new FakePodRepository();

export async function load() {
  return { repo };
}
