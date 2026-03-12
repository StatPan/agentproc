package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGenerateDirectionIncludesThreadExecutionContract(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	runDir := filepath.Join(root, "tasks", ".run", "run-contract")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("mkdir run dir: %v", err)
	}

	task := &Task{
		TaskID: "T-THREAD-001",
		Role:   "worker_code",
		Output: "outputs/result-T-THREAD-001.md",
	}
	cfg := &Config{
		AgentOSRoot: root,
		Layers: LayerConfig{
			Thread: "gemini",
		},
	}

	if err := generateDirection(task, cfg, runDir); err != nil {
		t.Fatalf("generate direction: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(runDir, "direction.md"))
	if err != nil {
		t.Fatalf("read direction.md: %v", err)
	}
	body := string(got)
	if !strings.Contains(body, "## Thread Execution Contract") {
		t.Fatalf("direction missing thread contract: %s", body)
	}
	if !strings.Contains(body, "Thread Wrapper: out/thread-evidence/invoke-thread") {
		t.Fatalf("direction missing thread wrapper: %s", body)
	}
	if !strings.Contains(body, "Persisted Evidence: outputs/thread-evidence-T-THREAD-001/") {
		t.Fatalf("direction missing persisted evidence path: %s", body)
	}
	if !strings.Contains(body, "Persisted Marker: outputs/thread-T-THREAD-001.md") {
		t.Fatalf("direction missing persisted thread marker: %s", body)
	}
	if !strings.Contains(body, "Management Summary: out/summary.md") {
		t.Fatalf("direction missing management summary path: %s", body)
	}
	if !strings.Contains(body, "read out/summary.md first, then runner-generated evidence") {
		t.Fatalf("direction missing process read-path guidance: %s", body)
	}
}

func TestGenerateSubprocessInstructionRequiresThreadLogUpdate(t *testing.T) {
	t.Parallel()

	runDir := t.TempDir()
	task := &Task{Role: "worker_code"}

	if err := generateSubprocessInstruction(task, runDir); err != nil {
		t.Fatalf("generate SUBPROCESS_INSTRUCTION.md: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(runDir, "SUBPROCESS_INSTRUCTION.md"))
	if err != nil {
		t.Fatalf("read SUBPROCESS_INSTRUCTION.md: %v", err)
	}
	body := string(got)
	if !strings.Contains(body, "# Subprocess Instruction") {
		t.Fatalf("instruction missing subprocess title: %s", body)
	}
	if !strings.Contains(body, "out/thread-log.md") {
		t.Fatalf("instruction missing thread log requirement: %s", body)
	}
	if !strings.Contains(body, "out/thread-evidence/invoke-thread") {
		t.Fatalf("instruction missing thread wrapper requirement: %s", body)
	}
	if !strings.Contains(body, "out/thread-evidence/invocations.jsonl") {
		t.Fatalf("instruction missing thread evidence requirement: %s", body)
	}
	if !strings.Contains(body, "Invoke the configured thread layer") {
		t.Fatalf("instruction missing thread invocation requirement: %s", body)
	}
	if !strings.Contains(body, "Even if the work looks small") {
		t.Fatalf("instruction missing thread-first mindset: %s", body)
	}
	if !strings.Contains(body, "decompose the work into smaller thread-friendly units") {
		t.Fatalf("instruction missing subprocess decomposition requirement: %s", body)
	}
	if !strings.Contains(body, "out/feedback.md") {
		t.Fatalf("instruction missing reinvocation feedback requirement: %s", body)
	}
	if !strings.Contains(body, "Do not handcraft direct shell commands") {
		t.Fatalf("instruction missing wrapper-only invocation rule: %s", body)
	}
	if !strings.Contains(body, "Default read order is") {
		t.Fatalf("instruction missing reviewer/process read order: %s", body)
	}
	if !strings.Contains(body, "Your value is not raw implementation speed") {
		t.Fatalf("instruction missing subprocess boundary requirement: %s", body)
	}
	if !strings.Contains(body, "Judge success by how effectively the thread layer was used") {
		t.Fatalf("instruction missing subprocess success framing: %s", body)
	}
	if !strings.Contains(body, "PASS or FAIL, thread invocation count, whether decomposition happened") {
		t.Fatalf("instruction missing reviewer summary requirement: %s", body)
	}
	if !strings.Contains(body, "out/summary.md") {
		t.Fatalf("instruction missing management summary requirement: %s", body)
	}
	if !strings.Contains(body, "status") || !strings.Contains(body, "what changed") || !strings.Contains(body, "thread usage") {
		t.Fatalf("instruction missing compact summary fields: %s", body)
	}
}

func TestInitializeThreadLogCreatesTemplate(t *testing.T) {
	t.Parallel()

	runDir := t.TempDir()
	outDir := filepath.Join(runDir, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("mkdir out dir: %v", err)
	}

	cfg := &Config{Layers: LayerConfig{Thread: "gemini"}}
	if err := initializeThreadLog(runDir, cfg); err != nil {
		t.Fatalf("initialize thread log: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(outDir, "thread-log.md"))
	if err != nil {
		t.Fatalf("read thread-log.md: %v", err)
	}
	body := string(got)
	if !strings.Contains(body, "thread_binary: gemini") {
		t.Fatalf("thread log missing binary: %s", body)
	}
	if !strings.Contains(body, "invocation_count: 0") {
		t.Fatalf("thread log missing invocation count: %s", body)
	}
	if !strings.Contains(body, "decomposition: none") {
		t.Fatalf("thread log missing decomposition field: %s", body)
	}
	if !strings.Contains(body, "last_review_status: pending") {
		t.Fatalf("thread log missing review status field: %s", body)
	}
}

func TestInitializeThreadEvidenceCreatesWrapperAndLedger(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	runDir := filepath.Join(root, "tasks", ".run", "run-evidence")
	if err := os.MkdirAll(filepath.Join(runDir, "out"), 0o755); err != nil {
		t.Fatalf("mkdir out dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "adapters"), 0o755); err != nil {
		t.Fatalf("mkdir adapters dir: %v", err)
	}
	adapter := "# gemini\n\n## agentic\n```\ngemini -m gemini-3.1-pro-preview -p \"{prompt}\"\n```\n"
	if err := os.WriteFile(filepath.Join(root, "adapters", "gemini.md"), []byte(adapter), 0o644); err != nil {
		t.Fatalf("write adapter: %v", err)
	}

	task := &Task{TaskID: "T-THREAD-003"}
	cfg := &Config{
		AgentOSRoot: root,
		Layers:      LayerConfig{Thread: "gemini"},
		Runner: RunnerConfig{
			ThreadFallbackModels: []string{"gemini-3-flash-preview"},
		},
	}

	if err := initializeThreadEvidence(runDir, task, cfg); err != nil {
		t.Fatalf("initialize thread evidence: %v", err)
	}

	evidenceDir := filepath.Join(runDir, "out", "thread-evidence")
	if _, err := os.Stat(filepath.Join(evidenceDir, "stdout")); err != nil {
		t.Fatalf("expected stdout dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(evidenceDir, "stderr")); err != nil {
		t.Fatalf("expected stderr dir: %v", err)
	}
	ledger, err := os.ReadFile(filepath.Join(evidenceDir, "invocations.jsonl"))
	if err != nil {
		t.Fatalf("read invocations ledger: %v", err)
	}
	if len(ledger) != 0 {
		t.Fatalf("expected empty ledger, got %q", ledger)
	}
	wrapperPath := filepath.Join(evidenceDir, "invoke-thread")
	wrapper, err := os.ReadFile(wrapperPath)
	if err != nil {
		t.Fatalf("read wrapper: %v", err)
	}
	body := string(wrapper)
	if !strings.Contains(body, "invocations.jsonl") {
		t.Fatalf("wrapper missing invocation ledger handling: %s", body)
	}
	if !strings.Contains(body, "\\\"TaskID\\\":\\\"T-THREAD-003\\\"") {
		t.Fatalf("wrapper missing task id payload: %s", body)
	}
	if !strings.Contains(body, "gemini-3-flash-preview") {
		t.Fatalf("wrapper missing fallback model payload: %s", body)
	}
	if !strings.Contains(body, "[thread-fallback] retrying with fallback template") {
		t.Fatalf("wrapper missing fallback logging: %s", body)
	}
	info, err := os.Stat(wrapperPath)
	if err != nil {
		t.Fatalf("stat wrapper: %v", err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("wrapper mode = %o, want 755", info.Mode().Perm())
	}
}

func TestBuildThreadTemplatesAddsGeminiFallbackModels(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Layers: LayerConfig{Thread: "gemini"},
		Runner: RunnerConfig{
			ThreadFallbackModels: []string{"gemini-3-flash-preview"},
		},
	}

	got := buildThreadTemplates(cfg, `gemini -m gemini-3.1-pro-preview -y -p "{prompt}" < /dev/null`)
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got[0] != `gemini -m gemini-3.1-pro-preview -y -p "{prompt}" < /dev/null` {
		t.Fatalf("unexpected primary template: %q", got[0])
	}
	if got[1] != `gemini -m gemini-3-flash-preview -y -p "{prompt}" < /dev/null` {
		t.Fatalf("unexpected fallback template: %q", got[1])
	}
}

func TestCopyThreadLogCopiesPersistentMarker(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	runDir := filepath.Join(root, "tasks", ".run", "run-thread-copy")
	outDir := filepath.Join(runDir, "out")
	outputsDir := filepath.Join(root, "outputs")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("mkdir out dir: %v", err)
	}
	if err := os.MkdirAll(outputsDir, 0o755); err != nil {
		t.Fatalf("mkdir outputs dir: %v", err)
	}

	if err := os.WriteFile(filepath.Join(outDir, "thread-log.md"), []byte("thread log"), 0o644); err != nil {
		t.Fatalf("write thread log: %v", err)
	}

	task := &Task{TaskID: "T-THREAD-002"}
	if err := copyThreadLog(task, runDir, outputsDir); err != nil {
		t.Fatalf("copy thread log: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(outputsDir, "thread-T-THREAD-002.md"))
	if err != nil {
		t.Fatalf("read copied thread marker: %v", err)
	}
	if string(got) != "thread log" {
		t.Fatalf("copied thread log = %q, want %q", got, "thread log")
	}
}

func TestCopyThreadEvidenceCopiesPersistentDirectory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	runDir := filepath.Join(root, "tasks", ".run", "run-thread-copy")
	evidenceDir := filepath.Join(runDir, "out", "thread-evidence")
	outputsDir := filepath.Join(root, "outputs")
	if err := os.MkdirAll(filepath.Join(evidenceDir, "stdout"), 0o755); err != nil {
		t.Fatalf("mkdir stdout dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(evidenceDir, "stderr"), 0o755); err != nil {
		t.Fatalf("mkdir stderr dir: %v", err)
	}
	if err := os.MkdirAll(outputsDir, 0o755); err != nil {
		t.Fatalf("mkdir outputs dir: %v", err)
	}

	if err := os.WriteFile(filepath.Join(evidenceDir, "invocations.jsonl"), []byte("{\"exit_code\":0}\n"), 0o644); err != nil {
		t.Fatalf("write invocations ledger: %v", err)
	}
	if err := os.WriteFile(filepath.Join(evidenceDir, "stdout", "one.log"), []byte("stdout"), 0o644); err != nil {
		t.Fatalf("write stdout log: %v", err)
	}

	task := &Task{TaskID: "T-THREAD-004"}
	if err := copyThreadEvidence(task, runDir, outputsDir); err != nil {
		t.Fatalf("copy thread evidence: %v", err)
	}

	dst := filepath.Join(outputsDir, "thread-evidence-T-THREAD-004", "invocations.jsonl")
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read copied invocations ledger: %v", err)
	}
	if string(got) != "{\"exit_code\":0}\n" {
		t.Fatalf("copied ledger = %q, want %q", got, "{\"exit_code\":0}\n")
	}
	if _, err := os.Stat(filepath.Join(outputsDir, "thread-evidence-T-THREAD-004", "stdout", "one.log")); err != nil {
		t.Fatalf("expected copied stdout log: %v", err)
	}
}

func TestEnsureCompletionArtifactCreatesDefaultMarkerWhenOutputMissing(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	paths := NewRuntimePaths(root, false)
	task := &Task{TaskID: "T-COMPLETE-001"}

	if err := ensureCompletionArtifact(task, paths); err != nil {
		t.Fatalf("ensureCompletionArtifact returned error: %v", err)
	}

	markerPath := filepath.Join(root, "outputs", "result-T-COMPLETE-001.md")
	got, err := os.ReadFile(markerPath)
	if err != nil {
		t.Fatalf("read completion artifact: %v", err)
	}
	body := string(got)
	if !strings.Contains(body, "Status: PASS") {
		t.Fatalf("completion artifact missing PASS status: %s", body)
	}
	if !strings.Contains(body, "TaskID: T-COMPLETE-001") {
		t.Fatalf("completion artifact missing task id: %s", body)
	}
}

func TestGenerateDirectionFailsWhenDesignRefMissing(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	runDir := filepath.Join(root, "tasks", ".run", "run-1")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("mkdir run dir: %v", err)
	}

	task := &Task{
		TaskID:    "T-018",
		Role:      "worker_code",
		DesignRef: "designs/missing.md",
	}
	cfg := &Config{
		AgentOSRoot: root,
	}

	err := generateDirection(task, cfg, runDir)
	if err == nil {
		t.Fatal("expected missing design ref to fail")
	}
	if !strings.Contains(err.Error(), "read design ref") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCopyTaskOutputCopiesNamedFileToExplicitOutputPath(t *testing.T) {
	t.Parallel()

	agentOSRoot := t.TempDir()
	runDir := filepath.Join(agentOSRoot, "tasks", ".run", "run-1")
	nestedDir := filepath.Join(runDir, "out", "nested")
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatalf("mkdir nested out dir: %v", err)
	}

	content := []byte("done")
	srcPath := filepath.Join(nestedDir, "fix-T-018.md")
	if err := os.WriteFile(srcPath, content, 0o644); err != nil {
		t.Fatalf("write output file: %v", err)
	}

	task := &Task{
		TaskID: "T-018",
		Output: "outputs/fix-T-018.md",
	}

	if err := copyTaskOutput(task, runDir, NewRuntimePaths(agentOSRoot, false)); err != nil {
		t.Fatalf("copy task output: %v", err)
	}

	dstPath := filepath.Join(agentOSRoot, "outputs", "fix-T-018.md")
	got, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatalf("read copied output: %v", err)
	}
	if string(got) != string(content) {
		t.Fatalf("copied content = %q, want %q", got, content)
	}
}

func TestCopyTaskOutputFallsBackToTaskScopedDirectory(t *testing.T) {
	t.Parallel()

	agentOSRoot := t.TempDir()
	runDir := filepath.Join(agentOSRoot, "tasks", ".run", "run-2")
	outDir := filepath.Join(runDir, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("mkdir out dir: %v", err)
	}

	if err := os.WriteFile(filepath.Join(outDir, "result.md"), []byte("ok"), 0o644); err != nil {
		t.Fatalf("write out file: %v", err)
	}

	task := &Task{TaskID: "T-099"}
	outputsDir := filepath.Join(agentOSRoot, "outputs")
	if err := copyTaskOutput(task, runDir, NewRuntimePaths(agentOSRoot, false)); err != nil {
		t.Fatalf("copy task output: %v", err)
	}

	if _, err := os.Stat(filepath.Join(outputsDir, "T-099", "result.md")); err != nil {
		t.Fatalf("expected task-scoped output file: %v", err)
	}
}

func TestCopyTaskOutputAcceptsExistingDestinationArtifact(t *testing.T) {
	t.Parallel()

	agentOSRoot := t.TempDir()
	runDir := filepath.Join(agentOSRoot, "tasks", ".run", "run-3")
	if err := os.MkdirAll(filepath.Join(runDir, "out"), 0o755); err != nil {
		t.Fatalf("mkdir out dir: %v", err)
	}

	task := &Task{
		TaskID: "T-777",
		Output: "outputs/result-T-777.md",
	}

	dstPath := filepath.Join(agentOSRoot, "outputs", "result-T-777.md")
	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		t.Fatalf("mkdir outputs dir: %v", err)
	}
	if err := os.WriteFile(dstPath, []byte("PASS\n"), 0o644); err != nil {
		t.Fatalf("write existing artifact: %v", err)
	}

	if err := copyTaskOutput(task, runDir, NewRuntimePaths(agentOSRoot, false)); err != nil {
		t.Fatalf("copy task output: %v", err)
	}

	got, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatalf("read existing artifact: %v", err)
	}
	if string(got) != "PASS\n" {
		t.Fatalf("existing artifact content = %q, want %q", got, "PASS\n")
	}
}

func TestRunQualityGateCommandsRunsSequentially(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := runQualityGateCommands([]string{
		"printf first > gate.txt",
		"printf second >> gate.txt",
	}, root); err != nil {
		t.Fatalf("runQualityGateCommands returned error: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(root, "gate.txt"))
	if err != nil {
		t.Fatalf("read gate output: %v", err)
	}
	if string(got) != "firstsecond" {
		t.Fatalf("gate output = %q, want %q", got, "firstsecond")
	}
}

func TestRunQualityGateCommandsReturnsCommandOutputOnFailure(t *testing.T) {
	t.Parallel()

	err := runQualityGateCommands([]string{"echo gate failed >&2; exit 1"}, t.TempDir())
	if err == nil {
		t.Fatal("expected quality gate error")
	}
	if !strings.Contains(err.Error(), "gate failed") {
		t.Fatalf("error %q does not include command output", err)
	}
}

func TestRunTaskExecutesQualityGateCommandsAfterSuccessfulSubprocess(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
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

	taskBody := `## Task ID: T-QG-001

## Title: quality gate run

## Depends On: []

## Execution: parallel

## Role: worker_code

## Assigned To: subprocess

## Output
` + "`outputs/result-T-QG-001.md`" + `

## Done Condition
- output exists

## QualityGate
- printf checked > gate-check.txt
`
	if err := os.WriteFile(filepath.Join(root, "tasks", "queue", "T-QG-001.md"), []byte(taskBody), 0o644); err != nil {
		t.Fatalf("write task: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "roles", "WORKER_CODE.md"), []byte("# role\n"), 0o644); err != nil {
		t.Fatalf("write role: %v", err)
	}

	adapter := "## agentic\n```\nsh -c 'run_dir=${1##*: }; mkdir -p \"$run_dir/out\"; printf \"# Result\\n\\n- Status: PASS\\n\" > \"$run_dir/out/result.md\"; printf \"artifact\\n\" > \"$run_dir/out/result-T-QG-001.md\"' sh {prompt}\n```\n"
	if err := os.WriteFile(filepath.Join(root, "adapters", "stub.md"), []byte(adapter), 0o644); err != nil {
		t.Fatalf("write adapter: %v", err)
	}

	cfg := &Config{
		AgentOSRoot: root,
		Layers: LayerConfig{
			Subprocess: "stub",
			Thread:     "stub",
		},
		Runner: RunnerConfig{
			QualityGateEnabled: true,
		},
	}
	task := &Task{
		TaskID:        "T-QG-001",
		Role:          "worker_code",
		Output:        "outputs/result-T-QG-001.md",
		DoneCondition: []string{"output exists"},
		QualityGate: []string{
			"printf checked > gate-check.txt",
		},
	}

	if err := RunTask(task, cfg); err != nil {
		t.Fatalf("RunTask returned error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, "outputs", "result-T-QG-001.md")); err != nil {
		t.Fatalf("expected copied output artifact: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(root, "gate-check.txt"))
	if err != nil {
		t.Fatalf("read quality gate marker: %v", err)
	}
	if string(got) != "checked" {
		t.Fatalf("gate marker = %q, want %q", got, "checked")
	}
}

func TestRunTaskValidatesAllDeclaredOutputsBeforeSuccess(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	setupRunTaskRetryFixture(t, root, "T-OUT-MULTI-001", "outputs/result-T-OUT-MULTI-001.md, reports/summary-T-OUT-MULTI-001.txt", `mkdir -p "$run_dir/out" "$run_dir/reports"
printf "# Result\n\n- Status: PASS\n" > "$run_dir/out/result.md"
printf "artifact\n" > "$run_dir/out/result-T-OUT-MULTI-001.md"
printf "summary\n" > "$run_dir/reports/summary-T-OUT-MULTI-001.txt"
`)

	task := &Task{
		TaskID:        "T-OUT-MULTI-001",
		Role:          "worker_code",
		Output:        "outputs/result-T-OUT-MULTI-001.md, reports/summary-T-OUT-MULTI-001.txt",
		DoneCondition: []string{"output exists"},
	}
	cfg := testRunnerConfig(root)

	if err := RunTask(task, cfg); err != nil {
		t.Fatalf("RunTask returned error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, "outputs", "result-T-OUT-MULTI-001.md")); err != nil {
		t.Fatalf("expected primary output artifact: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "outputs", "reports", "summary-T-OUT-MULTI-001.txt")); err != nil {
		t.Fatalf("expected secondary output artifact: %v", err)
	}
}

func TestRunTaskFailsWhenAnyDeclaredOutputIsMissing(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	setupRunTaskRetryFixture(t, root, "T-OUT-MULTI-002", "outputs/result-T-OUT-MULTI-002.md, reports/missing.txt", `mkdir -p "$run_dir/out"
printf "# Result\n\n- Status: PASS\n" > "$run_dir/out/result.md"
printf "artifact\n" > "$run_dir/out/result-T-OUT-MULTI-002.md"
`)

	task := &Task{
		TaskID:        "T-OUT-MULTI-002",
		Role:          "worker_code",
		Output:        "outputs/result-T-OUT-MULTI-002.md, reports/missing.txt",
		DoneCondition: []string{"output exists"},
	}
	cfg := testRunnerConfig(root)

	err := RunTask(task, cfg)
	if err == nil {
		t.Fatal("expected RunTask to fail")
	}
	if err.Error() != "Output missing: reports/missing.txt" {
		t.Fatalf("RunTask error = %q, want %q", err.Error(), "Output missing: reports/missing.txt")
	}
	if _, statErr := os.Stat(filepath.Join(root, "tasks", "queue", "T-OUT-MULTI-002.md")); !os.IsNotExist(statErr) {
		t.Fatalf("queue task should be removed after terminal failure finalization, stat err = %v", statErr)
	}
}

func TestRunTaskRetriesAfterSubprocessFailure(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	attemptFile := filepath.Join(root, "attempt.txt")
	promptCapture := filepath.Join(root, "second-prompt.txt")
	setupRunTaskRetryFixture(t, root, "T-RETRY-001", "outputs/result-T-RETRY-001.md", fmt.Sprintf(`attempt_file=%q
prompt_capture=%q
attempt=0
if [ -f "$attempt_file" ]; then
  attempt=$(cat "$attempt_file")
fi
attempt=$((attempt + 1))
printf "%%s" "$attempt" > "$attempt_file"
if [ "$attempt" -eq 1 ]; then
  echo "first failure" >&2
  exit 1
fi
printf "%%s" "$prompt" > "$prompt_capture"
if ! grep -q "first failure" "$run_dir/feedback.md"; then
  echo "missing feedback" >&2
  exit 1
fi
mkdir -p "$run_dir/out"
printf "# Result\n\n- Status: PASS\n" > "$run_dir/out/result.md"
printf "artifact\n" > "$run_dir/out/result-T-RETRY-001.md"
`, attemptFile, promptCapture))

	task := &Task{
		TaskID:        "T-RETRY-001",
		Role:          "worker_code",
		Output:        "outputs/result-T-RETRY-001.md",
		DoneCondition: []string{"output exists"},
		RetryCount:    1,
	}
	cfg := testRunnerConfig(root)

	if err := RunTask(task, cfg); err != nil {
		t.Fatalf("RunTask returned error: %v", err)
	}

	assertRetryArtifacts(t, root, attemptFile, promptCapture, "first failure")
}

func TestRunTaskRetriesAfterQualityGateCommandFailure(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	setupRunTaskRetryFixture(t, root, "T-RETRY-QG", "outputs/result-T-RETRY-QG.md", `mkdir -p "$run_dir/out"
printf "# Result\n\n- Status: PASS\n" > "$run_dir/out/result.md"
printf "artifact\n" > "$run_dir/out/result-T-RETRY-QG.md"
`)

	qgMarker := filepath.Join(root, "qg-attempt.txt")
	task := &Task{
		TaskID:        "T-RETRY-QG",
		Role:          "worker_code",
		Output:        "outputs/result-T-RETRY-QG.md",
		DoneCondition: []string{"output exists"},
		RetryCount:    1,
		QualityGate: []string{
			fmt.Sprintf("count=0; if [ -f %q ]; then count=$(cat %q); fi; count=$((count + 1)); printf '%%s' \"$count\" > %q; if [ \"$count\" -eq 1 ]; then echo quality busted >&2; exit 1; fi", qgMarker, qgMarker, qgMarker),
		},
	}
	cfg := testRunnerConfig(root)

	if err := RunTask(task, cfg); err != nil {
		t.Fatalf("RunTask returned error: %v", err)
	}

	assertRetryArtifacts(t, root, qgMarker, "", "quality busted")
}

func testRunnerConfig(root string) *Config {
	return &Config{
		AgentOSRoot: root,
		Layers: LayerConfig{
			Subprocess: "stub",
			Thread:     "stub",
		},
		Runner: RunnerConfig{
			QualityGateEnabled: true,
		},
	}
}

func setupRunTaskRetryFixture(t *testing.T, root, taskID, outputPath, behavior string) {
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

	taskBody := `## Task ID: ` + taskID + `

## Title: retry task

## Depends On: []

## Execution: parallel

## Role: worker_code

## Assigned To: subprocess

## RetryCount: 1

## Output
` + "`" + outputPath + "`" + `

## Done Condition
- output exists
`
	if err := os.WriteFile(filepath.Join(root, "tasks", "queue", taskID+".md"), []byte(taskBody), 0o644); err != nil {
		t.Fatalf("write task: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "roles", "WORKER_CODE.md"), []byte("# role\n"), 0o644); err != nil {
		t.Fatalf("write role: %v", err)
	}

	stubPath := filepath.Join(root, "adapters", "stub.sh")
	stubBody := "#!/usr/bin/env bash\nset -eu\nprompt=\"$1\"\nrun_dir=${prompt##*: }\nprompt_file=\"$run_dir/prompt.txt\"\nprintf \"%s\" \"$prompt\" > \"$prompt_file\"\n" + behavior + "\n"
	if err := os.WriteFile(stubPath, []byte(stubBody), 0o755); err != nil {
		t.Fatalf("write stub script: %v", err)
	}

	adapter := "## agentic\n```\n" + stubPath + " {prompt}\n```\n"
	if err := os.WriteFile(filepath.Join(root, "adapters", "stub.md"), []byte(adapter), 0o644); err != nil {
		t.Fatalf("write adapter: %v", err)
	}
}

func assertRetryArtifacts(t *testing.T, root, attemptFile, promptCapture, expectedFeedback string) {
	t.Helper()

	runRoot := filepath.Join(root, "outputs", "runs")
	entries, err := os.ReadDir(runRoot)
	if err != nil {
		t.Fatalf("read runs dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("run entry count = %d, want 1", len(entries))
	}

	stderrLog, err := os.ReadFile(filepath.Join(runRoot, entries[0].Name(), "logs", "stderr.log"))
	if err != nil {
		t.Fatalf("read stderr log: %v", err)
	}
	if !strings.Contains(string(stderrLog), "Retry 1 of 1") {
		t.Fatalf("stderr log missing retry marker: %s", string(stderrLog))
	}

	summary, err := os.ReadFile(filepath.Join(runRoot, entries[0].Name(), "summary.json"))
	if err != nil {
		t.Fatalf("read summary: %v", err)
	}
	if !strings.Contains(string(summary), "Retry 1 of 1") {
		t.Fatalf("summary missing retry marker: %s", string(summary))
	}

	attempts, err := os.ReadFile(attemptFile)
	if err != nil {
		t.Fatalf("read attempt file: %v", err)
	}
	if strings.TrimSpace(string(attempts)) != "2" {
		t.Fatalf("attempt count = %q, want %q", strings.TrimSpace(string(attempts)), "2")
	}

	if promptCapture != "" {
		prompt, err := os.ReadFile(promptCapture)
		if err != nil {
			t.Fatalf("read prompt capture: %v", err)
		}
		if !strings.Contains(string(prompt), "feedback.md") {
			t.Fatalf("prompt missing feedback reference: %s", string(prompt))
		}
	}

	if !strings.Contains(string(stderrLog), expectedFeedback) {
		t.Fatalf("stderr log missing %q: %s", expectedFeedback, string(stderrLog))
	}
}

func TestRunTaskPersistsCompletedRunState(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
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

	if err := os.WriteFile(filepath.Join(root, "roles", "WORKER_CODE.md"), []byte("# role\n"), 0o644); err != nil {
		t.Fatalf("write role: %v", err)
	}
	adapter := "## agentic\n```\nsh -c 'run_dir=${1##*: }; mkdir -p \"$run_dir/out\"; printf \"# Result\\n\\n- Status: PASS\\n\" > \"$run_dir/out/result.md\"; printf \"artifact\\n\" > \"$run_dir/out/result-T-STATE-001.md\"' sh {prompt}\n```\n"
	if err := os.WriteFile(filepath.Join(root, "adapters", "stub.md"), []byte(adapter), 0o644); err != nil {
		t.Fatalf("write adapter: %v", err)
	}

	task := &Task{
		TaskID:        "T-STATE-001",
		Role:          "worker_code",
		Output:        "outputs/result-T-STATE-001.md",
		DoneCondition: []string{"output exists"},
	}
	cfg := &Config{
		AgentOSRoot: root,
		Layers:      LayerConfig{Subprocess: "stub", Thread: "stub"},
		Runner:      RunnerConfig{QualityGateEnabled: true},
	}

	taskPath := filepath.Join(root, "tasks", "queue", "T-STATE-001.md")
	if err := os.WriteFile(taskPath, []byte("placeholder"), 0o644); err != nil {
		t.Fatalf("write task placeholder: %v", err)
	}
	if err := RunTask(task, cfg); err != nil {
		t.Fatalf("RunTask returned error: %v", err)
	}

	runDirs, err := os.ReadDir(filepath.Join(root, "outputs", "runs"))
	if err != nil {
		t.Fatalf("read completed runs dir: %v", err)
	}
	if len(runDirs) != 1 {
		t.Fatalf("completed run count = %d, want 1", len(runDirs))
	}

	state, err := loadRunState(filepath.Join(root, "outputs", "runs", runDirs[0].Name(), "run.json"))
	if err != nil {
		t.Fatalf("load completed run state: %v", err)
	}
	if state.TaskID != "T-STATE-001" || state.RunID == "" {
		t.Fatalf("unexpected run state identity: %#v", state)
	}
	if state.Status != "completed" {
		t.Fatalf("run state status = %q, want completed", state.Status)
	}
	if state.StartedAt == "" || state.UpdatedAt == "" {
		t.Fatalf("run state timestamps missing: %#v", state)
	}

	summary, _, err := loadRunSummary(NewRuntimePaths(root, false), runDirs[0].Name())
	if err != nil {
		t.Fatalf("load run summary: %v", err)
	}
	if summary.Status != "completed" {
		t.Fatalf("summary status = %q, want completed", summary.Status)
	}
	if summary.ResultPath != filepath.Join(root, "outputs", "result-T-STATE-001.md") {
		t.Fatalf("summary result path = %q, want completion artifact path", summary.ResultPath)
	}
	if _, err := os.Stat(taskPath); !os.IsNotExist(err) {
		t.Fatalf("queue task should be removed after completion finalization, stat err = %v", err)
	}
}

func TestRunTaskPersistsRunningStateWhileActive(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
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

	if err := os.WriteFile(filepath.Join(root, "roles", "WORKER_CODE.md"), []byte("# role\n"), 0o644); err != nil {
		t.Fatalf("write role: %v", err)
	}

	adapter := "## agentic\n```\nsh -c 'run_dir=${1##*: }; ready=\"$run_dir/ready\"; continue_file=\"$run_dir/continue\"; mkdir -p \"$run_dir/out\"; : > \"$ready\"; while [ ! -f \"$continue_file\" ]; do sleep 0.05; done; printf \"# Result\\n\\n- Status: PASS\\n\" > \"$run_dir/out/result.md\"; printf \"artifact\\n\" > \"$run_dir/out/result-T-STATE-RUNNING.md\"' sh {prompt}\n```\n"
	if err := os.WriteFile(filepath.Join(root, "adapters", "stub.md"), []byte(adapter), 0o644); err != nil {
		t.Fatalf("write adapter: %v", err)
	}

	task := &Task{
		TaskID:        "T-STATE-RUNNING",
		Role:          "worker_code",
		Output:        "outputs/result-T-STATE-RUNNING.md",
		DoneCondition: []string{"output exists"},
	}
	cfg := &Config{
		AgentOSRoot: root,
		Layers:      LayerConfig{Subprocess: "stub", Thread: "stub"},
		Runner:      RunnerConfig{QualityGateEnabled: true},
	}

	taskPath := filepath.Join(root, "tasks", "queue", "T-STATE-RUNNING.md")
	if err := os.WriteFile(taskPath, []byte("placeholder"), 0o644); err != nil {
		t.Fatalf("write task placeholder: %v", err)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- RunTask(task, cfg)
	}()

	var runDir string
	var state *RunState
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		runDirs, err := os.ReadDir(filepath.Join(root, "tasks", ".run"))
		if err != nil && !os.IsNotExist(err) {
			t.Fatalf("read active runs dir: %v", err)
		}
		for _, entry := range runDirs {
			if !entry.IsDir() {
				continue
			}
			candidate := filepath.Join(root, "tasks", ".run", entry.Name())
			if _, err := os.Stat(filepath.Join(candidate, "ready")); err != nil {
				continue
			}
			loadedState, err := loadRunState(filepath.Join(candidate, "run.json"))
			if err != nil {
				t.Fatalf("load active run state: %v", err)
			}
			runDir = candidate
			state = loadedState
			break
		}
		if runDir != "" {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}

	if runDir == "" || state == nil {
		t.Fatal("expected active run state to be present while task is running")
	}
	if state.TaskID != "T-STATE-RUNNING" || state.RunID == "" {
		t.Fatalf("unexpected active run state identity: %#v", state)
	}
	if state.Status != "running" {
		t.Fatalf("active run state status = %q, want running", state.Status)
	}
	if state.StartedAt == "" || state.UpdatedAt == "" {
		t.Fatalf("active run state timestamps missing: %#v", state)
	}

	if err := os.WriteFile(filepath.Join(runDir, "continue"), []byte("ok\n"), 0o644); err != nil {
		t.Fatalf("release subprocess: %v", err)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("RunTask returned error: %v", err)
	}
}

func TestRunTaskPersistsFailedRunState(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
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

	if err := os.WriteFile(filepath.Join(root, "roles", "WORKER_CODE.md"), []byte("# role\n"), 0o644); err != nil {
		t.Fatalf("write role: %v", err)
	}
	adapter := "## agentic\n```\nsh -c 'run_dir=${1##*: }; mkdir -p \"$run_dir/out\"; printf \"# Result\\n\\n- Status: FAIL\\n\" > \"$run_dir/out/result.md\"' sh {prompt}\n```\n"
	if err := os.WriteFile(filepath.Join(root, "adapters", "stub.md"), []byte(adapter), 0o644); err != nil {
		t.Fatalf("write adapter: %v", err)
	}

	task := &Task{
		TaskID:        "T-STATE-002",
		Role:          "worker_code",
		Output:        "outputs/result-T-STATE-002.md",
		DoneCondition: []string{"output exists"},
	}
	cfg := &Config{
		AgentOSRoot: root,
		Layers:      LayerConfig{Subprocess: "stub", Thread: "stub"},
		Runner:      RunnerConfig{QualityGateEnabled: true},
	}

	taskPath := filepath.Join(root, "tasks", "queue", "T-STATE-002.md")
	if err := os.WriteFile(taskPath, []byte("placeholder"), 0o644); err != nil {
		t.Fatalf("write task placeholder: %v", err)
	}
	if err := RunTask(task, cfg); err == nil {
		t.Fatal("expected RunTask to fail")
	}

	runDirs, err := os.ReadDir(filepath.Join(root, "outputs", "runs"))
	if err != nil {
		t.Fatalf("read completed runs dir: %v", err)
	}
	if len(runDirs) != 1 {
		t.Fatalf("completed run count = %d, want 1", len(runDirs))
	}

	state, err := loadRunState(filepath.Join(root, "outputs", "runs", runDirs[0].Name(), "run.json"))
	if err != nil {
		t.Fatalf("load completed run state: %v", err)
	}
	if state.Status != "failed" {
		t.Fatalf("run state status = %q, want failed", state.Status)
	}

	summary, _, err := loadRunSummary(NewRuntimePaths(root, false), runDirs[0].Name())
	if err != nil {
		t.Fatalf("load run summary: %v", err)
	}
	if summary.Status != "failed" {
		t.Fatalf("summary status = %q, want failed", summary.Status)
	}
	if _, err := os.Stat(taskPath); !os.IsNotExist(err) {
		t.Fatalf("queue task should be removed after failed finalization, stat err = %v", err)
	}
}

func TestRunTaskPersistsInterruptedRunStateOnEarlyFailure(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
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

	if err := os.WriteFile(filepath.Join(root, "adapters", "stub.md"), []byte("## agentic\n```\nexit 0\n```\n"), 0o644); err != nil {
		t.Fatalf("write adapter: %v", err)
	}

	task := &Task{
		TaskID:        "T-STATE-INTERRUPTED",
		Role:          "worker_code",
		Output:        "outputs/result-T-STATE-INTERRUPTED.md",
		DoneCondition: []string{"output exists"},
	}
	cfg := &Config{
		AgentOSRoot: root,
		Layers:      LayerConfig{Subprocess: "stub", Thread: "stub"},
		Runner:      RunnerConfig{QualityGateEnabled: true},
	}

	taskPath := filepath.Join(root, "tasks", "queue", "T-STATE-INTERRUPTED.md")
	if err := os.WriteFile(taskPath, []byte("placeholder"), 0o644); err != nil {
		t.Fatalf("write task placeholder: %v", err)
	}

	err := RunTask(task, cfg)
	if err == nil {
		t.Fatal("expected RunTask to fail without a queued task source")
	}

	runDirs, readErr := os.ReadDir(filepath.Join(root, "outputs", "runs"))
	if readErr != nil {
		t.Fatalf("read completed runs dir: %v", readErr)
	}
	if len(runDirs) != 1 {
		t.Fatalf("completed run count = %d, want 1", len(runDirs))
	}

	state, loadErr := loadRunState(filepath.Join(root, "outputs", "runs", runDirs[0].Name(), "run.json"))
	if loadErr != nil {
		t.Fatalf("load completed run state: %v", loadErr)
	}
	if state.Status != "interrupted" {
		t.Fatalf("run state status = %q, want interrupted", state.Status)
	}

	summary, _, err := loadRunSummary(NewRuntimePaths(root, false), runDirs[0].Name())
	if err != nil {
		t.Fatalf("load run summary: %v", err)
	}
	if summary.Status != "interrupted" {
		t.Fatalf("summary status = %q, want interrupted", summary.Status)
	}
	if _, err := os.Stat(taskPath); !os.IsNotExist(err) {
		t.Fatalf("queue task should be removed after interrupted finalization, stat err = %v", err)
	}
}

func TestWriteRunStateUpdatesTimestamp(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "run.json")
	state := &RunState{
		RunID:     "run-1",
		TaskID:    "T-1",
		Status:    "running",
		StartedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if err := writeRunState(path, state); err != nil {
		t.Fatalf("writeRunState returned error: %v", err)
	}
	if state.UpdatedAt == "" {
		t.Fatal("expected UpdatedAt to be populated")
	}
}
