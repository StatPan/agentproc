# AgentOS - Personal AI development operating system

## Purpose and core principles

1. Process hierarchy: the entrypoint AI spawns `subprocess`, and `subprocess` spawns `thread`
2. Parallel-first decomposition: split work by parallelism before anything else
3. File-based lightweight structure: manage tasks and roles through git-friendly files and folders
4. Stateless workers: each worker only sees its isolated workspace
5. Summary-first observation: process, subprocess, and reviewer read summaries and runner-generated evidence before any raw logs
6. English-first internal operation: use English for internal prompts, status fields, summaries, and event records whenever possible

## Process hierarchy

```text
[Human instruction]
     ↓
[process]
  - reads AGENTS.md, config.md, roles/
  - decomposes work into tasks
  - writes tasks to tasks/queue/
  - spawns subprocess workers
     ↓
[subprocess x N]
  - reads role.md / task.md / direction.md in an isolated run folder
  - may decompose further into thread-sized units
  - spawns thread workers
  - reviews results and retries when needed
     ↓
[thread x N]
  - performs only the assigned leaf task
  - writes results to out/
  - exits
```

There is always one entrypoint process. Different IDEs or CLIs do not change the logical flow: process -> subprocess -> thread.

## Execution model: isolated folder lifecycle

- On spawn, the upper layer creates one isolated folder at `tasks/.run/{run_id}/`
- The worker reads only that folder
- After completion, `out/` is copied to `outputs/` and the isolated folder is deleted
- `tasks/queue/` contains pending tasks only

## Observation policy

- Default reporting should use summaries and result markers, not raw stdout or stderr
- Raw logs belong in `outputs/runs/{run_id}/logs/`
- Read order is summary -> evidence -> raw log tail only when necessary
- Human-facing reporting should focus on final result and key risks

## Role map

| Role | Layer | Responsibility |
|------|-------|----------------|
| DESIGNER | process | turn user requests into design documents and top-level tasks |
| ORCHESTRATOR | process | inspect queue, prepare isolated folders, spawn subprocess workers |
| WORKER_CODE | subprocess -> thread | implement code changes |
| WORKER_DOC | subprocess -> thread | write or edit documentation |
| WORKER_SEARCH | subprocess -> thread | research and summarize |
| REVIEWER | subprocess | validate outputs and trigger retries |
