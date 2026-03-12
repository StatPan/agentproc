package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestLoadConfigUsesNumCPUWhenMaxConcurrentZero(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	content := `agentos_root: ../AgentOS
layers:
  process: claude
  subprocess: codex
  thread: gemini
runner:
  max_concurrent: 0
  poll_interval: 5s
  mode: daemon
`

	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}

	if cfg.AgentOSRoot != "../AgentOS" {
		t.Fatalf("unexpected agentos_root: %q", cfg.AgentOSRoot)
	}
	if cfg.Layers.Process != "claude" || cfg.Layers.Subprocess != "codex" || cfg.Layers.Thread != "gemini" {
		t.Fatalf("unexpected layers: %+v", cfg.Layers)
	}
	if cfg.Runner.MaxConcurrent != runtime.NumCPU() {
		t.Fatalf("unexpected max_concurrent: got %d want %d", cfg.Runner.MaxConcurrent, runtime.NumCPU())
	}
	if cfg.Runner.PollInterval != 5*time.Second {
		t.Fatalf("unexpected poll_interval: %v", cfg.Runner.PollInterval)
	}
	if cfg.Runner.Mode != "daemon" {
		t.Fatalf("unexpected mode: %q", cfg.Runner.Mode)
	}
}
