package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type RequestState struct {
	RequestID    string   `json:"request_id"`
	OriginalText string   `json:"original_text"`
	Status       string   `json:"status"`
	Summary      string   `json:"summary"`
	Questions    []string `json:"questions,omitempty"`
	Answers      []string `json:"answers,omitempty"`
	TaskID       string   `json:"task_id,omitempty"`
	TaskIDs      []string `json:"task_ids,omitempty"`
	RunID        string   `json:"run_id,omitempty"`
	CreatedAt    string   `json:"created_at"`
	UpdatedAt    string   `json:"updated_at"`
}

const (
	StatusQueued             = "queued"
	StatusRunning            = "running"
	StatusNeedsClarification = "needs-clarification"
	StatusCompleted          = "completed"
	StatusFailed             = "failed"
	StatusCancelled          = "cancelled"
)

func newRequestID() string {
	timestamp := time.Now().UTC().Format("01021504")
	b := make([]byte, 4)
	rand.Read(b)
	random := hex.EncodeToString(b)
	return fmt.Sprintf("req_%s%s", timestamp, strings.ToUpper(random))
}

func createRequest(paths *RuntimePaths, originalText string) (*RequestState, error) {
	questions := deriveClarifyingQuestions(originalText)
	status := StatusQueued
	if len(questions) > 0 {
		status = StatusNeedsClarification
	}

	request := &RequestState{
		RequestID:    newRequestID(),
		OriginalText: originalText,
		Status:       status,
		Summary:      summarizeRequest(originalText, questions),
		Questions:    questions,
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
		UpdatedAt:    time.Now().UTC().Format(time.RFC3339),
	}

	if err := appendCanonicalEvent(paths, request.RequestID, eventSessionCreated, "request", request.RequestID, map[string]any{
		"original_text": request.OriginalText,
		"summary":       request.Summary,
	}); err != nil {
		return nil, err
	}

	if len(questions) > 0 {
		if err := appendCanonicalEvent(paths, request.RequestID, eventClarificationAsked, "request", request.RequestID, clarificationPayload{
			Questions: questions,
		}); err != nil {
			return nil, err
		}
	}

	if len(questions) == 0 {
		_, _, err := createTaskFromRequest(paths, request)
		if err != nil {
			return nil, err
		}
	}

	return loadRequest(paths, request.RequestID)
}

func loadRequest(paths *RuntimePaths, requestID string) (*RequestState, error) {
	if session, err := loadCanonicalSession(paths, requestID); err == nil {
		return session.Request, nil
	}

	requestDir := paths.RequestsDir()
	requestPath := filepath.Join(requestDir, requestID+".json")
	data, err := os.ReadFile(requestPath)
	if err != nil {
		return nil, fmt.Errorf("read request: %w", err)
	}

	var request RequestState
	if err := json.Unmarshal(data, &request); err != nil {
		return nil, fmt.Errorf("parse request: %w", err)
	}
	return &request, nil
}

func saveRequest(requestDir string, request *RequestState) error {
	request.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	data, err := json.MarshalIndent(request, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}
	requestPath := filepath.Join(requestDir, request.RequestID+".json")
	if err := os.WriteFile(requestPath, data, 0o644); err != nil {
		return fmt.Errorf("write request: %w", err)
	}
	return nil
}

func answerRequest(paths *RuntimePaths, requestID string, answer string) (*RequestState, error) {
	request, err := loadRequest(paths, requestID)
	if err != nil {
		return nil, err
	}

	if err := appendCanonicalEvent(paths, requestID, eventClarificationAnswered, "request", requestID, clarificationPayload{
		Answer: answer,
	}); err != nil {
		return nil, err
	}

	request, err = loadRequest(paths, requestID)
	if err != nil {
		return nil, err
	}
	if len(request.Answers) >= len(request.Questions) {
		_, _, err := createTaskFromRequest(paths, request)
		if err != nil {
			return nil, err
		}
		request, err = loadRequest(paths, requestID)
		if err != nil {
			return nil, err
		}
	}
	return request, nil
}

