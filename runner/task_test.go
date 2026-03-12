package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseTaskAndLoadQueue(t *testing.T) {
	queueDir := t.TempDir()

	firstTask := `## Task ID: T-001

## Title: First task

## Depends On: []

## Execution: parallel

## Role: worker_code

## Assigned To: codex

## Output
- ` + "`outputs/research/result.md`" + `
- ` + "`outputs/research/extra.md`" + `
`

	secondTask := `## Task ID: T-002

## Title: Second task

## Depends On: [T-001, T-003]

## Execution: sequential

## Role: worker_doc

## Assigned To: gemini

## Output
- ` + "`outputs/docs/result.md`" + `
`

	firstPath := filepath.Join(queueDir, "a-task.md")
	secondPath := filepath.Join(queueDir, "b-task.md")
	ignoredPath := filepath.Join(queueDir, "ignore.txt")

	if err := os.WriteFile(firstPath, []byte(firstTask), 0o644); err != nil {
		t.Fatalf("write first task: %v", err)
	}
	if err := os.WriteFile(secondPath, []byte(secondTask), 0o644); err != nil {
		t.Fatalf("write second task: %v", err)
	}
	if err := os.WriteFile(ignoredPath, []byte("ignore"), 0o644); err != nil {
		t.Fatalf("write ignored file: %v", err)
	}

	parsed, err := ParseTask(firstPath)
	if err != nil {
		t.Fatalf("ParseTask() error = %v", err)
	}

	if parsed.TaskID != "T-001" {
		t.Fatalf("TaskID = %q, want %q", parsed.TaskID, "T-001")
	}
	if parsed.Title != "First task" {
		t.Fatalf("Title = %q, want %q", parsed.Title, "First task")
	}
	if len(parsed.DependsOn) != 0 {
		t.Fatalf("DependsOn length = %d, want 0", len(parsed.DependsOn))
	}
	if parsed.Execution != "parallel" {
		t.Fatalf("Execution = %q, want %q", parsed.Execution, "parallel")
	}
	if parsed.Role != "worker_code" {
		t.Fatalf("Role = %q, want %q", parsed.Role, "worker_code")
	}
	if parsed.AssignedTo != "codex" {
		t.Fatalf("AssignedTo = %q, want %q", parsed.AssignedTo, "codex")
	}
	if parsed.Output != "outputs/research/result.md" {
		t.Fatalf("Output = %q, want %q", parsed.Output, "outputs/research/result.md")
	}

	tasks, err := LoadQueue(queueDir)
	if err != nil {
		t.Fatalf("LoadQueue() error = %v", err)
	}

	if len(tasks) != 2 {
		t.Fatalf("LoadQueue() length = %d, want 2", len(tasks))
	}

	if tasks[0].TaskID != "T-001" {
		t.Fatalf("tasks[0].TaskID = %q, want %q", tasks[0].TaskID, "T-001")
	}
	if got := tasks[1].DependsOn; len(got) != 2 || got[0] != "T-001" || got[1] != "T-003" {
		t.Fatalf("tasks[1].DependsOn = %#v, want []string{\"T-001\", \"T-003\"}", got)
	}
	if tasks[1].Output != "outputs/docs/result.md" {
		t.Fatalf("tasks[1].Output = %q, want %q", tasks[1].Output, "outputs/docs/result.md")
	}
}
