package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

func RunTask(task *Task, cfg *Config) error {
	if task == nil {
		return fmt.Errorf("task is nil")
	}
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}
	if cfg.AgentOSRoot == "" {
		return fmt.Errorf("config AgentOSRoot is empty")
	}
	if cfg.Layers.Subprocess == "" {
		return fmt.Errorf("config layers.subprocess is empty")
	}

	runID := fmt.Sprintf("%s-%s", time.Now().Format("20060102-150405"), task.TaskID)
	runDir := filepath.Join(cfg.AgentOSRoot, "tasks", ".run", runID)
	if err := os.MkdirAll(filepath.Join(runDir, "out"), 0o755); err != nil {
		return fmt.Errorf("create run dir: %w", err)
	}
	defer os.RemoveAll(runDir)

	taskSrc := filepath.Join(cfg.AgentOSRoot, "tasks", "queue", task.TaskID+".md")
	roleSrc := filepath.Join(cfg.AgentOSRoot, "roles", roleFileName(task.Role))
	adapterPath := filepath.Join(cfg.AgentOSRoot, "adapters", cfg.Layers.Subprocess+".md")
	outputsDir := filepath.Join(cfg.AgentOSRoot, "outputs")
	_, stdoutPath, stderrPath, summaryPath, err := prepareRunArtifacts(cfg.AgentOSRoot, runID)
	if err != nil {
		return err
	}
	startedAt := time.Now()
	summary := newRunSummary(runID, task.TaskID, taskSrc, stdoutPath, stderrPath, startedAt)
	defer func() {
		summary.DurationMS = time.Since(startedAt).Milliseconds()
		if summary.FinishedAt == "" {
			summary.FinishedAt = time.Now().UTC().Format(time.RFC3339)
		}
		_ = saveRunSummary(summaryPath, summary)
	}()

	if err := copyFile(taskSrc, filepath.Join(runDir, "task.md")); err != nil {
		return fmt.Errorf("copy task.md: %w", err)
	}
	if err := copyFile(roleSrc, filepath.Join(runDir, "role.md")); err != nil {
		return fmt.Errorf("copy role.md: %w", err)
	}

	if err := generateDirection(task, cfg, runDir); err != nil {
		return fmt.Errorf("generate direction.md: %w", err)
	}
	if err := generateSubprocessInstruction(task, runDir); err != nil {
		return fmt.Errorf("generate SUBPROCESS_INSTRUCTION.md: %w", err)
	}
	if err := initializeThreadEvidence(runDir, task, cfg); err != nil {
		return fmt.Errorf("initialize thread evidence: %w", err)
	}
	if err := initializeThreadLog(runDir, cfg); err != nil {
		return fmt.Errorf("initialize thread-log.md: %w", err)
	}

	if err := ValidateThreadWrapperUsage(runDir); err != nil {
		return fmt.Errorf("validate thread wrapper: %w", err)
	}

	template, err := LoadAdapter(adapterPath)
	if err != nil {
		return fmt.Errorf("load adapter: %w", err)
	}

	stdoutFile, err := os.Create(stdoutPath)
	if err != nil {
		return fmt.Errorf("create stdout log: %w", err)
	}
	defer stdoutFile.Close()
	stderrFile, err := os.Create(stderrPath)
	if err != nil {
		return fmt.Errorf("create stderr log: %w", err)
	}
	defer stderrFile.Close()

	command := BuildCommand(template, buildPrompt(runDir))
	cmd := buildExecCommand(command)
	cmd.Dir = cfg.AgentOSRoot
	cmd.Stdout = stdoutFile
	cmd.Stderr = stderrFile
	summary.Events = append(summary.Events, "subprocess started")

	runErr := cmd.Run()
	if runErr != nil {
		summary.Events = append(summary.Events, "subprocess exited with error")
	} else {
		summary.Events = append(summary.Events, "subprocess completed")
	}

	if err := ValidateTaskMarker(task.TaskID, runDir); err != nil {
		generateResultMarker(task, runDir, "FAIL")
		summary.Status = "failed"
		summary.Error = fmt.Sprintf("validate task marker: %v", err)
		summary.Events = append(summary.Events, "result marker validation failed")
		return fmt.Errorf("validate task marker: %w", err)
	}

	if err := ValidateDoneCondition(*task, runDir); err != nil {
		summary.Status = "failed"
		summary.Error = fmt.Sprintf("validate done condition: %v", err)
		summary.Events = append(summary.Events, "done condition validation failed")
		return fmt.Errorf("validate done condition: %w", err)
	}

	if cfg.Runner.QualityGateEnabled {
		if err := ValidateQualityGate(*task, runDir); err != nil {
			summary.Status = "failed"
			summary.Error = fmt.Sprintf("validate quality gate: %v", err)
			summary.Events = append(summary.Events, "quality gate validation failed")
			return fmt.Errorf("validate quality gate: %w", err)
		}
	}

	// Assuming 0 for now as retry count logic isn't fully exposed in RunTask
	if err := ValidateRetryCount(*task, 0); err != nil {
		summary.Status = "failed"
		summary.Error = fmt.Sprintf("validate retry count: %v", err)
		return fmt.Errorf("validate retry count: %w", err)
	}

	if err := os.MkdirAll(outputsDir, 0o755); err != nil {
		return fmt.Errorf("create outputs dir: %w", err)
	}
	if err := copyTaskOutput(task, runDir, outputsDir, cfg.AgentOSRoot); err != nil {
		summary.Status = "failed"
		summary.Error = fmt.Sprintf("copy out to outputs: %v", err)
		return fmt.Errorf("copy out to outputs: %w", err)
	}
	if err := copyThreadEvidence(task, runDir, outputsDir); err != nil {
		summary.Status = "failed"
		summary.Error = fmt.Sprintf("copy thread evidence: %v", err)
		return fmt.Errorf("copy thread evidence to outputs: %w", err)
	}
	if err := copyThreadLog(task, runDir, outputsDir); err != nil {
		summary.Status = "failed"
		summary.Error = fmt.Sprintf("copy thread log: %v", err)
		return fmt.Errorf("copy thread log to outputs: %w", err)
	}
	summary.ThreadLog = filepath.Join(outputsDir, fmt.Sprintf("thread-%s.md", task.TaskID))
	summary.EvidencePath = filepath.Join(outputsDir, fmt.Sprintf("thread-evidence-%s", task.TaskID))
	summary.ResultPath = task.Output
	if summary.ResultPath != "" && !filepath.IsAbs(summary.ResultPath) {
		summary.ResultPath = filepath.Join(cfg.AgentOSRoot, summary.ResultPath)
	}

	if runErr != nil {
		summary.Status = "failed"
		summary.Error = fmt.Sprintf("subprocess exited abnormally: %v", runErr)
		return fmt.Errorf("subprocess exited abnormally: %w", runErr)
	}
	summary.Status = "completed"
	summary.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	summary.Events = append(summary.Events, "artifacts copied to outputs", "run completed")

	return nil
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

