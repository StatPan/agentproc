# aproc

`aproc` 는 AgentOS 런타임의 안정적인 진입점이다. 일반 사용자는 요청, 보충 답변, 상태 확인, 결과 확인에 집중하고, queue 파일이나 run 폴더 같은 내부 구조는 기본 흐름에서 숨겨진다.

현재 구현은 hidden-runtime UX를 향한 과도기 상태다. 기본 사용 흐름은 이미 `aproc` 하나로 시작할 수 있고, 일부 단계는 아직 `intake`, `reply`, `result`, `tail` 같은 하위 명령으로 노출된다.

## 빌드

```bash
go build -o aproc .
```

## 기본 사용자 흐름

### 1. 요청 제출

가장 단순한 진입은 자연어 요청을 바로 넘기는 것이다. 현재 구현에서는 이 호출이 내부적으로 `intake` 흐름으로 연결된다.

```bash
./aproc "로그 회전 구조 점검"
./aproc intake --root <agentos_root> --config ./config.yaml "로그 회전 구조 점검"
```

- 기본 출력은 전체 로그 스트리밍이 아니라 현재 상태 요약이다.
- 컨텍스트가 충분하면 바로 작업이 준비되고, 부족하면 보충 질문이 생성된다.
- 일반 사용자는 `tasks/queue/` 나 `tasks/.run/` 를 직접 열 필요가 없다.

### 2. 보충 답변 제출

추가 정보가 필요할 때는 세션에 답변을 이어서 제출한다.

```bash
./aproc reply --root <agentos_root> --config ./config.yaml <session-id> "runner/ 기준으로 수정 포함, 테스트도 진행"
```

- 이 단계는 hidden-runtime 설계의 `answer` 동작에 대응하는 현재 구현이다.
- 질문과 답변은 CLI 출력으로 확인하며, 내부 세션 파일 탐색을 기본 절차로 삼지 않는다.

### 3. 상태와 진행 상황 확인

현재 로컬 CLI에는 아직 최종 형태의 `aproc status <request-id>` 명령이 없다. 대신 다음 두 경로가 존재한다.

- 일반 사용자 관점에서는 요청 제출 직후와 답변 제출 직후의 요약 출력이 첫 상태 확인 수단이다.
- 통합 도구나 운영 경로에서는 `serve` 로 runner를 띄운 뒤 HTTP `/status` 또는 MCP `status` tool로 요약 상태를 조회한다.

이 저장소의 목표 surface는 `request -> status -> answer -> result` 순서를 분명하게 만드는 것이다. 다만 현재 코드 기준으로 `status` 는 서버/API 경로에 먼저 구현되어 있고, 단일 로컬 하위 명령으로는 아직 고정되지 않았다.

### 4. 결과 확인

완료 이후에는 `result` 가 기본 결과 조회 경로다. 먼저 요약을 확인하고, 더 낮은 수준의 증거는 필요할 때만 연다.

```bash
./aproc result --root <agentos_root> --config ./config.yaml <run-id>
```

- `result` 는 run 요약, 결과 마커 경로, 핵심 이벤트를 보여준다.
- 기본 읽기 순서는 요약 우선이다.
- 결과 확인을 위해 사용자가 raw stdout/stderr 또는 내부 run 폴더를 먼저 따라갈 필요는 없다.

### 5. 선택적 운영자 검사와 로그 접근

원시 로그 접근은 기본 사용자 흐름이 아니라 명시적 운영 작업이다.

```bash
./aproc tail --root <agentos_root> --config ./config.yaml --last 40 <run-id>
./aproc tail --root <agentos_root> --config ./config.yaml --stream stderr --last 40 <run-id>
```

- `tail` 은 hidden-runtime 설계의 `logs` 또는 `debug` 성격에 가까운 현재 구현이다.
- 기본 계약은 summary-first 이고, 로그는 문제가 있거나 세부 근거가 필요할 때만 본다.

## 명령 개요

