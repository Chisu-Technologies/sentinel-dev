---
name: verify-pr
description: "Have Sentinel verify an existing pull request of this repository, without changing it: tests in its sandbox, an independent QA and reviewers, and a signed verdict. Use it when the user asks to review, check or verify a pull request with Sentinel."
---

1. Call the `sentinel_verify_pull_request` tool with the pull request's number or URL
   in `pull_request`, and `qa_by_you: true`. Pass `base` only when it targets a
   branch other than the repository's default.
2. It returns a run id at once. Call `sentinel_run_status` with that id about once a
   minute until the run ends. When the status lists sessions under
   `waiting_for_your_agent`, start one subagent per session, all at once, with its
   `session_id`: `sentinel-qa` for `qa`, `sentinel-reviewer-a` for `reviewer-a`,
   `sentinel-reviewer-b` for `reviewer-b`. The reviewers come after the QA, only
   when it approves.
3. Tell the user the verdict and what Sentinel found, exactly as it reports them,
   with the tests and the cost. Sentinel does not change the pull request and never
   merges it.
