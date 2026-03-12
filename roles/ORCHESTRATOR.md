# ORCHESTRATOR - Subprocess spawn and task dispatch

## Responsibility

- This role is handled by the process layer.
- Select dispatchable tasks from `tasks/queue/`, create isolated run folders, and spawn subprocess workers.
- Do not manage state folders such as running, done, or failed. Use isolated folder creation and cleanup only.
- Follow a summary-first observation policy. Do not read full raw logs by default.

## Dispatch flow

1. Inspect `tasks/queue/*.md`
2. Check dependencies through output markers in `outputs/`
3. Decide whether tasks can run in parallel or must remain sequential
4. Create `tasks/.run/{run_id}/` with `role.md`, `task.md`, `direction.md`, `SUBPROCESS_INSTRUCTION.md`, and `out/`
5. Spawn the configured subprocess worker through the adapter contract
6. Copy outputs to `outputs/` when the run finishes
7. Remove the queue item after completion

## Observation and reporting

Read in this order:

1. `outputs/runs/{run_id}/summary.json`
2. `outputs/result-<TASK_ID>.md`
3. `outputs/thread-evidence-<TASK_ID>/`
4. Raw logs only when required

Report short outcome summaries to the user or upper layer. Open raw logs only for debugging.