| 사용자 의도 | 현재 명령 | 비고 |
|-------------|-----------|------|
| 요청 시작 | `aproc "<request>"` | 가장 간단한 진입점. 현재는 `intake` 로 연결 |
| 요청 시작 | `aproc intake ... "<request>"` | 명시적 intake 경로 |
| 보충 답변 | `aproc reply ... <session-id> "<answer>"` | 현재 `answer` 에 해당하는 구현 |
| 결과 요약 확인 | `aproc result ... <run-id>` | 기본 완료 조회 경로 |
| 운영자 로그 확인 | `aproc tail ... <run-id>` | 선택적 저수준 검사 |
| 서버/통합 진입 | `aproc serve ...` | HTTP 또는 MCP transport 제공 |
| 큐 1회 처리 | `aproc dispatch once ...` | 운영/자동화용 실행 경로 |

## 서버와 통합 경로

HTTP와 MCP는 같은 queue/dispatch 엔진을 여는 서로 다른 transport다. 이것들은 process 레이어를 대체하는 사용자 기본 UX가 아니라, 상위 도구가 hidden-runtime orchestration에 접속하는 통합 경로다.

### HTTP server

```bash
./aproc serve --root <agentos_root> --config ./config.yaml --transport http --port 8080
```

### MCP transport

```bash
./aproc serve --root <agentos_root> --config ./config.yaml --transport mcp
```

### dispatch once

queue를 한 번 평가하고 현재 즉시 실행 가능한 task만 처리한 뒤 종료하려면 `dispatch once` 를 사용한다.

```bash
./aproc dispatch once --root <agentos_root> --config ./config.yaml
```

이 명령은 운영이나 자동화에는 유효하지만, 일반 사용자의 기본 인터페이스를 queue 파일 탐색 중심으로 바꾸지는 않는다.

| 플래그 | 기본값 | 설명 |
|--------|--------|------|
| `--root` | `.` | AgentOS 루트 경로 |
| `--config` | `./config.yaml` | 설정 파일 경로 |
| `--port` | `8080` | HTTP 서버 포트 |
| `--transport` | `http` | runner transport 선택값. `http` 또는 `mcp` |

기존 `./aproc --transport ...` 형태도 하위 호환으로 계속 동작한다.

## config.yaml 필드

```yaml
agentos_root: ""          # 비우면 --root 값 사용
layers:
  subprocess: codex       # 레이어 계약은 adapters/subprocess.md, 실행 템플릿은 adapters/codex.md 참조
  thread: gemini          # 예약 필드. thread 레이어 식별자
runner:
  max_concurrent: 0       # 0이면 runtime.NumCPU()
  poll_interval: 5s       # 설정 파일 호환용 필드
  mode: daemon
```

| 필드 | 설명 |
|------|------|
| `agentos_root` | AgentOS 루트 경로. 비우면 `--root` 플래그 값을 사용 |
| `layers.subprocess` | 실제 실행할 adapter 선택값 |
| `layers.thread` | thread 레이어 바이너리 이름 |
| `runner.max_concurrent` | 동시 실행 상한. `0`이면 CPU 코어 수 사용 |
| `runner.poll_interval` | 설정 파일 스펙 호환용 주기 값 |
| `runner.mode` | 현재 지원 모드. `daemon` 사용 |

## HTTP API

| Method | Path | 설명 |
|--------|------|------|
| POST | /task | 태스크 생성 (JSON body) |
| GET | /tasks | 큐 목록 조회 |
| GET | /status | 러너 상태 조회 |

### POST /task 예시

```bash
curl -X POST http://localhost:8080/task \
  -H "Content-Type: application/json" \
  -d '{"id":"T-001","title":"example","role":"worker_code","input":"...","design_ref":"designs/foo.md","depends_on":[],"execution":"parallel","assigned_to":"codex"}'
```

## MCP Transport

MCP는 process 레이어를 대체하는 새 계층이 아니다. Cursor, Claude Code, Codex, Gemini CLI 같은 상위 도구가 같은 runner에 붙기 위한 공통 transport다. 즉 계층 구조는 여전히 `process -> subprocess -> thread` 이고, MCP는 process가 runner를 호출하는 인터페이스만 바꾼다.

### HTTP와 MCP의 역할 차이

