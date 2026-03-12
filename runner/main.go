package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"
)

var executeTaskFunc = executeTask

type taskClaim struct {
	task      *Task
	runID     string
	runDir    string
	queuePath string
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(args []string) error {
	if len(args) > 0 {
		switch args[0] {
		case "serve":
			return runServeCommand(args[1:])
		case "dispatch":
			return runDispatchCommand(args[1:])
		case "intake":
			return runIntakeCommand(args[1:])
		case "reply":
			return runReplyCommand(args[1:])
		case "result":
			return runResultCommand(args[1:])
		case "tail":
			return runTailCommand(args[1:])
		}
	}

	if looksLikeDirectRequest(args) {
		return runIntakeCommand(args)
	}

	return runLegacyCommand(args)
}

func looksLikeDirectRequest(args []string) bool {
	if len(args) == 0 {
		return false
	}
	if strings.HasPrefix(args[0], "-") {
		return false
	}
	return !slices.Contains([]string{"serve", "dispatch", "intake", "reply", "result", "tail"}, args[0])
}

func runLegacyCommand(args []string) error {
	fs := flag.NewFlagSet("aproc", flag.ContinueOnError)
	rootFlag := fs.String("root", ".", "AgentOS root path")
	configFlag := fs.String("config", "./config.yaml", "runner config path")
	portFlag := fs.String("port", "8080", "HTTP server port")
	transportFlag := fs.String("transport", "http", "runner transport: http or mcp")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, paths, err := loadRuntimeConfig(*rootFlag, *configFlag)
	if err != nil {
		return err
	}

	switch strings.ToLower(strings.TrimSpace(*transportFlag)) {
	case "http":
		return serveHTTP(cfg, paths.QueueDir(), ":"+*portFlag, newDispatchReinvokeFunc(cfg, *rootFlag, *configFlag))
	case "mcp":
		return StartMCPServer(cfg, paths.QueueDir(), newDispatchReinvokeFunc(cfg, *rootFlag, *configFlag))
	default:
		return fmt.Errorf("unsupported transport %q", *transportFlag)
	}
}

func runServeCommand(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	rootFlag := fs.String("root", ".", "AgentOS root path")
	configFlag := fs.String("config", "./config.yaml", "runner config path")
	portFlag := fs.String("port", "8080", "HTTP server port")
	transportFlag := fs.String("transport", "http", "runner transport: http or mcp")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, paths, err := loadRuntimeConfig(*rootFlag, *configFlag)
	if err != nil {
		return err
	}

	switch strings.ToLower(strings.TrimSpace(*transportFlag)) {
	case "http":
		return serveHTTP(cfg, paths.QueueDir(), ":"+*portFlag, newDispatchReinvokeFunc(cfg, *rootFlag, *configFlag))
	case "mcp":
		return StartMCPServer(cfg, paths.QueueDir(), newDispatchReinvokeFunc(cfg, *rootFlag, *configFlag))
	default:
		return fmt.Errorf("unsupported transport %q", *transportFlag)
	}
}

func runDispatchCommand(args []string) error {
	if len(args) == 0 || args[0] != "once" {
		return fmt.Errorf("usage: aproc dispatch once [--root PATH] [--config PATH]")
	}

	fs := flag.NewFlagSet("dispatch once", flag.ContinueOnError)
	rootFlag := fs.String("root", ".", "AgentOS root path")
	configFlag := fs.String("config", "./config.yaml", "runner config path")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	cfg, paths, err := loadRuntimeConfig(*rootFlag, *configFlag)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT)
	defer stop()

	return RunDispatchOnce(ctx, cfg, paths)
}

