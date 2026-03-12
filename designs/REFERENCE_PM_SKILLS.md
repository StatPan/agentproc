# PM Skills 레퍼런스

## 개요

- **레포:** [phuryn/pm-skills](https://github.com/phuryn/pm-skills) — PM 스킬·커맨드·플러그인 모음 (discovery, strategy, execution, launch, growth).
- **역할:** 지시가 제품/기획/런치/전략 관련일 때 설계·태스크 분해·문서 작업 시 참고용 스킬로 사용.

## 클론 위치 (워크스페이스 내)

```
{워크스페이스 루트}/pm-skills/
```

- 분석·참조 시 **이 경로를 Input으로** 넘기면 됨.
- **Gemini로 분석** 시: 격리 폴더의 `task.md`에 "Input: pm-skills/ 경로(또는 절대경로)"를 넣고, worker_search 역할로 실행하면 해당 폴더 구조·README·스킬 목록을 읽고 요약·분석하게 할 수 있음.

## 플러그인 (8개)

- pm-product-discovery, pm-product-strategy, pm-execution, pm-market-research, pm-data-analytics, pm-go-to-market, pm-marketing-growth, pm-toolkit

각 플러그인은 `skills/*/SKILL.md` 형식. Claude Code / Cowork 외에 Gemini, OpenCode, Cursor, Codex 등은 스킬 폴더만 복사해 쓰면 됨.
