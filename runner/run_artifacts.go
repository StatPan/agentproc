package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type RunSummary struct {
	RunID        string   `json:"run_id"`
	TaskID       string   `json:"task_id"`
	Status       string   `json:"status"`
	StartedAt    string   `json:"started_at"`
	FinishedAt   string   `json:"finished_at,omitempty"`
	DurationMS   int64    `json:"duration_ms,omitempty"`
	TaskPath     string   `json:"task_path,omitempty"`
	ResultPath   string   `json:"result_path,omitempty"`
	StdoutPath   string   `json:"stdout_path,omitempty"`
	StderrPath   string   `json:"stderr_path,omitempty"`
	ThreadLog    string   `json:"thread_log,omitempty"`
	EvidencePath string   `json:"evidence_path,omitempty"`
	Error        string   `json:"error,omitempty"`
	Events       []string `json:"events,omitempty"`
}

type RunState struct {
	RunID     string `json:"run_id"`
	TaskID    string `json:"task_id"`
	Status    string `json:"status"`
	StartedAt string `json:"started_at"`
	UpdatedAt string `json:"updated_at"`
	PID       int    `json:"pid,omitempty"`
}

func runResultCommand(args []string) error {
	fs := flag.NewFlagSet("result", flag.ContinueOnError)
	rootFlag := fs.String("root", ".", "AgentOS root path")
	configFlag := fs.String("config", "./config.yaml", "runner config path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(fs.Args()) != 1 {
		return errors.New("usage: aproc result [--root PATH] [--config PATH] <run-id>")
	}

	_, paths, err := loadRuntimeConfig(*rootFlag, *configFlag)
	if err != nil {
		return err
	}
	summary, _, err := loadRunSummary(paths, fs.Arg(0))
	if err != nil {
		return err
	}

	printRunSummary(summary)
	return nil
}

func runTailCommand(args []string) error {
	fs := flag.NewFlagSet("tail", flag.ContinueOnError)
	rootFlag := fs.String("root", ".", "AgentOS root path")
	configFlag := fs.String("config", "./config.yaml", "runner config path")
	lastFlag := fs.Int("last", 40, "number of log lines to show")
	streamFlag := fs.String("stream", "stdout", "log stream: stdout or stderr")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(fs.Args()) != 1 {
		return errors.New("usage: aproc tail [--root PATH] [--config PATH] [--stream stdout|stderr] [--last N] <run-id>")
	}

	_, paths, err := loadRuntimeConfig(*rootFlag, *configFlag)
	if err != nil {
		return err
	}
	summary, _, err := loadRunSummary(paths, fs.Arg(0))
	if err != nil {
		return err
	}

	logPath := summary.StdoutPath
	if strings.EqualFold(strings.TrimSpace(*streamFlag), "stderr") {
		logPath = summary.StderrPath
	}
	if strings.TrimSpace(logPath) == "" {
		return fmt.Errorf("no %s log path recorded for run %s", *streamFlag, summary.RunID)
	}

	lines, err := readLastLines(logPath, *lastFlag)
	if err != nil {
		return err
	}
	for _, line := range lines {
		fmt.Println(line)
	}
	return nil
}

func prepareRunArtifacts(paths *RuntimePaths, runID string) (string, string, string, string, error) {
	runBaseDir := paths.CompletedRunDir(runID)
	logsDir := filepath.Join(runBaseDir, "logs")
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		return "", "", "", "", fmt.Errorf("mkdir run logs dir: %w", err)
	}
	return runBaseDir,
		filepath.Join(logsDir, "stdout.log"),
		filepath.Join(logsDir, "stderr.log"),
		filepath.Join(runBaseDir, "summary.json"),
		nil
}

func writeRunState(path string, state *RunState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir run state dir: %w", err)
	}
	state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal run state: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write run state: %w", err)
	}
	return nil
}

func loadRunState(path string) (*RunState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read run state: %w", err)
	}

	var state RunState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parse run state: %w", err)
	}
	return &state, nil
}

func saveRunSummary(path string, summary *RunSummary) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir run summary dir: %w", err)
	}
	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal run summary: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write run summary: %w", err)
	}
	return nil
}

func loadRunSummary(paths *RuntimePaths, runID string) (*RunSummary, string, error) {
	path := paths.RunSummaryPath(runID)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("read run summary: %w", err)
	}

	var summary RunSummary
	if err := json.Unmarshal(data, &summary); err != nil {
		return nil, "", fmt.Errorf("parse run summary: %w", err)
	}
	return &summary, path, nil
}

func printRunSummary(summary *RunSummary) {
	fmt.Printf("RUN %s\n", summary.RunID)
	fmt.Printf("TASK %s\n", summary.TaskID)
	fmt.Printf("STATUS %s\n", summary.Status)
	if summary.StartedAt != "" {
		fmt.Printf("STARTED %s\n", summary.StartedAt)
	}
	if summary.FinishedAt != "" {
		fmt.Printf("FINISHED %s\n", summary.FinishedAt)
	}
	if summary.ResultPath != "" {
		fmt.Printf("RESULT %s\n", summary.ResultPath)
	}
	if summary.Error != "" {
		fmt.Printf("ERROR %s\n", summary.Error)
	}
	for i, event := range summary.Events {
		fmt.Printf("EVENT%d %s\n", i+1, event)
	}
}

func readLastLines(path string, last int) ([]string, error) {
	if last <= 0 {
		last = 40
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}
	defer file.Close()

	lines := make([]string, 0, last)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		if len(lines) > last {
			lines = lines[1:]
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan log file: %w", err)
	}
	return lines, nil
}

func newRunSummary(runID, taskID, taskPath, stdoutPath, stderrPath string, startedAt time.Time) *RunSummary {
	return &RunSummary{
		RunID:      runID,
		TaskID:     taskID,
		Status:     "running",
		StartedAt:  startedAt.UTC().Format(time.RFC3339),
		TaskPath:   taskPath,
		StdoutPath: stdoutPath,
		StderrPath: stderrPath,
		Events: []string{
			"run started",
		},
	}
}
