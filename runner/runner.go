package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"time"
)

type terminalState string

const (
	terminalStateCompleted   terminalState = "completed"
	terminalStateFailed      terminalState = "failed"
	terminalStateInterrupted terminalState = "interrupted"
)

func RunTask(task *Task, cfg *Config) error {
	if task == nil {
		return fmt.Errorf("task is nil")
	}
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}

	paths := cfg.RuntimePaths()
	runID := fmt.Sprintf("%s-%s", time.Now().Format("20060102-150405"), task.TaskID)
	runDir := paths.ActiveRunDir(runID)
	if err := os.MkdirAll(filepath.Join(runDir, "out"), 0o755); err != nil {
		return fmt.Errorf("create run dir: %w", err)
	}

	taskSrc := paths.QueueTaskPath(task.TaskID)
	runTaskPath := filepath.Join(runDir, "task.md")
	if err := copyFile(taskSrc, runTaskPath); err != nil {
		return fmt.Errorf("copy task.md: %w", err)
	}

	return runTask(task, cfg, runID, runDir, taskSrc)
}

func runTask(task *Task, cfg *Config, runID, runDir, queuePath string) error {
	if task == nil {
		return fmt.Errorf("task is nil")
	}
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}

	paths := cfg.RuntimePaths()
	startedAt := time.Now()
	taskSrc := filepath.Join(runDir, "task.md")
	activeStatePath := paths.ActiveRunStatePath(runID)
	completedStatePath := paths.CompletedRunStatePath(runID)
	_, stdoutPath, stderrPath, summaryPath, err := prepareRunArtifacts(paths, runID)
	if err != nil {
		return err
	}

	summary := newRunSummary(runID, task.TaskID, taskSrc, stdoutPath, stderrPath, startedAt)
	runState := &RunState{
		RunID:     runID,
		TaskID:    task.TaskID,
		Status:    "running",
		StartedAt: startedAt.UTC().Format(time.RFC3339),
		PID:       os.Getpid(),
	}

	finalized := false
	finalize := func(status terminalState, err error) error {
		if finalized {
			return err
		}
		finalized = true
		summary.DurationMS = time.Since(startedAt).Milliseconds()
		if summary.FinishedAt == "" {
			summary.FinishedAt = time.Now().UTC().Format(time.RFC3339)
		}
		summary.Status = string(status)
		if err != nil {
			summary.Error = err.Error()
		}
		if status == terminalStateCompleted && strings.TrimSpace(summary.ResultPath) == "" {
			summary.ResultPath = completionArtifactPath(task, paths)
		}

		runState.Status = string(status)
		runState.PID = 0

		if err := saveRunSummary(summaryPath, summary); err != nil {
			return fmt.Errorf("write run summary: %w", err)
		}
		if err := writeRunState(activeStatePath, runState); err != nil {
			return fmt.Errorf("write active run state: %w", err)
		}
		if err := writeRunState(completedStatePath, runState); err != nil {
			return fmt.Errorf("write completed run state: %w", err)
		}

		if queuePath != "" {
			if err := os.Remove(queuePath); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("remove queue task after finalization: %w", err)
			}
		}

		return err
	}

	defer func() {
		if !finalized {
			_ = finalize(terminalStateInterrupted, fmt.Errorf("run terminated prematurely"))
		}
	}()

	if cfg.AgentOSRoot == "" {
		return fmt.Errorf("config AgentOSRoot is empty")
	}
	if cfg.Layers.Subprocess == "" {
		return fmt.Errorf("config layers.subprocess is empty")
	}

	if err := writeRunState(activeStatePath, runState); err != nil {
		return fmt.Errorf("write active run state: %w", err)
	}

	roleSrc := filepath.Join(cfg.AgentOSRoot, "roles", roleFileName(task.Role))
	adapterPath := filepath.Join(cfg.AgentOSRoot, "adapters", cfg.Layers.Subprocess+".md")
	outputsDir := paths.OutputsDir()

	defer os.RemoveAll(runDir)

	if err := copyFile(roleSrc, filepath.Join(runDir, "role.md")); err != nil {
		return finalize(terminalStateInterrupted, fmt.Errorf("copy role.md: %w", err))
	}

	if err := generateDirection(task, cfg, runDir); err != nil {
		return finalize(terminalStateInterrupted, fmt.Errorf("generate direction.md: %w", err))
	}
	if err := generateSubprocessInstruction(task, runDir); err != nil {
		return finalize(terminalStateInterrupted, fmt.Errorf("generate SUBPROCESS_INSTRUCTION.md: %w", err))
	}
	if err := initializeThreadEvidence(runDir, task, cfg); err != nil {
		return finalize(terminalStateInterrupted, fmt.Errorf("initialize thread evidence: %w", err))
	}
	if err := initializeThreadLog(runDir, cfg); err != nil {
		return finalize(terminalStateInterrupted, fmt.Errorf("initialize thread-log.md: %w", err))
	}

	if err := ValidateThreadWrapperUsage(runDir); err != nil {
		return finalize(terminalStateInterrupted, fmt.Errorf("validate thread wrapper: %w", err))
	}

	template, err := LoadAdapter(adapterPath)
	if err != nil {
		return finalize(terminalStateInterrupted, fmt.Errorf("load adapter: %w", err))
	}

	stdoutFile, err := os.Create(stdoutPath)
	if err != nil {
		return finalize(terminalStateInterrupted, fmt.Errorf("create stdout log: %w", err))
	}
	defer stdoutFile.Close()
	stderrFile, err := os.Create(stderrPath)
	if err != nil {
		return finalize(terminalStateInterrupted, fmt.Errorf("create stderr log: %w", err))
	}
	defer stderrFile.Close()

	retryCount := 0
	var runErr error
	for {
		attempt := retryCount + 1
		summary.Events = append(summary.Events, fmt.Sprintf("subprocess attempt %d started", attempt))

		command := BuildCommand(template, buildPrompt(runDir, retryCount > 0))
		cmd := buildExecCommand(command)
		cmd.Dir = cfg.AgentOSRoot
		cmd.Stdout = stdoutFile
		cmd.Stderr = stderrFile

		runErr = cmd.Run()
		if runErr != nil {
			summary.Events = append(summary.Events, fmt.Sprintf("subprocess attempt %d exited with error", attempt))
		} else {
			summary.Events = append(summary.Events, fmt.Sprintf("subprocess attempt %d completed", attempt))
		}

		attemptErr := validateTaskAttempt(task, cfg, runDir, runErr, stderrPath)
		if attemptErr == nil {
			break
		}

		if !shouldRetryTask(*task, retryCount) {
			status := terminalState(statusForAttemptError(runErr))
			return finalize(status, attemptErr)
		}

		retryCount++
		retryMessage := fmt.Sprintf("Retry %d of %d", retryCount, task.RetryCount)
		summary.Events = append(summary.Events, retryMessage)
		if _, err := fmt.Fprintln(stderrFile, retryMessage); err != nil {
			return finalize(terminalStateInterrupted, fmt.Errorf("write retry log: %w", err))
		}
		if _, err := fmt.Fprintln(stderrFile, strings.TrimSpace(attemptErr.Error())); err != nil {
			return finalize(terminalStateInterrupted, fmt.Errorf("write retry error: %w", err))
		}
		if err := writeRetryFeedback(runDir, retryCount, task.RetryCount, attemptErr); err != nil {
			return finalize(terminalStateInterrupted, fmt.Errorf("write retry feedback: %w", err))
		}
	}

	if err := os.MkdirAll(outputsDir, 0o755); err != nil {
		return finalize(terminalStateFailed, fmt.Errorf("create outputs dir: %w", err))
	}
	if err := copyTaskOutput(task, runDir, paths); err != nil {
		return finalize(terminalStateFailed, fmt.Errorf("copy out to outputs: %w", err))
	}
	if err := copyThreadEvidence(task, runDir, outputsDir); err != nil {
		return finalize(terminalStateFailed, fmt.Errorf("copy thread evidence to outputs: %w", err))
	}
	if err := copyThreadLog(task, runDir, outputsDir); err != nil {
		return finalize(terminalStateFailed, fmt.Errorf("copy thread log to outputs: %w", err))
	}
	if err := ensureCompletionArtifact(task, paths); err != nil {
		return finalize(terminalStateFailed, fmt.Errorf("ensure completion artifact: %w", err))
	}
	summary.ThreadLog = filepath.Join(outputsDir, fmt.Sprintf("thread-%s.md", task.TaskID))
	summary.EvidencePath = filepath.Join(outputsDir, fmt.Sprintf("thread-evidence-%s", task.TaskID))
	summary.ResultPath = completionArtifactPath(task, paths)

	summary.Events = append(summary.Events, "artifacts copied to outputs", "run completed")
	return finalize(terminalStateCompleted, nil)
}

