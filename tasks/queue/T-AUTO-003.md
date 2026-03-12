## Task ID: T-AUTO-003

## Title: Deterministic Output Existence Check

## Depends On: [T-AUTO-002]

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
- Runner automatically checks if task.Output paths exist before marking a task as success
- Supports basic file/directory existence checks
- Triggers task failure (and retry if applicable) if the expected output is missing

## Design Ref
designs/WORK_LOOP.md

## Description
Implement `Output Existence Check` to verify that the AI worker actually created the required output artifact.

Workers sometimes claim to have done the work, but don't actually generate the requested file(s).
The runner should verify that any path defined in `task.Output` exists in the file system before deciding the task is successful and letting the task be archived.
`task.Output` might be a single path or comma-separated paths. The runner must check them all.

1. Edit `runner/runner.go` or validation logic to parse `task.Output`.
2. Check if the file/directory exists relative to the project root or the isolated directory.
3. If it doesn't exist, generate an error "Output missing: {path}" and pass it to the retry loop (`T-AUTO-002`).
