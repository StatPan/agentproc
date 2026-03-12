## Task ID: T-AUTO-002

## Title: Automatic Retry on Failure (Auto-Retry & Feedback Loop)

## Depends On: [T-AUTO-001]

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
- Runner automatically catches errors from thread subprocess or QualityGate commands
- Runner restarts the thread automatically without a human prompt up to task.RetryCount times
- Runner injects the stderr/error message into the thread's context as feedback on retry
- Logs accurately reflect 'Retry X of Y' on failure

## Design Ref
designs/WORK_LOOP.md

## Description
Implement the `Auto-Retry & Feedback Loop` directly into `agentos-runner`.

When the thread fails (either the subprocess returns non-zero exit code or `T-AUTO-001`'s QualityGate fails), the orchestrator used to have to manually restart the process.
Now, the Go runner itself should catch this failure. If `retryCount` is less than `task.RetryCount`, it should increment a retry counter and re-invoke the subprocess in the same isolated `.run/` directory.
Critically, it must inject the error log (`stderr`) into a file like `.run/{run_id}/feedback.md` and instruct the thread to read it and fix the code.

1. Edit `runner/runner.go` to add a retry loop around the subprocess execution.
2. If an error occurs, write the error output to a file like `feedback.md` in the run directory.
3. Keep track of retries up to the task's `RetryCount`.
4. Ensure the system works cleanly without stalling forever (timeout/retry count respect).
