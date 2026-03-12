# subprocess-thread 정렬

## 목적

`subprocess` 레이어가 직접 작업을 끝내는 worker로 붕괴되지 않도록, `config.md` 의 레이어 계약을 실제 실행 모델에 반영한다. 현재 설정에서는 `subprocess=codex`, `thread=gemini` 이므로, `subprocess`는 `thread` 작업을 분해·호출·검수해야 한다.

## 문제 정의

- 문서상 의도는 이미 명확하다.
  - `config.md`: `subprocess=codex`, `thread=gemini`
  - `designs/CODEX_GEMINI_LOOP.md`: `subprocess`가 `thread`를 호출하고 검수해야 한다
- 하지만 실제 수동 실행에서는 `subprocess`가 thread 호출 없이 직접 작업을 끝낼 수 있었다.
- 이 상태가 반복되면 `thread` 레이어는 형식만 남고, 실제 실행은 `process -> subprocess` 2계층으로 무너진다.

## 목표 상태

### 1. subprocess는 위임 범위를 관리한다

- subprocess는 task를 읽고 바로 코드를 끝내는 worker가 아니다.
- subprocess는 받은 task를 thread가 처리하기 쉬운 단위로 더 쪼개거나, 최소한 thread 호출 지시를 구성한다.
- subprocess는 thread 실행 결과를 검수하고, 부족하면 피드백을 넣어 다시 thread를 호출한다.

### 2. thread 실행은 config 기반이다

- `thread` 바이너리는 하드코딩하지 않는다.
- subprocess는 `config.md` 또는 runner config에서 현재 thread layer를 읽고 해당 adapter를 사용한다.
- 현재 기본값은 `gemini` 이므로 지금 구조에서는 `gemini`가 호출되어야 한다.

### 3. 실행 흔적이 남아야 한다

- thread가 실제 호출됐는지 `outputs/` 또는 run `out/`에 남아야 한다.
- reviewer는 “산출물이 괜찮다” 뿐 아니라 “config에 맞는 thread 레이어가 실제 집행됐는가”도 검증해야 한다.

## 필요한 구조

### A. subprocess용 실행 계약 강화

- `SUBPROCESS_INSTRUCTION.md` 는 특정 도구명이 아니라 `subprocess` 레이어 계약을 담아야 한다.
- instruction은 단순히 “작업하라”가 아니라 다음을 명시해야 한다.
  - config의 thread layer를 확인하라
  - thread adapter를 사용해 실제 호출하라
  - 필요 시 thread task를 더 작은 단위로 분해하라
  - 결과 검수 후 미흡하면 재호출하라

### B. thread용 입력 정리

- thread가 잘 일하게 하려면 task를 thread 친화적으로 만들어야 한다.
- 따라서 subprocess는 다음을 해야 한다.
  - role/task에서 thread에 필요한 부분만 추출
  - `direction.md`, conventions, design ref를 정리
  - 필요하면 thread별 소작업으로 나눔

### C. 호출 로그 / 마커

- 예:
  - `out/thread-log.md`
  - `outputs/thread-<TASK_ID>.md`
- 최소 기록:
  - thread binary
  - adapter
  - 호출 횟수
  - 재호출 여부
  - 시도별 메모(선택)
- 권장 동작:
  - runner가 run 시작 시 `out/thread-log.md` 템플릿을 만든다
  - subprocess는 thread 호출 때마다 해당 파일을 갱신한다
  - runner는 실행 종료 시 `out/thread-log.md` 를 `outputs/thread-<TASK_ID>.md` 로 복사해 reviewer가 사후 확인할 수 있게 한다

## 설계 원칙

- `process`와 `subprocess`는 모두 분해·배분·검토를 할 수 있다. 차이는 책임 범위다.
- `subprocess` 는 위임받은 범위를 관리하고, `thread` 는 leaf worker 역할을 유지한다.
- `thread` 바이너리는 config 기반으로 선택한다.
- 강제 대상은 특정 제품명이 아니라 “현재 config가 지정한 마지막 레이어”다.
- 품질 기준은 산출물뿐 아니라 레이어 계약 준수까지 포함한다.

## 태스크 목록

| TASK_ID | 제목 | Role | Depends On | Execution |
|---------|------|------|------------|-----------|
| T-LOOP-001 | subprocess 계약 문서 및 adapter 지시 정리 | worker_doc | [] | parallel |
| T-LOOP-003 | thread 호출 흔적 마커/로그 규약 추가 | worker_code | [T-LOOP-001] | parallel |
| T-LOOP-005 | subprocess 실행 루프 정렬 | worker_code | [T-LOOP-001, T-LOOP-003] | sequential |
| T-LOOP-006 | subprocess-thread 실행 루프 검증 | reviewer | [T-LOOP-005] | sequential |
| T-LOOP-007 | subprocess-thread 실실행 증거 남기기 | worker_code | [T-LOOP-005, T-LOOP-006] | sequential |
| T-LOOP-008 | subprocess-thread 재검증 | reviewer | [T-LOOP-007] | sequential |

## 완료 조건

- [ ] subprocess가 thread 분해·호출·검수 역할을 수행해야 한다는 계약이 문서와 instruction에 반영된다
- [ ] thread layer는 config 기반으로 결정된다
- [ ] thread 호출 흔적이 outputs 또는 out에 남는다
- [ ] reviewer가 레이어 계약 준수를 검증한다

## 태스크 재정렬 메모

- `T-LOOP-002`, `T-LOOP-004` 는 남은 작업을 너무 잘게 잘라 `thread` 수준처럼 보이게 만든 점이 있었다.
- 후속 작업은 `subprocess` 기준으로 다시 묶어 `T-LOOP-005`, `T-LOOP-006` 으로 재정렬한다.
- `T-LOOP-005` 는 instruction, review loop, thread 흔적 활용을 한 번의 `subprocess` 태스크로 정렬한다.
- `T-LOOP-006` 는 그 결과가 실제로 `subprocess -> thread` 실행 루프로 검증 가능한지 확인한다.
- `T-LOOP-007` 은 이미 정렬된 계약을 바탕으로 실제 `thread` 호출 흔적과 reviewer 확인 경로를 남기는 재실행 태스크다.
- `T-LOOP-008` 은 `T-LOOP-007` 산출물로 실제 실행 증거가 남았는지 다시 검증한다.

## 현재 상태

- `T-LOOP-005` 에서 subprocess instruction 과 review loop 계약 정리는 됐다.
- `T-LOOP-007` 에서 thread 호출 주장과 흔적은 남겼다.
- 하지만 `T-LOOP-008` reviewer 기준으로 현재 흔적은 모두 subprocess 자기보고 markdown 이라 독립 증거로는 부족하다.
- 후속 실전 작업은 `designs/thread-execution-evidence.md` 의 runner-generated evidence pipeline 으로 이어진다.
