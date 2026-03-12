# Code Conventions — 코드 품질 기준

이 문서는 **role: worker_code** 태스크에 항상 적용된다.
Codex(subprocess)는 Gemini(thread)를 호출할 때 이 내용을 direction.md에 포함한다.
Codex는 결과 검수 시 이 기준으로 PASS/FAIL을 판정한다.

출처: Google Style Guides, Effective Go, Clean Code (Robert C. Martin), OWASP Secure Coding.

---

## 1. 일반 원칙

| 원칙 | 내용 |
|------|------|
| **단일 책임** | 함수·클래스 하나는 하나의 책임만 진다. 200줄이 넘는 파일은 역할별로 분리한다. |
| **최소 구현** | 요청된 범위만 구현한다. 미사용 코드·과도한 추상화·미래 대비 코드를 넣지 않는다. |
| **일관성** | 프로젝트 내 네이밍·포맷·임포트 순서를 통일한다. 기존 코드 스타일이 있으면 따른다. |
| **가독성 우선** | 영리한 코드보다 명확한 코드를 선택한다. 이름만으로 의도를 알 수 있게 한다. |
| **DRY** | 같은 로직을 세 번 이상 반복하면 함수로 추출한다. 단, 한 번만 쓰이면 추출하지 않는다. |

---

## 2. 네이밍 (Google Style 기반)

### 공통
- **파일·폴더**: 소문자, 단어 구분은 `_` 또는 `-` (언어 관례 따름). 예: `user_service.go`, `task-runner.ts`
- **상수**: `UPPER_SNAKE_CASE`
- **클래스·타입·인터페이스**: `PascalCase`
- **약어**: 연속 대문자 사용. Go: `userID`, `parseURL` / JS/TS: `userId`, `parseUrl` (관례 따름)

### 언어별
| 언어 | 변수·함수 | 참고 |
|------|-----------|------|
| Go | `camelCase` (exported: `PascalCase`) | Effective Go |
| Python | `snake_case` | PEP 8 |
| JavaScript/TypeScript | `camelCase` | Google JS Style Guide |
| Java | `camelCase` | Google Java Style Guide |
| Shell | `UPPER_SNAKE` (env), `lower_snake` (local) | Google Shell Style Guide |

---

## 3. 구조

- **진입점**: 하나의 명확한 진입점(`main`, `index`, `run`)을 두고, 흐름이 위→아래로 읽히게 한다.
- **임포트 순서**: 표준 라이브러리 → 서드파티 → 내부 패키지 순으로 그룹핑하고 빈 줄로 구분한다.
- **Early Return**: 중첩 if를 피하고 가드 절(guard clause)로 먼저 실패 케이스를 처리한다.
- **함수 길이**: 한 함수가 40줄을 넘으면 분리를 검토한다. 화면 한 페이지에 들어와야 한다.
- **패키지/모듈 순환 참조**: 순환 참조는 만들지 않는다. 의존 방향은 단방향으로 유지한다.

---

## 4. 에러 처리 (최우선 기준)

- **무시 금지**: 에러를 `_`로 버리지 않는다. 의도적으로 무시하는 경우 반드시 주석을 남긴다.
- **컨텍스트 포함**: 에러를 래핑할 때 "어디서 무엇을 하다가"를 포함한다.
  Go: `fmt.Errorf("load config: %w", err)`
  Python: `raise RuntimeError("load config") from err`
- **Fail Fast**: 전제 조건이 충족되지 않으면 함수 초반에 즉시 반환한다.
- **Panic/Exception 남용 금지**: 복구 가능한 오류에 panic/throw를 쓰지 않는다. 예외는 정말 예외적인 상황에만.
- **사용자 메시지**: CLI·API 오류 메시지는 원인과 조치 방법을 담는다.

---

## 5. 보안 (OWASP 기반)

- **비밀 하드코딩 금지**: 비밀번호·API 키·토큰을 코드에 직접 쓰지 않는다. 환경 변수 또는 설정 파일(gitignore 대상)로 분리한다.
- **입력 검증**: 외부 입력(사용자 입력·API 응답·파일)은 사용 전 반드시 검증한다.
- **커맨드 인젝션 방지**: shell 실행 시 사용자 입력을 직접 삽입하지 않는다. 인자 배열 방식을 사용한다.
- **경로 탈출 방지**: 파일 경로 조합 시 `filepath.Clean` 또는 동등한 함수로 정규화한다.

---

## 6. 테스트 및 실행 가능성

- **실행 방법 명시**: README 또는 파일 상단 주석에 `실행 방법`을 한 줄 이상 남긴다.
- **테스트 가능 구조**: 함수는 I/O·부작용을 인자로 주입받아야 테스트하기 쉽다. 전역 상태 직접 접근을 피한다.
- **단위 테스트**: 핵심 로직(파싱·변환·계산)은 단위 테스트를 작성한다. 최소 Happy Path + 에러 케이스 1개.
- **빌드 성공**: 제출 코드는 컴파일/린트 오류 없이 빌드되어야 한다.

---

## 7. 언어별 추가 규칙

### Go (Effective Go + Google Go Style)
- `gofmt` 포맷 준수. 탭 들여쓰기.
- exported 심볼에 godoc 주석 필수 (`// FuncName does ...`).
- goroutine을 시작할 때 종료 조건을 명확히 한다 (context, done channel 등).
- `init()` 사용을 최소화하고, 필요 시 이유를 주석으로 남긴다.
- named return value는 defer 내 에러 수정 목적 외에는 사용하지 않는다.

### Python (PEP 8 + Google Python Style)
- 타입 힌트(`def foo(x: int) -> str:`) 사용을 권장한다.
- docstring: Google 스타일 (`Args:`, `Returns:`, `Raises:`).
- `import *` 사용 금지.

### TypeScript (Google TS Style)
- `any` 사용 금지. 불가피하면 `unknown` + 타입 가드.
- `null`과 `undefined`를 혼용하지 않는다. 프로젝트 내 하나로 통일.
- `interface`와 `type`을 일관되게 사용한다.

---

## 적용 범위

| 태스크 Role | 적용 |
|------------|------|
| `worker_code` | **필수** — direction.md에 이 파일 포함 |
| `reviewer` | **필수** — 검수 기준으로 사용 |
| `worker_doc`, `worker_search` | 적용 안 함 |
| `orchestrator`, `designer` | 적용 안 함 |