// isCodeRole returns true for roles that require code quality conventions.
// Covers worker_code and reviewer roles.
func isCodeRole(role string) bool {
	r := strings.ToLower(strings.TrimSpace(role))
	return r == "worker_code" || r == "reviewer"
}

func roleFileName(role string) string {
	replacer := strings.NewReplacer("-", "_", " ", "_")
	return strings.ToUpper(replacer.Replace(strings.TrimSpace(role))) + ".md"
}

func buildPrompt(runDir string, includeFeedback bool) string {
	if includeFeedback {
		return fmt.Sprintf("Read role.md, task.md, direction.md, SUBPROCESS_INSTRUCTION.md, and feedback.md, then execute the task in: %s", runDir)
	}
	return fmt.Sprintf("Read role.md, task.md, direction.md, and SUBPROCESS_INSTRUCTION.md, then execute the task in: %s", runDir)
}

func validateTaskAttempt(task *Task, cfg *Config, runDir string, runErr error, stderrPath string) error {
	if runErr != nil {
		stderrData, _ := os.ReadFile(stderrPath)
		if len(stderrData) > 0 {
			return fmt.Errorf("subprocess exited abnormally: %v\nStderr:\n%s", runErr, string(stderrData))
		}
		return fmt.Errorf("subprocess exited abnormally: %w", runErr)
	}

	if err := ValidateTaskMarker(task.TaskID, runDir); err != nil {
		_ = generateResultMarker(task, runDir, "FAIL")
		return fmt.Errorf("validate task marker: %w", err)
	}

	if err := ValidateDoneCondition(*task, runDir); err != nil {
		return fmt.Errorf("validate done condition: %w", err)
	}

	if err := ValidateOutputPaths(*task, cfg.AgentOSRoot, runDir); err != nil {
		return err
	}

	if err := runQualityGateCommands(task.QualityGate, cfg.AgentOSRoot); err != nil {
		return fmt.Errorf("run quality gate commands: %w", err)
	}

	return nil
}

