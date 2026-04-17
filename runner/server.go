package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var (
	runnerStartedAt = time.Now()
	taskIDPattern   = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
	timeNow         = time.Now
)

type taskCreateRequest struct {
	ID         string   `json:"id"`
	Title      string   `json:"title"`
	Role       string   `json:"role"`
	Input      string   `json:"input"`
	DesignRef  string   `json:"design_ref"`
	DependsOn  []string `json:"depends_on"`
	Execution  string   `json:"execution"`
	AssignedTo string   `json:"assigned_to"`
}

type taskResponse struct {
	ID         string   `json:"id"`
	Title      string   `json:"title"`
	Role       string   `json:"role"`
	Input      string   `json:"input"`
	DesignRef  string   `json:"design_ref"`
	DependsOn  []string `json:"depends_on"`
	Execution  string   `json:"execution"`
	AssignedTo string   `json:"assigned_to"`
	Output     string   `json:"output,omitempty"`
}

type statusResponse struct {
	Mode          string `json:"mode"`
	MaxConcurrent int    `json:"max_concurrent"`
	AgentOSRoot   string `json:"agentos_root"`
	QueueSize     int    `json:"queue_size"`
	Uptime        string `json:"uptime"`
}

func startServer(ctx context.Context, cfg *Config, queueDir, addr string, dispatch func(context.Context) error) error {
	mux := http.NewServeMux()
	store := newWorkSessionStore(cfg.RuntimePaths())
	mux.HandleFunc("/task", func(w http.ResponseWriter, r *http.Request) {
		handleCreateTask(w, r, queueDir, dispatch)
	})
	mux.HandleFunc("/tasks", func(w http.ResponseWriter, r *http.Request) {
		handleListTasks(w, r, queueDir)
	})
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		handleStatus(w, r, cfg, queueDir)
	})
	mux.HandleFunc("/api/work-sessions", func(w http.ResponseWriter, r *http.Request) {
		handleWorkSessions(w, r, store, dispatch)
	})
	mux.HandleFunc("/api/work-sessions/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/events") || strings.HasSuffix(r.URL.Path, "/timeline") {
			handleWorkSessionTimeline(w, r, store)
			return
		}
		handleWorkSessionDetail(w, r, store)
	})
	mux.HandleFunc("/api/tasks/", func(w http.ResponseWriter, r *http.Request) {
		handleTaskDetail(w, r, store)
	})
	mux.HandleFunc("/api/runs/", func(w http.ResponseWriter, r *http.Request) {
		handleRunRoutes(w, r, cfg.RuntimePaths())
	})
	mux.HandleFunc("/api/nodes/", func(w http.ResponseWriter, r *http.Request) {
		handleNodeRoutes(w, r, cfg.RuntimePaths(), dispatch)
	})
	mux.HandleFunc("/", handleUI)

	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("http server shutdown failed: %v", err)
		}
	}()

	log.Printf("http server listening on %s", addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("http server failed: %w", err)
	}
	return nil
}

func handleCreateTask(w http.ResponseWriter, r *http.Request, queueDir string, dispatch func(context.Context) error) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	defer r.Body.Close()

	var req taskCreateRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid json: %v", err), http.StatusBadRequest)
		return
	}

	if decoder.More() {
		http.Error(w, "invalid json: multiple values", http.StatusBadRequest)
		return
	}

	if err := validateTaskCreateRequest(req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	content := renderTaskMarkdown(req, timeNow())
	path := filepath.Join(queueDir, req.ID+".md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		http.Error(w, fmt.Sprintf("write task file: %v", err), http.StatusInternalServerError)
		return
	}

	if dispatch != nil {
		if err := dispatch(r.Context()); err != nil {
			http.Error(w, fmt.Sprintf("trigger dispatch: %v", err), http.StatusInternalServerError)
			return
		}
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"status": "created",
		"path":   path,
	})
}

