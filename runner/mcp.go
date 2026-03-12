package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type mcpStatusPayload struct {
	Mode          string `json:"mode"`
	MaxConcurrent int    `json:"max_concurrent"`
	AgentOSRoot   string `json:"agentos_root"`
	QueueSize     int    `json:"queue_size"`
	Uptime        string `json:"uptime"`
}

type mcpCreateTaskPayload struct {
	Status string `json:"status"`
	Path   string `json:"path"`
}

// StartMCPServer starts the runner MCP server over stdio.
func StartMCPServer(cfg *Config, queueDir string, dispatch func(context.Context) error) error {
	logger := log.New(os.Stderr, "", log.LstdFlags)
	return server.ServeStdio(newRunnerMCPServer(cfg, queueDir, dispatch), server.WithErrorLogger(logger))
}

func newRunnerMCPServer(cfg *Config, queueDir string, dispatch func(context.Context) error) *server.MCPServer {
	mcpServer := server.NewMCPServer(
		"agentos-runner",
		"1.0.0",
		server.WithToolCapabilities(true),
	)

	mcpServer.AddTool(mcp.NewTool("status",
		mcp.WithDescription("Return the current runner status snapshot."),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handleMCPStatus(ctx, request, cfg, queueDir)
	})

	mcpServer.AddTool(mcp.NewTool("list_tasks",
		mcp.WithDescription("List queued tasks using the existing AgentOS queue format."),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handleMCPListTasks(ctx, request, queueDir)
	})

	mcpServer.AddTool(mcp.NewTool("create_task",
		mcp.WithDescription("Create a queue task. Include an outputs/result-*.md marker in output or notes because tasks leave the queue even on failure."),
		mcp.WithString("id", mcp.Required(), mcp.Description("Task ID.")),
		mcp.WithString("title", mcp.Required(), mcp.Description("Task title.")),
		mcp.WithString("role", mcp.Required(), mcp.Description("Assigned role.")),
		mcp.WithString("input", mcp.Required(), mcp.Description("Task input body.")),
		mcp.WithString("design_ref", mcp.Required(), mcp.Description("Design reference path.")),
		mcp.WithArray("depends_on", mcp.Description("Dependency task IDs.")),
		mcp.WithString("execution", mcp.Required(), mcp.Description("Execution mode such as parallel or sequential.")),
		mcp.WithString("assigned_to", mcp.Required(), mcp.Description("Assigned worker.")),
		mcp.WithString("output", mcp.Description("Optional output marker path such as outputs/result-T-123.md.")),
		mcp.WithString("notes", mcp.Description("Optional notes appended to the task template.")),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handleMCPCreateTask(ctx, request, queueDir, dispatch)
	})

	return mcpServer
}

func handleMCPStatus(_ context.Context, _ mcp.CallToolRequest, cfg *Config, queueDir string) (*mcp.CallToolResult, error) {
	tasks, err := LoadQueue(queueDir)
	if err != nil {
		return nil, fmt.Errorf("load queue: %w", err)
	}

	payload := mcpStatusPayload{
		Mode:          cfg.Runner.Mode,
		MaxConcurrent: cfg.Runner.MaxConcurrent,
		AgentOSRoot:   cfg.AgentOSRoot,
		QueueSize:     len(tasks),
		Uptime:        time.Since(runnerStartedAt).Round(time.Second).String(),
	}

	return structuredResult(payload)
}

func handleMCPListTasks(_ context.Context, _ mcp.CallToolRequest, queueDir string) (*mcp.CallToolResult, error) {
	tasks, err := LoadQueue(queueDir)
	if err != nil {
		return nil, fmt.Errorf("load queue: %w", err)
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

	return structuredResult(response)
}

func handleMCPCreateTask(ctx context.Context, request mcp.CallToolRequest, queueDir string, dispatch func(context.Context) error) (*mcp.CallToolResult, error) {
	req, err := taskCreateRequestFromMCP(request)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	if err := validateTaskCreateRequest(req); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	outputPath := request.GetString("output", "")
	notes := request.GetString("notes", "")
	content := renderMCPTaskMarkdown(req, outputPath, notes, timeNow())
	path := filepath.Join(queueDir, req.ID+".md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return nil, fmt.Errorf("write task file: %w", err)
	}

	if dispatch != nil {
		if err := dispatch(ctx); err != nil {
			return nil, fmt.Errorf("trigger dispatch: %w", err)
		}
	}

	return structuredResult(mcpCreateTaskPayload{
		Status: "created",
		Path:   path,
	})
}

func taskCreateRequestFromMCP(request mcp.CallToolRequest) (taskCreateRequest, error) {
	req := taskCreateRequest{
		ID:         request.GetString("id", ""),
		Title:      request.GetString("title", ""),
		Role:       request.GetString("role", ""),
		Input:      request.GetString("input", ""),
		DesignRef:  request.GetString("design_ref", ""),
		DependsOn:  request.GetStringSlice("depends_on", []string{}),
		Execution:  request.GetString("execution", ""),
		AssignedTo: request.GetString("assigned_to", ""),
	}

	if request.GetArguments() == nil {
		return req, fmt.Errorf("arguments must be an object")
	}

	return req, nil
}

func renderMCPTaskMarkdown(req taskCreateRequest, outputPath string, notes string, now time.Time) string {
	outputSection := ""
	if outputPath != "" {
		outputSection = fmt.Sprintf("`%s`\n", outputPath)
	}

	notesSection := markerPolicyNote
	if notes != "" {
		notesSection = markerPolicyNote + "\n" + notes
	}

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

%s
## Done Condition

## Quality Gate

## Timeout: 30m

## Retry Count: 0

## Notes:
%s
`, req.ID, req.Title, now.Format("2006-01-02"), joinDependsOn(req.DependsOn), req.Execution, req.Role, req.DesignRef, req.AssignedTo, req.Input, outputSection, notesSection)
}

func joinDependsOn(dependsOn []string) string {
	return strings.Join(dependsOn, ", ")
}

func structuredResult(payload any) (*mcp.CallToolResult, error) {
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal result: %w", err)
	}

	return mcp.NewToolResultStructured(payload, string(data)), nil
}

const markerPolicyNote = "- Queue task files are removed after execution even on failure. Leave an outputs/result-*.md marker for dependency checks and postmortem analysis."
