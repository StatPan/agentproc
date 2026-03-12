package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	adapter := "# gemini\n\n## agentic\n```\ngemini -p \"{prompt}\"\n```\n"
	if err := os.WriteFile(filepath.Join(root, "adapters", "gemini.md"), []byte(adapter), 0o644); err != nil {
		t.Fatalf("write adapter: %v", err)
	}

	task := &Task{TaskID: "T-THREAD-003"}
	cfg := &Config{
		AgentOSRoot: root,
		Layers:      LayerConfig{Thread: "gemini"},
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
	info, err := os.Stat(wrapperPath)
	if err != nil {
		t.Fatalf("stat wrapper: %v", err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("wrapper mode = %o, want 755", info.Mode().Perm())
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

	if err := copyTaskOutput(task, runDir, filepath.Join(agentOSRoot, "outputs"), agentOSRoot); err != nil {
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
	if err := copyTaskOutput(task, runDir, outputsDir, agentOSRoot); err != nil {
		t.Fatalf("copy task output: %v", err)
	}

	if _, err := os.Stat(filepath.Join(outputsDir, "T-099", "result.md")); err != nil {
		t.Fatalf("expected task-scoped output file: %v", err)
	}
}
