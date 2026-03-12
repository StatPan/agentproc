package main

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLoadRuntimeConfigDetectsProjectRootAndLocalConfig(t *testing.T) {
	root := t.TempDir()
	projectRoot := filepath.Join(root, "repo")
	workspace := filepath.Join(projectRoot, "pkg", "service")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	if err := os.Mkdir(filepath.Join(projectRoot, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	config := `layers:
  subprocess: codex
runner:
  hidden_runtime: true
`
	if err := os.WriteFile(filepath.Join(projectRoot, ".aproc.yml"), []byte(config), 0o644); err != nil {
		t.Fatalf("write .aproc.yml: %v", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer func() {
		if chdirErr := os.Chdir(cwd); chdirErr != nil {
			t.Fatalf("restore cwd: %v", chdirErr)
		}
	}()
	if err := os.Chdir(workspace); err != nil {
		t.Fatalf("chdir workspace: %v", err)
	}

	cfg, paths, err := loadRuntimeConfig(".", "./config.yaml")
	if err != nil {
		t.Fatalf("loadRuntimeConfig returned error: %v", err)
	}

	if cfg.AgentOSRoot != canonicalProjectPath(projectRoot) {
		t.Fatalf("AgentOSRoot = %q", cfg.AgentOSRoot)
	}
	if !cfg.Runner.HiddenRuntime {
		t.Fatal("expected hidden runtime to be enabled")
	}
	if got := paths.QueueDir(); !strings.Contains(got, filepath.Join("aproc", "projects")) {
		t.Fatalf("QueueDir() = %q", got)
	}
}

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
	executeTaskFunc = func(cfg *Config, claim *taskClaim) {
		mu.Lock()
		executed = append(executed, claim.task.TaskID)
		mu.Unlock()
	}
	defer func() { executeTaskFunc = original }()

	cfg := &Config{AgentOSRoot: root, Runner: RunnerConfig{MaxConcurrent: 1}}
	if err := RunDispatchOnce(context.Background(), cfg, NewRuntimePaths(root, false)); err != nil {
		t.Fatalf("dispatch once: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(executed) != 0 {
		t.Fatalf("task should not execute before dependency output exists: %v", executed)
	}
}

func TestExecuteTaskLeavesFailedTaskClaimedInActiveRun(t *testing.T) {
	root := t.TempDir()
	paths := NewRuntimePaths(root, false)
	for _, dir := range []string{paths.QueueDir(), paths.ActiveRunsDir()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	task := &Task{TaskID: "T-FAIL-001"}
	taskPath := filepath.Join(paths.QueueDir(), task.TaskID+".md")
	if err := os.WriteFile(taskPath, []byte("placeholder"), 0o644); err != nil {
		t.Fatalf("write task file: %v", err)
	}
	claim, err := claimTask(paths, task)
	if err != nil {
		t.Fatalf("claimTask returned error: %v", err)
	}

	executeTask(&Config{AgentOSRoot: root}, claim)

	if _, err := os.Stat(taskPath); !os.IsNotExist(err) {
		t.Fatalf("expected claimed task to leave queue, got err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(claim.runDir, "task.md")); err != nil {
		t.Fatalf("expected failed claimed task to remain in active run: %v", err)
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
	executeTaskFunc = func(cfg *Config, claim *taskClaim) {
		mu.Lock()
		executed = append(executed, claim.task.TaskID)
		mu.Unlock()

		if claim.task.TaskID == "T-200" {
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
		if err := dispatchCycle(context.Background(), cfg, NewRuntimePaths(root, false), func() {
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
	executeTaskFunc = func(cfg *Config, claim *taskClaim) {
		mu.Lock()
		executed = append(executed, claim.task.TaskID)
		mu.Unlock()

		if claim.task.TaskID == "T-200" {
			outputPath := filepath.Join(cfg.AgentOSRoot, "outputs", "result-T-200.md")
			if err := os.WriteFile(outputPath, []byte("PASS\n"), 0o644); err != nil {
				t.Errorf("write dependency output: %v", err)
			}
		}
	}
	defer func() { executeTaskFunc = original }()

	cfg := &Config{AgentOSRoot: root, Runner: RunnerConfig{MaxConcurrent: 1}}
	if err := RunDispatchOnce(context.Background(), cfg, NewRuntimePaths(root, false)); err != nil {
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
	state := &RunState{
		RunID:     "run-T-300",
		TaskID:    "T-300",
		Status:    "running",
		StartedAt: time.Now().UTC().Format(time.RFC3339),
		PID:       os.Getpid(),
	}
	if err := writeRunState(filepath.Join(runTaskDir, "run.json"), state); err != nil {
		t.Fatalf("write run state: %v", err)
	}

	tasks, err := collectDispatchableTasks(NewRuntimePaths(root, false))
	if err != nil {
		t.Fatalf("collect dispatchable tasks: %v", err)
	}
	if len(tasks) != 0 {
		t.Fatalf("dispatchable tasks = %d, want 0", len(tasks))
	}
}

func TestCollectDispatchableTasksIgnoresStaleRunDirectoryWithoutRunningState(t *testing.T) {
	root := t.TempDir()
	queueDir := filepath.Join(root, "tasks", "queue")
	runIndexDir := filepath.Join(root, "tasks", ".run")
	outputsDir := filepath.Join(root, "outputs")
	for _, dir := range []string{queueDir, runIndexDir, outputsDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	taskContent := `## Task ID: T-301

## Title: stale run should not block

## Depends On: []

## Execution: parallel

## Role: worker_code

## Design Ref: designs/ref.md

## Assigned To: codex

## Input

example
`
	if err := os.WriteFile(filepath.Join(queueDir, "T-301.md"), []byte(taskContent), 0o644); err != nil {
		t.Fatalf("write queue task: %v", err)
	}
	runTaskDir := filepath.Join(runIndexDir, "run-T-301")
	if err := os.MkdirAll(runTaskDir, 0o755); err != nil {
		t.Fatalf("mkdir run task dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runTaskDir, "task.md"), []byte(taskContent), 0o644); err != nil {
		t.Fatalf("write run task: %v", err)
	}
	state := &RunState{
		RunID:     "run-T-301",
		TaskID:    "T-301",
		Status:    "failed",
		StartedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if err := writeRunState(filepath.Join(runTaskDir, "run.json"), state); err != nil {
		t.Fatalf("write run state: %v", err)
	}

	tasks, err := collectDispatchableTasks(NewRuntimePaths(root, false))
	if err != nil {
		t.Fatalf("collect dispatchable tasks: %v", err)
	}
	if len(tasks) != 1 || tasks[0].TaskID != "T-301" {
		t.Fatalf("dispatchable tasks = %#v, want [T-301]", tasks)
	}
}

func TestCollectDispatchableTasksRecoversStaleRunningRun(t *testing.T) {
	root := t.TempDir()
	queueDir := filepath.Join(root, "tasks", "queue")
	runIndexDir := filepath.Join(root, "tasks", ".run")
	outputsDir := filepath.Join(root, "outputs")
	for _, dir := range []string{queueDir, runIndexDir, outputsDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	taskContent := `## Task ID: T-301A

## Title: stale running run should recover

## Depends On: []

## Execution: parallel

## Role: worker_code

## Design Ref: designs/ref.md

## Assigned To: codex

## Input

example
`
	if err := os.WriteFile(filepath.Join(queueDir, "T-301A.md"), []byte(taskContent), 0o644); err != nil {
		t.Fatalf("write queue task: %v", err)
	}
	runTaskDir := filepath.Join(runIndexDir, "run-T-301A")
	if err := os.MkdirAll(runTaskDir, 0o755); err != nil {
		t.Fatalf("mkdir run task dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runTaskDir, "task.md"), []byte(taskContent), 0o644); err != nil {
		t.Fatalf("write run task: %v", err)
	}
	state := &RunState{
		RunID:     "run-T-301A",
		TaskID:    "T-301A",
		Status:    "running",
		StartedAt: time.Now().UTC().Format(time.RFC3339),
		PID:       999999,
	}
	if err := writeRunState(filepath.Join(runTaskDir, "run.json"), state); err != nil {
		t.Fatalf("write run state: %v", err)
	}

	paths := NewRuntimePaths(root, false)
	tasks, err := collectDispatchableTasks(paths)
	if err != nil {
		t.Fatalf("collect dispatchable tasks: %v", err)
	}
	if len(tasks) != 1 || tasks[0].TaskID != "T-301A" {
		t.Fatalf("dispatchable tasks = %#v, want [T-301A]", tasks)
	}

	recoveredState, err := loadRunState(filepath.Join(runTaskDir, "run.json"))
	if err != nil {
		t.Fatalf("load recovered active state: %v", err)
	}
	if recoveredState.Status != "interrupted" {
		t.Fatalf("recovered active status = %q, want interrupted", recoveredState.Status)
	}

	completedState, err := loadRunState(filepath.Join(outputsDir, "runs", "run-T-301A", "run.json"))
	if err != nil {
		t.Fatalf("load recovered completed state: %v", err)
	}
	if completedState.Status != "interrupted" {
		t.Fatalf("recovered completed status = %q, want interrupted", completedState.Status)
	}

	summary, _, err := loadRunSummary(paths, "run-T-301A")
	if err != nil {
		t.Fatalf("load recovered run summary: %v", err)
	}
	if summary.Status != "interrupted" {
		t.Fatalf("summary status = %q, want interrupted", summary.Status)
	}
	if !slices.Contains(summary.Events, "recovered stale active run before dispatch") {
		t.Fatalf("summary events = %#v, want stale recovery marker", summary.Events)
	}
}

func TestCollectDispatchableTasksIgnoresLegacyRunDirectoryWithoutRunState(t *testing.T) {
	root := t.TempDir()
	queueDir := filepath.Join(root, "tasks", "queue")
	runIndexDir := filepath.Join(root, "tasks", ".run")
	outputsDir := filepath.Join(root, "outputs")
	for _, dir := range []string{queueDir, runIndexDir, outputsDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	taskContent := `## Task ID: T-302

## Title: legacy run should not block

## Depends On: []

## Execution: parallel

## Role: worker_code

## Design Ref: designs/ref.md

## Assigned To: codex

## Input

example
`
	if err := os.WriteFile(filepath.Join(queueDir, "T-302.md"), []byte(taskContent), 0o644); err != nil {
		t.Fatalf("write queue task: %v", err)
	}
	runTaskDir := filepath.Join(runIndexDir, "run-T-302")
	if err := os.MkdirAll(runTaskDir, 0o755); err != nil {
		t.Fatalf("mkdir run task dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runTaskDir, "task.md"), []byte(taskContent), 0o644); err != nil {
		t.Fatalf("write legacy run task: %v", err)
	}

	tasks, err := collectDispatchableTasks(NewRuntimePaths(root, false))
	if err != nil {
		t.Fatalf("collect dispatchable tasks: %v", err)
	}
	if len(tasks) != 1 || tasks[0].TaskID != "T-302" {
		t.Fatalf("dispatchable tasks = %#v, want [T-302]", tasks)
	}
}

func TestClaimTaskCopiesQueueEntryIntoActiveRun(t *testing.T) {
	root := t.TempDir()
	queueDir := filepath.Join(root, "tasks", "queue")
	runDir := filepath.Join(root, "tasks", ".run")
	for _, dir := range []string{queueDir, runDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	taskContent := `## Task ID: T-CLAIM-001

## Title: claim me

## Depends On: []

## Execution: parallel

## Role: worker_code

## Design Ref: designs/ref.md

## Assigned To: codex

## Input

example
`
	queuePath := filepath.Join(queueDir, "T-CLAIM-001.md")
	if err := os.WriteFile(queuePath, []byte(taskContent), 0o644); err != nil {
		t.Fatalf("write queue task: %v", err)
	}

	task, err := ParseTask(queuePath)
	if err != nil {
		t.Fatalf("parse task: %v", err)
	}

	claim, err := claimTask(NewRuntimePaths(root, false), task)
	if err != nil {
		t.Fatalf("claimTask returned error: %v", err)
	}
	if claim == nil {
		t.Fatal("expected claim")
	}
	if _, err := os.Stat(queuePath); err != nil {
		t.Fatalf("queue task should still exist after claim, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(claim.runDir, "task.md")); err != nil {
		t.Fatalf("claimed task missing from run dir: %v", err)
	}
	state, err := loadRunState(filepath.Join(claim.runDir, "run.json"))
	if err != nil {
		t.Fatalf("load run state: %v", err)
	}
	if state.Status != "running" || state.TaskID != "T-CLAIM-001" {
		t.Fatalf("unexpected claim state: %#v", state)
	}
}

func TestExecuteTaskUsesClaimedRunDirectory(t *testing.T) {
	root := t.TempDir()
	paths := NewRuntimePaths(root, false)
	for _, dir := range []string{
		paths.QueueDir(),
		paths.ActiveRunsDir(),
		filepath.Join(root, "roles"),
		filepath.Join(root, "adapters"),
		filepath.Join(root, "outputs"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "roles", "WORKER_CODE.md"), []byte("# role\n"), 0o644); err != nil {
		t.Fatalf("write role: %v", err)
	}
	adapter := "## agentic\n```\nsh -c 'run_dir=${1##*: }; mkdir -p \"$run_dir/out\"; printf \"# Result\\n\\n- Status: PASS\\n\" > \"$run_dir/out/result.md\"; printf \"artifact\\n\" > \"$run_dir/out/result-T-CLAIM-002.md\"' sh {prompt}\n```\n"
	if err := os.WriteFile(filepath.Join(root, "adapters", "stub.md"), []byte(adapter), 0o644); err != nil {
		t.Fatalf("write adapter: %v", err)
	}

	queuePath := filepath.Join(paths.QueueDir(), "T-CLAIM-002.md")
	taskContent := `## Task ID: T-CLAIM-002

## Title: claimed task execution

## Depends On: []

## Execution: parallel

## Role: worker_code

## Assigned To: subprocess

## Output
` + "`outputs/result-T-CLAIM-002.md`" + `

## Done Condition
- output exists
`
	if err := os.WriteFile(queuePath, []byte(taskContent), 0o644); err != nil {
		t.Fatalf("write queue task: %v", err)
	}
	task, err := ParseTask(queuePath)
	if err != nil {
		t.Fatalf("parse task: %v", err)
	}
	claim, err := claimTask(paths, task)
	if err != nil {
		t.Fatalf("claimTask returned error: %v", err)
	}

	cfg := &Config{
		AgentOSRoot: root,
		Layers:      LayerConfig{Subprocess: "stub", Thread: "stub"},
		Runner:      RunnerConfig{QualityGateEnabled: true},
	}
	executeTask(cfg, claim)

	if _, err := os.Stat(filepath.Join(root, "outputs", "result-T-CLAIM-002.md")); err != nil {
		t.Fatalf("expected output artifact: %v", err)
	}
	if _, err := os.Stat(queuePath); !os.IsNotExist(err) {
		t.Fatalf("queue task should remain claimed, stat err = %v", err)
	}
}
