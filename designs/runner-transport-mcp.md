# Runner Transport 확장 — HTTP + MCP

## 목적

AgentOS의 runner를 특정 CLI 진입점에 묶지 않고, 여러 상위 도구가 공통으로 붙을 수 있는 커널로 연다.
기존 HTTP transport는 유지하고, MCP transport를 추가해 Cursor, Claude Code, Codex, Gemini CLI 같은 MCP 클라이언트가 동일한 queue/dispatch 엔진을 호출할 수 있게 한다.

## 배경

- 현재 AgentOS의 역할 모델은 `process -> subprocess -> thread` 이며, process는 DESIGNER + ORCHESTRATOR 역할을 수행한다.
- `config.md` 의 레이어 매핑은 기본 실행 전략이지, 프로토콜 고정이 아니다.
- 상위 진입점이 여러 도구가 될 수 있으려면 runner는 특정 CLI가 아니라 공통 transport를 제공해야 한다.
- 운영상 중요한 제약이 하나 있다: task는 성공/실패와 무관하게 queue에서 제거된다.
  따라서 각 태스크는 다음 태스크의 dependency check와 사후 분석을 위해 `outputs/` 하위에 완료/실패 마커를 남겨야 한다.

## 접근 방법

### 1. transport 분리

- `runner/main.go` 에 `--transport` 플래그를 둔다.
- 기본값은 `http` 로 유지해 하위 호환을 지킨다.
- `mcp` 선택 시 HTTP 서버 대신 MCP 서버를 시작한다.

### 2. MCP를 process 진입점으로 사용

- MCP 서버는 runner 내부 queue 조작 기능을 tool surface로 노출한다.
- 최소 범위:
  - `status`
  - `list_tasks`
  - `create_task`
- 선택 범위:
  - `dispatch_once`
  - `read_output`
- MCP는 process 레이어를 대체하는 것이 아니라, process가 runner를 호출하는 공통 인터페이스가 된다.

### 3. 실패 추적 정책

- 현재 구현상 task는 실패해도 queue에서 제거된다.
- 그래서 worker 태스크는 `outputs/result-*.md` 같은 마커 파일을 반드시 남기도록 Done Condition/Quality Gate에 명시한다.
- 실패 재시도는 queue 잔존이 아니라 새 태스크 생성으로 다룬다.

## 태스크 목록

| TASK_ID | 제목 | Role | Depends On | Execution |
|---------|------|------|------------|-----------|
| T-MCP-001 | MCP server transport 구현 | worker_code | [] | parallel |
| T-MCP-002 | `main.go` 에 `--transport` 분기 추가 | worker_code | [T-MCP-001] | sequential |
| T-MCP-003 | MCP tool surface 및 실패 마커 정책 문서화 | worker_doc | [] | parallel |
| T-MCP-004 | MCP mode 통합 검증 태스크 추가 | reviewer | [T-MCP-001, T-MCP-002, T-MCP-003] | sequential |

## 완료 조건

- [ ] runner가 `http` 와 `mcp` transport를 선택적으로 실행할 수 있다
- [ ] MCP 클라이언트가 queue 조회와 태스크 생성 기능을 사용할 수 있다
- [ ] 기본 `http` 모드는 기존 동작을 유지한다
- [ ] 실패해도 task가 queue에서 사라지는 운영 특성이 문서와 검증 태스크에 반영된다
- [ ] 후속 subprocess가 dependency check를 위해 `outputs/` 마커 파일을 사용한다