func loadRuntimeConfig(rootArg string, configArg string) (*Config, *RuntimePaths, error) {
	root, err := filepath.Abs(rootArg)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve root path: %w", err)
	}

	projectRoot, err := detectProjectRoot(root)
	if err != nil {
		return nil, nil, fmt.Errorf("detect project root: %w", err)
	}

	cfg, configPath, err := loadProjectConfig(projectRoot, configArg)
	if err != nil {
		return nil, nil, fmt.Errorf("load config: %w", err)
	}

	if cfg.AgentOSRoot == "" {
		cfg.AgentOSRoot = projectRoot
	} else if !filepath.IsAbs(cfg.AgentOSRoot) {
		baseDir := projectRoot
		if configPath != "" {
			baseDir = filepath.Dir(configPath)
		}
		cfg.AgentOSRoot = filepath.Clean(filepath.Join(baseDir, cfg.AgentOSRoot))
	}
	cfg.AgentOSRoot = canonicalProjectPath(cfg.AgentOSRoot)

	if cfg.Runner.MaxConcurrent <= 0 {
		cfg.Runner.MaxConcurrent = runtime.NumCPU()
	}
	if cfg.Runner.PollInterval <= 0 {
		cfg.Runner.PollInterval = 5 * time.Second
	}
	if strings.TrimSpace(cfg.Runner.Mode) == "" {
		cfg.Runner.Mode = "daemon"
	}

	return cfg, cfg.RuntimePaths(), nil
}

func serveHTTP(cfg *Config, queueDir, addr string, dispatch func(context.Context) error) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT)
	defer stop()

	return startServer(ctx, cfg, queueDir, addr, dispatch)
}

func triggerDispatch(dispatchRequests chan struct{}) {
	select {
	case dispatchRequests <- struct{}{}:
	default:
	}
}

func newDispatchReinvokeFunc(cfg *Config, rootArg, configArg string) func(context.Context) error {
	rootPath, err := filepath.Abs(rootArg)
	if err != nil {
		log.Printf("resolve dispatch root path: %v", err)
		rootPath = rootArg
	}

	configPath, err := filepath.Abs(configArg)
	if err != nil {
		log.Printf("resolve dispatch config path: %v", err)
		configPath = configArg
	}

	executable, err := os.Executable()
	if err != nil {
		log.Printf("resolve runner executable: %v", err)
		return nil
	}

	return func(ctx context.Context) error {
		cmd := exec.CommandContext(ctx, executable, "dispatch", "once", "--root", rootPath, "--config", configPath)
		cmd.Dir = cfg.AgentOSRoot
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("start dispatch once subprocess: %w", err)
		}
		return cmd.Process.Release()
	}
}

func RunDispatchOnce(ctx context.Context, cfg *Config, paths *RuntimePaths) error {
	return dispatchTasks(ctx, cfg, paths, nil)
}

func dispatchCycle(
	ctx context.Context,
	cfg *Config,
	paths *RuntimePaths,
	onTaskComplete func(),
) error {
	return dispatchTasks(ctx, cfg, paths, onTaskComplete)
}

func dispatchTasks(
	ctx context.Context,
	cfg *Config,
	paths *RuntimePaths,
	onTaskComplete func(),
) error {
	sem := make(chan struct{}, cfg.Runner.MaxConcurrent)

	tasks, err := collectDispatchableTasks(paths)
	if err != nil {
		return err
	}

	var wg sync.WaitGroup
	for _, task := range tasks {
		if ctx.Err() != nil {
			break
		}
		claim, err := claimTask(paths, task)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return err
		}
		if claim == nil {
			continue
		}

		switch strings.ToLower(strings.TrimSpace(task.Execution)) {
		case "parallel":
			if !acquireSemaphore(ctx, sem) {
				if err := releaseTaskClaim(paths, claim); err != nil {
					log.Printf("release claim %s: %v", claim.task.TaskID, err)
				}
				break
			}
			wg.Add(1)
			go func(claim *taskClaim) {
				defer wg.Done()
				defer func() {
					<-sem
					if onTaskComplete != nil {
						onTaskComplete()
					}
				}()
				executeTaskFunc(cfg, claim)
			}(claim)
		default:
			executeTaskFunc(cfg, claim)
			if onTaskComplete != nil {
				onTaskComplete()
			}
		}
	}

	wg.Wait()
	return nil
}

