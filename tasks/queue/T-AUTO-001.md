## Task ID: T-AUTO-001

## Title: Automatic QualityGate Execution (Post-Hook)

## Depends On: []

## Execution: parallel

## Role: WORKER_CODE

## Assigned To: subprocess

## Output
runner/runner.go

## Input
runner/runner.go, runner/task.go

## QualityGate
- cd runner && go test ./...
- cd runner && go build -o agentos-runner

## Done Condition
- Runner automatically parses QualityGate array from task.md
- Runner executes each command in QualityGate sequentially after the subprocess finishes successfully
- If any QualityGate command fails, the task fails and returns an error with the stderr

## Design Ref
designs/WORK_LOOP.md

## Description
Update `agentos-runner` to automatically execute the shell commands defined in a task's `QualityGate` field *after* the thread/subprocess completes its main work.

Currently, `QualityGate` is just a field in `task.md`. AI workers are expected to read it and run it manually via prompts. This is unreliable. We want the Go `agentos-runner` to strictly enforce this.
In `runner.go` (or wherever tasks are finalized), after the subprocess exits with success, loop through `task.QualityGate` commands.
Execute each command using `exec.Command("sh", "-c", cmd)`.
If a command fails, capture the combined stdout/stderr and return an error.
If all pass, proceed to finalize the task (e.g., move to archive).

1. Edit `runner/runner.go` (or appropriate file) where the subprocess execution finishes.
2. Implement a function or inline logic to execute `QualityGate` commands sequentially.
3. Ensure the working directory for these commands is the project root.
4. Do NOT modify the core architecture. Just add the validation step.
5. Make sure the unit tests pass (`go test ./...`).