func handleListTasks(w http.ResponseWriter, r *http.Request, queueDir string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	tasks, err := LoadQueue(queueDir)
	if err != nil {
		http.Error(w, fmt.Sprintf("load queue: %v", err), http.StatusInternalServerError)
		return
	}

	response := make([]taskResponse, 0, len(tasks))
	for _, task := range tasks {
		response = append(response, taskResponse{
			ID:         task.TaskID,
			Title:      task.Title,
			Role:       task.Role,
			Input:      task.Input,
			DesignRef:  task.DesignRef,
			DependsOn:  task.DependsOn,
			Execution:  task.Execution,
			AssignedTo: task.AssignedTo,
			Output:     task.Output,
		})
	}

	writeJSON(w, http.StatusOK, response)
}

func handleStatus(w http.ResponseWriter, r *http.Request, cfg *Config, queueDir string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	tasks, err := LoadQueue(queueDir)
	if err != nil {
		http.Error(w, fmt.Sprintf("load queue: %v", err), http.StatusInternalServerError)
		return
	}

	resp := statusResponse{
		Mode:          cfg.Runner.Mode,
		MaxConcurrent: cfg.Runner.MaxConcurrent,
		AgentOSRoot:   cfg.AgentOSRoot,
		QueueSize:     len(tasks),
		Uptime:        time.Since(runnerStartedAt).Round(time.Second).String(),
	}

	writeJSON(w, http.StatusOK, resp)
}

func validateTaskCreateRequest(req taskCreateRequest) error {
	switch {
	case strings.TrimSpace(req.ID) == "":
		return errors.New("id is required")
	case !taskIDPattern.MatchString(req.ID):
		return errors.New("id contains invalid characters")
	case strings.TrimSpace(req.Title) == "":
		return errors.New("title is required")
	case strings.TrimSpace(req.Role) == "":
		return errors.New("role is required")
	case strings.TrimSpace(req.Input) == "":
		return errors.New("input is required")
	case strings.TrimSpace(req.DesignRef) == "":
		return errors.New("design_ref is required")
	case req.DependsOn == nil:
		return errors.New("depends_on is required")
	case strings.TrimSpace(req.Execution) == "":
		return errors.New("execution is required")
	case strings.TrimSpace(req.AssignedTo) == "":
		return errors.New("assigned_to is required")
	}

	for _, dep := range req.DependsOn {
		if strings.TrimSpace(dep) == "" {
			return errors.New("depends_on must not contain empty values")
		}
	}

	return nil
}

func renderTaskMarkdown(req taskCreateRequest, now time.Time) string {
	return fmt.Sprintf(`## Task ID: %s

## Title: %s

## Priority: normal

## Status: queue

## Created: %s

## Depends On: [%s]

## Execution: %s

## Role: %s

## Design Ref: %s

## Assigned To: %s

## Input

%s

## Output

## Done Condition

## Quality Gate

## Timeout: 30m

## Retry Count: 0

## Notes:
`, req.ID, req.Title, now.Format("2006-01-02"), strings.Join(req.DependsOn, ", "), req.Execution, req.Role, req.DesignRef, req.AssignedTo, req.Input)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("write json response: %v", err)
	}
}

