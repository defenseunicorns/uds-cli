---
name: testing
description: Apply the UDS CLI testing strategy when adding or changing unit, Legacy E2E, Next command integration, Next library integration, or Next cluster integration tests.
---

# Testing

Read `references/TESTING.md` and [ADR-0027](../../../docs/adr/0027-public-api-test-pyramid.md) before changing Next tests. Choose the smallest test layer that verifies the behavior, and run the corresponding `uds run` task before broadening validation.
