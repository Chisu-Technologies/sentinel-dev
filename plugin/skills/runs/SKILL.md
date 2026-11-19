---
name: runs
description: "Show the user the work Sentinel is doing or did on this computer: the latest runs, where one of them is, its pull request, verdict, tests and cost, or stop one. Use it when the user asks what Sentinel is doing, how a run went, or to stop a run."
---

- To list the latest runs, call `sentinel_list_runs` (pass `limit` for more or fewer).
- To see one run, call `sentinel_run_status` with its id.
- To stop a run that has not ended, call `sentinel_stop_run` with its id, and only
  when the user asked for it.

Show what the tools return as a short table or list, with the pull request links as
they come. The panel shows the same and more at http://localhost:8080 when Sentinel
runs on this computer.
