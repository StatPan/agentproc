package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const (
	eventSessionCreated        = "session_created"
	eventClarificationAsked    = "clarification_asked"
	eventClarificationAnswered = "clarification_answered"
	eventTaskQueued            = "task_queued"
	eventNodeMessageRecorded   = "node_message_recorded"
	eventControlApplied        = "control_applied"
	eventRunClaimed            = "run_claimed"
	eventRunFinished           = "run_finished"
	eventSessionCancelled      = "session_cancelled"
)

type canonicalStore struct {
	db *sql.DB
}

type sessionEventRecord struct {
	Sequence  int64
	SessionID string
	EventType string
	NodeType  string
	NodeID    string
	CreatedAt string
	Payload   json.RawMessage
}

type canonicalSession struct {
	Request       *RequestState
	Tasks         map[string]*canonicalTask
	TaskOrder     []string
	Messages      []Intervention
	Interventions []Intervention
}

type canonicalTask struct {
	TaskID      string
	Title       string
	Role        string
	AssignedTo  string
	Execution   string
	Input       string
	DependsOn   []string
	OutputPaths []string
	Status      string
	Runs        []*RunView
}

type taskQueuedPayload struct {
	TaskID      string   `json:"task_id"`
	Title       string   `json:"title"`
	Role        string   `json:"role"`
	AssignedTo  string   `json:"assigned_to"`
	Execution   string   `json:"execution"`
	Input       string   `json:"input"`
	DependsOn   []string `json:"depends_on"`
	OutputPaths []string `json:"output_paths"`
}

type clarificationPayload struct {
	Questions []string `json:"questions,omitempty"`
	Answer    string   `json:"answer,omitempty"`
}

type nodeMessagePayload struct {
	Message string `json:"message"`
}

type controlPayload struct {
	Command string `json:"command"`
}

type runClaimedPayload struct {
	RunID     string `json:"run_id"`
	TaskID    string `json:"task_id"`
	StartedAt string `json:"started_at"`
}

type runFinishedPayload struct {
	RunID        string   `json:"run_id"`
	TaskID       string   `json:"task_id"`
	Status       string   `json:"status"`
	StartedAt    string   `json:"started_at"`
	FinishedAt   string   `json:"finished_at"`
	DurationMS   int64    `json:"duration_ms"`
	ResultPath   string   `json:"result_path,omitempty"`
	StdoutPath   string   `json:"stdout_path,omitempty"`
	StderrPath   string   `json:"stderr_path,omitempty"`
	EvidencePath string   `json:"evidence_path,omitempty"`
	Error        string   `json:"error,omitempty"`
	Events       []string `json:"events,omitempty"`
}

type TimelineEvent struct {
	Sequence        int64          `json:"sequence"`
	SessionID       string         `json:"session_id"`
	EventType       string         `json:"event_type"`
	NodeType        string         `json:"node_type"`
	NodeID          string         `json:"node_id"`
	NodeRef         string         `json:"node_ref"`
	Title           string         `json:"title"`
	Status          string         `json:"status"`
	CreatedAt       string         `json:"created_at"`
	Payload         map[string]any `json:"payload,omitempty"`
	ConsumedByRunID string         `json:"consumed_by_run_id,omitempty"`
}

