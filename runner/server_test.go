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

func TestHandleWorkSessionsBuildsSessionGraph(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	paths := NewRuntimePaths(root, false)
	for _, dir := range []string{paths.RequestsDir(), paths.CompletedRunsDir(), paths.OutputsDir()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	request := &RequestState{
		RequestID:    "req_1234",
		OriginalText: "Audit the runner UI",
		Status:       StatusRunning,
		Summary:      "Task created: T-AUTO-001",
		TaskID:       "T-AUTO-001",
		TaskIDs:      []string{"T-AUTO-001"},
		CreatedAt:    "2026-04-01T00:00:00Z",
		UpdatedAt:    "2026-04-01T00:00:00Z",
	}
	if err := saveRequest(paths.RequestsDir(), request); err != nil {
		t.Fatalf("saveRequest: %v", err)
	}

	runDir := paths.CompletedRunDir("20260401-101010-T-AUTO-001")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("mkdir run dir: %v", err)
	}
	summary := &RunSummary{
		RunID:        "20260401-101010-T-AUTO-001",
		TaskID:       "T-AUTO-001",
		Status:       "completed",
		StartedAt:    "2026-04-01T10:10:10Z",
		FinishedAt:   "2026-04-01T10:10:20Z",
		ResultPath:   filepath.Join(paths.OutputsDir(), "result-T-AUTO-001.md"),
		EvidencePath: filepath.Join(paths.OutputsDir(), "thread-evidence-T-AUTO-001"),
	}
	if err := saveRunSummary(paths.RunSummaryPath(summary.RunID), summary); err != nil {
		t.Fatalf("saveRunSummary: %v", err)
	}

	store := newWorkSessionStore(paths)

	req := httptest.NewRequest(http.MethodGet, "/api/work-sessions/req_1234", nil)
	rec := httptest.NewRecorder()
	handleWorkSessionDetail(rec, req, store)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var detail WorkSessionDetail
	if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil {
		t.Fatalf("unmarshal detail: %v", err)
	}

	if detail.Session.ID != "req_1234" || detail.Session.Status != StatusCompleted {
		t.Fatalf("unexpected session detail: %+v", detail.Session)
	}
	if detail.Session.Stage != "done" {
		t.Fatalf("session stage = %q, want done", detail.Session.Stage)
	}
	if len(detail.Tasks) != 1 || detail.Tasks[0].ID != "T-AUTO-001" {
		t.Fatalf("unexpected tasks: %+v", detail.Tasks)
	}
	if detail.Tasks[0].Stage != "done" || len(detail.Tasks[0].Runs) != 1 || detail.Tasks[0].Runs[0].Stage != "done" {
		t.Fatalf("expected task/run stage projection, got task=%+v run=%+v", detail.Tasks[0], detail.Tasks[0].Runs)
	}
	if len(detail.Topology.Nodes) < 2 {
		t.Fatalf("expected topology nodes, got %+v", detail.Topology)
	}
}

