# Runner Dispatch Retrigger 버그 수정

## 목적

runner가 dependency 기반 sequential task를 자동으로 이어서 실행하지 못하는 버그를 수정한다.

## 문제 요약

- 현재 runner는 시작 시점 또는 `tasks/queue/` 에 새 파일이 생성될 때만 `dispatchCycle` 을 돈다.
- parallel 또는 선행 sequential task가 끝나 `outputs/` 마커가 생겨도, 기존 queue에 남아 있던 후속 task를 다시 평가하지 않는다.
- 그 결과 dependency는 충족됐지만 queue에 남아 있는 task가 영구 대기 상태가 된다.

## 재현 시나리오

1. `T-A` 는 `parallel`, output 마커를 남긴다.
2. `T-B` 는 `Depends On: [T-A]`, `Execution: sequential` 이다.
3. runner 시작 시 `T-B` 는 dependency 미충족으로 skip 된다.
4. 이후 `T-A` 가 완료돼도 `T-B` 는 다시 dispatch 되지 않는다.

## 접근 방법

### 1. 재평가 트리거 추가

- task 완료 직후 `dispatchCycle` 을 다시 실행할 수 있어야 한다.
- 또는 `outputs/` / `.run/` 상태 변화도 감지해 queue를 재평가해야 한다.
- 핵심은 dependency 상태가 바뀌는 시점에 기존 queue를 다시 훑는 것이다.

### 2. 동시성 안전성 유지

- 이미 실행 중인 task를 중복 실행하면 안 된다.
- 기존 `runningSet` 과 semaphore 정책은 유지한다.
- 재평가가 여러 번 일어나도 같은 task는 한 번만 시작되어야 한다.

### 3. 회귀 방지 테스트 추가

- dependency가 처음엔 미충족이지만, 선행 task 완료 후 후속 task가 실행되는 테스트를 추가한다.
- queue 생성 이벤트가 없어도 후속 sequential task가 dispatch 되는지 검증한다.

## 태스크 목록

| TASK_ID | 제목 | Role | Depends On | Execution |
|---------|------|------|------------|-----------|
| T-RUN-001 | dispatch retrigger 버그 수정 | worker_code | [] | parallel |
| T-RUN-002 | sequential dependency 회귀 테스트 추가 | worker_code | [T-RUN-001] | sequential |
| T-RUN-003 | 수정 사항 검증 및 운영 메모 작성 | reviewer | [T-RUN-001, T-RUN-002] | sequential |

## 완료 조건

- [ ] dependency 충족 후 기존 queue 항목이 재평가된다
- [ ] 후속 sequential task가 새 queue 파일 생성 없이 자동 실행된다
- [ ] 중복 실행이 발생하지 않는다
- [ ] 회귀 테스트가 추가되고 통과한다