func openCanonicalStore(paths *RuntimePaths) (*canonicalStore, error) {
	if err := os.MkdirAll(paths.CanonicalDir(), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir canonical dir: %w", err)
	}
	db, err := sql.Open("sqlite", paths.CanonicalDBPath())
	if err != nil {
		return nil, fmt.Errorf("open canonical db: %w", err)
	}
	store := &canonicalStore{db: db}
	if err := store.init(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *canonicalStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *canonicalStore) init() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS work_session (
			session_id TEXT PRIMARY KEY,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS session_event (
			sequence INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT NOT NULL,
			event_type TEXT NOT NULL,
			node_type TEXT,
			node_id TEXT,
			created_at TEXT NOT NULL,
			payload TEXT NOT NULL,
			FOREIGN KEY(session_id) REFERENCES work_session(session_id)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_session_event_session_seq ON session_event(session_id, sequence);`,
		`CREATE INDEX IF NOT EXISTS idx_session_event_node ON session_event(node_type, node_id);`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("init canonical schema: %w", err)
		}
	}
	return nil
}

func appendCanonicalEvent(paths *RuntimePaths, sessionID, eventType, nodeType, nodeID string, payload any) error {
	store, err := openCanonicalStore(paths)
	if err != nil {
		return err
	}
	defer store.Close()

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal canonical event: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	tx, err := store.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(
		`INSERT INTO work_session(session_id, created_at, updated_at)
		 VALUES(?, ?, ?)
		 ON CONFLICT(session_id) DO UPDATE SET updated_at=excluded.updated_at`,
		sessionID, now, now,
	); err != nil {
		return fmt.Errorf("upsert work session: %w", err)
	}
	if _, err := tx.Exec(
		`INSERT INTO session_event(session_id, event_type, node_type, node_id, created_at, payload)
		 VALUES(?, ?, ?, ?, ?, ?)`,
		sessionID, eventType, nodeType, nodeID, now, string(data),
	); err != nil {
		return fmt.Errorf("insert session event: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return rebuildSessionProjection(paths, sessionID)
}

func canonicalSessionIDs(paths *RuntimePaths) ([]string, error) {
	store, err := openCanonicalStore(paths)
	if err != nil {
		return nil, err
	}
	defer store.Close()

	rows, err := store.db.Query(`SELECT session_id FROM work_session ORDER BY updated_at DESC`)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func loadCanonicalSession(paths *RuntimePaths, sessionID string) (*canonicalSession, error) {
	events, err := loadCanonicalEventRecords(paths, sessionID)
	if err != nil {
		return nil, err
	}
	if len(events) == 0 {
		return nil, os.ErrNotExist
	}
	return reduceCanonicalSession(events)
}

func loadCanonicalEventRecords(paths *RuntimePaths, sessionID string) ([]sessionEventRecord, error) {
	store, err := openCanonicalStore(paths)
	if err != nil {
		return nil, err
	}
	defer store.Close()

	rows, err := store.db.Query(
		`SELECT sequence, session_id, event_type, COALESCE(node_type,''), COALESCE(node_id,''), created_at, payload
		 FROM session_event
		 WHERE session_id = ?
		 ORDER BY sequence ASC`,
		sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []sessionEventRecord
	for rows.Next() {
		var rec sessionEventRecord
		var payload string
		if err := rows.Scan(&rec.Sequence, &rec.SessionID, &rec.EventType, &rec.NodeType, &rec.NodeID, &rec.CreatedAt, &payload); err != nil {
			return nil, err
		}
		rec.Payload = json.RawMessage(payload)
		events = append(events, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

func reduceCanonicalSession(events []sessionEventRecord) (*canonicalSession, error) {
	session := &canonicalSession{
		Request: &RequestState{
			Status: StatusQueued,
		},
		Tasks: make(map[string]*canonicalTask),
	}

	for _, event := range events {
		session.Request.RequestID = event.SessionID
		if session.Request.CreatedAt == "" {
			session.Request.CreatedAt = event.CreatedAt
		}
		session.Request.UpdatedAt = event.CreatedAt

		switch event.EventType {
		case eventSessionCreated:
			var payload struct {
				OriginalText string `json:"original_text"`
				Summary      string `json:"summary"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				return nil, err
			}
			session.Request.OriginalText = payload.OriginalText
			session.Request.Summary = payload.Summary
		case eventClarificationAsked:
			var payload clarificationPayload
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				return nil, err
			}
			session.Request.Questions = append([]string(nil), payload.Questions...)
			session.Request.Status = StatusNeedsClarification
		case eventClarificationAnswered:
			var payload clarificationPayload
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				return nil, err
			}
			session.Request.Answers = append(session.Request.Answers, payload.Answer)
			if len(session.Request.Answers) < len(session.Request.Questions) {
				session.Request.Status = StatusNeedsClarification
				session.Request.Summary = fmt.Sprintf("Answered %d of %d questions", len(session.Request.Answers), len(session.Request.Questions))
			}
		case eventTaskQueued:
			var payload taskQueuedPayload
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				return nil, err
			}
			task := session.ensureTask(payload.TaskID)
			task.Title = payload.Title
			task.Role = payload.Role
			task.AssignedTo = payload.AssignedTo
			task.Execution = payload.Execution
			task.Input = payload.Input
			task.DependsOn = append([]string(nil), payload.DependsOn...)
			task.OutputPaths = append([]string(nil), payload.OutputPaths...)
			task.Status = StatusQueued
			session.Request.Status = StatusRunning
			session.Request.Summary = fmt.Sprintf("Task created: %s", payload.TaskID)
			setCurrentTask(session.Request, payload.TaskID)
		case eventNodeMessageRecorded:
			var payload nodeMessagePayload
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				return nil, err
			}
			item := Intervention{
				ID:        fmt.Sprintf("evt-%d", event.Sequence),
				NodeID:    event.NodeType + ":" + event.NodeID,
				NodeType:  event.NodeType,
				Action:    "message",
				Message:   payload.Message,
				Status:    "recorded",
				CreatedAt: event.CreatedAt,
				Metadata: map[string]any{
					"request_id": event.SessionID,
				},
			}
			session.Interventions = append(session.Interventions, item)
			if event.NodeType == "request" {
				session.Messages = append(session.Messages, item)
			}
		case eventControlApplied:
			var payload controlPayload
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				return nil, err
			}
			item := Intervention{
				ID:        fmt.Sprintf("evt-%d", event.Sequence),
				NodeID:    event.NodeType + ":" + event.NodeID,
				NodeType:  event.NodeType,
				Action:    "control",
				Command:   payload.Command,
				Status:    "applied",
				CreatedAt: event.CreatedAt,
				Metadata: map[string]any{
					"request_id": event.SessionID,
				},
			}
			session.Interventions = append(session.Interventions, item)
		case eventRunClaimed:
			var payload runClaimedPayload
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				return nil, err
			}
			task := session.ensureTask(payload.TaskID)
			task.Status = StatusRunning
			run := &RunView{
				ID:                payload.RunID,
				TaskID:            payload.TaskID,
				Status:            StatusRunning,
				StartedAt:         payload.StartedAt,
				Active:            true,
				AvailableControls: availableRunControls(StatusRunning),
			}
			task.Runs = prependOrReplaceRun(task.Runs, run)
			session.Request.RunID = payload.RunID
			session.Request.Status = StatusRunning
		case eventRunFinished:
			var payload runFinishedPayload
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				return nil, err
			}
			task := session.ensureTask(payload.TaskID)
			run := &RunView{
				ID:                payload.RunID,
				TaskID:            payload.TaskID,
				Status:            payload.Status,
				StartedAt:         payload.StartedAt,
				FinishedAt:        payload.FinishedAt,
				DurationMS:        payload.DurationMS,
				ResultPath:        payload.ResultPath,
				StdoutPath:        payload.StdoutPath,
				StderrPath:        payload.StderrPath,
				EvidencePath:      payload.EvidencePath,
				Error:             payload.Error,
				Events:            append([]string(nil), payload.Events...),
				AvailableControls: availableRunControls(payload.Status),
			}
			task.Runs = prependOrReplaceRun(task.Runs, run)
			task.Status = payload.Status
			session.Request.RunID = payload.RunID
			switch payload.Status {
			case terminalStateCompleted.String(), StatusCompleted:
				session.Request.Status = StatusCompleted
				session.Request.Summary = fmt.Sprintf("Run completed: %s", payload.RunID)
			case terminalStateFailed.String(), terminalStateInterrupted.String(), StatusFailed:
				session.Request.Status = StatusFailed
				session.Request.Summary = fmt.Sprintf("Run failed: %s", payload.RunID)
			default:
				session.Request.Status = payload.Status
				session.Request.Summary = fmt.Sprintf("Run status: %s", payload.Status)
			}
		case eventSessionCancelled:
			session.Request.Status = StatusCancelled
			session.Request.Summary = "Cancelled from UI"
			for _, task := range session.Tasks {
				if task.Status == StatusQueued || task.Status == StatusRunning {
					task.Status = StatusCancelled
				}
			}
		}
	}

	return session, nil
}

