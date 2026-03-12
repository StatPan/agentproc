# AgentOS Runner — Go 구현 설계

## 목적

AgentOS의 "커널" 역할을 코드로 구현한다.
현재 모델이 직접 수행하는 격리 폴더 생성 → subprocess spawn → 결과 복사 → 폴더 삭제를 자동화한다.

## 코드 위치

```
analysis/
├── AgentOS/        ← 기존 (정의·태스크·설계)
└── runner/         ← 신규 Go 프로젝트
    ├── main.go
    ├── config.go
    ├── task.go
    ├── adapter.go
    ├── runner.go
    ├── config.yaml
    └── go.mod
```

## 실행 모드

**daemon** — 기동 후 queue를 주기적으로 polling. 태스크 감지 시 자동 spawn.

## config.yaml 스펙

```yaml
agentos_root: ../AgentOS   # AgentOS 폴더 경로 (상대 또는 절대)
layers:
  subprocess: codex
  thread: gemini
runner:
  max_concurrent: 0         # 0 = runtime.NumCPU()
  poll_interval: 5s
  mode: daemon
```

## 핵심 동작 흐름

```
[main loop]
  poll tasks/queue/*.md
    → 실행 가능한 태스크 선택
        · Execution: parallel  → goroutine으로 동시 spawn
        · Execution: sequential → Depends On Output 파일 존재 확인 후 순서대로
    → 각 태스크마다:
        1. tasks/.run/{run_id}/ 생성
        2. role.md, task.md, direction.md, SUBPROCESS_INSTRUCTION.md 복사
        3. out/ 빈 폴더 생성
        4. adapter 명령어 템플릿 로딩 → subprocess spawn
        5. 완료 대기
        6. out/ → outputs/ 복사
        7. tasks/.run/{run_id}/ 삭제
        8. queue에서 태스크 파일 제거
```

## 파일별 책임

| 파일 | 역할 |
|------|------|
| `main.go` | 진입점. config 로드, daemon loop 시작 |
| `config.go` | config.yaml 파싱. Layer → 바이너리 매핑 |
| `task.go` | task.md 파싱. `## Key: value` 형식 처리 |
| `adapter.go` | adapters/*.md 에서 agentic command 템플릿 추출 |
| `runner.go` | 격리 폴더 생성, subprocess exec, 결과 복사, 정리 |

## 의존성 체크 방법

`Depends On: [T-002]` 인 경우:
- T-002의 task.md Output 필드에 명시된 파일이 `outputs/` 에 존재하면 → 완료로 간주
- 없으면 → 이번 사이클 스킵

## 동시 실행 제어

```go
sem := make(chan struct{}, maxConcurrent)  // semaphore
// parallel 태스크마다 goroutine + semaphore로 상한 제어
```

## Go 모듈

```
module agentos-runner
go 1.22
```

외부 의존성 최소화. yaml 파싱은 `gopkg.in/yaml.v3` 만 사용.

## 태스크 목록

| TASK_ID | 제목 | Execution |
|---------|------|-----------|
| T-010 | Go 모듈 초기화 + 프로젝트 스켈레톤 | parallel |
| T-011 | config.yaml + adapter 파서 | sequential (T-010 후) |
| T-012 | task.md 파서 | sequential (T-010 후, T-011과 병렬) |
| T-013 | runner 코어 (격리 폴더 + subprocess exec) | sequential (T-011, T-012 후) |
| T-014 | main daemon loop (queue polling + dispatch) | sequential (T-013 후) |
