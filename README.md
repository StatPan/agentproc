# agentproc

File-based orchestration for multi-layer agent execution.

## Overview

- `process -> subprocess -> thread` execution model
- Isolated run folders under `tasks/.run/`
- Queue-based task dispatch through `tasks/queue/`
- Summary-first observation with logs on demand
- Go runner CLI exposed as `aproc`

## Layout

- `AGENTS.md` - core operating model
- `roles/` - role contracts
- `designs/` - design documents
- `tasks/queue/` - pending task definitions
- `runner/` - Go runner and CLI

## Runner

```bash
cd runner
go build -o aproc .
./aproc "inspect the queue and prepare the next task"
```
