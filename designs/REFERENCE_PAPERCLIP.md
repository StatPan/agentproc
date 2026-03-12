# Paperclip 레퍼런스 — AgentOS 도입 개념

## 1. AgentOS 의도 (컴공 시스템 대비)

AgentOS는 **컴퓨터공학 시스템에 맞춘** 세팅이다.

- **프로세스처럼** — 태스크 하나 = 실행 단위 하나; 시작·종료·상태가 명확하다.
- **멀티프로세스처럼** — 여러 태스크가 동시에 진행될 수 있다 (병렬 워커).
- **재시작 가능** — 세션/프로세스가 끊겨도 queue·상태만 보면 이어서 돌릴 수 있다.

형태를 발전시키기 위해 **Paperclip의 “회사” 개념**을 레퍼런스로 둔다.  
Paperclip은 “OpenClaw가 직원이면 Paperclip은 회사”라고 정의하며, 한 인스턴스에서 **여러 회사**를 돌리고, 회사 단위로 **에이전트(직원)·목표·예산·태스크 계층**을 관리한다.

---

## 2. Paperclip에서 가져온 개념 요약

### 2.1 Company (회사)

- **최상위 단위.** 한 Paperclip 인스턴스가 여러 Company를 가짐.
- 회사마다: **goal**, **employees(에이전트)**, **org structure**, **budget**, **task hierarchy**.
- DB: `companies` 테이블 — id, name, description, status, issuePrefix, budgetMonthlyCents, spentMonthlyCents 등.

### 2.2 Agents (에이전트 = 직원)

- **Adapter type + config** — 어떻게 실행하는지 (Claude Code, Codex, process, HTTP).
- **Role & reporting** — 직책, 누구에게 보고하는지 (트리 구조, CEO 제외 1명의 매니저).
- **Budget** — 에이전트별 월 예산.
- **Status** — active, idle, running, error, paused, terminated.
- **Heartbeat policy** — `maxConcurrentRuns`(동시 실행 수), `intervalSec`, `wakeOnAssignment` 등.

### 2.3 Issues (태스크)

- **단위 작업.** title, description, status, priority, assignee(한 번에 한 에이전트), parent issue(목표까지 계층).
- **상태 흐름:** backlog → todo → **in_progress** → in_review → done (또는 blocked).
- **in_progress 전환 = atomic checkout** — 동시에 한 에이전트만 소유. 동시에 claim 시 409 Conflict.

### 2.4 Heartbeats (심박 = 실행 창구)

- 에이전트는 **상시 가동이 아니라** “심박” 단위로 깨어남.
- 트리거: **Schedule(timer)**, **Assignment**, **On-demand**, **Automation**.
- 한 번 깨어나면: adapter 실행 → 컨텍스트 전달 → 완료/타임아웃/취소까지 실행 → 결과 저장.
- **큐 semantics:** DB 기반 queue, FIFO, `maxConcurrentRuns`만큼 동시 run. `claimQueuedRun`으로 atomic하게 선점 후 `executeRun`.

### 2.5 Governance (거버넌스)

- Board(사람) 승인: 신규 에이전트 채용, CEO 전략 등.
- Board 권한: 예산 수정, 에이전트 pause/terminate, 태스크 재배정, 감사 로그.

---

## 3. 코드/아키텍처에서 본 것 (요약)

| 개념 | Paperclip 쪽 구현 |
|------|-------------------|
| Company | `packages/db/src/schema/companies.ts`, `shared/src/types/company.ts` |
| Agent 실행 | `server/src/services/heartbeat.ts` — `startNextQueuedRunForAgent`, `claimQueuedRun`, `executeRun` |
| 병렬 run | `maxConcurrentRuns`, `availableSlots = maxConcurrentRuns - runningCount`, queued run 여러 개 claim 후 동시 `executeRun` |
| Atomic checkout | 이슈의 in_progress 전환 시 단일 assignee; 409 Conflict로 동시 claim 방지 |
| Adapter | claude_local, codex_local, process, http — 실행 방식만 다르고 control plane은 동일 |