func shouldRetryTask(task Task, retryCount int) bool {
	if task.RetryCount <= 0 {
		return false
	}
	return retryCount < task.RetryCount
}

func writeRetryFeedback(runDir string, retryCount, maxRetries int, attemptErr error) error {
	content := fmt.Sprintf(
		"# Retry Feedback\n\nRetry %d of %d\n\nThe previous attempt failed:\n\n%s\n",
		retryCount,
		maxRetries,
		strings.TrimSpace(attemptErr.Error()),
	)
	return os.WriteFile(filepath.Join(runDir, "feedback.md"), []byte(content), 0o644)
}

func statusForAttemptError(runErr error) string {
	if runErr == nil {
		return "failed"
	}

	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		if status, ok := exitErr.Sys().(syscall.WaitStatus); ok && status.Signaled() {
			return "interrupted"
		}
	}

	return "failed"
}

func buildExecCommand(command string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.Command("cmd", "/C", command)
	}
	return exec.Command("sh", "-c", command)
}

func runQualityGateCommands(commands []string, workingDir string) error {
	for _, command := range commands {
		trimmed := strings.TrimSpace(command)
		if trimmed == "" {
			continue
		}

		cmd := exec.Command("sh", "-c", trimmed)
		cmd.Dir = workingDir
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("quality gate command %q failed: %w: %s", trimmed, err, strings.TrimSpace(string(output)))
		}
	}

	return nil
}