| 항목 | HTTP | MCP |
|------|------|-----|
| 주 용도 | 서비스형 API 호출 | IDE/CLI 에이전트 도구 호출 |
| 연결 방식 | 포트 기반 요청/응답 | MCP 클라이언트 세션 기반 tool call |
| 대상 | 외부 자동화, curl, 서버 간 연동 | Cursor, Claude Code, Codex, Gemini CLI |
| 내부 의미 | queue/dispatch 엔진 노출 | 같은 queue/dispatch 엔진 노출 |

핵심은 transport가 달라도 runner의 역할은 동일하다는 점이다. 상위 진입점 도구가 여럿이어도 queue 작성, 조회, dispatch 정책은 하나의 runner가 공통으로 담당한다.

### 다중 진입점 운영

- process는 여전히 단일 오케스트레이터 역할을 가진다.
- MCP를 지원하는 각 도구는 process를 대체하지 않고 runner의 공통 transport를 호출한다.
- 따라서 `config.yaml` 은 실제 spawn 대상인 `subprocess` 와 `thread` 만 정의하고, `AGENTS.md` 는 역할 구조를 유지한다.
- transport 선택은 통신 방식의 차이일 뿐, subprocess와 thread의 책임을 바꾸지 않는다.

### 실패 마커 정책

운영상 중요한 특성 하나가 있다. 태스크는 성공 여부와 무관하게 `tasks/queue/` 에서 제거된다.

그래서 모든 태스크는 `outputs/` 아래에 결과 마커 파일을 반드시 남겨야 한다. 권장 형식은 `outputs/result-<TASK_ID>.md` 이다. 이 마커는 다음 용도로 사용한다.

- 후속 태스크의 dependency 확인
- 실패 원인 추적과 재실행 판단
- queue 밖에서 완료/실패 이력 유지

실패 재시도는 queue 잔존으로 처리하지 않는다. 실패 마커를 확인한 뒤 새 태스크를 생성해 다시 실행한다.

### 결과 마커 예시

```md
PASS
- task: T-MCP-003
- doc: runner/README.md
```

```md
FAIL
- task: T-MCP-003
- reason: MCP transport section was not written
```

### thread 호출 증거 규약

subprocess는 thread layer를 직접 임의 호출하지 않고 runner wrapper 를 통해 호출해야 한다. runner 는 run 시작 시 `out/thread-evidence/` 를 만들고, wrapper 실행마다 runner-generated evidence 를 기록한다.

기본 경로 예시:

- `out/thread-evidence/invocations.jsonl`
- `out/thread-evidence/stdout/`
- `out/thread-evidence/stderr/`

`invocations.jsonl` 최소 필드는 다음과 같다.

- `invocation_id`
- `task_id`
- `thread_layer`
- `thread_binary`
- `adapter_path`
- `argv` 또는 `rendered_command`
- `started_at`
- `finished_at`
- `exit_code`
- `stdout_path`
- `stderr_path`

`out/thread-log.md` 는 계속 둘 수 있지만 사람이 읽는 요약일 뿐이고, reviewer 원본은 runner-generated evidence 다.

실행이 끝나면 runner는 다음을 `outputs/` 로 복사한다.

- `outputs/thread-evidence-<TASK_ID>/invocations.jsonl`
- 필요 시 대응하는 `stdout/`, `stderr/`
- `outputs/thread-<TASK_ID>.md` 요약 마커

따라서 process/reviewer 기본 읽기 순서는 `out/summary.md` -> runner-generated evidence -> 필요 시 `out/thread-log.md`, `out/result.md` 다. reviewer 는 run 중에는 `out/thread-evidence/`, 실행 종료 후에는 `outputs/thread-evidence-<TASK_ID>/` 를 우선 보고 thread 집행 여부를 검증한다.

### MCP 사용 예시

아래 예시는 MCP 클라이언트가 runner의 tool surface를 통해 상태를 조회하고 태스크를 만드는 흐름을 보여준다.

1. 상위 도구가 `status` 를 호출해 runner 상태를 확인한다.
2. 상위 도구가 `list_tasks` 로 현재 queue를 조회한다.
3. 상위 도구가 `create_task` 로 새 태스크를 등록한다.
4. 실행이 끝나면 `outputs/result-*.md` 마커를 읽어 성공/실패를 판정한다.

이 흐름에서도 process는 작업 분해와 spawn 정책을 담당하고, MCP는 그 process가 runner와 통신하는 transport로만 동작한다.
