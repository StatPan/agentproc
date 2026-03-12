package main

import (
	"context"
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
)

var executeTaskFunc = executeTask

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

	cfg, queueDir, _, _, err := loadRuntimeConfig(*rootFlag, *configFlag)
	if err != nil {
		return err
	}

	switch strings.ToLower(strings.TrimSpace(*transportFlag)) {
	case "http":
		return serveHTTP(cfg, queueDir, ":"+*portFlag, newDispatchReinvokeFunc(cfg, *rootFlag, *configFlag))
	case "mcp":
		return StartMCPServer(cfg, queueDir, newDispatchReinvokeFunc(cfg, *rootFlag, *configFlag))
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

	cfg, queueDir, _, _, err := loadRuntimeConfig(*rootFlag, *configFlag)
	if err != nil {
		return err
	}

	switch strings.ToLower(strings.TrimSpace(*transportFlag)) {
	case "http":
		return serveHTTP(cfg, queueDir, ":"+*portFlag, newDispatchReinvokeFunc(cfg, *rootFlag, *configFlag))
	case "mcp":
		return StartMCPServer(cfg, queueDir, newDispatchReinvokeFunc(cfg, *rootFlag, *configFlag))
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

	cfg, queueDir, runIndexDir, outputsDir, err := loadRuntimeConfig(*rootFlag, *configFlag)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT)
	defer stop()

	return RunDispatchOnce(ctx, cfg, queueDir, runIndexDir, outputsDir)
}

func loadRuntimeConfig(rootArg string, configArg string) (*Config, string, string, string, error) {
	root, err := filepath.Abs(rootArg)
	if err != nil {
		return nil, "", "", "", fmt.Errorf("resolve root path: %w", err)
	}

	configPath, err := filepath.Abs(configArg)
	if err != nil {
		return nil, "", "", "", fmt.Errorf("resolve config path: %w", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		return nil, "", "", "", fmt.Errorf("load config: %w", err)
	}

	if cfg.AgentOSRoot == "" {
		cfg.AgentOSRoot = root
	} else if !filepath.IsAbs(cfg.AgentOSRoot) {
		cfg.AgentOSRoot = filepath.Clean(filepath.Join(root, cfg.AgentOSRoot))
	}

	if cfg.Runner.MaxConcurrent <= 0 {
		cfg.Runner.MaxConcurrent = runtime.NumCPU()
	}

	queueDir := filepath.Join(cfg.AgentOSRoot, "tasks", "queue")
	runIndexDir := filepath.Join(cfg.AgentOSRoot, "tasks", ".run")
	outputsDir := filepath.Join(cfg.AgentOSRoot, "outputs")
	return cfg, queueDir, runIndexDir, outputsDir, nil
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

type runningSet struct {
	mu    sync.Mutex
	tasks map[string]struct{}
}

func newRunningSet() *runningSet {
	return &runningSet{tasks: make(map[string]struct{})}
}

func (r *runningSet) TryStart(taskID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.tasks[taskID]; exists {
		return false
	}
	r.tasks[taskID] = struct{}{}
	return true
}

func (r *runningSet) Done(taskID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.tasks, taskID)
}

func RunDispatchOnce(ctx context.Context, cfg *Config, queueDir, runIndexDir, outputsDir string) error {
	return dispatchTasks(ctx, cfg, queueDir, runIndexDir, outputsDir, nil)
}

func dispatchCycle(
	ctx context.Context,
	cfg *Config,
	queueDir string,
	runIndexDir string,
	outputsDir string,
	onTaskComplete func(),
) error {
	return dispatchTasks(ctx, cfg, queueDir, runIndexDir, outputsDir, onTaskComplete)
}

func dispatchTasks(
	ctx context.Context,
	cfg *Config,
	queueDir string,
	runIndexDir string,
	outputsDir string,
	onTaskComplete func(),
) error {
	sem := make(chan struct{}, cfg.Runner.MaxConcurrent)
	running := newRunningSet()

	tasks, err := collectDispatchableTasks(queueDir, runIndexDir, outputsDir)
	if err != nil {
		return err
	}

	var wg sync.WaitGroup
	for _, task := range tasks {
		if ctx.Err() != nil {
			break
		}
		if !running.TryStart(task.TaskID) {
			continue
		}

		switch strings.ToLower(strings.TrimSpace(task.Execution)) {
		case "parallel":
			if !acquireSemaphore(ctx, sem) {
				running.Done(task.TaskID)
				break
			}
			wg.Add(1)
			go func(task *Task) {
				defer wg.Done()
				defer func() {
					<-sem
					running.Done(task.TaskID)
					if onTaskComplete != nil {
						onTaskComplete()
					}
				}()
				executeTaskFunc(cfg, queueDir, task)
			}(task)
		default:
			executeTaskFunc(cfg, queueDir, task)
			running.Done(task.TaskID)
			if onTaskComplete != nil {
				onTaskComplete()
			}
		}
	}

	wg.Wait()
	return nil
}

func collectDispatchableTasks(queueDir, runIndexDir, outputsDir string) ([]*Task, error) {
	tasks, err := LoadQueue(queueDir)
	if err != nil {
		return nil, err
	}

	dependencyIndex, err := buildDependencyIndex(queueDir, runIndexDir)
	if err != nil {
		return nil, err
	}

	runningIDs, err := loadRunningTaskIDs(runIndexDir)
	if err != nil {
		return nil, err
	}

	dispatchable := make([]*Task, 0, len(tasks))
	for _, task := range tasks {
		if slices.Contains(runningIDs, task.TaskID) {
			continue
		}
		ready, err := dependenciesSatisfied(task, dependencyIndex, outputsDir)
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

func executeTask(cfg *Config, queueDir string, task *Task) {
	if err := RunTask(task, cfg); err != nil {
		log.Printf("task %s failed: %v", task.TaskID, err)
	}

	taskPath := filepath.Join(queueDir, task.TaskID+".md")
	if err := os.Remove(taskPath); err != nil && !os.IsNotExist(err) {
		log.Printf("remove task %s from queue: %v", task.TaskID, err)
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
		taskPath := filepath.Join(runIndexDir, entry.Name(), "task.md")
		task, err := ParseTask(taskPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("parse running task %s: %w", taskPath, err)
		}
		taskIDs = append(taskIDs, task.TaskID)
	}

	return taskIDs, nil
}

func dependenciesSatisfied(task *Task, dependencyIndex map[string]string, outputsDir string) (bool, error) {
	for _, depID := range task.DependsOn {
		outputPath, ok := dependencyIndex[depID]
		if !ok {
			outputPath = defaultDependencyOutputPath(depID)
		}

		resolvedPath := resolveOutputPath(outputsDir, outputPath)
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

func resolveOutputPath(outputsDir string, outputPath string) string {
	cleaned := filepath.Clean(outputPath)
	prefix := "outputs" + string(filepath.Separator)
	if cleaned == "outputs" {
		return outputsDir
	}
	if strings.HasPrefix(cleaned, prefix) {
		return filepath.Join(filepath.Dir(outputsDir), cleaned)
	}
	return filepath.Join(outputsDir, cleaned)
}