func listRequests(paths *RuntimePaths) ([]*RequestState, error) {
	if ids, err := canonicalRequestIDs(paths); err == nil && len(ids) > 0 {
		requests := make([]*RequestState, 0, len(ids))
		for _, id := range ids {
			request, loadErr := loadRequest(paths, id)
			if loadErr == nil {
				requests = append(requests, request)
			}
		}
		sort.Slice(requests, func(i, j int) bool {
			return requests[i].UpdatedAt > requests[j].UpdatedAt
		})
		return requests, nil
	} else if err != nil {
		return nil, err
	}

	requestDir := paths.RequestsDir()
	entries, err := os.ReadDir(requestDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []*RequestState{}, nil
		}
		return nil, fmt.Errorf("read requests dir: %w", err)
	}

	requests := make([]*RequestState, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		requestID := strings.TrimSuffix(entry.Name(), ".json")
		request, err := loadRequest(paths, requestID)
		if err != nil {
			continue
		}
		requests = append(requests, request)
	}

	return requests, nil
}

func setCurrentTask(request *RequestState, taskID string) {
	request.TaskID = taskID
	if strings.TrimSpace(taskID) == "" {
		return
	}
	for _, existing := range request.TaskIDs {
		if existing == taskID {
			return
		}
	}
	request.TaskIDs = append(request.TaskIDs, taskID)
}

func allTaskIDs(request *RequestState) []string {
	if request == nil {
		return nil
	}
	ids := make([]string, 0, len(request.TaskIDs)+1)
	seen := make(map[string]struct{}, len(request.TaskIDs)+1)
	for _, id := range request.TaskIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if id := strings.TrimSpace(request.TaskID); id != "" {
		if _, ok := seen[id]; !ok {
			ids = append(ids, id)
		}
	}
	return ids
}

func summarizeRequest(originalText string, questions []string) string {
	trimmed := strings.TrimSpace(originalText)
	if len([]rune(trimmed)) > 60 {
		trimmed = string([]rune(trimmed)[:60]) + "..."
	}

	if len(questions) > 0 {
		return fmt.Sprintf("%s (needs clarification)", trimmed)
	}
	return trimmed
}

func createTaskFromRequest(paths *RuntimePaths, request *RequestState) (string, string, error) {
	taskPayload, err := buildTaskForRequest(paths, request)
	if err != nil {
		return "", "", err
	}
	if err := appendCanonicalEvent(paths, request.RequestID, eventTaskQueued, "task", taskPayload.TaskID, taskPayload); err != nil {
		return "", "", err
	}
	return paths.QueueTaskPath(taskPayload.TaskID), taskPayload.TaskID, nil
}

func buildTaskForRequest(paths *RuntimePaths, request *RequestState) (*taskQueuedPayload, error) {
	queueDir := paths.QueueDir()
	if err := os.MkdirAll(queueDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir queue dir: %w", err)
	}

	taskID, err := nextAutoTaskIDAfter(queueDir, maxAutoTaskSequence(allTaskIDs(request)))
	if err != nil {
		return nil, err
	}

	title := request.OriginalText
	if len([]rune(title)) > 60 {
		title = string([]rune(title)[:60])
	}

	input := strings.TrimSpace(request.OriginalText)
	if len(request.Answers) > 0 {
		input += "\n\n## Request Answers\n"
		for i, answer := range request.Answers {
			input += fmt.Sprintf("%d. %s\n", i+1, answer)
		}
	}

	return &taskQueuedPayload{
		TaskID:      taskID,
		Title:       sanitizeSingleLine(title),
		Role:        "designer",
		AssignedTo:  defaultAssignedTo(),
		Execution:   "parallel",
		Input:       input,
		DependsOn:   []string{},
		OutputPaths: []string{fmt.Sprintf("outputs/result-%s.md", taskID)},
	}, nil
}

func nextAutoTaskIDAfter(queueDir string, minID int) (string, error) {
	taskID, err := nextAutoTaskID(queueDir)
	if err != nil {
		return "", err
	}
	current := parseAutoTaskSequence(taskID)
	if current <= minID {
		return fmt.Sprintf("T-AUTO-%03d", minID+1), nil
	}
	return taskID, nil
}

func maxAutoTaskSequence(taskIDs []string) int {
	maxID := 0
	for _, taskID := range taskIDs {
		if n := parseAutoTaskSequence(taskID); n > maxID {
			maxID = n
		}
	}
	return maxID
}

func parseAutoTaskSequence(taskID string) int {
	re := regexp.MustCompile(`^T-AUTO-(\d+)$`)
	matches := re.FindStringSubmatch(strings.TrimSpace(taskID))
	if len(matches) != 2 {
		return 0
	}
	var n int
	fmt.Sscanf(matches[1], "%d", &n)
	return n
}
