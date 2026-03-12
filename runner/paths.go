package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// RuntimePaths resolves the runner's shared runtime directories.
// It preserves the workspace-local layout unless hidden runtime is enabled.
type RuntimePaths struct {
	agentOSRoot   string
	canonicalRoot string
	hiddenRuntime bool
	stateRoot     string
	projectKey    string
}

func NewRuntimePaths(agentOSRoot string, hiddenRuntime bool) *RuntimePaths {
	root := filepath.Clean(agentOSRoot)
	paths := &RuntimePaths{
		agentOSRoot:   root,
		canonicalRoot: canonicalProjectPath(root),
		hiddenRuntime: hiddenRuntime,
		stateRoot:     defaultStateRoot(),
	}
	paths.projectKey = projectMappingKey(paths.canonicalRoot)
	return paths
}

func (p *RuntimePaths) projectRoot() string {
	if !p.hiddenRuntime || p.stateRoot == "" || p.projectKey == "" {
		return p.agentOSRoot
	}
	return filepath.Join(p.stateRoot, "aproc", "projects", p.projectKey)
}

func (p *RuntimePaths) QueueDir() string {
	if p.hiddenRuntime {
		return filepath.Join(p.projectRoot(), "tasks", "queue")
	}
	return filepath.Join(p.agentOSRoot, "tasks", "queue")
}

func (p *RuntimePaths) QueueTaskPath(taskID string) string {
	return filepath.Join(p.QueueDir(), taskID+".md")
}

func (p *RuntimePaths) IntakeSessionsDir() string {
	if p.hiddenRuntime {
		return filepath.Join(p.projectRoot(), "tmp", "sessions")
	}
	return filepath.Join(p.agentOSRoot, "tasks", ".intake")
}

func (p *RuntimePaths) ActiveRunsDir() string {
	if p.hiddenRuntime {
		return filepath.Join(p.projectRoot(), "runs", "active")
	}
	return filepath.Join(p.agentOSRoot, "tasks", ".run")
}

func (p *RuntimePaths) ActiveRunDir(runID string) string {
	return filepath.Join(p.ActiveRunsDir(), runID)
}

func (p *RuntimePaths) CompletedRunsDir() string {
	if p.hiddenRuntime {
		return filepath.Join(p.projectRoot(), "runs", "completed")
	}
	return filepath.Join(p.agentOSRoot, "outputs", "runs")
}

func (p *RuntimePaths) CompletedRunDir(runID string) string {
	return filepath.Join(p.CompletedRunsDir(), runID)
}

func (p *RuntimePaths) RunSummaryPath(runID string) string {
	return filepath.Join(p.CompletedRunDir(runID), "summary.json")
}

func (p *RuntimePaths) ActiveRunStatePath(runID string) string {
	return filepath.Join(p.ActiveRunDir(runID), "run.json")
}

func (p *RuntimePaths) CompletedRunStatePath(runID string) string {
	return filepath.Join(p.CompletedRunDir(runID), "run.json")
}

func (p *RuntimePaths) OutputsDir() string {
	if p.hiddenRuntime {
		return filepath.Join(p.projectRoot(), "outputs", "results")
	}
	return filepath.Join(p.agentOSRoot, "outputs")
}

func (p *RuntimePaths) ResolveOutputPath(outputPath string) string {
	cleaned := filepath.Clean(outputPath)
	prefix := "outputs" + string(filepath.Separator)

	if filepath.IsAbs(cleaned) {
		return cleaned
	}
	if cleaned == "outputs" {
		return p.OutputsDir()
	}
	if strings.HasPrefix(cleaned, prefix) {
		return filepath.Join(p.OutputsDir(), strings.TrimPrefix(cleaned, prefix))
	}
	return filepath.Join(p.OutputsDir(), cleaned)
}

func detectProjectRoot(start string) (string, error) {
	original, err := filepath.Abs(start)
	if err != nil {
		return "", fmt.Errorf("resolve start path: %w", err)
	}
	original = filepath.Clean(original)

	current := original
	for {
		if hasProjectMarker(current) {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return original, nil
		}
		current = parent
	}
}

func hasProjectMarker(dir string) bool {
	for _, marker := range []string{".git", ".aproc.yml"} {
		if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
			return true
		}
	}
	return false
}

func canonicalProjectPath(root string) string {
	abs, err := filepath.Abs(root)
	if err != nil {
		return filepath.Clean(root)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return filepath.Clean(abs)
	}
	return filepath.Clean(resolved)
}

func projectMappingKey(canonicalRoot string) string {
	if canonicalRoot == "" {
		return ""
	}
	base := filepath.Base(canonicalRoot)
	if base == "." || base == string(filepath.Separator) || base == "" {
		base = "project"
	}
	hash := sha256.Sum256([]byte(canonicalRoot))
	return fmt.Sprintf("%s-%x", base, hash[:6])
}

func defaultStateRoot() string {
	if stateHome := strings.TrimSpace(os.Getenv("XDG_STATE_HOME")); stateHome != "" {
		return filepath.Clean(stateHome)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil || homeDir == "" {
		return ""
	}

	switch runtime.GOOS {
	case "windows":
		if localAppData := strings.TrimSpace(os.Getenv("LocalAppData")); localAppData != "" {
			return filepath.Clean(localAppData)
		}
		return filepath.Join(homeDir, "AppData", "Local")
	case "darwin":
		return filepath.Join(homeDir, "Library", "Application Support")
	default:
		return filepath.Join(homeDir, ".local", "state")
	}
}