func generateDirection(task *Task, cfg *Config, runDir string) error {
	var sb strings.Builder

	sb.WriteString("# Direction\n\n")
	sb.WriteString("## AgentOS Root\n")
	sb.WriteString(cfg.AgentOSRoot + "\n\n")

	if task.Input != "" {
		sb.WriteString("## Input\n")
		sb.WriteString(task.Input + "\n\n")
	}

	if task.DesignRef != "" {
		designPath := filepath.Join(cfg.AgentOSRoot, task.DesignRef)
		data, err := os.ReadFile(designPath)
		if err != nil {
			return fmt.Errorf("read design ref %s: %w", designPath, err)
		} else {
			sb.WriteString("## Design Reference (" + task.DesignRef + ")\n\n")
			sb.Write(data)
			sb.WriteString("\n")
		}
	}

	if task.Output != "" {
		sb.WriteString("## Output Target\n")
		sb.WriteString(task.Output + "\n")
	}

	sb.WriteString("\n## Thread Execution Contract\n")
	sb.WriteString("- Thread Binary: " + cfg.Layers.Thread + "\n")
	sb.WriteString("- Thread Adapter: adapters/" + cfg.Layers.Thread + ".md\n")
	sb.WriteString("- Thread Wrapper: out/thread-evidence/invoke-thread\n")
	sb.WriteString("- Evidence Directory: out/thread-evidence/\n")
	sb.WriteString("- Invocation Ledger: out/thread-evidence/invocations.jsonl\n")
	sb.WriteString("- Trace File: out/thread-log.md\n")
	sb.WriteString("- Management Summary: out/summary.md\n")
	sb.WriteString("- Persisted Evidence: outputs/thread-evidence-" + task.TaskID + "/\n")
	sb.WriteString("- Persisted Marker: outputs/thread-" + task.TaskID + ".md (if thread-log.md exists)\n")
	sb.WriteString("- Required Fields: invocation command, timestamps, exit code, stdout/stderr path\n")
	sb.WriteString("- Process Read Path: process/reviewer should read out/summary.md first, then runner-generated evidence, and open thread-log/result details only when needed\n")

	if isCodeRole(task.Role) {
		conventionsPath := filepath.Join(cfg.AgentOSRoot, "conventions", "CODE_CONVENTIONS.md")
		data, err := os.ReadFile(conventionsPath)
		if err == nil {
			sb.WriteString("\n## Code Quality Conventions\n\n")
			sb.Write(data)
			sb.WriteString("\n")
		} else {
			sb.WriteString("\n## Code Quality Conventions\n")
			sb.WriteString("(conventions/CODE_CONVENTIONS.md not found — apply industry-standard Go/language style)\n")
		}
	}

	return os.WriteFile(filepath.Join(runDir, "direction.md"), []byte(sb.String()), 0o644)
}

func generateSubprocessInstruction(task *Task, runDir string) error {
	content := fmt.Sprintf("# Subprocess Instruction\n\n"+
		"## Role\n"+
		"You are acting as %s at the subprocess layer. Your value is not raw implementation speed. Your job is to complete the assigned scope by delegating appropriately to the thread layer, reviewing the result, and driving it to completion.\n\n"+
		"## Required behavior\n"+
		"1. Read `task.md`, `direction.md`, and the Thread Execution Contract first.\n"+
		"2. Check the configured thread layer before acting. Even if the work looks small, first identify the smallest viable unit that can be delegated to a thread worker.\n"+
		"3. Invoke the configured thread layer through `out/thread-evidence/invoke-thread`. Do not handcraft direct shell commands when the wrapper is available.\n"+
		"4. If helpful, decompose the work into smaller thread-friendly units and run only independent units in parallel.\n"+
		"5. Pass only the necessary context to the thread worker. Keep subprocess attention on scope management and review.\n"+
		"6. Judge success by how effectively the thread layer was used to complete the task, not by how much work you did directly.\n"+
		"7. Treat runner-generated evidence as the source of truth for thread execution. Use `out/thread-evidence/invocations.jsonl`, `stdout/`, and `stderr/` first. Keep `out/thread-log.md` as a human-readable summary only.\n"+
		"8. Review each thread result against Done Condition, Quality Gate, and marker/log contracts.\n"+
		"9. If the result is insufficient, write `out/feedback.md` with concrete gaps and direction, narrow the failed scope into a smaller corrective leaf task, and re-invoke the thread worker with that narrower corrective task. Do not simply repeat the same broad task unless no narrower split is possible.\n"+
		"10. Do not fall back to direct subprocess completion immediately after one weak thread result. Prefer `review -> narrower corrective re-delegation -> review`. Direct subprocess completion is an exception path.\n"+
		"11. If you must complete work directly at the subprocess layer, only do so after at least one corrective re-delegation attempt or when you can clearly justify why the task cannot be decomposed further for the thread layer.\n"+
		"12. When direct subprocess completion happens, record the justification in both `out/summary.md` and `out/result.md`, including why corrective re-delegation was insufficient.\n"+
		"13. Leave a short management summary in `out/summary.md` so process and reviewer do not need to reread everything. Default read order is `out/summary.md` -> runner-generated evidence -> detailed logs only when needed. A compact structure with `status`, `what changed`, `thread usage`, `evidence`, `risks`, and `next action` is sufficient.\n"+
		"14. Write `out/result.md` with PASS or FAIL, thread invocation count, whether decomposition happened, review and retry summary, whether corrective re-delegation was attempted, and changed or verified file paths.\n",
		strings.ToUpper(task.Role))

	return os.WriteFile(filepath.Join(runDir, "SUBPROCESS_INSTRUCTION.md"), []byte(content), 0o644)
}

