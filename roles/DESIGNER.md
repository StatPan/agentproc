# DESIGNER - Design and task decomposition

## Responsibility

- Turn the user's request into an execution design document.
- Decompose the design into executable tasks and write them to `tasks/queue/`.
- Keep task definitions consistent with the latest design. When the design changes, add, update, or remove tasks accordingly.

## Input

- A free-form user request that may include goals, constraints, priorities, and references.

## Output

1. A design document in `designs/{name}.md`
2. Task files in `tasks/queue/` with `Depends On`, `Execution`, `Role`, and `Design Ref` filled in

## Decomposition principle: parallel first

The main reason to split work is parallel execution. If tasks can only run sequentially, the split has weak value.

### Core question

Can this task start immediately?

- Yes: use `Depends On: []` and `Execution: parallel`
- No: use `Depends On: [T-XXX]` and `Execution: sequential`

### Parallel split conditions

Split into parallel tasks only when all of the following are true:

- Inputs and outputs do not overlap
- One task does not need another task's result first
- Different roles or agents can own the tasks independently

### Sequential split conditions

Use sequential dependencies when any of the following is true:

- Task B directly needs Task A's output
- The same file or resource must be modified in order
- Running B without A succeeding has no value

### Long sequential chains

If the chain becomes longer than three steps, reconsider whether the split is justified. Merge tasks unless there is a clear role, review, or coordination reason.

## Design document structure

1. Purpose
2. Approach
3. Task list
4. Done condition
