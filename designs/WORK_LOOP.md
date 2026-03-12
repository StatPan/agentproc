# Work loop design - one run, one task, isolated folders, stateless workers

## Premises

- No infinite worker loop. One execution handles one task.
- Use one project root as the stable reference point.
- Workers are stateless. The upper layer passes the current task and the worker exits after finishing.

## Folder model

```text
AgentOS/
├── AGENTS.md
├── roles/
├── designs/
├── tasks/
│   ├── queue/
│   └── .run/
│       └── {run_id}/
│           ├── role.md
│           ├── task.md
│           └── out/
└── outputs/
```

During execution, a worker should read only `tasks/.run/{run_id}/`.

## One execution equals one task

One run should do exactly one of the following:

- The human explicitly asks to run one task
- The dispatcher selects one dispatchable task from the queue

The next task should start only through a new trigger.

## Upper-layer lifecycle

1. Select one task
2. Create `tasks/.run/{run_id}/`
3. Copy role and task inputs into the isolated folder
4. Invoke the worker against that folder only
5. Copy outputs to `outputs/`
6. Delete the isolated folder

## Summary-first / logs-on-demand

- Do not stream raw stdout or stderr into the main conversation by default
- Persist run summaries at `outputs/runs/{run_id}/summary.json`
- Persist raw logs at `outputs/runs/{run_id}/logs/`
- Default read order:
  1. `summary.json` or `out/summary.md`
  2. runner-generated evidence
  3. raw log tail only when needed

## Internal language policy

- Prefer English for internal prompts, summaries, statuses, and events
- Use Korean only at the human-facing interface when needed