func initializeThreadLog(runDir string, cfg *Config) error {
	content := fmt.Sprintf(`# Thread Log

- thread_binary: %s
- adapter: adapters/%s.md
- invocation_count: 0
- reinvoked: no
- decomposition: none
- last_review_status: pending
- attempts:
`, cfg.Layers.Thread, cfg.Layers.Thread)

	return os.WriteFile(filepath.Join(runDir, "out", "thread-log.md"), []byte(content), 0o644)
}

func initializeThreadEvidence(runDir string, task *Task, cfg *Config) error {
	evidenceDir := filepath.Join(runDir, "out", "thread-evidence")
	stdoutDir := filepath.Join(evidenceDir, "stdout")
	stderrDir := filepath.Join(evidenceDir, "stderr")
	for _, dir := range []string{stdoutDir, stderrDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	adapterPath := filepath.Join(cfg.AgentOSRoot, "adapters", cfg.Layers.Thread+".md")
	template, err := LoadAdapter(adapterPath)
	if err != nil {
		return fmt.Errorf("load thread adapter: %w", err)
	}

	if err := os.WriteFile(filepath.Join(evidenceDir, "invocations.jsonl"), nil, 0o644); err != nil {
		return err
	}

	script, err := renderThreadWrapper(threadWrapperConfig{
		AgentOSRoot: cfg.AgentOSRoot,
		TaskID:      task.TaskID,
		RunID:       filepath.Base(runDir),
		ThreadLayer: cfg.Layers.Thread,
		AdapterPath: adapterPath,
		Templates:   buildThreadTemplates(cfg, template),
		EvidenceDir: evidenceDir,
	})
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(evidenceDir, "invoke-thread"), []byte(script), 0o755)
}

type threadWrapperConfig struct {
	AgentOSRoot string
	TaskID      string
	RunID       string
	ThreadLayer string
	AdapterPath string
	Templates   []string
	EvidenceDir string
}

var geminiModelPattern = regexp.MustCompile(`(^|[[:space:]])-m[[:space:]]+([^[:space:]]+)`)

func buildThreadTemplates(cfg *Config, primaryTemplate string) []string {
	templates := []string{primaryTemplate}
	if cfg == nil || cfg.Layers.Thread != "gemini" {
		return templates
	}

	seen := map[string]struct{}{
		primaryTemplate: {},
	}
	for _, model := range cfg.Runner.ThreadFallbackModels {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		fallbackTemplate := replaceGeminiModel(primaryTemplate, model)
		if fallbackTemplate == primaryTemplate {
			continue
		}
		if _, ok := seen[fallbackTemplate]; ok {
			continue
		}
		seen[fallbackTemplate] = struct{}{}
		templates = append(templates, fallbackTemplate)
	}

	return templates
}

func replaceGeminiModel(template, model string) string {
	loc := geminiModelPattern.FindStringSubmatchIndex(template)
	if loc == nil || len(loc) < 6 {
		return template
	}
	return template[:loc[4]] + model + template[loc[5]:]
}

