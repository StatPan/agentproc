# gemini

## agentic
```
gemini -m gemini-3-flash-preview -y -p "{prompt}" < /dev/null
```

## context
프롬프트에 격리 폴더 경로 포함.
예: `"role.md, task.md 읽고 실행: /path/to/.run/{run_id}/"`
