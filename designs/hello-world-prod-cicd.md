# Hello World + Prod 표준 CI/CD 설계

## 목적

- **hello world 레포**를 생성하고, **prod 표준 CI/CD 개발이 가능한 상태**로 둔다.
- 목표: 최소 실행 가능한 레포 구조 + CI(빌드/테스트 자동화) + CD(배포 파이프라인) 준비까지 한 번에 세팅.

## 접근 방법

- **레포 위치:** 워크스페이스 내 `hello-world/` (또는 지정 경로). Git init, README, 최소 진입점(예: 스크립트 또는 main).
- **CI:** GitHub Actions 기준. push/PR 시 lint·test(또는 build) 실행. prod 표준 = main 브랜치 보호 + 필수 체크 통과.
- **CD:** 배포 가능 상태 = 배포 스크립트 또는 workflow 초안 존재. 실제 배포 대상(예: GitHub Pages, Vercel)은 선택 사항으로 placeholder 가능.
- **기술 스택:** 언어/프레임워크 무관. 단순히 “실행 가능한 것 하나 + CI workflow + CD placeholder”로 통일.

## 태스크 목록

| TASK_ID | 제목 | Role | Depends On |
|---------|------|------|------------|
| T-002 | hello world 레포 생성 | worker_code | — |
| T-003 | prod 표준 CI (GitHub Actions) 설정 | worker_code | T-002 |
| T-004 | prod 표준 CD(배포) 가능 상태 구성 | worker_code | T-003 |

## 완료 조건

- [ ] hello-world 레포가 워크스페이스에 존재하고, 실행 가능한 최소 코드(또는 문서)가 있음.
- [ ] `.github/workflows/` 에 CI용 workflow가 있고, push 시 동작함.
- [ ] CD용 workflow 또는 배포 스크립트 초안이 있어, “배포 가능 상태”로 확장하기만 하면 됨.