func renderThreadWrapper(cfg threadWrapperConfig) (string, error) {
	payload, err := json.Marshal(cfg)
	if err != nil {
		return "", fmt.Errorf("marshal thread wrapper config: %w", err)
	}

	return fmt.Sprintf(`#!/usr/bin/env bash
set -euo pipefail

python3 - "$@" <<'PY'
import json
import os
import pathlib
import subprocess
import sys
import threading
import time
from datetime import datetime, timezone

cfg = json.loads(%q)
prompt = " ".join(sys.argv[1:])
if not prompt:
    print("invoke-thread requires a prompt", file=sys.stderr)
    sys.exit(2)

templates = cfg["Templates"]
evidence_dir = pathlib.Path(cfg["EvidenceDir"])
stdout_dir = evidence_dir / "stdout"
stderr_dir = evidence_dir / "stderr"
stdout_dir.mkdir(parents=True, exist_ok=True)
stderr_dir.mkdir(parents=True, exist_ok=True)

stamp = datetime.now(timezone.utc)
invocation_id = stamp.strftime("%%Y%%m%%dT%%H%%M%%S%%fZ") + "-" + str(os.getpid())
stdout_path = stdout_dir / f"{invocation_id}.log"
stderr_path = stderr_dir / f"{invocation_id}.log"
started_at = stamp.isoformat()
started_ns = time.time_ns()
rendered_commands = []
selected_command = ""
selected_template_index = -1

with stdout_path.open("wb") as stdout_file, stderr_path.open("wb") as stderr_file:
    exit_code = 1

    def tee(stream, sink):
        try:
            while True:
                chunk = stream.read(4096)
                if not chunk:
                    break
                sink.write(chunk)
                sink.flush()
        finally:
            stream.close()

    def should_fallback(code, stderr_text):
        if code == 0:
            return False
        if cfg["ThreadLayer"] != "gemini":
            return False
        checks = [
            "status 429",
            '"status": 429',
            "too many requests",
            "resource_exhausted",
            "model_capacity_exhausted",
            "no capacity available for model",
        ]
        lowered = stderr_text.lower()
        return any(token in lowered for token in checks)

    for idx, template in enumerate(templates):
        command = template.replace("{prompt}", prompt)
        rendered_commands.append(command)
        selected_command = command
        selected_template_index = idx
        if idx > 0:
            stderr_file.write(
                f"\n[thread-fallback] retrying with fallback template {idx + 1} of {len(templates)}\n".encode("utf-8")
            )
            stderr_file.flush()

        proc = subprocess.Popen(
            command,
            shell=True,
            cwd=cfg["AgentOSRoot"],
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
        )

        threads = [
            threading.Thread(target=tee, args=(proc.stdout, stdout_file), daemon=True),
            threading.Thread(target=tee, args=(proc.stderr, stderr_file), daemon=True),
        ]
        for worker in threads:
            worker.start()

        exit_code = proc.wait()
        for worker in threads:
            worker.join()

        if exit_code == 0 or idx == len(templates) - 1:
            break

        stderr_text = stderr_path.read_text(encoding="utf-8", errors="replace")
        if not should_fallback(exit_code, stderr_text):
            break

finished = datetime.now(timezone.utc)
record = {
    "invocation_id": invocation_id,
    "task_id": cfg["TaskID"],
    "run_id": cfg["RunID"],
    "thread_layer": cfg["ThreadLayer"],
    "thread_binary": cfg["ThreadLayer"],
    "adapter_path": cfg["AdapterPath"],
    "argv": ["/bin/sh", "-c", selected_command],
    "rendered_command": selected_command,
    "rendered_commands": rendered_commands,
    "template_count": len(templates),
    "selected_template_index": selected_template_index,
    "started_at": started_at,
    "finished_at": finished.isoformat(),
    "duration_ms": max(0, (time.time_ns() - started_ns) // 1_000_000),
    "exit_code": exit_code,
    "stdout_path": str(stdout_path),
    "stderr_path": str(stderr_path),
    "workspace": cfg["AgentOSRoot"],
}

with (evidence_dir / "invocations.jsonl").open("a", encoding="utf-8") as ledger:
    ledger.write(json.dumps(record, ensure_ascii=False) + "\n")

sys.exit(exit_code)
PY
`, string(payload)), nil
}

func generateResultMarker(task *Task, runDir, status string) error {
	markerPath := filepath.Join(runDir, "out", "result.md")
	content := fmt.Sprintf("# Result\n\n- Status: %s\n- TaskID: %s\n- Evidence: out/thread-evidence/\n", status, task.TaskID)
	return os.WriteFile(markerPath, []byte(content), 0o644)
}