func (s *canonicalSession) ensureTask(taskID string) *canonicalTask {
	if task, ok := s.Tasks[taskID]; ok {
		return task
	}
	task := &canonicalTask{TaskID: taskID, Status: StatusQueued}
	s.Tasks[taskID] = task
	s.TaskOrder = append(s.TaskOrder, taskID)
	return task
}

func prependOrReplaceRun(runs []*RunView, next *RunView) []*RunView {
	for i, run := range runs {
		if run.ID == next.ID {
			runs[i] = next
			sort.Slice(runs, func(i, j int) bool {
				return runs[i].StartedAt > runs[j].StartedAt
			})
			return runs
		}
	}
	runs = append(runs, next)
	sort.Slice(runs, func(i, j int) bool {
		return runs[i].StartedAt > runs[j].StartedAt
	})
	return runs
}

func rebuildSessionProjection(paths *RuntimePaths, sessionID string) error {
	session, err := loadCanonicalSession(paths, sessionID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(paths.RequestsDir(), 0o755); err != nil {
		return err
	}
	if err := saveRequest(paths.RequestsDir(), session.Request); err != nil {
		return err
	}

	for _, taskID := range session.TaskOrder {
		task := session.Tasks[taskID]
		if task == nil {
			continue
		}
		queuePath := paths.QueueTaskPath(taskID)
		if task.Status == StatusQueued {
			if err := writeProjectedTask(paths, session.Request.RequestID, task); err != nil {
				return err
			}
		} else {
			if err := os.Remove(queuePath); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
	}
	return nil
}

func writeProjectedTask(paths *RuntimePaths, sessionID string, task *canonicalTask) error {
	if task == nil {
		return nil
	}
	if err := os.MkdirAll(paths.QueueDir(), 0o755); err != nil {
		return err
	}
	content := fmt.Sprintf(`## Task ID: %s

## Session ID: %s

## Title: %s

## Depends On: [%s]

## Execution: %s

## Role: %s

## Design Ref: none

## Assigned To: %s

## Input

%s

## Output

%s

## Done Condition

- [ ] 요청이 구조화된 태스크 입력으로 정리됨
- [ ] 결과 마커가 outputs/ 하위에 기록됨

## Quality Gate

- [ ] 컨텍스트 누락 없이 실행 가능한 수준으로 작성됨

## Timeout: 30m

## Retry Count: 0
`, task.TaskID, sessionID, sanitizeSingleLine(task.Title), strings.Join(task.DependsOn, ", "), task.Execution, task.Role, task.AssignedTo, task.Input, formatOutputSection(task.OutputPaths))
	return os.WriteFile(paths.QueueTaskPath(task.TaskID), []byte(content), 0o644)
}

func formatOutputSection(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	lines := make([]string, 0, len(paths))
	for _, path := range paths {
		lines = append(lines, "`"+path+"`")
	}
	return strings.Join(lines, "\n")
}

func canonicalRequestIDs(paths *RuntimePaths) ([]string, error) {
	ids, err := canonicalSessionIDs(paths)
	if err == nil && len(ids) > 0 {
		return ids, nil
	}
	if err != nil && !strings.Contains(err.Error(), "no such table") {
		return nil, err
	}
	return nil, nil
}

func loadSessionTimeline(paths *RuntimePaths, sessionID string) ([]TimelineEvent, error) {
	records, err := loadCanonicalEventRecords(paths, sessionID)
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return []TimelineEvent{}, nil
	}
	session, err := reduceCanonicalSession(records)
	if err != nil {
		return nil, err
	}

	timeline := make([]TimelineEvent, 0, len(records))
	for _, rec := range records {
		item := TimelineEvent{
			Sequence:  rec.Sequence,
			SessionID: rec.SessionID,
			EventType: rec.EventType,
			NodeType:  rec.NodeType,
			NodeID:    rec.NodeID,
			NodeRef:   rec.NodeType + ":" + rec.NodeID,
			Title:     timelineTitle(rec),
			Status:    derivedTimelineStatus(records, rec, session),
			CreatedAt: rec.CreatedAt,
		}
		if len(rec.Payload) > 0 {
			var payload map[string]any
			if err := json.Unmarshal(rec.Payload, &payload); err == nil {
				item.Payload = payload
			}
		}
		if item.Status == "queued" || item.Status == "applied" {
			item.ConsumedByRunID = consumedByRunID(records, rec, session)
		}
		timeline = append(timeline, item)
	}
	return timeline, nil
}

func timelineTitle(rec sessionEventRecord) string {
	switch rec.EventType {
	case eventSessionCreated:
		return "Session created"
	case eventClarificationAsked:
		return "Clarification requested"
	case eventClarificationAnswered:
		return "Clarification answered"
	case eventTaskQueued:
		return "Task branch queued"
	case eventNodeMessageRecorded:
		return "Intervention recorded"
	case eventControlApplied:
		return "Control requested"
	case eventRunClaimed:
		return "Run started"
	case eventRunFinished:
		return "Run finished"
	case eventSessionCancelled:
		return "Session cancelled"
	default:
		return rec.EventType
	}
}

func derivedTimelineStatus(records []sessionEventRecord, rec sessionEventRecord, session *canonicalSession) string {
	switch rec.EventType {
	case eventNodeMessageRecorded, eventControlApplied:
		runID := consumedByRunID(records, rec, session)
		if runID == "" {
			return "recorded"
		}
		for _, later := range records {
			if later.EventType == eventRunFinished {
				var payload runFinishedPayload
				if json.Unmarshal(later.Payload, &payload) == nil && payload.RunID == runID {
					return "applied"
				}
			}
		}
		return "queued"
	case eventTaskQueued:
		runID := consumedByRunID(records, rec, session)
		if runID != "" {
			return "running"
		}
		return "queued"
	case eventRunClaimed:
		return "running"
	case eventRunFinished:
		var payload runFinishedPayload
		if json.Unmarshal(rec.Payload, &payload) == nil {
			return payload.Status
		}
		return "completed"
	case eventSessionCancelled:
		return "cancelled"
	case eventClarificationAsked:
		return "needs-clarification"
	case eventClarificationAnswered:
		return "recorded"
	default:
		return "recorded"
	}
}

func consumedByRunID(records []sessionEventRecord, rec sessionEventRecord, session *canonicalSession) string {
	targetTaskID := ""
	switch rec.NodeType {
	case "task":
		targetTaskID = rec.NodeID
	case "run":
		targetTaskID = taskIDForRun(session, rec.NodeID)
	case "request":
		targetTaskID = ""
	}

	for _, later := range records {
		if later.Sequence <= rec.Sequence || later.EventType != eventRunClaimed {
			continue
		}
		var payload runClaimedPayload
		if err := json.Unmarshal(later.Payload, &payload); err != nil {
			continue
		}
		if targetTaskID == "" || payload.TaskID == targetTaskID {
			return payload.RunID
		}
	}
	return ""
}

func taskIDForRun(session *canonicalSession, runID string) string {
	if session == nil {
		return ""
	}
	for _, task := range session.Tasks {
		for _, run := range task.Runs {
			if run.ID == runID {
				return task.TaskID
			}
		}
	}
	return ""
}

func sessionIDForTask(paths *RuntimePaths, taskID string) (string, error) {
	ids, err := canonicalSessionIDs(paths)
	if err != nil {
		return "", err
	}
	for _, id := range ids {
		session, err := loadCanonicalSession(paths, id)
		if err != nil {
			continue
		}
		if _, ok := session.Tasks[taskID]; ok {
			return id, nil
		}
	}
	return "", os.ErrNotExist
}

func ensureCanonicalSessionSeed(paths *RuntimePaths, request *RequestState) error {
	if request == nil || strings.TrimSpace(request.RequestID) == "" {
		return nil
	}
	if _, err := loadCanonicalSession(paths, request.RequestID); err == nil {
		return nil
	}

	if err := appendCanonicalEvent(paths, request.RequestID, eventSessionCreated, "request", request.RequestID, map[string]any{
		"original_text": request.OriginalText,
		"summary":       request.Summary,
	}); err != nil {
		return err
	}
	if len(request.Questions) > 0 {
		if err := appendCanonicalEvent(paths, request.RequestID, eventClarificationAsked, "request", request.RequestID, clarificationPayload{
			Questions: append([]string(nil), request.Questions...),
		}); err != nil {
			return err
		}
		for _, answer := range request.Answers {
			if err := appendCanonicalEvent(paths, request.RequestID, eventClarificationAnswered, "request", request.RequestID, clarificationPayload{
				Answer: answer,
			}); err != nil {
				return err
			}
		}
	}
	for _, taskID := range allTaskIDs(request) {
		if err := appendCanonicalEvent(paths, request.RequestID, eventTaskQueued, "task", taskID, taskQueuedPayload{
			TaskID:      taskID,
			Title:       taskID,
			Role:        "designer",
			AssignedTo:  defaultAssignedTo(),
			Execution:   "parallel",
			Input:       request.OriginalText,
			DependsOn:   []string{},
			OutputPaths: []string{fmt.Sprintf("outputs/result-%s.md", taskID)},
		}); err != nil {
			return err
		}
	}
	if request.Status == StatusCancelled {
		if err := appendCanonicalEvent(paths, request.RequestID, eventSessionCancelled, "request", request.RequestID, map[string]string{
			"reason": "imported cancelled session",
		}); err != nil {
			return err
		}
	}
	return nil
}
