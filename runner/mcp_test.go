package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/mcptest"
	"github.com/mark3labs/mcp-go/server"
)

func TestMCPToolsStatusListAndCreateTask(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	queueDir := t.TempDir()
	cfg := &Config{
		AgentOSRoot: "/tmp/agentos",
		Runner: RunnerConfig{
			MaxConcurrent: 4,
			Mode:          "daemon",
		},
	}

	server := mcptest.NewUnstartedServer(t)
	server.AddTools(runnerMCPTools(cfg, queueDir, nil)...)
	if err := server.Start(ctx); err != nil {
		t.Fatalf("start server: %v", err)
	}
	defer server.Close()

	statusResult := callTool(t, ctx, server, "status", nil)
	var status mcpStatusPayload
	decodeStructured(t, statusResult, &status)
	if status.Mode != "daemon" || status.MaxConcurrent != 4 || status.AgentOSRoot != "/tmp/agentos" || status.QueueSize != 0 {
		t.Fatalf("unexpected status: %+v", status)
	}

	createResult := callTool(t, ctx, server, "create_task", map[string]any{
		"id":          "T-MCP-101",
		"title":       "Create via MCP",
		"role":        "worker_code",
		"input":       "Implement transport",
		"design_ref":  "designs/runner-transport-mcp.md",
		"depends_on":  []string{"T-000"},
		"execution":   "parallel",
		"assigned_to": "codex",
		"output":      "outputs/result-T-MCP-101.md",
	})
	var created mcpCreateTaskPayload
	decodeStructured(t, createResult, &created)
	if created.Status != "created" {
		t.Fatalf("unexpected create result: %+v", created)
	}

	writtenPath := filepath.Join(queueDir, "T-MCP-101.md")
	data, err := os.ReadFile(writtenPath)
	if err != nil {
		t.Fatalf("read created task: %v", err)
	}
	body := string(data)
	if !strings.Contains(body, "`outputs/result-T-MCP-101.md`") {
		t.Fatalf("created task missing output marker: %s", body)
	}
	if !strings.Contains(body, "Queue task files are removed after execution even on failure") {
		t.Fatalf("created task missing marker policy note: %s", body)
	}

	listResult := callTool(t, ctx, server, "list_tasks", nil)
	var tasks []taskResponse
	decodeStructured(t, listResult, &tasks)
	if len(tasks) != 1 || tasks[0].ID != "T-MCP-101" || tasks[0].Output != "outputs/result-T-MCP-101.md" {
		t.Fatalf("unexpected tasks: %+v", tasks)
	}
}

func runnerMCPTools(cfg *Config, queueDir string, dispatch func(context.Context) error) []server.ServerTool {
	return []server.ServerTool{
		{
			Tool: mcp.NewTool("status"),
			Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return handleMCPStatus(ctx, req, cfg, queueDir)
			},
		},
		{
			Tool: mcp.NewTool("list_tasks"),
			Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return handleMCPListTasks(ctx, req, queueDir)
			},
		},
		{
			Tool: mcp.NewTool("create_task"),
			Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return handleMCPCreateTask(ctx, req, queueDir, dispatch)
			},
		},
	}
}

func TestMCPCreateTaskTriggersDispatchHook(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	queueDir := t.TempDir()
	cfg := &Config{AgentOSRoot: "/tmp/agentos"}

	called := 0
	server := mcptest.NewUnstartedServer(t)
	server.AddTools(runnerMCPTools(cfg, queueDir, func(ctx context.Context) error {
		called++
		return nil
	})...)
	if err := server.Start(ctx); err != nil {
		t.Fatalf("start server: %v", err)
	}
	defer server.Close()

	callTool(t, ctx, server, "create_task", map[string]any{
		"id":          "T-MCP-102",
		"title":       "Create via MCP",
		"role":        "worker_code",
		"input":       "queue then dispatch",
		"design_ref":  "designs/runner-transport-mcp.md",
		"depends_on":  []string{},
		"execution":   "parallel",
		"assigned_to": "codex",
	})

	if called != 1 {
		t.Fatalf("dispatch hook calls = %d, want 1", called)
	}
}

func callTool(t *testing.T, ctx context.Context, srv *mcptest.Server, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()

	var req mcp.CallToolRequest
	req.Params.Name = name
	req.Params.Arguments = args

	result, err := srv.Client().CallTool(ctx, req)
	if err != nil {
		t.Fatalf("call tool %s: %v", name, err)
	}
	if result.IsError {
		t.Fatalf("tool %s returned error: %+v", name, result)
	}

	return result
}

func decodeStructured(t *testing.T, result *mcp.CallToolResult, target any) {
	t.Helper()

	if result.StructuredContent == nil {
		t.Fatal("missing structured content")
	}

	data, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatalf("unmarshal structured content: %v", err)
	}
}
