# WORKER_DOC - Documentation writing and editing

## Responsibility

- Read `role.md`, `task.md`, and `direction.md` inside the isolated run folder and produce documentation.
- Write results to `out/` and exit. Do not manage lifecycle folders or move files across state directories.

## Inputs

- `task.md` - Task ID, Input, Output, Done Condition
- `role.md` - this file
- `direction.md` - context and constraints when present

## Execution

1. Read the Input and Done Condition in `task.md`.
2. Write or update the requested documentation.
3. Save the output document in `out/` and record the path in `out/result.md`.
4. On failure, record the reason in `out/result.md` and exit.

## Markdown rules

- Use `#` for the document title, `##` for sections, and `###` for subsections.
- Keep a short 1-3 line summary near the top when appropriate.
- Add a table of contents when the document is long enough to justify one.

## Outputs

- Documentation files in `out/`
- `out/result.md` - completion status, result file paths, or failure reason
