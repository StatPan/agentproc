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
  subprocess: codex
  thread: gemini
runner:
  max_concurrent: 0
  poll_interval: 5s
  mode: daemon
  hidden_runtime: true
  thread_fallback_models:
    - gemini-3-flash-preview
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
	if cfg.Layers.Subprocess != "codex" || cfg.Layers.Thread != "gemini" {
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
	if !cfg.Runner.HiddenRuntime {
		t.Fatalf("expected hidden_runtime to be true")
	}
	if len(cfg.Runner.ThreadFallbackModels) != 1 || cfg.Runner.ThreadFallbackModels[0] != "gemini-3-flash-preview" {
		t.Fatalf("unexpected thread_fallback_models: %#v", cfg.Runner.ThreadFallbackModels)
	}
}

func TestLoadProjectConfigUsesLocalAprocYAMLWhenImplicitConfigMissing(t *testing.T) {
	t.Parallel()

	projectRoot := t.TempDir()
	content := `layers:
  subprocess: codex
runner:
  hidden_runtime: true
`
	if err := os.WriteFile(filepath.Join(projectRoot, ".aproc.yml"), []byte(content), 0o644); err != nil {
		t.Fatalf("write local config: %v", err)
	}

	cfg, loadedPath, err := loadProjectConfig(projectRoot, "./config.yaml")
	if err != nil {
		t.Fatalf("loadProjectConfig returned error: %v", err)
	}

	if loadedPath != filepath.Join(projectRoot, ".aproc.yml") {
		t.Fatalf("loadedPath = %q", loadedPath)
	}
	if cfg.Layers.Subprocess != "codex" {
		t.Fatalf("unexpected subprocess layer: %q", cfg.Layers.Subprocess)
	}
	if !cfg.Runner.HiddenRuntime {
		t.Fatalf("expected hidden_runtime to be true")
	}
}

func TestLoadProjectConfigUsesRunnerConfigWhenProjectConfigMissing(t *testing.T) {
	t.Parallel()

	projectRoot := t.TempDir()
	runnerDir := filepath.Join(projectRoot, "runner")
	if err := os.MkdirAll(runnerDir, 0o755); err != nil {
		t.Fatalf("mkdir runner dir: %v", err)
	}

	content := `layers:
  subprocess: codex
  thread: gemini
runner:
  hidden_runtime: true
`
	configPath := filepath.Join(runnerDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write runner config: %v", err)
	}

	cfg, loadedPath, err := loadProjectConfig(projectRoot, "./config.yaml")
	if err != nil {
		t.Fatalf("loadProjectConfig returned error: %v", err)
	}

	if loadedPath != configPath {
		t.Fatalf("loadedPath = %q, want %q", loadedPath, configPath)
	}
	if cfg.Layers.Subprocess != "codex" || cfg.Layers.Thread != "gemini" {
		t.Fatalf("unexpected layers: %+v", cfg.Layers)
	}
	if !cfg.Runner.HiddenRuntime {
		t.Fatalf("expected hidden_runtime to be true")
	}
}

func TestLoadProjectConfigUsesDefaultsWhenImplicitConfigMissing(t *testing.T) {
	t.Parallel()

	projectRoot := t.TempDir()

	cfg, loadedPath, err := loadProjectConfig(projectRoot, "./config.yaml")
	if err != nil {
		t.Fatalf("loadProjectConfig returned error: %v", err)
	}

	if loadedPath != "" {
		t.Fatalf("loadedPath = %q, want empty", loadedPath)
	}
	if cfg.Runner.MaxConcurrent != runtime.NumCPU() {
		t.Fatalf("unexpected max_concurrent: %d", cfg.Runner.MaxConcurrent)
	}
	if cfg.Runner.PollInterval != 5*time.Second {
		t.Fatalf("unexpected poll_interval: %v", cfg.Runner.PollInterval)
	}
	if cfg.Runner.Mode != "daemon" {
		t.Fatalf("unexpected mode: %q", cfg.Runner.Mode)
	}
}

func TestLoadProjectConfigFailsForMissingExplicitPath(t *testing.T) {
	t.Parallel()

	projectRoot := t.TempDir()
	_, _, err := loadProjectConfig(projectRoot, filepath.Join(projectRoot, "missing.yaml"))
	if err == nil {
		t.Fatal("expected error for explicit missing config path")
	}
}
