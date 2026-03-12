# aproc

AgentOS 태스크 큐를 감시하고 서브프로세스를 실행하는 Go 바이너리.

runner는 process 레이어 자체를 대체하지 않는다.
여러 상위 도구가 공통으로 접속할 수 있는 transport를 제공한다.
HTTP와 MCP는 같은 queue/dispatch 엔진을 여는 서로 다른 진입 방식이다.

## 빌드

```bash
go build -o aproc .
```

## 실행

```bash
./aproc serve --root <agentos_root> --config ./config.yaml --transport http --port 8080
```

가장 단순한 사용 방식은 자연어 요청을 바로 넘기는 것이다. 컨텍스트가 부족하면 질문 세션을 만들고, 충분하면 queue task를 자동 생성한다.

```bash
./aproc "로그 회전 구조 점검"
```

실행 중에는 전체 로그를 대화창으로 스트리밍하지 않고, 완료 후 요약만 확인한 다음 필요할 때만 특정 run의 로그 일부를 볼 수 있다.

```bash
./aproc result <run-id>
./aproc tail --last 40 <run-id>
```

MCP transport를 사용할 때는 transport만 바꿔서 같은 `serve` 경로를 쓴다.

```bash
./aproc serve --root <agentos_root> --config ./config.yaml --transport mcp
```

queue를 한 번 평가하고 현재 즉시 실행 가능한 task만 처리한 뒤 종료하려면 `dispatch once` 를 사용한다.

```bash
./aproc dispatch once --root <agentos_root> --config ./config.yaml
```

사용자 요청을 먼저 받고, 컨텍스트가 부족하면 질문 세션을 만든 뒤, 답변이 모이면 queue task를 자동 생성하려면 `intake` 와 `reply` 를 사용한다.

```bash
./aproc intake --root <agentos_root> --config ./config.yaml "로그 회전 구조 점검"
./aproc reply --root <agentos_root> --config ./config.yaml <session-id> "runner/ 기준으로 수정 포함, 테스트도 진행"
```

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
  process: claude         # 예약 필드. 상위 레이어 식별자
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
| `layers.process` | process 레이어 바이너리 이름 |
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

MCP는 process 레이어를 대체하는 새 계층이 아니다.
Cursor, Claude Code, Codex, Gemini CLI 같은 상위 도구가 같은 runner에 붙기 위한 공통 transport다.
즉 계층 구조는 여전히 `process -> subprocess -> thread` 이고, MCP는 process가 runner를 호출하는 인터페이스만 바꾼다.

### HTTP와 MCP의 역할 차이

| 항목 | HTTP | MCP |
|------|------|-----|
| 주 용도 | 서비스형 API 호출 | IDE/CLI 에이전트 도구 호출 |
| 연결 방식 | 포트 기반 요청/응답 | MCP 클라이언트 세션 기반 tool call |
| 대상 | 외부 자동화, curl, 서버 간 연동 | Cursor, Claude Code, Codex, Gemini CLI |
| 내부 의미 | queue/dispatch 엔진 노출 | 같은 queue/dispatch 엔진 노출 |

핵심은 transport가 달라도 runner의 역할은 동일하다는 점이다.
상위 진입점 도구가 여럿이어도 queue 작성, 조회, dispatch 정책은 하나의 runner가 공통으로 담당한다.

### 다중 진입점 운영

- process는 여전히 단일 오케스트레이터 역할을 가진다.
- MCP를 지원하는 각 도구는 process를 대체하지 않고 runner의 공통 transport를 호출한다.
- 따라서 `config.yaml` 의 layer 정의와 `AGENTS.md` 의 역할 구조는 유지된다.
- transport 선택은 통신 방식의 차이일 뿐, subprocess와 thread의 책임을 바꾸지 않는다.

### 실패 마커 정책

운영상 중요한 특성 하나가 있다.
태스크는 성공 여부와 무관하게 `tasks/queue/` 에서 제거된다.

그래서 모든 태스크는 `outputs/` 아래에 결과 마커 파일을 반드시 남겨야 한다.
권장 형식은 `outputs/result-<TASK_ID>.md` 이다.
이 마커는 다음 용도로 사용한다.

- 후속 태스크의 dependency 확인
- 실패 원인 추적과 재실행 판단
- queue 밖에서 완료/실패 이력 유지

실패 재시도는 queue 잔존으로 처리하지 않는다.
실패 마커를 확인한 뒤 새 태스크를 생성해 다시 실행한다.

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

subprocess는 thread layer를 직접 임의 호출하지 않고 runner wrapper 를 통해 호출해야 한다.
runner 는 run 시작 시 `out/thread-evidence/` 를 만들고, wrapper 실행마다 runner-generated evidence 를 기록한다.

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

따라서 process/reviewer 기본 읽기 순서는 `out/summary.md` -> runner-generated evidence -> 필요 시 `out/thread-log.md`, `out/result.md` 다.
reviewer 는 run 중에는 `out/thread-evidence/`, 실행 종료 후에는 `outputs/thread-evidence-<TASK_ID>/` 를 우선 보고 thread 집행 여부를 검증한다.

### MCP 사용 예시

아래 예시는 MCP 클라이언트가 runner의 tool surface를 통해 상태를 조회하고 태스크를 만드는 흐름을 보여준다.

1. 상위 도구가 `status` 를 호출해 runner 상태를 확인한다.
2. 상위 도구가 `list_tasks` 로 현재 queue를 조회한다.
3. 상위 도구가 `create_task` 로 새 태스크를 등록한다.
4. 실행이 끝나면 `outputs/result-*.md` 마커를 읽어 성공/실패를 판정한다.

이 흐름에서도 process는 작업 분해와 spawn 정책을 담당하고, MCP는 그 process가 runner와 통신하는 transport로만 동작한다.
