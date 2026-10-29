# CLI-363

- [x] Read ticket/comments and trace schema handling in Legacy, Next, and Zarf.
- [x] Implement schema-neutral handling and regression coverage for both modes; document variable compatibility.
- [x] Run focused tests and repository checks.
- [x] Remove the final inspect alpha conversion and verify its output formatting.
- [x] Verify CLI backwards compatibility and unchanged Next public Go APIs.
- [ ] Run brutal-review; fix verified high-confidence, high-value findings and rerun affected checks.
- [ ] Commit with sign-off; present diff and validation for review. Do not push.

## PR #1621 review comments

- [x] Complete preflight: clean worktree, refreshed refs, required checks passed, base included.
- [x] Gather PR context and all open comments.
- [x] Fact-check each thread in parallel.
- [x] Aggregate findings and write the approval report.
- [x] Display the complete report and wait for approval.
- [x] Implement the approved report in order.
- [x] Run focused tests, lint, and the stale-code scan.
- [x] Commit with sign-off, excluding the local review report; show the full diff and validation.
- [ ] Wait for approval of the exact changes before pushing.