func TestHandleWorkSessionsCreatesNewSessionAndDispatches(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	paths := NewRuntimePaths(root, false)
	for _, dir := range []string{paths.RequestsDir(), paths.QueueDir()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	called := 0
	req := httptest.NewRequest(http.MethodPost, "/api/work-sessions", strings.NewReader(`{"request":"Implement dashboard retries in runner/main.go with tests"}`))
	rec := httptest.NewRecorder()

	handleWorkSessions(rec, req, newWorkSessionStore(paths), func(ctx context.Context) error {
		called++
		return nil
	})

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if called != 1 {
		t.Fatalf("dispatch call count = %d, want 1", called)
	}

	var request RequestState
	if err := json.Unmarshal(rec.Body.Bytes(), &request); err != nil {
		t.Fatalf("unmarshal request: %v", err)
	}
	if request.RequestID == "" || request.TaskID == "" || request.Status != StatusRunning {
		t.Fatalf("unexpected request payload: %+v", request)
	}
}

func TestHandleNodeMessageAnswersClarification(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	paths := NewRuntimePaths(root, false)
	for _, dir := range []string{paths.RequestsDir(), paths.QueueDir()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	request := &RequestState{
		RequestID:    "req_q1",
		OriginalText: "Inspect docs",
		Status:       StatusNeedsClarification,
		Summary:      "Inspect docs (needs clarification)",
		Questions:    []string{"Which docs?"},
		CreatedAt:    "2026-04-01T00:00:00Z",
		UpdatedAt:    "2026-04-01T00:00:00Z",
	}
	if err := saveRequest(paths.RequestsDir(), request); err != nil {
		t.Fatalf("saveRequest: %v", err)
	}

	body := `{"message":"Start from docs/PRD.md"}`
	req := httptest.NewRequest(http.MethodPost, "/api/nodes/request:req_q1/message", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handleNodeRoutes(rec, req, paths, nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	updated, err := loadRequest(paths, "req_q1")
	if err != nil {
		t.Fatalf("loadRequest: %v", err)
	}
	if len(updated.Answers) != 1 || updated.TaskID == "" || updated.Status != StatusRunning {
		t.Fatalf("request was not advanced after answer: %+v", updated)
	}
}

func TestHandleNodeMessageDispatchesAfterClarificationCompletes(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	paths := NewRuntimePaths(root, false)
	for _, dir := range []string{paths.RequestsDir(), paths.QueueDir()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	request := &RequestState{
		RequestID:    "req_q2",
		OriginalText: "Inspect docs",
		Status:       StatusNeedsClarification,
		Summary:      "Inspect docs (needs clarification)",
		Questions:    []string{"Which docs?"},
		CreatedAt:    "2026-04-01T00:00:00Z",
		UpdatedAt:    "2026-04-01T00:00:00Z",
	}
	if err := saveRequest(paths.RequestsDir(), request); err != nil {
		t.Fatalf("saveRequest: %v", err)
	}

	called := 0
	body := `{"message":"Start from docs/PRD.md"}`
	req := httptest.NewRequest(http.MethodPost, "/api/nodes/request:req_q2/message", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handleNodeRoutes(rec, req, paths, func(ctx context.Context) error {
		called++
		return nil
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if called != 1 {
		t.Fatalf("dispatch call count = %d, want 1", called)
	}
}

func TestHandleNodeControlRetryCreatesNewTask(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	paths := NewRuntimePaths(root, false)
	for _, dir := range []string{paths.RequestsDir(), paths.QueueDir()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	request := &RequestState{
		RequestID:    "req_retry",
		OriginalText: "Retry this request",
		Status:       StatusFailed,
		Summary:      "Task failed",
		TaskID:       "T-AUTO-001",
		TaskIDs:      []string{"T-AUTO-001"},
		CreatedAt:    "2026-04-01T00:00:00Z",
		UpdatedAt:    "2026-04-01T00:00:00Z",
	}
	if err := saveRequest(paths.RequestsDir(), request); err != nil {
		t.Fatalf("saveRequest: %v", err)
	}

	body := `{"command":"retry"}`
	req := httptest.NewRequest(http.MethodPost, "/api/nodes/request:req_retry/control", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handleNodeRoutes(rec, req, paths, nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	updated, err := loadRequest(paths, "req_retry")
	if err != nil {
		t.Fatalf("loadRequest: %v", err)
	}
	if updated.TaskID == "" || updated.TaskID == "T-AUTO-001" || len(updated.TaskIDs) != 2 {
		t.Fatalf("retry did not create a new task: %+v", updated)
	}
	if _, err := os.Stat(paths.QueueTaskPath(updated.TaskID)); err != nil {
		t.Fatalf("expected new queue task file: %v", err)
	}
}

func TestHandleWorkSessionTimelineShowsRecordedThenQueued(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	paths := NewRuntimePaths(root, false)
	request := &RequestState{
		RequestID:    "req_timeline",
		OriginalText: "Trace the timeline",
		Status:       StatusRunning,
		Summary:      "Task created: T-AUTO-001",
		TaskID:       "T-AUTO-001",
		TaskIDs:      []string{"T-AUTO-001"},
		CreatedAt:    "2026-04-01T00:00:00Z",
		UpdatedAt:    "2026-04-01T00:00:00Z",
	}
	if err := ensureCanonicalSessionSeed(paths, request); err != nil {
		t.Fatalf("seed canonical session: %v", err)
	}
	if err := appendCanonicalEvent(paths, request.RequestID, eventNodeMessageRecorded, "request", request.RequestID, nodeMessagePayload{
		Message: "apply this on next run",
	}); err != nil {
		t.Fatalf("append node message: %v", err)
	}
	if err := appendCanonicalEvent(paths, request.RequestID, eventRunClaimed, "run", "run-1", runClaimedPayload{
		RunID:     "run-1",
		TaskID:    "T-AUTO-001",
		StartedAt: "2026-04-01T01:00:00Z",
	}); err != nil {
		t.Fatalf("append run claim: %v", err)
	}

	store := newWorkSessionStore(paths)
	req := httptest.NewRequest(http.MethodGet, "/api/work-sessions/req_timeline/timeline", nil)
	rec := httptest.NewRecorder()
	handleWorkSessionTimeline(rec, req, store)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var timeline []TimelineEvent
	if err := json.Unmarshal(rec.Body.Bytes(), &timeline); err != nil {
		t.Fatalf("unmarshal timeline: %v", err)
	}
	foundQueued := false
	for _, item := range timeline {
		if item.EventType == eventNodeMessageRecorded {
			foundQueued = item.Status == "queued" && item.ConsumedByRunID == "run-1"
		}
	}
	if !foundQueued {
		t.Fatalf("expected node message to become queued with consumed run, got %+v", timeline)
	}
}