func copyTaskOutput(task *Task, runDir string, paths *RuntimePaths) error {
	outDir := filepath.Join(runDir, "out")
	if task.Output == "" {
		outputsDir := paths.OutputsDir()
		taskOutputDir := filepath.Join(outputsDir, task.TaskID)
		if err := os.MkdirAll(taskOutputDir, 0o755); err != nil {
			return fmt.Errorf("create task output dir: %w", err)
		}
		return copyDir(outDir, taskOutputDir)
	}

	for _, out := range task.ExpectedOutputPaths() {
		dstFile := paths.ResolveOutputPath(out)

		// Accept workers that already wrote the expected artifact directly to its final path.
		if info, err := os.Stat(dstFile); err == nil && !info.IsDir() {
			continue
		} else if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("stat output file %s: %w", dstFile, err)
		}

		fileName := filepath.Base(out)
		srcFile, err := findOutputFile(outDir, fileName)
		if err != nil {
			// Check if it's in runDir but not in out/
			runPath := filepath.Join(runDir, out)
			if info, err := os.Stat(runPath); err == nil && !info.IsDir() {
				if err := copyFile(runPath, dstFile); err != nil {
					return fmt.Errorf("copy output file %s from runDir: %w", out, err)
				}
				continue
			}
			// Check if it's in runDir/out with the relative path
			runOutPath := filepath.Join(runDir, "out", out)
			if info, err := os.Stat(runOutPath); err == nil && !info.IsDir() {
				if err := copyFile(runOutPath, dstFile); err != nil {
					return fmt.Errorf("copy output file %s from runDir/out: %w", out, err)
				}
				continue
			}
			// Already in project root - no need to copy
			if _, err := os.Stat(filepath.Join(paths.agentOSRoot, out)); err == nil {
				continue
			}
			return err
		}
		if err := copyFile(srcFile, dstFile); err != nil {
			return fmt.Errorf("copy output file %s: %w", fileName, err)
		}
	}
	return nil
}

func completionArtifactPath(task *Task, paths *RuntimePaths) string {
	if outputs := task.ExpectedOutputPaths(); len(outputs) > 0 {
		return paths.ResolveOutputPath(outputs[0])
	}
	return paths.ResolveOutputPath(filepath.Join("outputs", "result-"+task.TaskID+".md"))
}

func ensureCompletionArtifact(task *Task, paths *RuntimePaths) error {
	markerPath := completionArtifactPath(task, paths)
	if info, err := os.Stat(markerPath); err == nil && !info.IsDir() {
		return nil
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("stat completion artifact: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(markerPath), 0o755); err != nil {
		return fmt.Errorf("mkdir completion artifact dir: %w", err)
	}

	content := fmt.Sprintf("# Result\n\n- Status: PASS\n- TaskID: %s\n- Completion Artifact: %s\n", task.TaskID, filepath.Base(markerPath))
	if err := os.WriteFile(markerPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write completion artifact: %w", err)
	}
	return nil
}

func copyThreadLog(task *Task, runDir string, outputsDir string) error {
	src := filepath.Join(runDir, "out", "thread-log.md")
	if _, err := os.Stat(src); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	dst := filepath.Join(outputsDir, fmt.Sprintf("thread-%s.md", task.TaskID))
	return copyFile(src, dst)
}

func copyThreadEvidence(task *Task, runDir string, outputsDir string) error {
	src := filepath.Join(runDir, "out", "thread-evidence")
	if _, err := os.Stat(src); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	dst := filepath.Join(outputsDir, fmt.Sprintf("thread-evidence-%s", task.TaskID))
	return copyDir(src, dst)
}

func findOutputFile(outDir string, fileName string) (string, error) {
	var matches []string

	err := filepath.WalkDir(outDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if d.Name() == fileName {
			matches = append(matches, path)
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("walk out dir %s: %w", outDir, err)
	}

	switch len(matches) {
	case 0:
		return "", fmt.Errorf("output file %s not found in out/", fileName)
	case 1:
		return matches[0], nil
	default:
		sort.Strings(matches)
		return "", fmt.Errorf("multiple output files named %s found in out/: %s", fileName, strings.Join(matches, ", "))
	}
}

func copyOptionalFile(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("%s is a directory", src)
	}
	return copyFile(src, dst)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}

	return out.Close()
}

func copyDir(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := os.MkdirAll(dstPath, 0o755); err != nil {
				return err
			}
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
			continue
		}

		if err := copyFile(srcPath, dstPath); err != nil {
			return err
		}
	}

	return nil
}
