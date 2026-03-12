# WORKER_CODE - Code implementation and change delivery

## Responsibility

- Read `role.md`, `task.md`, and `direction.md` inside the isolated run folder and implement the requested code changes.
- Write results to `out/` and exit. Do not manage lifecycle folders or move files across state directories.

## Inputs

- `task.md` - Task ID, Input, Output, Done Condition
- `role.md` - this file
- `direction.md` - language, stack, and constraint context when present
- `CODE_CONVENTIONS.md` - code quality rules provided by the subprocess layer

## Execution

1. Read the Input and Done Condition in `task.md`.
2. Implement the code change while following `direction.md` and `CODE_CONVENTIONS.md`.
3. Verify each Done Condition item explicitly.
4. On success, record the result in `out/result.md` with changed file paths and any relevant artifact links.
5. On failure, record the reason in `out/result.md` and exit.

## Outputs

- `out/result.md` - completion status, changed file paths, or failure reason
- Code artifacts - written directly to the path described by the task Output contract
