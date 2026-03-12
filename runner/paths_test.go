package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRuntimePathsLegacyLayout(t *testing.T) {
	t.Parallel()

	root := "/workspace/agentproc"
	paths := NewRuntimePaths(root, false)

	if got := paths.QueueDir(); got != filepath.Join(root, "tasks", "queue") {
		t.Fatalf("QueueDir() = %q", got)
	}
	if got := paths.IntakeSessionsDir(); got != filepath.Join(root, "tasks", ".intake") {
		t.Fatalf("IntakeSessionsDir() = %q", got)
	}
	if got := paths.ActiveRunDir("run-1"); got != filepath.Join(root, "tasks", ".run", "run-1") {
		t.Fatalf("ActiveRunDir() = %q", got)
	}
	if got := paths.CompletedRunDir("run-1"); got != filepath.Join(root, "outputs", "runs", "run-1") {
		t.Fatalf("CompletedRunDir() = %q", got)
	}
	if got := paths.ResolveOutputPath("outputs/result-T-001.md"); got != filepath.Join(root, "outputs", "result-T-001.md") {
		t.Fatalf("ResolveOutputPath() = %q", got)
	}
}

func TestRuntimePathsHiddenLayout(t *testing.T) {
	t.Parallel()

	root := "/workspace/agentproc"
	paths := NewRuntimePaths(root, true)
	paths.stateRoot = "/home/tester/.local/state"

	projectRoot := paths.projectRoot()
	if !strings.HasPrefix(projectRoot, filepath.Join("/home/tester", ".local", "state", "aproc", "projects", "agentproc-")) {
		t.Fatalf("projectRoot() = %q", projectRoot)
	}
	if got := paths.QueueDir(); got != filepath.Join(projectRoot, "tasks", "queue") {
		t.Fatalf("QueueDir() = %q", got)
	}
	if got := paths.IntakeSessionsDir(); got != filepath.Join(projectRoot, "tmp", "sessions") {
		t.Fatalf("IntakeSessionsDir() = %q", got)
	}
	if got := paths.ActiveRunDir("run-1"); got != filepath.Join(projectRoot, "runs", "active", "run-1") {
		t.Fatalf("ActiveRunDir() = %q", got)
	}
	if got := paths.CompletedRunDir("run-1"); got != filepath.Join(projectRoot, "runs", "completed", "run-1") {
		t.Fatalf("CompletedRunDir() = %q", got)
	}
	if got := paths.RunSummaryPath("run-1"); got != filepath.Join(projectRoot, "runs", "completed", "run-1", "summary.json") {
		t.Fatalf("RunSummaryPath() = %q", got)
	}
	if got := paths.ActiveRunStatePath("run-1"); got != filepath.Join(projectRoot, "runs", "active", "run-1", "run.json") {
		t.Fatalf("ActiveRunStatePath() = %q", got)
	}
	if got := paths.CompletedRunStatePath("run-1"); got != filepath.Join(projectRoot, "runs", "completed", "run-1", "run.json") {
		t.Fatalf("CompletedRunStatePath() = %q", got)
	}
	if got := paths.ResolveOutputPath("outputs/result-T-001.md"); got != filepath.Join(projectRoot, "outputs", "results", "result-T-001.md") {
		t.Fatalf("ResolveOutputPath() = %q", got)
	}
}

func TestDetectProjectRootPrefersNearestProjectMarker(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	projectRoot := filepath.Join(root, "repo")
	workspace := filepath.Join(projectRoot, "pkg", "service")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	if err := os.Mkdir(filepath.Join(projectRoot, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}

	got, err := detectProjectRoot(workspace)
	if err != nil {
		t.Fatalf("detectProjectRoot returned error: %v", err)
	}
	if got != projectRoot {
		t.Fatalf("detectProjectRoot() = %q, want %q", got, projectRoot)
	}
}

func TestDetectProjectRootFallsBackToWorkspace(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()

	got, err := detectProjectRoot(workspace)
	if err != nil {
		t.Fatalf("detectProjectRoot returned error: %v", err)
	}
	if got != workspace {
		t.Fatalf("detectProjectRoot() = %q, want %q", got, workspace)
	}
}

func TestProjectMappingUsesCanonicalRoot(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	projectRoot := filepath.Join(base, "repo")
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		t.Fatalf("mkdir project root: %v", err)
	}
	linkPath := filepath.Join(base, "repo-link")
	if err := os.Symlink(projectRoot, linkPath); err != nil {
		t.Fatalf("symlink project root: %v", err)
	}

	canonicalPaths := NewRuntimePaths(projectRoot, true)
	symlinkPaths := NewRuntimePaths(linkPath, true)
	canonicalPaths.stateRoot = filepath.Join(base, "state")
	symlinkPaths.stateRoot = canonicalPaths.stateRoot

	if canonicalPaths.projectKey != symlinkPaths.projectKey {
		t.Fatalf("projectKey mismatch: %q vs %q", canonicalPaths.projectKey, symlinkPaths.projectKey)
	}
	if canonicalPaths.projectRoot() != symlinkPaths.projectRoot() {
		t.Fatalf("projectRoot mismatch: %q vs %q", canonicalPaths.projectRoot(), symlinkPaths.projectRoot())
	}
}
