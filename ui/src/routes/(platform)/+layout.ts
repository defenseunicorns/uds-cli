import { FakePodRepository } from '../../../tests/fakerepos/FakePodRepo';

const repo = new FakePodRepository();

export async function load() {
  return { repo };
}
