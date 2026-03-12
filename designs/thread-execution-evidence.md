# thread execution evidence pipeline

Go runner 가 thread 실행 증거를 자동 생성하고 reviewer 가 그 증거를 우선 검토하도록 정리한다.
대상은 `process -> subprocess -> thread` 구조를 유지한 채 thread 호출 경로와 outputs 보존 정책을 명확히 하는 것이다.
핵심 결론은 markdown 자기보고는 보조 요약으로만 두고 runner-generated evidence 를 원본으로 삼는다는 점이다.

## 목적

`subprocess` 와 `reviewer` 가 자기보고 markdown 만으로 thread 실행을 주장하는 상태를 끝내고, Go runner가 독립적인 실행 증거를 남기도록 한다.

핵심은 다음이다.

- thread 실행 증거는 LLM이 작성한 문서가 아니라 시스템이 자동 생성해야 한다.
- reviewer 는 `out/thread-log.md` 같은 자기보고 요약보다 runner-generated evidence 를 우선 본다.
- 현재 `subprocess=codex`, `thread=gemini` 이지만 설계는 특정 제품명이 아니라 config 기반 layer 에 붙는다.

## 왜 markdown 자기보고만으로는 부족한가

- `out/thread-log.md` 와 비슷한 문서는 작성 주체가 `subprocess` 이므로 실행 사실과 동일한 신뢰 수준을 갖지 못한다.
- 실제 thread command, 종료 코드, stdout/stderr 경로가 없으면 reviewer 는 "호출했다"는 주장만 보고 통과시키게 된다.
- 재호출이 있었는지, 어떤 adapter 가 사용됐는지, config 와 일치하는 layer 가 집행됐는지를 자기보고만으로는 검증하기 어렵다.
- queue 에서 task 가 제거된 뒤에도 남는 것은 `outputs/` 인데, 자기보고만 보존되면 사후 검증 시 원본 실행 흔적이 사라진다.

## 현재 한계

- runner 는 `subprocess` 만 직접 실행한다.
- `thread` 실행은 `subprocess` 내부에서 일어나므로, 현재 runner 는 실제 thread command, exit code, stdout, stderr 를 알지 못한다.
- `out/thread-log.md` 는 runner 가 템플릿만 만들고 실제 내용은 `subprocess` 가 채우므로 독립 증거가 아니다.

## 목표 상태

### 1. runner-generated evidence

run 폴더 아래에 runner 가 고정 포맷 evidence 경로를 준비한다.

예:

- `out/thread-evidence/`
- `out/thread-evidence/invocations.jsonl`
- `out/thread-evidence/stdout/`
- `out/thread-evidence/stderr/`

최소 기록 항목:

- `invocation_id`
- `task_id`
- `run_id`
- `thread_layer`
- `thread_binary`
- `adapter_path`
- `argv` 또는 `rendered_command`
- `started_at`
- `finished_at`
- `duration_ms`
- `exit_code`
- `stdout_path`
- `stderr_path`

선택 필드:

- `attempt`
- `feedback_source`
- `workspace`

최소 필드만으로도 reviewer 는 "어떤 layer 가 어떤 명령으로 실제 실행됐는지"를 재구성할 수 있어야 한다.

### 2. subprocess 는 runner wrapper 를 통해 thread 를 호출한다

독립 증거를 남기려면 `subprocess` 가 thread 를 직접 임의 호출하지 말고, runner 가 만든 wrapper 또는 helper 를 통해 호출해야 한다.

예:

- `out/thread-evidence/invoke-thread`
- `agentos-runner thread invoke ...`

이 wrapper 가 실제 adapter command 실행 전후를 기록한다.

wrapper 동작 원칙:

- 현재 config 의 `layers.thread` 값을 읽어 thread layer 를 결정한다.
- 대응하는 `adapters/<thread>.md` 를 사용해 실제 command 를 렌더링한다.
- 실행 직전 invocation 레코드를 생성하고, 종료 후 exit code 와 stdout/stderr 경로를 갱신한다.
- `subprocess` 는 직접 shell command 를 조립하지 않고 wrapper 에 task 입력과 direction 만 전달한다.

### 3. reviewer 확인 경로 고정

reviewer 는 다음 순서로 evidence 를 본다.

1. `out/thread-evidence/invocations.jsonl` 또는 outputs 로 복사된 동등 파일
2. 해당 invocation 이 가리키는 stdout/stderr capture
3. 그 다음에 `out/thread-log.md` 와 `out/result.md`

즉 markdown 은 요약이고, runner evidence 가 원본이 된다.

reviewer 체크포인트:

- `thread_layer` 와 `adapter_path` 가 현재 config 와 일치하는가
- 최소 한 번 이상 invocation 이 기록됐는가
- 실패 또는 재시도가 있었다면 마지막 성공/실패 상태가 분명한가
- markdown 요약이 runner-generated evidence 와 모순되지 않는가

## 설계 방향

### A. 최소 침습

- 기존 `process -> subprocess -> thread` 계층은 유지한다.
- `subprocess` 의 역할은 그대로 둔다.
- 다만 thread 호출 경로를 runner-generated wrapper 로 통일한다.

### B. config 기반 유지

- wrapper 는 `cfg.Layers.Thread` 와 `adapters/<thread>.md` 를 사용한다.
- 특정 도구명 하드코딩은 금지한다.

### C. outputs 보존

- queue 에서 task 가 사라져도 검증 가능해야 한다.
- 실행 종료 후 `out/thread-evidence/` 의 필수 산출물을 `outputs/` 로 복사한다.

권장 보존 형태:

- `outputs/thread-evidence-<TASK_ID>/invocations.jsonl`
- `outputs/thread-evidence-<TASK_ID>/stdout/<invocation_id>.log`
- `outputs/thread-evidence-<TASK_ID>/stderr/<invocation_id>.log`
- `outputs/thread-<TASK_ID>.md` 는 사람이 읽는 요약 마커로 유지

운영 원칙:

- reviewer 와 후속 태스크는 `outputs/thread-evidence-<TASK_ID>/` 만으로도 사후 검증이 가능해야 한다.
- 요약 마커는 남기되, evidence 원본을 대체하지 않는다.
- 저장 비용이 크면 전체 stdout/stderr 대신 압축 또는 일부 보존 정책을 둘 수 있지만 `invocations.jsonl` 은 항상 남긴다.

## 태스크 목록

| TASK_ID | 제목 | Role | Depends On | Execution |
|---------|------|------|------------|-----------|
| T-EVID-001 | thread evidence pipeline 설계/문서 정리 | worker_doc | [] | parallel |
| T-EVID-002 | runner wrapper 기반 thread evidence 구현 | worker_code | [T-EVID-001] | sequential |
| T-EVID-003 | subprocess instruction을 wrapper 사용 기준으로 정렬 | worker_code | [T-EVID-002] | sequential |
| T-EVID-004 | runner-generated thread evidence 검증 | reviewer | [T-EVID-002, T-EVID-003] | sequential |

## 완료 조건

- [ ] runner 가 독립적인 thread 실행 evidence 를 남긴다
- [ ] subprocess 는 runner wrapper 를 통해 thread 를 호출한다
- [ ] reviewer 는 runner evidence 로 실제 thread 실행을 확인할 수 있다
- [ ] outputs 에서 사후 검증이 가능하다
