## Task ID: T-{n}

## Title: 한 줄 요약

## Priority: high / normal / low

## Status: queue

## Created: YYYY-MM-DD

## Depends On: []          # 의존 태스크 없으면 빈 배열

## Execution: parallel / sequential
# parallel  = Depends On이 없고 다른 태스크와 동시에 실행 가능
# sequential = 앞 태스크 완료 후 실행 (Depends On 명시 필수)

## Role: designer / orchestrator / worker_code / worker_doc / worker_search / reviewer

## Design Ref: designs/{파일명}.md   # 없으면 none

## Assigned To:             # 실행할 에이전트 (예: gemini-cli, claude-code, jules)

## Input

(이 태스크를 시작하기 위해 필요한 데이터, 파일 경로, 조건)

## Output

# 반드시 `outputs/` 하위 경로로 지정한다. 예: `outputs/result-T-{n}.md`
# 실제 작업물(코드·디렉터리 등)은 어디든 만들어도 되지만,
# 완료 신호(마커 파일)는 항상 outputs/ 안에 두어야 다음 태스크의 dependency check가 동작한다.
`outputs/result-T-{n}.md`

## Done Condition

(완료로 판단하는 기준 — 체크리스트 형식 권장)

- [ ]  조건 1
- [ ]  조건 2

## Quality Gate

(Reviewer가 체크할 품질 기준)

- [ ]  기준 1
- [ ]  기준 2

## Timeout: 30m   # 초과 시 failed 처리

## Retry Count: 0  # Reviewer 반환 횟수

## Notes:

(기타 메모, 컨텍스트, 주의사항)