func collectDispatchableTasks(paths *RuntimePaths) ([]*Task, error) {
	tasks, err := LoadQueue(paths.QueueDir())
	if err != nil {
		return nil, err
	}

	if err := recoverStaleActiveRuns(paths); err != nil {
		return nil, err
	}

	dependencyIndex, err := buildDependencyIndex(paths.QueueDir(), paths.ActiveRunsDir())
	if err != nil {
		return nil, err
	}

	runningIDs, err := loadRunningTaskIDs(paths.ActiveRunsDir())
	if err != nil {
		return nil, err
	}

	dispatchable := make([]*Task, 0, len(tasks))
	for _, task := range tasks {
		if slices.Contains(runningIDs, task.TaskID) {
			continue
		}
		ready, err := dependenciesSatisfied(task, dependencyIndex, paths)
		if err != nil {
			log.Printf("skip %s: %v", task.TaskID, err)
			continue
		}
		if ready {
			dispatchable = append(dispatchable, task)
		}
	}

	return dispatchable, nil
}

func recoverStaleActiveRuns(paths *RuntimePaths) error {
	entries, err := os.ReadDir(paths.ActiveRunsDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read active runs dir: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if err := recoverStaleActiveRun(paths, entry.Name()); err != nil {
			return err
		}
	}

	return nil
}

func recoverStaleActiveRun(paths *RuntimePaths, runID string) error {
	statePath := paths.ActiveRunStatePath(runID)
	state, err := loadRunState(statePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("load active run state %s: %w", statePath, err)
	}
	if !strings.EqualFold(strings.TrimSpace(state.Status), "running") {
		return nil
	}
	if processAlive(state.PID) {
		return nil
	}

	recoveredStatus := "interrupted"
	reason := "missing process liveness for active run"

	summary := newRunSummary(
		state.RunID,
		state.TaskID,
		filepath.Join(paths.ActiveRunDir(runID), "task.md"),
		"",
		"",
		parseRunStateStartedAt(state.StartedAt),
	)
	summary.Status = recoveredStatus
	summary.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	summary.Events = append(summary.Events, "recovered stale active run before dispatch")
	summary.Error = reason

	state.Status = recoveredStatus
	state.PID = 0

	if err := saveRunSummary(paths.RunSummaryPath(runID), summary); err != nil {
		return fmt.Errorf("write recovered run summary %s: %w", runID, err)
	}
	if err := writeRunState(statePath, state); err != nil {
		return fmt.Errorf("write recovered active run state %s: %w", runID, err)
	}
	if err := writeRunState(paths.CompletedRunStatePath(runID), state); err != nil {
		return fmt.Errorf("write recovered completed run state %s: %w", runID, err)
	}

	return nil
}

func parseRunStateStartedAt(value string) time.Time {
	startedAt, err := time.Parse(time.RFC3339, strings.TrimSpace(value))
	if err != nil {
		return time.Now().UTC()
	}
	return startedAt
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}

func claimTask(paths *RuntimePaths, task *Task) (*taskClaim, error) {
	if task == nil {
		return nil, fmt.Errorf("task is nil")
	}

	runID := fmt.Sprintf("%s-%s", time.Now().Format("20060102-150405"), task.TaskID)
	runDir := paths.ActiveRunDir(runID)
	if err := os.MkdirAll(filepath.Join(runDir, "out"), 0o755); err != nil {
		return nil, fmt.Errorf("create run dir: %w", err)
	}

	queuePath := paths.QueueTaskPath(task.TaskID)
	claimedPath := filepath.Join(runDir, "task.md")
	if err := copyFile(queuePath, claimedPath); err != nil {
		if os.IsNotExist(err) {
			_ = os.RemoveAll(runDir)
			return nil, os.ErrNotExist
		}
		_ = os.RemoveAll(runDir)
		return nil, fmt.Errorf("claim task %s: %w", task.TaskID, err)
	}

	state := &RunState{
		RunID:     runID,
		TaskID:    task.TaskID,
		Status:    "running",
		StartedAt: time.Now().UTC().Format(time.RFC3339),
		PID:       os.Getpid(),
	}
	if err := writeRunState(paths.ActiveRunStatePath(runID), state); err != nil {
		_ = os.RemoveAll(runDir)
		return nil, fmt.Errorf("write claim state %s: %w", task.TaskID, err)
	}

	return &taskClaim{task: task, runID: runID, runDir: runDir, queuePath: queuePath}, nil
}

