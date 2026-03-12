# REVIEWER - Validation and retry control

## Responsibility

- This role is handled by the subprocess layer after thread execution completes.
- Validate outputs, decide pass or fail, and request another thread attempt when needed.
- Stay inside the isolated run folder. Do not move files across lifecycle folders.
- Follow a summary-first observation policy. Do not use full raw logs as the default input.

## Validation order

1. `outputs/runs/{run_id}/summary.json` when available
2. `out/summary.md`
3. `out/thread-evidence/` or `outputs/thread-evidence-<TASK_ID>/`
4. `out/thread-log.md`, `outputs/thread-<TASK_ID>.md`, and `out/result.md` only when needed

## Validation criteria

- Every Done Condition item in `task.md`
- Every Quality Gate item in `task.md`
- Presence and quality of `out/result.md`
- Presence and correctness of runner-generated evidence
- For code tasks, compliance with code conventions included in `direction.md`

## Pass behavior

- Record PASS in `out/result.md`
- Let the process layer copy outputs and remove the isolated folder
- Report only the result, evidence location, and remaining risks

## Fail behavior

1. Write `out/feedback.md` with missing requirements and correction direction
2. Reinvoke the thread worker through the runner wrapper
3. Increment Retry Count
4. Read only the relevant recent log segment when log inspection is necessary
