package main

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
)

func TestLooksLikeDirectRequest(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{name: "empty", args: nil, want: false},
		{name: "legacy flag", args: []string{"--transport", "http"}, want: false},
		{name: "serve subcommand", args: []string{"serve"}, want: false},
		{name: "natural language request", args: []string{"로그 회전 구조 점검"}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := looksLikeDirectRequest(tt.args)
			if got != tt.want {
				t.Fatalf("looksLikeDirectRequest(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}

func TestRunDispatchOnceSkipsTaskUntilDependenciesSatisfied(t *testing.T) {
	root := t.TempDir()
	queueDir := filepath.Join(root, "tasks", "queue")
	runIndexDir := filepath.Join(root, "tasks", ".run")
	outputsDir := filepath.Join(root, "outputs")
	for _, dir := range []string{queueDir, runIndexDir, outputsDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	taskContent := `## Task ID: T-200

## Title: wait for dependency

## Depends On: [T-100]

## Execution: parallel

## Role: worker_code

## Design Ref: designs/ref.md

## Assigned To: codex

## Input

example
`
	if err := os.WriteFile(filepath.Join(queueDir, "T-200.md"), []byte(taskContent), 0o644); err != nil {
		t.Fatalf("write task: %v", err)
	}

	var (
		mu       sync.Mutex
		executed []string
	)
	original := executeTaskFunc
	executeTaskFunc = func(cfg *Config, queueDir string, task *Task) {
		mu.Lock()
		executed = append(executed, task.TaskID)
		mu.Unlock()
	}
	defer func() { executeTaskFunc = original }()

	cfg := &Config{AgentOSRoot: root, Runner: RunnerConfig{MaxConcurrent: 1}}
	if err := RunDispatchOnce(context.Background(), cfg, queueDir, runIndexDir, outputsDir); err != nil {
		t.Fatalf("dispatch once: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(executed) != 0 {
		t.Fatalf("task should not execute before dependency output exists: %v", executed)
	}
}

func TestDispatchCycleRetriggersSequentialTaskOnNextInvocation(t *testing.T) {
	root := t.TempDir()
	queueDir := filepath.Join(root, "tasks", "queue")
	runIndexDir := filepath.Join(root, "tasks", ".run")
	outputsDir := filepath.Join(root, "outputs")
	for _, dir := range []string{queueDir, runIndexDir, outputsDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	sequentialTask := `## Task ID: T-100

## Title: wait for dependency

## Depends On: [T-200]

## Execution: sequential

## Role: worker_code

## Design Ref: designs/ref.md

## Assigned To: codex

## Input

run after dependency
`
	parallelTask := `## Task ID: T-200

## Title: produce dependency

## Depends On: []

## Execution: parallel

## Role: worker_code

## Design Ref: designs/ref.md

## Assigned To: codex

## Output

` + "`outputs/result-T-200.md`" + `

## Input

run first
`
	if err := os.WriteFile(filepath.Join(queueDir, "T-100.md"), []byte(sequentialTask), 0o644); err != nil {
		t.Fatalf("write sequential task: %v", err)
	}
	if err := os.WriteFile(filepath.Join(queueDir, "T-200.md"), []byte(parallelTask), 0o644); err != nil {
		t.Fatalf("write parallel task: %v", err)
	}

	var (
		mu       sync.Mutex
		executed []string
	)
	original := executeTaskFunc
	executeTaskFunc = func(cfg *Config, queueDir string, task *Task) {
		mu.Lock()
		executed = append(executed, task.TaskID)
		mu.Unlock()

		if err := os.Remove(filepath.Join(queueDir, task.TaskID+".md")); err != nil && !os.IsNotExist(err) {
			t.Errorf("remove task from queue: %v", err)
		}

		if task.TaskID == "T-200" {
			outputPath := filepath.Join(cfg.AgentOSRoot, "outputs", "result-T-200.md")
			if err := os.WriteFile(outputPath, []byte("PASS\n"), 0o644); err != nil {
				t.Errorf("write dependency output: %v", err)
			}
		}
	}
	defer func() { executeTaskFunc = original }()

	cfg := &Config{AgentOSRoot: root, Runner: RunnerConfig{MaxConcurrent: 1}}
	dispatchRequests := make(chan struct{}, 4)
	runCycle := func() {
		if err := dispatchCycle(context.Background(), cfg, queueDir, runIndexDir, outputsDir, func() {
			triggerDispatch(dispatchRequests)
		}); err != nil {
			t.Fatalf("dispatch cycle: %v", err)
		}
	}

	runCycle()

	for {
		select {
		case <-dispatchRequests:
			runCycle()
		default:
			mu.Lock()
			got := append([]string(nil), executed...)
			mu.Unlock()
			if !slices.Equal(got, []string{"T-200", "T-100"}) {
				t.Fatalf("executed tasks = %v, want [T-200 T-100]", got)
			}
			return
		}
	}
}

func TestRunDispatchOnceDoesNotReevaluateNewlySatisfiedDependencies(t *testing.T) {
	root := t.TempDir()
	queueDir := filepath.Join(root, "tasks", "queue")
	runIndexDir := filepath.Join(root, "tasks", ".run")
	outputsDir := filepath.Join(root, "outputs")
	for _, dir := range []string{queueDir, runIndexDir, outputsDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	sequentialTask := `## Task ID: T-100

## Title: wait for dependency

## Depends On: [T-200]

## Execution: sequential

## Role: worker_code

## Design Ref: designs/ref.md

## Assigned To: codex

## Input

run after dependency
`
	parallelTask := `## Task ID: T-200

## Title: produce dependency

## Depends On: []

## Execution: parallel

## Role: worker_code

## Design Ref: designs/ref.md

## Assigned To: codex

## Output

` + "`outputs/result-T-200.md`" + `

## Input

run first
`
	if err := os.WriteFile(filepath.Join(queueDir, "T-100.md"), []byte(sequentialTask), 0o644); err != nil {
		t.Fatalf("write sequential task: %v", err)
	}
	if err := os.WriteFile(filepath.Join(queueDir, "T-200.md"), []byte(parallelTask), 0o644); err != nil {
		t.Fatalf("write parallel task: %v", err)
	}

	var (
		mu       sync.Mutex
		executed []string
	)
	original := executeTaskFunc
	executeTaskFunc = func(cfg *Config, queueDir string, task *Task) {
		mu.Lock()
		executed = append(executed, task.TaskID)
		mu.Unlock()

		if err := os.Remove(filepath.Join(queueDir, task.TaskID+".md")); err != nil && !os.IsNotExist(err) {
			t.Errorf("remove task from queue: %v", err)
		}

		if task.TaskID == "T-200" {
			outputPath := filepath.Join(cfg.AgentOSRoot, "outputs", "result-T-200.md")
			if err := os.WriteFile(outputPath, []byte("PASS\n"), 0o644); err != nil {
				t.Errorf("write dependency output: %v", err)
			}
		}
	}
	defer func() { executeTaskFunc = original }()

	cfg := &Config{AgentOSRoot: root, Runner: RunnerConfig{MaxConcurrent: 1}}
	if err := RunDispatchOnce(context.Background(), cfg, queueDir, runIndexDir, outputsDir); err != nil {
		t.Fatalf("dispatch once: %v", err)
	}

	mu.Lock()
	got := append([]string(nil), executed...)
	mu.Unlock()
	if !slices.Equal(got, []string{"T-200"}) {
		t.Fatalf("executed tasks = %v, want [T-200]", got)
	}
}

func TestCollectDispatchableTasksSkipsTasksAlreadyInRunIndex(t *testing.T) {
	root := t.TempDir()
	queueDir := filepath.Join(root, "tasks", "queue")
	runIndexDir := filepath.Join(root, "tasks", ".run")
	outputsDir := filepath.Join(root, "outputs")
	for _, dir := range []string{queueDir, runIndexDir, outputsDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	taskContent := `## Task ID: T-300

## Title: already running

## Depends On: []

## Execution: parallel

## Role: worker_code

## Design Ref: designs/ref.md

## Assigned To: codex

## Input

example
`
	if err := os.WriteFile(filepath.Join(queueDir, "T-300.md"), []byte(taskContent), 0o644); err != nil {
		t.Fatalf("write queue task: %v", err)
	}
	runTaskDir := filepath.Join(runIndexDir, "run-T-300")
	if err := os.MkdirAll(runTaskDir, 0o755); err != nil {
		t.Fatalf("mkdir run task dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runTaskDir, "task.md"), []byte(taskContent), 0o644); err != nil {
		t.Fatalf("write run task: %v", err)
	}

	tasks, err := collectDispatchableTasks(queueDir, runIndexDir, outputsDir)
	if err != nil {
		t.Fatalf("collect dispatchable tasks: %v", err)
	}
	if len(tasks) != 0 {
		t.Fatalf("dispatchable tasks = %d, want 0", len(tasks))
	}
}
