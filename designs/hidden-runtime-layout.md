# Hidden Runtime Layout for `~/.aproc`

## Purpose

Define the runtime-global directory layout for `aproc` under a hidden user-owned path so internal orchestration state stays out of normal project flow while still supporting process -> subprocess -> thread execution, summary-first observation, and logs-on-demand debugging.

## Approach

### 1. Root location and split of concerns

`aproc` keeps runtime-global state under a hidden per-user root:

```text
~/.aproc/
```

This root stores:

- install-independent runtime state shared across all projects
- immutable or versioned prompt and role assets used by workers
- per-project runtime state addressed through a deterministic mapping
- operational logs, summaries, cache, and temporary files

This root does not store:

- user source code
- project working files
- repository-local task definitions that the human is expected to edit directly

Project-scoped state is still runtime-global in storage location, but isolated under per-project directories inside `~/.aproc/projects/`. Optional project-local configuration such as `.aproc.yml` remains in the workspace and is not duplicated into the runtime tree.

### 2. Concrete directory tree

```text
~/.aproc/
├── config/
│   ├── config.yaml
│   └── profiles/
├── assets/
│   ├── prompts/
│   ├── roles/
│   ├── adapters/
│   └── templates/
├── projects/
│   └── {project-key}/
│       ├── meta/
│       │   ├── project.json
│       │   ├── paths.json
│       │   └── locks/
│       ├── tasks/
│       │   ├── queue/
│       │   └── archive/
│       ├── runs/
│       │   ├── active/
│       │   │   └── {run-id}/
│       │   │       ├── role.md
│       │   │       ├── task.md
│       │   │       ├── direction.md
│       │   │       ├── SUBPROCESS_INSTRUCTION.md
│       │   │       └── out/
│       │   └── completed/
│       │       └── {run-id}/
│       │           ├── summary.json
│       │           ├── result.md
│       │           ├── evidence/
│       │           └── logs/
│       ├── outputs/
│       │   ├── results/
│       │   ├── runs/
│       │   └── exports/
│       ├── state/
│       │   ├── latest.json
│       │   ├── checkpoints/
│       │   └── reviewer/
│       ├── cache/
│       └── tmp/
├── logs/
│   ├── cli/
│   └── maintenance/
├── cache/
│   ├── downloads/
│   ├── model-responses/
│   └── content-addressed/
├── tmp/
│   └── sessions/
└── gc/
    ├── retention.yaml
    └── tombstones/
```

### 3. Directory responsibilities

`config/`

- Stores user-level configuration such as default model layers, binary paths, retention policy overrides, and profile-specific settings.
- Contains only stable configuration, not live run state.

`assets/`

- Stores internal prompt, role, adapter, and template material used to seed isolated run folders.
- Versioned with the installed `aproc` release or copied on first run from packaged assets.
- Read-mostly; workers never write here during normal execution.

`projects/{project-key}/meta/`

- Stores the workspace identity and mapping metadata.
- `project.json` records the canonical root path, mapping key, created timestamp, and last-seen timestamp.
- `paths.json` records derived paths the orchestrator can reconstruct without rescanning historical runs.
- `locks/` contains file locks for orchestration, queue mutation, and retention tasks.

`projects/{project-key}/tasks/queue/`

- Stores pending task definitions for the project.
- Mirrors the repository reference layout from `AGENTS.md`, but lives in hidden runtime storage for the installed product.

`projects/{project-key}/tasks/archive/`

- Stores completed or cancelled task definitions when retention policy keeps task history.
- Keeps `queue/` limited to dispatchable work.

`projects/{project-key}/runs/active/{run-id}/`

- The isolated folder for an in-flight worker execution.
- Upper layers create exactly one run folder per dispatched task.
- Workers read only this folder.
- The folder is deleted after promotion of selected outputs into completed storage unless a failure-retention rule keeps it for debugging.

`projects/{project-key}/runs/completed/{run-id}/`

- Durable execution record for one run.
- `summary.json` is the first read surface for process/reviewer.
- `result.md` or equivalent marker captures PASS/FAIL and changed paths.
- `evidence/` holds runner-generated invocation ledgers, stdout/stderr paths, and thread/subprocess metadata.
- `logs/` stores raw logs only for on-demand debugging.

`projects/{project-key}/outputs/`

- Human-facing retained outputs copied from the run folder.
- `results/` contains task-level result markers.
- `runs/` contains run-level summaries and evidence intended for later review.
- `exports/` is reserved for explicit user-requested artifacts rather than internal state.

`projects/{project-key}/state/`

- Stores compact project runtime state needed across runs, such as the latest successful run, reviewer markers, and resumable checkpoints.
- Must stay small and index-like; large evidence belongs under `runs/`.

`projects/{project-key}/cache/`

- Project-specific cache for derived inputs that are expensive to recompute but safe to discard.
- Examples: normalized repository snapshots, dependency scans, file digests.

