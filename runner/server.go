package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	mux.HandleFunc("/task", func(w http.ResponseWriter, r *http.Request) {
		handleCreateTask(w, r, queueDir, dispatch)
	})
	mux.HandleFunc("/tasks", func(w http.ResponseWriter, r *http.Request) {
		handleListTasks(w, r, queueDir)
	})
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		handleStatus(w, r, cfg, queueDir)
	})

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