func buildPrompt(runDir string) string {
	return fmt.Sprintf("Read role.md, task.md, direction.md, and SUBPROCESS_INSTRUCTION.md, then execute the task in: %s", runDir)
}

func buildExecCommand(command string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.Command("cmd", "/C", command)
	}
	return exec.Command("sh", "-c", command)
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
		"9. If the result is insufficient, write `out/feedback.md` with concrete gaps and direction, then re-invoke the thread worker. Be specific about goals and constraints instead of micromanaging edits.\n"+
		"10. Leave a short management summary in `out/summary.md` so process and reviewer do not need to reread everything. Default read order is `out/summary.md` -> runner-generated evidence -> detailed logs only when needed. A compact structure with `status`, `what changed`, `thread usage`, `evidence`, `risks`, and `next action` is sufficient.\n"+
		"11. Write `out/result.md` with PASS or FAIL, thread invocation count, whether decomposition happened, review and retry summary, and changed or verified file paths.\n",
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
		Template:    template,
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
	Template    string
	EvidenceDir string
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

template = cfg["Template"]
command = template.replace("{prompt}", prompt)
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

with stdout_path.open("wb") as stdout_file, stderr_path.open("wb") as stderr_file:
    proc = subprocess.Popen(
        command,
        shell=True,
        cwd=cfg["AgentOSRoot"],
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )

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

    threads = [
        threading.Thread(target=tee, args=(proc.stdout, stdout_file), daemon=True),
        threading.Thread(target=tee, args=(proc.stderr, stderr_file), daemon=True),
    ]
    for worker in threads:
        worker.start()

    exit_code = proc.wait()
    for worker in threads:
        worker.join()

finished = datetime.now(timezone.utc)
record = {
    "invocation_id": invocation_id,
    "task_id": cfg["TaskID"],
    "run_id": cfg["RunID"],
    "thread_layer": cfg["ThreadLayer"],
    "thread_binary": cfg["ThreadLayer"],
    "adapter_path": cfg["AdapterPath"],
    "argv": ["/bin/sh", "-c", command],
    "rendered_command": command,
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

func copyTaskOutput(task *Task, runDir string, outputsDir string, agentOSRoot string) error {
	outDir := filepath.Join(runDir, "out")
	if task.Output != "" {
		fileName := filepath.Base(task.Output)
		srcFile, err := findOutputFile(outDir, fileName)
		if err != nil {
			return err
		}
		dstFile := task.Output
		cleaned := filepath.Clean(dstFile)
		prefix := "outputs" + string(filepath.Separator)
		if cleaned == "outputs" || strings.HasPrefix(cleaned, prefix) {
			dstFile = filepath.Join(agentOSRoot, cleaned)
		} else if !filepath.IsAbs(dstFile) {
			dstFile = filepath.Join(agentOSRoot, dstFile)
		}
		if err := copyFile(srcFile, dstFile); err != nil {
			return fmt.Errorf("copy output file %s: %w", fileName, err)
		}
		return nil
	}
	taskOutputDir := filepath.Join(outputsDir, task.TaskID)
	if err := os.MkdirAll(taskOutputDir, 0o755); err != nil {
		return fmt.Errorf("create task output dir: %w", err)
	}
	return copyDir(outDir, taskOutputDir)
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
