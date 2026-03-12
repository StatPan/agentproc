# Hidden Runtime CLI Surface for `aproc`

## Purpose

Define a user-facing CLI contract for `aproc` where orchestration state is hidden by default. Normal usage should expose request, clarification, progress, and results without requiring users to navigate internal run folders or raw logs.

## Approach

- Treat `aproc` as the stable entrypoint installed through npm-first distribution.
- Keep the process model from `AGENTS.md`: a user submits one request to the entrypoint process, and all subprocess/thread work remains internal unless the user explicitly asks for inspection or debug output.
- Make the default read path summary-first: command output shows concise status, clarifications, and result summaries before any raw evidence paths.
- Use a durable request identifier as the public handle for follow-up commands. Internal run IDs may exist, but they are not part of the default UX.

## CLI Contract

### 1. Request submission

```bash
aproc request "Add retry logic to the runner"
aproc request --file prompt.md
aproc request "Investigate failing tests" --json
```

- Creates a new top-level request and immediately returns:
  - `request_id`
  - current state such as `queued`, `running`, or `needs-clarification`
  - a short summary of what `aproc` understood
  - the next user action when needed
- Default human output is compact text.
- `--json` returns the same contract in machine-readable form for editors, scripts, and agents.

Example default output:

```text
Request: req_01HV...
Status: running
Summary: Investigate failing tests and report the likely root cause.
Next: Run `aproc status req_01HV...` for progress or `aproc result req_01HV...` when complete.
```

### 2. Clarification flow

```bash
aproc status req_01HV...
aproc answer req_01HV... "Target only the npm packaging path."
aproc answer req_01HV... --file answer.md
```

- If the process requires user input, `status` and `result` both surface `needs-clarification`.
- The CLI prints the pending question directly instead of pointing users at an internal file.
- `answer` submits one clarification response against the request and resumes orchestration.
- If multiple questions are pending, `status` shows them in order and `answer` accepts either the next response or a named prompt key in JSON mode.

Example clarification output:

```text
Request: req_01HV...
Status: needs-clarification
Question: Should the change preserve the current output format for existing scripts?
Next: Run `aproc answer req_01HV... "<your answer>"`
```

### 3. Status and progress

```bash
aproc status req_01HV...
aproc status req_01HV... --watch
aproc list
```

- `status` is the primary progress surface.
- Default output includes:
  - request status
  - short progress summary
  - active stage if known, such as `design`, `implementation`, or `review`
  - whether user action is blocked on clarification
  - most recent update time
- `--watch` refreshes the summary view until completion or clarification is required.
- `list` shows recent requests with status and summary, not filesystem paths.

### 4. Result retrieval

```bash
aproc result req_01HV...
aproc result req_01HV... --open
aproc result req_01HV... --json
```

- `result` is the canonical completion surface.
- On success, it returns:
  - final status
  - concise summary of what changed or was found
  - key risks or follow-ups
  - references to user-relevant artifacts such as a design doc or changed files
- On failure, it returns:
  - failure status
  - short reason
  - any required user retry action
- `--open` prints or opens the main result artifact, such as `outputs/result-*.md`, without exposing internal transient run folders.

### 5. Inspection without debug mode

```bash
aproc inspect req_01HV...
aproc inspect req_01HV... --json
```

- `inspect` is an explicit but still summary-first diagnostics view.
- It may show:
  - top-level task decomposition
  - completed stages
  - produced output artifacts
  - evidence markers such as invocation counts or reviewer status
- It should not dump raw stdout/stderr by default.
- This is the right command for advanced users who want more than `status` but do not want full debug logs.

### 6. Explicit debug and log access

```bash
aproc debug req_01HV...
aproc logs req_01HV...
aproc logs req_01HV... --tail stderr
```

- Debug access is opt-in and clearly separated from normal commands.
- `debug` reveals internal identifiers and durable filesystem locations for operators.
- `logs` accesses raw stdout/stderr or runner evidence only when explicitly requested.
- Commands should warn that the output is low-level and not stable as a public automation contract unless `--json` defines a dedicated schema.

### 7. Public request states

The public lifecycle should use a small stable state set:

- `queued`
- `running`
- `needs-clarification`
- `completed`
- `failed`
- `cancelled`

These states must remain consistent across `request`, `status`, `result`, `list`, and `inspect`.

## Decisions

- Public handles use `request_id`; internal `run_id` remains hidden unless `debug` is used.
- Normal commands never require users to read `tasks/.run/` or raw logs.
- Clarifications are first-class CLI interactions, not side-channel files.
- `status` and `result` are separate so users can poll progress without opening final artifacts.
- `inspect` exists for structured visibility without collapsing the summary-first model.
- `debug` and `logs` preserve operator access while keeping internals out of the default surface.
- `--json` should be supported on every user-facing command that returns structured state.

## Done Condition

- Normal user flow avoids direct exposure of internal files.
- Clarification and result surfaces are explicit and command-driven.
- Debug and log access remain opt-in.
- The contract matches the `process -> subprocess -> thread` model by keeping orchestration internals behind the process entrypoint.
- Commands are plausible for npm-installed `aproc` usage and preserve summary-first observation by default.
