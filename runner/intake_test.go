package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunIntakeCommandRequestsMissingContext(t *testing.T) {
	root := t.TempDir()
	writeConfigFile(t, root)

	output := captureStdout(t, func() {
		if err := runIntakeCommand([]string{"--root", root, "--config", filepath.Join(root, "config.yaml"), "로그 회전 구조 점검"}); err != nil {
			t.Fatalf("run intake: %v", err)
		}
	})

	if !strings.Contains(output, "NEEDS_INPUT I-") {
		t.Fatalf("unexpected output: %s", output)
	}

	intakeDir := filepath.Join(root, "tasks", ".intake")
	entries, err := os.ReadDir(intakeDir)
	if err != nil {
		t.Fatalf("read intake dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 intake session, got %d", len(entries))
	}
}

func TestRunReplyCommandCreatesTaskAfterAnswersComplete(t *testing.T) {
	root := t.TempDir()
	writeConfigFile(t, root)

	if err := os.MkdirAll(filepath.Join(root, "tasks", ".intake"), 0o755); err != nil {
		t.Fatalf("mkdir intake: %v", err)
	}

	session := &IntakeSession{
		ID:        "I-20260312-000001",
		Request:   "로그 회전 구조 점검",
		Questions: []string{"범위는?", "검증은?"},
		Answers:   []string{"runner/ 기준"},
		Status:    "needs_input",
		CreatedAt: "2026-03-12T00:00:00Z",
	}
	if err := saveIntakeSession(filepath.Join(root, "tasks", ".intake"), session); err != nil {
		t.Fatalf("save intake session: %v", err)
	}

	output := captureStdout(t, func() {
		if err := runReplyCommand([]string{"--root", root, "--config", filepath.Join(root, "config.yaml"), session.ID, "테스트 포함"}); err != nil {
			t.Fatalf("run reply: %v", err)
		}
	})

	if !strings.Contains(output, "READY "+session.ID) {
		t.Fatalf("unexpected output: %s", output)
	}

	queueEntries, err := os.ReadDir(filepath.Join(root, "tasks", "queue"))
	if err != nil {
		t.Fatalf("read queue: %v", err)
	}
	if len(queueEntries) != 1 {
		t.Fatalf("expected 1 generated task, got %d", len(queueEntries))
	}

	taskPath := filepath.Join(root, "tasks", "queue", queueEntries[0].Name())
	data, err := os.ReadFile(taskPath)
	if err != nil {
		t.Fatalf("read task: %v", err)
	}
	taskBody := string(data)
	if !strings.Contains(taskBody, "로그 회전 구조 점검") || !strings.Contains(taskBody, "runner/ 기준") || !strings.Contains(taskBody, "테스트 포함") {
		t.Fatalf("generated task missing intake context: %s", taskBody)
	}
}

func writeConfigFile(t *testing.T, root string) {
	t.Helper()
	config := `agentos_root: ""
layers:
  process: claude
  subprocess: codex
  thread: gemini
runner:
  max_concurrent: 1
  poll_interval: 5s
  mode: daemon
  default_retry_count: 0
  quality_gate_enabled: true
`
	if err := os.WriteFile(filepath.Join(root, "config.yaml"), []byte(config), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	originalStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = writer

	defer func() {
		os.Stdout = originalStdout
	}()

	fn()

	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(reader); err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	return buf.String()
}
