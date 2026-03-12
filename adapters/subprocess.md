# subprocess

## context
이 파일은 특정 바이너리가 아니라 `subprocess` 레이어 공통 계약이다.
- `subprocess` 는 직접 끝내는 실행자보다, `thread` 를 활용해 위임 범위를 완성하는 중간 레이어다.
- 작업이 작아 보여도 먼저 현재 `config.md` 에 정의된 `thread` 레이어로 넘길 수 있는 leaf 작업이 있는지 판단한다.
- 격리 폴더의 `role.md`, `task.md`, `SUBPROCESS_INSTRUCTION.md`, `direction.md` 를 읽고 thread 작업을 분해·배분한다.
- thread 호출은 직접 shell command 를 조립하지 않고 runner-generated wrapper/helper 경로를 기본으로 사용한다.
- 원본 증거는 `out/thread-evidence/` 와 실행 종료 후 `outputs/thread-evidence-<TASK_ID>/` 이다. `out/thread-log.md` 는 사람이 읽는 요약이다.
- reviewer/process 기본 확인 순서는 `out/summary.md` -> runner-generated evidence -> 필요 시 상세 markdown 이다.
- 직접 수정한 양보다, thread 를 어떻게 활용하고 검토해 완성도를 끌어올렸는지가 더 중요한 성과다.
- thread 출력을 검수하고, 부족하면 피드백과 함께 재호출한다.
- 재호출할 때는 같은 큰 작업을 그대로 반복하지 말고, 실패 지점을 더 작은 corrective leaf task 로 좁혀서 다시 위임한다.
- 기본 정책은 `review -> narrower corrective re-delegation -> review` 이다. thread 가 부족했다고 바로 subprocess 가 직접 마무리하면 안 된다.
- subprocess 가 직접 구현이나 직접 마감을 선택할 수 있는 경우는 corrective re-delegation 을 최소 한 번 시도한 뒤에도 thread 결과가 여전히 부족하거나, thread 로 더 쪼갤 수 없는 사유가 명확할 때뿐이다.
- subprocess 가 직접 마감한 경우에는 `out/summary.md` 와 `out/result.md` 에 왜 corrective re-delegation 으로 충분하지 않았는지, 왜 direct completion 이 필요했는지 남겨야 한다.
- 최종 보고는 원문 전체가 아니라 `out/summary.md` 중심의 짧은 관리 요약으로 남긴다.
