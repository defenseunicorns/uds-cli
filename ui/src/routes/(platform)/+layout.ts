import {FakePolicyLogsRepo} from "../../../tests/fakerepos/FakePolicyRepo";
import {PolicyLogsRepo} from "$lib/repos/PolicyLogsRepo";

const policyLogsRepo = new FakePolicyLogsRepo();
// const policyLogsRepo = new PolicyLogsRepo('http://localhost:8080/api/v1/policies');

export async function load() {
  return { policyLogsRepo };
}