func releaseTaskClaim(paths *RuntimePaths, claim *taskClaim) error {
	if claim == nil || claim.task == nil {
		return nil
	}

	if err := os.RemoveAll(claim.runDir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove run dir %s: %w", claim.runID, err)
	}
	return nil
}

func executeTask(cfg *Config, claim *taskClaim) {
	if claim == nil || claim.task == nil {
		log.Printf("skip empty task claim")
		return
	}

	if err := runTask(claim.task, cfg, claim.runID, claim.runDir, claim.queuePath); err != nil {
		log.Printf("task %s failed: %v", claim.task.TaskID, err)
		return
	}
}

func acquireSemaphore(ctx context.Context, sem chan struct{}) bool {
	select {
	case sem <- struct{}{}:
		return true
	case <-ctx.Done():
		return false
	}
}

func buildDependencyIndex(queueDir string, runIndexDir string) (map[string]string, error) {
	index := make(map[string]string)

	queueTasks, err := LoadQueue(queueDir)
	if err != nil {
		return nil, err
	}
	for _, task := range queueTasks {
		if task.Output != "" {
			index[task.TaskID] = task.Output
		}
	}

	runEntries, err := os.ReadDir(runIndexDir)
	if err != nil {
		if os.IsNotExist(err) {
			return index, nil
		}
		return nil, fmt.Errorf("read run dir: %w", err)
	}

	for _, entry := range runEntries {
		if !entry.IsDir() {
			continue
		}

		taskPath := filepath.Join(runIndexDir, entry.Name(), "task.md")
		task, err := ParseTask(taskPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("parse dependency task %s: %w", taskPath, err)
		}
		if task.Output != "" {
			if _, exists := index[task.TaskID]; !exists {
				index[task.TaskID] = task.Output
			}
		}
	}

	return index, nil
}

func loadRunningTaskIDs(runIndexDir string) ([]string, error) {
	entries, err := os.ReadDir(runIndexDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read run dir: %w", err)
	}

	taskIDs := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		statePath := filepath.Join(runIndexDir, entry.Name(), "run.json")
		state, err := loadRunState(statePath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("load running state %s: %w", statePath, err)
		}
		if !strings.EqualFold(strings.TrimSpace(state.Status), "running") {
			continue
		}
		if strings.TrimSpace(state.TaskID) != "" {
			taskIDs = append(taskIDs, state.TaskID)
		}
	}

	return taskIDs, nil
}

func dependenciesSatisfied(task *Task, dependencyIndex map[string]string, paths *RuntimePaths) (bool, error) {
	for _, depID := range task.DependsOn {
		outputPath, ok := dependencyIndex[depID]
		if !ok {
			outputPath = defaultDependencyOutputPath(depID)
		}

		resolvedPath := paths.ResolveOutputPath(outputPath)
		if _, err := os.Stat(resolvedPath); err != nil {
			if os.IsNotExist(err) {
				return false, nil
			}
			return false, fmt.Errorf("check dependency %s output: %w", depID, err)
		}
	}

	return true, nil
}

func defaultDependencyOutputPath(taskID string) string {
	return filepath.Join("outputs", "result-"+taskID+".md")
}
