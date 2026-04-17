# AgentProc

File-based orchestration kernel for multi-layer agent execution -- process → subprocess → thread hierarchy.

## Stack
- Runner: Go 1.23 (aproc binary)
- Tools: Python 3 (WebSocket server, OpenCode poller)
- Config: YAML (config.yaml)
- Task definitions: Markdown files
- Transport: HTTP (8080), MCP, CLI

## Architecture
```
[Human instruction]
  → [Process] reads AGENTS.md, config.md, roles/ → decomposes into tasks/queue/
    → [Subprocess x N] isolated .run/ folder, role.md + task.md
      → [Thread x N] leaf task execution → out/
        → [Reviewer] validates → PASS (outputs/) or FAIL (corrective task)
```

## Current State
- Go runner 구현 완료 (CLI + HTTP + MCP 3가지 트랜스포트)
- 큐 시스템, 태스크 파싱, 격리 폴더 lifecycle 작동
- 6개 역할 정의 (DESIGNER, ORCHESTRATOR, WORKER_CODE/DOC/SEARCH, REVIEWER)
- 4개 어댑터 (Claude, Codex, Gemini, OpenCode)
- T-AUTO 태스크 3개 완료 (T-AUTO-001, T-AUTO-002, T-AUTO-003 모두 PASS)
- **CLI hang 수정 완료** (2026-04-11):
  - `runner/runner.go` `renderThreadWrapper`: Python `Popen`에 `stdin=subprocess.DEVNULL` 추가
  - `runner/runner.go` `runTask`: `exec.CommandContext` + `parseTaskTimeout` (기본 30분 timeout), `cmd.Stdin = nil`
  - `adapters/claude-code.md`: `< /dev/null` 추가
- `runner/config.yaml` `agentos_root` 수정 완료: `/home/statpan/workspace/libs/agentproc`
- Hidden runtime project key 변경: `agentproc-9a925ea55656` → `agentproc-1d62bffd7fe1`
  - 이전 run 이력은 `~/.aproc/projects/agentproc-9a925ea55656/`에 잔존 (참조만 가능)
  - 신규 dispatch는 `~/.aproc/projects/agentproc-1d62bffd7fe1/` 사용

## Active Blocker
없음. CLI hang 문제 해소됨.

## Immediate Next
1. 새로운 태스크 정의 후 `cd runner && go run . dispatch once` 로 end-to-end 검증
2. Session OS 방향: canonical work-session 스키마 정의 (HANDOFF_SESSION_OS.md 참조)

## PM Role

이 세션(PM)의 역할은 분석·계획·검토다. 코드 편집은 Dev 세션에 위임한다.

- **분석**: 관련 파일을 Read로 읽어 현재 구조와 맥락을 파악한다
- **스펙 작성**: 대상 파일·변경 내용·이유·완료 기준을 포함한 구현 스펙을 작성한다
- **Dev 세션 호출**: `cd {project_dir} && claude -p "{스펙}" --model sonnet --output-format json --dangerously-skip-permissions`
- **결과 검토**: 변경된 파일을 Read로 확인하여 스펙 준수 여부와 코드 품질을 검토한다

코드를 직접 Edit/Write/Bash로 수정하지 않는다.

## Key Files
- runner/main.go -- CLI entry, subcommand dispatch
- runner/runner.go -- task execution orchestration
- runner/task.go -- task parsing, queue loading
- runner/mcp.go -- MCP server, tool definitions
- runner/paths.go -- path resolution, isolation folders
- AGENTS.md -- operating model, layer contracts
- config.md -- layer configuration (claude/codex/gemini)
- roles/*.md -- 6 role specifications
- adapters/*.md -- CLI adapter contracts
- conventions/CODE_CONVENTIONS.md -- code quality rules
- tasks/queue/ -- pending tasks
- CURRENT_STATUS.md -- immediate blockers and next actions

## Architectural Direction (Session OS)
agentproc은 orchestration kernel이면서 canonical work-session store가 되는 방향.
- agentproc이 canonical work state 소유 (goal, constraints, decisions, plan, artifacts)
- vendor/runtime sessions는 attachment (transcript, session ID 등은 vendor-private)
- 같은 work session이 runtime 변경에도 유지되어야 함

## Decisions Made
- File-based lightweight structure (git-friendly)
- Parallel-first decomposition
- Stateless workers with isolated run folders
- Summary-first observation (summary → evidence → raw logs)
- English-first internal operation

## Dependencies
- Claude CLI, Codex CLI, Gemini CLI, OpenCode CLI (adapters)
