import { FakePolicyLogsRepo } from '../../../tests/fakerepos/FakePolicyRepo';
import { PUBLIC_BACKEND_URL } from '$env/static/public';
import { PolicyLogsRepo } from '$lib/repos/PolicyLogsRepo';

const policyLogsRepo = FakePolicyLogsRepo;
// const policyLogsRepo = PolicyLogsRepo;

const url: string = PUBLIC_BACKEND_URL;

export async function load() {
  return {
    policyLogsRepo,
    url
  };
}