---

## 4. AgentOS에 도입할 개념 매핑

AgentOS는 **파일 기반**이므로 DB 대신 디렉터리/파일로 대응한다.

| Paperclip 개념 | AgentOS 대응 |
|----------------|--------------|
| **Company** | (선택) 한 “프로젝트/미션” 단위 디렉터리. 예: `AgentOS/` 자체가 한 “회사” 또는 `AgentOS/companies/{name}/` 로 다중 “회사”. 현재는 단일 루트로 두고, 나중에 `companies/` 확장 가능. |
| **Agent** | **Role** (designer, orchestrator, worker_*, reviewer) + **Assigned To** (claude-code, gemini-cli, jules 등). 역할별 .md는 “직책 정의서”. |
| **Issue/Task** | **tasks/queue, running, done, failed, escalate** 의 TASK_*.md. 한 태스크 = 한 파일. |
| **Atomic checkout** | **한 태스크를 running으로 옮길 때 해당 태스크만 이동 + {TASK_ID}.lock** 생성. 동일 리소스(파일) 건드리는 다른 태스크는 queue에 유지. (동시 소유 방지) |
| **Heartbeat** | **ORCHESTRATOR 실행 주기.** “queue 스캔 → 의존성/충돌 확인 → running 이동 + lock → 디스패치”. 세션 끊겨도 다음에 queue만 다시 보면 “다음 심박”으로 재시작. |
| **병렬 워커** | **동시에 여러 태스크를 running으로** 보낼 수 있음. 단, (1) 서로 다른 파일/리소스를 쓰는 태스크만, (2) 역할별로 “동시 실행 상한”을 두려면 ORCHESTRATOR에서 running 개수 제한 (선택). |
| **Adapter** | **Assigned To** 값이 곧 “어떤 실행체를 쓸지” (gemini-cli, claude-code, jules). 각 WORKER_*.md에 “기본 실행 도구” 명시. |
| **Budget/Governance** | 현재 AgentOS에는 미구현. 도입 시: 태스크별/역할별 “비용 상한” 메모 필드, 또는 사람 승인(escalate)으로 대체. |

---

## 5. Sub-agent / Gemini 활용 제안

- **Sub-agent:** ORCHESTRATOR가 “이 태스크는 worker_doc, Assigned To = gemini-cli”로 정했을 때, 실제 실행을 **서브 에이전트**에게 넘기면, 그 서브 에이전트는 `roles/WORKER_DOC.md` + 해당 태스크 파일만 읽고 실행하도록 설계할 수 있음.
- **Gemini binary:** 문서/리서치 같은 반복 작업은 `gemini` CLI로 실행해 비용을 줄이고, 코드/설계는 Claude/Codex에 맡기는 식으로 역할을 나누면 Paperclip의 “에이전트별 adapter + budget”과 유사한 효과를 파일 기반으로 낼 수 있음.

---

## 6. 참고 경로 (Paperclip 코드베이스)

- `README.md` — “company”, “orchestration”, “heartbeats”
- `docs/start/core-concepts.md` — Company, Agents, Issues, Heartbeats, Governance
- `docs/start/what-is-paperclip.md` — control plane vs execution (adapters)
- `doc/spec/agents-runtime.md`, `doc/spec/agent-runs.md` — heartbeat queue, enqueueWakeup, maxConcurrentRuns
- `server/src/services/heartbeat.ts` — `claimQueuedRun`, `startNextQueuedRunForAgent`, `executeRun`
- `packages/db/src/schema/companies.ts`, `packages/shared/src/types/company.ts` — company 모델

이 문서는 Designer가 “회사 단위 확장”이나 “병렬/심박/atomic checkout”을 설계할 때 참고하는 레퍼런스다.