`projects/{project-key}/tmp/`

- Scratch space for the current project only.
- Never treated as durable state.
- Safe to wipe at process start or by background garbage collection.

`logs/cli/` and `logs/maintenance/`

- User-level operational logs not tied to one project run, such as CLI bootstrap failures or scheduled cleanup activity.
- These logs are not the default observation surface.

Global `cache/`

- Cross-project reusable cache for downloads, model response fragments, and content-addressed blobs.
- Must never be the only copy of user-visible results.

Global `tmp/`

- Cross-project scratch space for installation and short-lived orchestration operations.
- Must tolerate abrupt deletion without correctness loss.

`gc/`

- Stores retention policy state, tombstones, and sweep checkpoints so cleanup can run safely without scanning everything every time.

### 4. Global vs project-scoped runtime data

Global runtime data under `~/.aproc/`:

- configuration
- packaged prompt and role assets
- CLI and maintenance logs
- shared download and content cache
- retention policy metadata
- cross-project temporary files

Project-scoped runtime data under `~/.aproc/projects/{project-key}/`:

- queue and archived tasks
- active and completed run folders
- run summaries, evidence, and raw logs
- project state markers and reviewer state
- project-specific cache and temp files

Workspace-local data in the repository:

- source files
- optional `.aproc.yml`
- any user-authored documentation or outputs they intentionally export back into the project

### 5. Summary-first and logs-on-demand surfaces

The layout must make the default read path physically obvious:

1. `projects/{project-key}/outputs/results/` or `runs/completed/{run-id}/summary.json`
2. `runs/completed/{run-id}/evidence/`
3. `runs/completed/{run-id}/logs/`

Rules:

- every completed run must have a compact summary before raw logs are considered complete
- raw stdout/stderr live only under `logs/` or referenced evidence paths
- queue inspection and reviewer decisions should rely on summaries and result markers, not full logs
- active run folders may contain `out/summary.md`, but durable summaries are copied into completed storage before cleanup

### 6. Retention boundaries

Retention should be policy-driven and directory-specific:

- `runs/active/`: retain only while a run is executing; failed crash leftovers may be kept for a short crash-recovery window such as 24 hours
- `runs/completed/*/summary.json`, `result.md`, and `evidence/`: retain by default because they are the primary audit surface
- `runs/completed/*/logs/`: short retention by default, such as 7 to 14 days, because they are bulky and on-demand only
- `outputs/results/`: retain until explicitly pruned or superseded by policy
- `outputs/runs/`: medium retention, such as 30 to 90 days
- project `cache/` and global `cache/`: size-bounded LRU retention
- project `tmp/` and global `tmp/`: aggressive cleanup on startup, shutdown, or scheduled maintenance
- `tasks/archive/`: retain only if task history is useful for review; otherwise collapse into summaries and prune old task bodies

Retention policy defaults belong in `config/config.yaml`, with effective state and sweep markers recorded under `gc/`.

### 7. Mapping from workspace to project key

This layout assumes the project mapping design provides a deterministic `{project-key}` derived from the canonical workspace root path. A recommended format is:

```text
{basename}-{short-hash}
```

This keeps hidden runtime state human-inspectable while preventing collisions between similarly named repositories.

### 8. Visibility and normal user flow

The hidden runtime tree is implementation-facing, not part of the normal project UX:

- commands should report high-level results and explicit exported artifacts, not internal paths by default
- humans only need `~/.aproc/...` when inspecting failures, reviewing history, or changing config
- project working trees remain clean because task queues, run folders, evidence, and logs never live in the repository by default

## Task list

1. Use `~/.aproc/` as the stable hidden root for runtime-global state.
2. Store prompts, roles, adapters, and templates under `assets/` as read-mostly internal assets.
3. Isolate each workspace under `projects/{project-key}/` using deterministic project mapping.
4. Keep active run folders ephemeral under `runs/active/` and promote durable summaries, evidence, and selected outputs into `runs/completed/` and `outputs/`.
5. Separate summary surfaces from raw logs so default readers hit summaries first and logs only on demand.
6. Keep configuration, shared cache, maintenance logs, and retention metadata global rather than duplicating them per project.
7. Enforce retention boundaries by directory class so logs, cache, temp files, and active runs do not accumulate indefinitely.
8. Keep internal orchestration files hidden from normal repository flow; only optional `.aproc.yml` is project-local.

## Done condition

- A concrete hidden-runtime directory tree is defined under `~/.aproc/`.
- Global and project-scoped runtime data are explicitly separated.
- Prompts, roles, run state, logs, summaries, cache, and temp files each have a defined home.
- Summary-first observation and logs-on-demand are reflected in the storage layout.
- Retention boundaries are concrete enough to guide implementation and cleanup behavior.
- The layout remains consistent with the process model in `AGENTS.md` and keeps internal orchestration state out of the normal user flow.
