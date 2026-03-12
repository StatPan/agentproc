package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTerminalFinalizationAndQueueRemoval(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	taskID := "T-FINALIZE-101"
	setupFinalizationFixture(t, root, taskID, `mkdir -p "$run_dir/out"
printf "# Result\n\n- Status: PASS\n" > "$run_dir/out/result.md"
printf "artifact\n" > "$run_dir/out/result-T-FINALIZE-101.md"
`)

	cfg := testRunnerConfig(root)
	task := &Task{
		TaskID:        taskID,
		Role:          "worker_code",
		Output:        "outputs/result-" + taskID + ".md",
		DoneCondition: []string{"output exists"},
	}

	if err := RunTask(task, cfg); err != nil {
		t.Fatalf("RunTask failed: %v", err)
	}

	assertFinalizedRunState(t, root, taskID, "completed")
	if _, err := os.Stat(filepath.Join(root, "outputs", "result-"+taskID+".md")); err != nil {
		t.Fatalf("completion artifact not found: %v", err)
	}
}

func TestTerminalFinalizationOnFailure(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	taskID := "T-FINALIZE-FAIL"
	setupFinalizationFixture(t, root, taskID, `mkdir -p "$run_dir/out"
printf "# Result\n\n- Status: FAIL\n" > "$run_dir/out/result.md"
`)

	cfg := testRunnerConfig(root)
	task := &Task{
		TaskID:        taskID,
		Role:          "worker_code",
		Output:        "outputs/result-" + taskID + ".md",
		DoneCondition: []string{"output exists"},
	}

	if err := RunTask(task, cfg); err == nil {
		t.Fatal("expected RunTask to fail")
	}

	assertFinalizedRunState(t, root, taskID, "failed")
}

func setupFinalizationFixture(t *testing.T, root, taskID, behavior string) {
	t.Helper()

	for _, dir := range []string{
		filepath.Join(root, "tasks", "queue"),
		filepath.Join(root, "roles"),
		filepath.Join(root, "adapters"),
		filepath.Join(root, "outputs"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	taskPath := filepath.Join(root, "tasks", "queue", taskID+".md")
	taskBody := `## Task ID: ` + taskID + `

## Title: finalization fixture

## Depends On: []

## Execution: parallel

## Role: worker_code

## Assigned To: subprocess

## Output
` + "`outputs/result-" + taskID + ".md`" + `

## Done Condition
- output exists
`
	if err := os.WriteFile(taskPath, []byte(taskBody), 0o644); err != nil {
		t.Fatalf("write task: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "roles", "WORKER_CODE.md"), []byte("# role\n"), 0o644); err != nil {
		t.Fatalf("write role: %v", err)
	}

	stubPath := filepath.Join(root, "adapters", "stub.sh")
	stubBody := "#!/usr/bin/env bash\nset -eu\nprompt=\"$1\"\nrun_dir=${prompt##*: }\n" + behavior
	if err := os.WriteFile(stubPath, []byte(stubBody), 0o755); err != nil {
		t.Fatalf("write stub script: %v", err)
	}

	adapter := "## agentic\n```\n" + stubPath + " {prompt}\n```\n"
	if err := os.WriteFile(filepath.Join(root, "adapters", "stub.md"), []byte(adapter), 0o644); err != nil {
		t.Fatalf("write adapter: %v", err)
	}
}

func assertFinalizedRunState(t *testing.T, root, taskID, wantStatus string) {
	t.Helper()

	taskPath := filepath.Join(root, "tasks", "queue", taskID+".md")
	if _, err := os.Stat(taskPath); !os.IsNotExist(err) {
		t.Fatalf("queue task should be removed after finalization, stat err = %v", err)
	}

	runEntries, err := os.ReadDir(filepath.Join(root, "outputs", "runs"))
	if err != nil {
		t.Fatalf("read runs dir: %v", err)
	}
	if len(runEntries) != 1 {
		t.Fatalf("run count = %d, want 1", len(runEntries))
	}

	runID := runEntries[0].Name()
	state, err := loadRunState(filepath.Join(root, "outputs", "runs", runID, "run.json"))
	if err != nil {
		t.Fatalf("load run state: %v", err)
	}
	if state.Status != wantStatus {
		t.Fatalf("run state status = %q, want %q", state.Status, wantStatus)
	}
	if _, err := os.Stat(filepath.Join(root, "outputs", "runs", runID, "summary.json")); err != nil {
		t.Fatalf("summary file not found: %v", err)
	}
}
