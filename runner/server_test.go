package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHandleCreateTaskWritesTemplateMarkdown(t *testing.T) {
	t.Parallel()

	queueDir := t.TempDir()
	now := time.Date(2026, time.March, 10, 0, 0, 0, 0, time.UTC)
	body := `{
		"id": "T-020",
		"title": "API task",
		"role": "worker_code",
		"input": "입력 내용",
		"design_ref": "designs/foo.md",
		"depends_on": [],
		"execution": "parallel",
		"assigned_to": "codex"
	}`

	req := httptest.NewRequest(http.MethodPost, "/task", strings.NewReader(body))
	rec := httptest.NewRecorder()

	originalNow := timeNow
	timeNow = func() time.Time { return now }
	defer func() { timeNow = originalNow }()

	handleCreateTask(rec, req, queueDir, nil)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	writtenPath := filepath.Join(queueDir, "T-020.md")
	data, err := os.ReadFile(writtenPath)
	if err != nil {
		t.Fatalf("read written task: %v", err)
	}

	want := renderTaskMarkdown(taskCreateRequest{
		ID:         "T-020",
		Title:      "API task",
		Role:       "worker_code",
		Input:      "입력 내용",
		DesignRef:  "designs/foo.md",
		DependsOn:  []string{},
		Execution:  "parallel",
		AssignedTo: "codex",
	}, now)

	if string(data) != want {
		t.Fatalf("task markdown mismatch:\n got:\n%s\nwant:\n%s", string(data), want)
	}
}

func TestHandleCreateTaskRejectsInvalidRequest(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodPost, "/task", strings.NewReader(`{"id":"T-021"}`))
	rec := httptest.NewRecorder()

	handleCreateTask(rec, req, t.TempDir(), nil)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleListTasksReturnsQueueJSON(t *testing.T) {
	t.Parallel()

	queueDir := t.TempDir()
	content := `## Task ID: T-001

## Title: First task

## Depends On: [T-000]

## Execution: parallel

## Role: worker_code

## Design Ref: designs/ref.md

## Assigned To: codex

## Input

example
`
	if err := os.WriteFile(filepath.Join(queueDir, "T-001.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write task: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/tasks", nil)
	rec := httptest.NewRecorder()

	handleListTasks(rec, req, queueDir)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var tasks []taskResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &tasks); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if len(tasks) != 1 || tasks[0].ID != "T-001" {
		t.Fatalf("unexpected tasks response: %+v", tasks)
	}
}

func TestHandleCreateTaskTriggersDispatchOnceHook(t *testing.T) {
	t.Parallel()

	queueDir := t.TempDir()
	body := `{
		"id": "T-022",
		"title": "API task",
		"role": "worker_code",
		"input": "queue then dispatch",
		"design_ref": "designs/foo.md",
		"depends_on": [],
		"execution": "parallel",
		"assigned_to": "codex"
	}`

	req := httptest.NewRequest(http.MethodPost, "/task", strings.NewReader(body))
	rec := httptest.NewRecorder()

	called := 0
	handleCreateTask(rec, req, queueDir, func(ctx context.Context) error {
		called++
		return nil
	})

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if called != 1 {
		t.Fatalf("dispatch hook calls = %d, want 1", called)
	}
}

func TestHandleStatusReturnsRunnerSnapshot(t *testing.T) {
	t.Parallel()

	queueDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(queueDir, "T-001.md"), []byte("## Task ID: T-001\n"), 0o644); err != nil {
		t.Fatalf("write task: %v", err)
	}

	cfg := &Config{
		AgentOSRoot: "/tmp/agentos",
		Runner: RunnerConfig{
			MaxConcurrent: 8,
			Mode:          "daemon",
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	rec := httptest.NewRecorder()

	handleStatus(rec, req, cfg, queueDir)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var status statusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if status.Mode != "daemon" || status.MaxConcurrent != 8 || status.AgentOSRoot != "/tmp/agentos" || status.QueueSize != 1 {
		t.Fatalf("unexpected status response: %+v", status)
	}
	if status.Uptime == "" {
		t.Fatalf("uptime should not be empty")
	}

	body := rec.Body.String()
	if strings.Contains(body, "agentosRoot") || strings.Contains(body, "queueSize") || strings.Contains(body, "maxConcurrent") {
		t.Fatalf("status response should use snake_case json tags: %s", body)
	}
}