func handleWorkSessions(w http.ResponseWriter, r *http.Request, store *workSessionStore, dispatch func(context.Context) error) {
	switch r.Method {
	case http.MethodGet:
		items, err := store.list()
		if err != nil {
			http.Error(w, fmt.Sprintf("load work sessions: %v", err), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, items)
	case http.MethodPost:
		var payload createWorkSessionRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "invalid json body", http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(payload.Request) == "" {
			http.Error(w, "request is required", http.StatusBadRequest)
			return
		}
		request, err := createRequest(store.paths, payload.Request)
		if err != nil {
			http.Error(w, fmt.Sprintf("create work session: %v", err), http.StatusInternalServerError)
			return
		}
		if dispatch != nil && request.Status == StatusRunning {
			if err := dispatch(r.Context()); err != nil {
				http.Error(w, fmt.Sprintf("trigger dispatch: %v", err), http.StatusInternalServerError)
				return
			}
		}
		writeJSON(w, http.StatusCreated, request)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleWorkSessionDetail(w http.ResponseWriter, r *http.Request, store *workSessionStore) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	requestID := strings.TrimPrefix(r.URL.Path, "/api/work-sessions/")
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		http.Error(w, "missing request id", http.StatusBadRequest)
		return
	}
	detail, err := store.load(requestID)
	if err != nil {
		http.Error(w, fmt.Sprintf("load work session: %v", err), http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func handleWorkSessionTimeline(w http.ResponseWriter, r *http.Request, store *workSessionStore) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/work-sessions/")
	path = strings.TrimSuffix(path, "/events")
	path = strings.TrimSuffix(path, "/timeline")
	requestID := strings.Trim(path, "/")
	if requestID == "" {
		http.Error(w, "missing request id", http.StatusBadRequest)
		return
	}
	timeline, err := loadSessionTimeline(store.paths, requestID)
	if err != nil {
		http.Error(w, fmt.Sprintf("load timeline: %v", err), http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, timeline)
}

func handleTaskDetail(w http.ResponseWriter, r *http.Request, store *workSessionStore) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	taskID := strings.TrimPrefix(r.URL.Path, "/api/tasks/")
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		http.Error(w, "missing task id", http.StatusBadRequest)
		return
	}
	allInterventions, err := loadInterventions(store.paths, "")
	if err != nil {
		http.Error(w, fmt.Sprintf("load interventions: %v", err), http.StatusInternalServerError)
		return
	}
	task, err := store.buildTaskView(taskID, allInterventions)
	if err != nil {
		http.Error(w, fmt.Sprintf("load task: %v", err), http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, task)
}

func handleRunRoutes(w http.ResponseWriter, r *http.Request, paths *RuntimePaths) {
	path := strings.TrimPrefix(r.URL.Path, "/api/runs/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		http.Error(w, "missing run id", http.StatusBadRequest)
		return
	}
	runID := strings.TrimSpace(parts[0])
	if len(parts) == 1 {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		view, err := loadRunView(paths, runID)
		if err != nil {
			http.Error(w, fmt.Sprintf("load run: %v", err), http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, view)
		return
	}

	switch parts[1] {
	case "logs":
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		stream := r.URL.Query().Get("stream")
		last := 80
		if raw := strings.TrimSpace(r.URL.Query().Get("tail")); raw != "" {
			fmt.Sscanf(raw, "%d", &last)
		}
		payload, err := loadRunLogs(paths, runID, stream, last)
		if err != nil {
			http.Error(w, fmt.Sprintf("load run logs: %v", err), http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, payload)
	case "evidence":
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		payload, err := loadRunEvidence(paths, runID)
		if err != nil {
			http.Error(w, fmt.Sprintf("load run evidence: %v", err), http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, payload)
	default:
		http.NotFound(w, r)
	}
}

func handleNodeRoutes(w http.ResponseWriter, r *http.Request, paths *RuntimePaths, dispatch func(context.Context) error) {
	path := strings.TrimPrefix(r.URL.Path, "/api/nodes/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}
	nodeID := strings.TrimSpace(parts[0])
	if nodeID == "" {
		http.Error(w, "missing node id", http.StatusBadRequest)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("read request body: %v", err), http.StatusBadRequest)
		return
	}

	switch parts[1] {
	case "message":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var payload nodeMessageRequest
		if err := json.Unmarshal(body, &payload); err != nil || strings.TrimSpace(payload.Message) == "" {
			http.Error(w, "message is required", http.StatusBadRequest)
			return
		}
		intervention, err := handleNodeMessage(paths, nodeID, payload.Message)
		if err != nil {
			http.Error(w, fmt.Sprintf("record message: %v", err), http.StatusBadRequest)
			return
		}
		maybeDispatchNodeAction(r.Context(), paths, nodeID, dispatch)
		response := nodeActionResponse{Status: "recorded", NodeID: nodeID, Intervention: intervention}
		if requestID, ok := intervention.Metadata["request_id"].(string); ok {
			if detail, loadErr := newWorkSessionStore(paths).load(requestID); loadErr == nil {
				applyNodeActionProjection(&response, detail, nodeID)
			}
		}
		writeJSON(w, http.StatusOK, response)
	case "control":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var payload nodeControlRequest
		if err := json.Unmarshal(body, &payload); err != nil || strings.TrimSpace(payload.Command) == "" {
			http.Error(w, "command is required", http.StatusBadRequest)
			return
		}
		intervention, err := handleNodeControl(paths, nodeID, payload.Command)
		if err != nil {
			http.Error(w, fmt.Sprintf("apply control: %v", err), http.StatusBadRequest)
			return
		}
		maybeDispatchNodeAction(r.Context(), paths, nodeID, dispatch)
		response := nodeActionResponse{Status: "applied", NodeID: nodeID, Intervention: intervention}
		if requestID, ok := intervention.Metadata["request_id"].(string); ok {
			if detail, loadErr := newWorkSessionStore(paths).load(requestID); loadErr == nil {
				applyNodeActionProjection(&response, detail, nodeID)
			}
		}
		writeJSON(w, http.StatusOK, response)
	default:
		http.NotFound(w, r)
	}
}

func maybeDispatchNodeAction(ctx context.Context, paths *RuntimePaths, nodeID string, dispatch func(context.Context) error) {
	if dispatch == nil {
		return
	}
	nodeType, id, err := parseNodeID(nodeID)
	if err != nil {
		return
	}
	request, err := resolveRequestForNode(paths, nodeType, id)
	if err != nil {
		return
	}
	if request.Status == StatusRunning && strings.TrimSpace(request.TaskID) != "" {
		_ = dispatch(ctx)
	}
}

func applyNodeActionProjection(response *nodeActionResponse, detail *WorkSessionDetail, nodeID string) {
	if response == nil || detail == nil {
		return
	}
	node := findNodeDetail(detail, nodeID)
	switch model := node.(type) {
	case RequestView:
		response.PendingCount = model.PendingCount
		response.NextApplication = nextApplicationForNode(detail.Timeline, nodeID)
		response.LatestActivity = coalesce(model.LastActivity, model.LatestEvent, model.Summary)
	case *RequestView:
		response.PendingCount = model.PendingCount
		response.NextApplication = nextApplicationForNode(detail.Timeline, nodeID)
		response.LatestActivity = coalesce(model.LastActivity, model.LatestEvent, model.Summary)
	case TaskView:
		response.PendingCount = model.PendingCount
		response.NextApplication = model.NextApplication
		response.LatestActivity = coalesce(model.LastActivity, model.LatestEvent, model.Status)
	case *TaskView:
		response.PendingCount = model.PendingCount
		response.NextApplication = model.NextApplication
		response.LatestActivity = coalesce(model.LastActivity, model.LatestEvent, model.Status)
	case RunView:
		response.PendingCount = model.PendingCount
		response.NextApplication = model.NextApplication
		response.LatestActivity = coalesce(model.LastActivity, model.LatestEvent, model.Status)
	case *RunView:
		response.PendingCount = model.PendingCount
		response.NextApplication = model.NextApplication
		response.LatestActivity = coalesce(model.LastActivity, model.LatestEvent, model.Status)
	}
}

func findNodeDetail(detail *WorkSessionDetail, nodeID string) any {
	if detail == nil {
		return nil
	}
	nodeType, id, err := parseNodeID(nodeID)
	if err != nil {
		return nil
	}
	switch nodeType {
	case "request":
		if detail.Session.ID == id {
			return detail.Session
		}
	case "task":
		for _, task := range detail.Tasks {
			if task.ID == id {
				return task
			}
		}
	case "run":
		for _, task := range detail.Tasks {
			for _, run := range task.Runs {
				if run.ID == id {
					return run
				}
			}
		}
	}
	return nil
}

func handleUI(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	data, err := fs.ReadFile(uiFS(), "index.html")
	if err != nil {
		http.Error(w, fmt.Sprintf("load ui: %v", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}
