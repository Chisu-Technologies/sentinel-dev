# Glossary

Status: planned design, not implemented.

> **Planned.** These terms describe the intended design of Sentinel for developers,
> which is pre-alpha. Nothing here is available yet.

This page fixes the English terms used across the documentation. It also serves as the
term list for translations: the "Español" column gives the Spanish term to use, so that
every translated page says the same thing. Where a term stays in English, the column
says so. English is the source of truth.

Two points about the Spanish column:

- "Stage" and "phase" both translate to "fase". Use "fase del plan" for a stage of a
  plan and "fase de construcción" for a phase of the [roadmap](roadmap.md), whenever
  the context does not make clear which one is meant.
- "Brain" is a product term and is not translated.

| Term | Meaning | Español |
|---|---|---|
| Agent | The coding agent you already use, Claude Code or Codex. It does the thinking, with your own membership. | agente |
| Approve | Accept a plan so that its tasks can start. Only your own web session can approve a plan; the plugin token cannot. | aprobar |
| Board | The web view of a plan's tasks, in four columns: in progress, needs a decision, needs attention and done. Tasks not yet started sit in the backlog. | tablero |
| Brain | Sentinel's map of your repository: which files and symbols exist and how they relate to each other. | Brain (sin traducir) |
| Brain projection | The part of the Brain that is sent to the hosted service: the names of files and symbols and how they relate, not their contents. The index itself stays on your machine. | proyección del Brain |
| Elicitation (URL mode) | A client feature that lets the local program open the sign-in page in your browser. If the client does not offer it, the URL and a code appear as text. | elicitación en modo URL |
| Evidence | What Sentinel keeps to show that a task was checked. It is "reported", not "attested". | evidencia |
| Heads | A feature of the paid plans of the hosted service. It is closed and is not part of this repository. | Heads (sin traducir) |
| Hosted service | The service operated by Sentinel that stores plans, task state, the Brain projection and runner reports, and serves the web board. Closed source. | servicio alojado |
| Human task | A task that a person does and marks done on the web instead of the agent. | tarea humana |
| Local program | The program that will run on your machine. The plugin will start it as an MCP server. It will have five parts: sign-in, the MCP server, the runner, the Brain indexer and the receipt verifier. | programa local |
| Marketplace | The catalog from which the plugin is installed. Its planned name is `sentinel-dev`, to be confirmed when its manifest exists. | marketplace |
| MCP server | A program that offers tools to your agent through the Model Context Protocol. The local program will run as one. | servidor MCP |
| Phase | One of the four steps of the [roadmap](roadmap.md). A number gives order, not a date. | fase de construcción |
| Plan | A goal broken into stages, sprints and tasks, made by your agent acting as architect and stored in the hosted service. | plan |
| Plugin | The part you install in your agent: the commands, the subagents and the MCP server configuration. Its name is `sentinel`. | plugin |
| Receipt | A signed record of a closed task, planned for Phase 2. The hosted service signs it as "reported" evidence, and the receipt verifier can check one without a connection. | recibo |
| Reported | The label on evidence that your machine reports to the hosted service. It resists casual tampering, not a determined user. | reportada |
| Reservation | The time-limited claim a session holds on a task. If the session dies, the reservation expires and the task returns to pending. | reserva |
| Runner | The part of the local program that runs your tests, your app and a browser in Docker containers on your machine. | runner (sin traducir) |
| Runner report | The record the runner makes of a run: commands with exit codes and durations, a filtered tail of the output, and hashes. A task is not closed without one. | informe del runner |
| Sprint | A group of tasks within a stage of a plan. | sprint (sin traducir) |
| Stage | The top level of a plan. In the plan format it is called `stages`. | fase del plan |
| Subagent | A helper agent inside your session with its own restricted list of tools. Three are planned for Phase 2: QA, security review and code review. | subagente |
| Task | The unit of work in a plan: it has an id, title, description, acceptance criteria and dependencies, and it may be marked as a human task. | tarea |
| Worktree | A separate checkout of the same git repository. Sentinel is planned to implement each task in one, so your working folder is not touched. | git worktree (sin traducir) |
