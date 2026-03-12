package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReproQueueRemovalOnEarlyFailure(t *testing.T) {
	root := t.TempDir()
	paths := NewRuntimePaths(root, false)
	if err := os.MkdirAll(paths.QueueDir(), 0o755); err != nil {
		t.Fatalf("mkdir queue: %v", err)
	}

	taskID := "T-REPRO-001"
	taskPath := filepath.Join(paths.QueueDir(), taskID+".md")
	if err := os.WriteFile(taskPath, []byte("## Task ID: "+taskID), 0o644); err != nil {
		t.Fatalf("write task: %v", err)
	}

	task := &Task{TaskID: taskID}
	claim, err := claimTask(paths, task)
	if err != nil {
		t.Fatalf("claimTask: %v", err)
	}

	// Forcing an early return in runTask by providing empty Subprocess layer
	cfg := &Config{AgentOSRoot: root}
	executeTask(cfg, claim)

	if _, err := os.Stat(taskPath); !os.IsNotExist(err) {
		t.Errorf("expected queue task to be removed even on early failure, but it still exists (err=%v)", err)
	}
}
