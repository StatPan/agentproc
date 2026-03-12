# AgentOS Config

## Layers

| Layer     | Binary | Adapter                  |
|-----------|--------|--------------------------|
| process   | claude | adapters/claude.md       |
| subprocess| codex  | adapters/subprocess.md + adapters/codex.md |
| thread    | gemini | adapters/gemini.md       |

## Layer Roles

- **Process (`claude`)**: 진입점 레이어. 사람 요청을 설계로 바꾸고, 상위 태스크를 분해하고, `subprocess`를 생성하고, 최종 결과를 확인해 사람에게 보고한다.
- **Subprocess (`codex`)**: process에게 위임받은 범위를 관리하는 레이어. 레이어 계약은 `adapters/subprocess.md` 에 있고, 현재 실행 바이너리 템플릿은 `adapters/codex.md` 를 쓴다. 자신의 태스크를 더 작은 작업으로 다시 분해할 수 있고, `thread`를 병렬 호출하고, 결과를 검토하고, 필요하면 재지시한다.
- **Thread (`gemini`)**: 실제 작업을 수행하는 마지막 레이어. subprocess의 지시에 따라 원자적인 작업을 수행한다. 현재 기본값은 `gemini`이다.
