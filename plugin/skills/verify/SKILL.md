---
name: verify
description: "Send the change you made in this repository to Sentinel, which runs the tests in its sandbox, has an independent QA and reviewers check it, and opens a pull request. Use it when a change is ready, before you call it done, and when the user asks to verify, check or ship a change with Sentinel."
---

Sentinel runs on this computer and verifies a change outside your own session: it
applies the work to the repository's last commit on GitHub, runs the tests in a
sandbox, has a QA and reviewers that are not you check it, proves that new tests
fail without the fix, signs the result and opens a pull request. It never merges.

1. Make sure the change is complete and the repository's own tests pass where you
   can run them. Do not commit or push: Sentinel takes the work as it is in the
   folder, including what is not on GitHub yet.
2. Call the `sentinel_run_task` tool with a one or two sentence `task` that says what
   the change does, the way a person would describe it in a pull request, and
   `qa_by_you: true`, so your subagents answer its QA and reviewers on the user's
   plan instead of a key. Pass `base` only when the pull request should target a
   branch other than the repository's default.
3. It returns a run id at once. Call `sentinel_run_status` with that id about once a
   minute until the run ends (`completed`, `blocked`, `failed`, `stopped` or
   `cancelled`). Do not start a second run for the same change while one is going.
   When the status lists sessions under `waiting_for_your_agent`, start one subagent
   per session, all at once, and give each its `session_id`: `sentinel-qa` for the
   role `qa`, `sentinel-reviewer-a` for `reviewer-a` and `sentinel-reviewer-b` for
   `reviewer-b`. Then go back to checking the status. Sentinel stops the run if no
   subagent takes a session within 30 minutes.
4. Tell the user what Sentinel reports, exactly: the final status, the pull request
   link, the QA verdict, the tests that passed out of the total, and the cost.
5. When the run is `blocked` or the verdict is not approved, read what Sentinel says
   in its status, fix that in the folder, and send it again. Never merge the pull
   request yourself: merging is the user's decision.

If the `sentinel_*` tools are missing, Sentinel is not set up on this computer: tell
the user to install it (see https://github.com/Chisu-Technologies/sentinel-dev) and
run `sentinel up`, then `sentinel login`.
