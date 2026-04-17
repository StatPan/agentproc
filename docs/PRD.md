# PRD: aproc — Agent Work Session Dashboard

## 1. 문제 정의

AI 코딩 에이전트(Claude, Codex, Gemini 등)를 사용하는 개발자는 다음 문제를 겪는다:

1. **세션 휘발** — 에이전트 세션이 끊기면 작업 맥락이 사라진다. 다시 설명하는 데 수 분~십 수 분 소요.
2. **런타임 Lock-in** — 하나의 에이전트에 종속되면, 그 서비스가 다운될 때 작업 자체가 중단된다.
3. **작업 가시성 부재** — 에이전트가 무엇을 하고 있는지, 어디서 막혔는지 한눈에 볼 수 없다.
4. **병렬 작업 불가** — 여러 에이전트를 동시에 돌리려면 직접 스크립팅해야 한다.

## 2. 타겟 사용자

**1차: 개인 개발자 (파워유저)**
- AI 코딩 에이전트를 일상적으로 사용
- 런타임 전환 필요성을 체감 (서비스 장애, 모델별 강점 차이)
- 팀 설득 없이 혼자 설치해서 바로 가치를 느낄 수 있어야 함

**2차 (후순위): 팀**
- 개인 가치 증명 후 팀 확장
- 에이전트 작업 공유, 리뷰, 실시간 협업

## 3. 핵심 가치 (한 문장)

> 에이전트는 도구, aproc은 작업대 — 어떤 에이전트를 쓰든, 작업 맥락은 하나로 이어진다.

## 4. 사용자 시나리오

### 시나리오 B (킬러): 런타임 전환

```
1. 개발자가 aproc으로 작업 세션을 시작한다
2. opencode + Claude 모델로 설계 토론을 한다
3. Claude API가 다운된다
4. opencode 프로바이더를 Gemini로 전환한다
5. 작업 맥락(goal, decisions, 변경 파일)이 그대로 유지된다
6. 0분 만에 이어서 작업한다
```

### 시나리오 A: 세션 연속성

```
1. opencode에서 30분 작업 후 세션이 끊긴다
2. aproc이 보관한 작업 맥락으로 새 세션을 시작한다
3. 다시 설명할 필요 없이 이어서 작업한다
```

### 시나리오 D: 병렬 작업

```
1. 하나의 작업을 여러 task로 분할한다
2. 각 task가 독립적으로 실행된다
3. 대시보드에서 모든 task의 진행 상황을 실시간으로 본다
```

## 5. 인터페이스 방향

### 원칙
- CLI 1회성 명령이 아닌, **opencode의 연속 세션** 기반
- 작업은 **분할된 task 단위**로 실행/관찰 가능
- **Figma 스타일** 시각화 — task 카드 + 실시간 로그 스트림

### 대시보드 구성

```
┌─────────────────────────────────────────────────┐
│  aproc dashboard                                │
│                                                  │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐        │
│  │ Task 1   │ │ Task 2   │ │ Task 3   │        │
│  │ auth API │ │ tests    │ │ docs     │        │
│  │ ●running │ │ ◐blocked │ │ ✓done    │        │
│  │ [logs ↓] │ │ [logs ↓] │ │ [logs ↓] │        │
│  └──────────┘ └──────────┘ └──────────┘        │
│                                                  │
│  실시간 로그 스트림 (선택한 task)                  │
│  [14:58:48] 🧠 reasoning: ...                    │
│  [14:58:48] 💬 text: ...                         │
│  [14:58:49] 🔧 tool: bash (completed)            │
└─────────────────────────────────────────────────┘
```

## 6. 아키텍처

### 얕은 연동 (Supabase 컨테이너 모델)

```
aproc-dashboard (web UI)
    ↕ WebSocket
aproc-server (orchestration + polling)
    ↕ SQLite read-only polling (0.5s)
opencode (실행 엔진, 수정 안 함)
    → ~/.local/share/opencode/opencode.db (WAL mode)
```

### 설계 결정

| 결정 | 이유 |
|------|------|
| opencode 포크 안 함 | 커밋 하루 5+, 스키마 변경 빈번 → 유지보수 비용 과다 |
| SQLite 그대로 유지 | sync 트랜잭션(NotPromise<T>) 호환 문제 회피 |
| 외부 폴링으로 관찰 | WAL 모드로 read-only 동시 접근 가능 |
| PG는 aproc 레이어에서만 | 대시보드/협업용 정규화 데이터는 별도 관리 |

## 7. opencode DB 구조 (검증 완료)

경로: `~/.local/share/opencode/opencode.db`

| 테이블 | 용도 | 건수 |
|--------|------|------|
| session | 작업 세션 | 39 |
| message | 대화 메시지 (role, model, summary) | 2,144 |
| part | 메시지 세부 — 대시보드 핵심 데이터 소스 | 7,854 |
| todo | 작업 항목 (status, priority) | 46 |
| project | 프로젝트 (worktree, name) | 3 |
| event | 이벤트 소싱용 (스키마만 존재) | 0 |

### Part types (= 실시간 로그 원천)

| type | 설명 |
|------|------|
| text | 텍스트 응답 |
| tool | 도구 호출 (bash, file read/edit 등) |
| patch | 코드 변경 |
| reasoning | 추론 과정 |
| step-start | 단계 시작 |
| step-finish | 단계 종료 |
| compaction | 컨텍스트 압축 |

## 8. 기술 검증 결과

**SQLite 폴링 PoC** (`tools/poll_opencode.py`)

- ✅ read-only 모드로 WAL 충돌 없이 접근
- ✅ part 7종 타입 전부 캡처
- ✅ session title 연결로 세션 구분 가능
- ✅ 0.5초 폴링으로 체감 실시간
- ✅ opencode 코드 수정 0

## 9. 마일스톤

### M0: 폴링 서버 (현재 → )
- [x] SQLite 폴링 PoC 검증
- [ ] WebSocket 서버로 감싸기 (FastAPI or Hono)
- [ ] 브라우저에서 실시간 로그 수신 확인

### M1: 대시보드 MVP
- [ ] session 목록 → task 카드 렌더링
- [ ] 선택한 task의 part 로그 실시간 스트림
- [ ] task 상태 표시 (running/blocked/done)

### M2: 작업 분할
- [ ] aproc에서 task 생성 → opencode 세션에 매핑
- [ ] task 간 맥락 전달 방식 구현
- [ ] 분할된 task의 병렬 실행

### M3: 런타임 전환
- [ ] 작업 맥락(goal, decisions, artifacts) 추출/주입
- [ ] opencode 프로바이더 전환 시 맥락 연속성
- [ ] 세션 끊김 후 자동 복구

### M4: 클라우드 확장 (후순위)
- [ ] aproc-server를 클라우드 배포
- [ ] SQLite → Litestream/libSQL로 원격 복제
- [ ] 팀 공유 기능
