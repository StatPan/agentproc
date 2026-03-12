# subprocess → thread 재귀 루프

## 원칙

- **process**: `subprocess` 레이어를 호출해 태스크를 넘긴다. 사용자는 직접 `subprocess`를 호출하지 않는다.
- **subprocess**:
    - 실제 실행은 `config.md`에 정의된 `thread` 레이어에게 맡긴다.
    - 자신의 태스크를 thread가 처리하기 쉬운 leaf 작업으로 다시 분해할 수 있다.
    - `thread`가 작업한 결과를 검수한다.
- **재귀 루프**: 검수 결과가 불완전하면 `subprocess`가 피드백을 작성해 `thread`를 다시 호출한다. 모든 요구사항이 충족되어 완성될 때까지 이 과정을 반복한다.

## 격리 폴더에 넣는 것

- `role.md`, `task.md` — 워커가 볼 작업 내용.
- **`SUBPROCESS_INSTRUCTION.md`** — `subprocess` 레이어 공통 지시 파일. 특정 도구명이 아니라 레이어 계약을 담는다.
- **`direction.md`** — process가 Design Ref·태스크·지시에서 뽑은 컨텍스트·방향성(언어 선호 예: Python, 스택, 제약). `thread`가 추측하지 않도록 명시. 없으면 생략 가능.
- **코드 태스크(worker_code)일 때:** `conventions/CODE_CONVENTIONS.md` + `direction.md`(있으면) 를 `thread` 호출 시 함께 넘긴다.

## thread worker·코드 품질

- **현재 thread 기본값** — `gemini`. 빠르고 지시 받은 동작만 수행하는 데 유리하다.
- **코드 품질** — 코드를 작성하는 태스크에서는 **conventions/CODE_CONVENTIONS.md** 를 고정으로 적용. `subprocess`가 `thread`를 호출할 때마다 이 파일(또는 내용)을 함께 넘겨 일정한 품질의 코드가 나오게 한다. 매번 지시에 포함하지 않고, 항상 이 문서를 기준으로 강제한다.

## process가 하는 일

1. 격리 폴더 생성, role.md·task.md·instruction 파일 작성. Design Ref·태스크에서 방향성을 뽑아 `direction.md` 로 넣거나 task Input 상단에 명시한다.
2. `config.md`에 정의된 `subprocess` 바이너리를 실행한다.
3. 필요 시 병렬로 여러 태스크에 대해 여러 `subprocess`를 각각 백그라운드 실행한다.

## CLI 옵션 (에러 방지)

- **현재 subprocess 기본값:** `codex`. 샌드박스 없이 실행해야 하위에서 파일 읽기/쓰기·thread 호출이 막히지 않는다. `codex exec ... --dangerously-bypass-approvals-and-sandbox` 사용. (-p는 Codex에서 profile이므로 프롬프트는 위치 인자로 넘김.)
- **현재 thread 기본값:** `gemini`. 모델 고정 + 자동 승인 필수. `gemini -m gemini-3-flash-preview -y -p "프롬프트"` (모델명은 하이픈: gemini-3-flash-preview, -y=yolo=자동 승인, -p=비대화형). 코드 작성 등 도구 사용 시 -y 없으면 승인 대기로 실패할 수 있음.
