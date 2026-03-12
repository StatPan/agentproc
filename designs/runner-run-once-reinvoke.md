# Runner Run-Once + Reinvoke 전환 설계

## 목적

runner를 장시간 살아 있는 내부 이벤트 루프 중심 구조에서, 한 번 실행 후 종료하는 `run once` 구조로 옮긴다. 다음 dispatch는 같은 프로세스 내부 콜백이 아니라 상위 process 또는 얇은 외부 dispatcher가 새 프로세스를 다시 호출해 이어간다.

## 배경

- AgentOS의 기본 철학은 `process -> subprocess -> thread` 로 이어지는 짧고 격리된 실행 단위다.
- 현재 runner는 HTTP 서버와 fsnotify watcher, 내부 dispatch retrigger를 통해 메모리 안에서 상태 전이를 이어간다.
- 이 방식은 실용적이지만, dependency 충족 후 재평가 같은 흐름이 runner 내부 state machine에 묶인다.
- 사용자 관점에서 더 설득력 있는 모델은 `새 프로세스 재호출` 이다.

## 목표 상태

### 1. runner는 1회 평가기다

- 한 번 실행되면 현재 `tasks/queue/` 와 `outputs/` 를 기준으로 실행 가능한 task를 계산한다.
- 이번 invocation에서 처리할 task만 spawn 한다.
- 필요한 output 복사와 queue 제거까지 마치면 종료한다.

### 2. 다음 단계는 새 프로세스로 이어진다

- task 완료 후 같은 runner가 내부적으로 다음 queue를 계속 소비하지 않는다.
- 상위 process 또는 얇은 dispatcher가 runner를 다시 실행한다.
- dependency 충족 여부는 메모리 이벤트가 아니라 파일 상태(`outputs/`, `tasks/queue/`)로 재계산한다.

### 3. transport와 dispatch를 분리한다

- `http`, `mcp` 는 외부 진입 transport다.
- 실제 dispatch는 별도 `run once` 경로로 분리한다.
- transport 서버는 필요할 때 runner를 subprocess로 1회 호출하거나, queue 작성 후 process에게 다음 실행을 위임한다.

## 제안 구조

### A. 실행 모드 분리

- `serve http`
  - HTTP transport만 제공한다.
  - 직접 긴 dispatch loop를 들고 있지 않는다.
- `serve mcp`
  - MCP transport만 제공한다.
  - task 생성, 상태 조회 같은 tool surface만 제공한다.
- `dispatch once`
  - queue를 한 번 평가하고 실행 가능한 task를 처리한 뒤 종료한다.

CLI 표면은 예를 들어 다음처럼 둘 수 있다.

```text
agentos-runner --transport http
agentos-runner --transport mcp
agentos-runner --dispatch once
```

또는 subcommand 기반으로 옮겨도 된다.

```text
agentos-runner serve --transport http
agentos-runner serve --transport mcp
agentos-runner dispatch once
```

핵심은 `serve` 와 `dispatch` 를 분리하는 것이다.

### B. 재호출 정책

- task 완료 후 상위 process가 `dispatch once` 를 다시 실행한다.
- 병렬 task가 여러 개라면 completion마다 재호출할 수 있고, 중복 호출은 파일 상태 기준으로 자연스럽게 수렴한다.
- 또는 얇은 외부 dispatcher가 debounce를 둘 수 있다.

### C. 진실 공급원

- 실행 가능 여부 판단은 매 invocation마다 파일 시스템에서 다시 계산한다.
- `tasks/queue/` 에 task가 있는가
- `outputs/result-*.md` 같은 dependency 산출물이 있는가
- `.run/` 에 이미 동일 task가 실행 중인가

## 단계적 전환

### 1단계. `dispatch once` 경로 추가

- 기존 서버 코드와 공존하게 `RunDispatchOnce(...)` 같은 경로를 도입한다.
- 현재 `dispatchCycle` 에 있는 task 선택/실행 로직을 1회 실행 함수로 정리한다.

### 2단계. 내부 retrigger 의존도 제거

- `T-RUN-001` 에서 넣은 내부 dispatch retrigger는 하위 호환 안전장치로 유지한다.
- 하지만 새 경로에서는 completion callback이 다시 자기 자신을 깨우지 않도록 한다.

### 3단계. transport에서 재호출 전략 연결

- HTTP/MCP 서버는 queue 작성 또는 task 완료 후 상위 process가 runner를 재호출하도록 문서화한다.
- 필요하면 아주 얇은 wrapper script를 둔다.

### 4단계. 운영 문서와 검증 갱신

- `designs/WORK_LOOP.md` 와 `runner/README.md` 를 새 모델과 일치시킨다.
- queue 소멸 정책과 output marker 정책을 함께 명시한다.

## 장점

- AgentOS의 프로세스 모델과 더 잘 맞는다.
- 내부 채널, coalescing, busy loop 같은 메모리 기반 오케스트레이션 복잡도가 줄어든다.
- 재현과 디버깅이 쉬워진다. 각 invocation이 독립적이기 때문이다.
- transport와 dispatch 책임이 분리된다.

## 리스크

- 외부 재호출 주체가 필요하다.
- `dispatch once` 가 병렬 task를 어디까지 같은 invocation에서 시작할지 정책을 정해야 한다.
- 중복 invocation이 동시에 들어와도 같은 task가 중복 실행되지 않도록 `.run/` 또는 락 정책을 명확히 해야 한다.

## 권장 정책

- 한 invocation에서는 "현재 즉시 실행 가능한 task들"만 시작하고 종료한다.
- dependency가 새로 충족되어 생기는 후속 task는 다음 invocation에서 처리한다.
- 중복 방지는 `.run/` 스캔 + 원자적 run directory 생성으로 보강한다.

## 태스크 목록

| TASK_ID | 제목 | Role | Depends On | Execution |
|---------|------|------|------------|-----------|
| T-RUN-004 | run-once dispatch 설계 반영 및 CLI 경로 추가 | worker_code | [] | parallel |
| T-RUN-005 | transport/server와 dispatch 책임 분리 | worker_code | [T-RUN-004] | sequential |
| T-RUN-006 | run-once 재호출 모델 문서화 | worker_doc | [T-RUN-004, T-RUN-005] | parallel |
| T-RUN-007 | run-once 재호출 모델 통합 검증 | reviewer | [T-RUN-004, T-RUN-005, T-RUN-006] | sequential |

## 완료 조건

- [ ] runner가 `dispatch once` 경로를 제공한다
- [ ] dependency 전이는 새 프로세스 재호출로 처리할 수 있다
- [ ] transport와 dispatch 책임이 분리된다
- [ ] 운영 문서가 새 모델과 일치한다
- [ ] 통합 검증 결과가 `outputs/` 에 남는다
