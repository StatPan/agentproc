package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	
	"sort"
	"strings"
	"time"
)

func runRequestCommand(args []string) error {
	fs := flag.NewFlagSet("request", flag.ContinueOnError)
	rootFlag := fs.String("root", ".", "AgentOS root path")
	configFlag := fs.String("config", "./config.yaml", "runner config path")
	fileFlag := fs.String("file", "", "read request from file")
	jsonFlag := fs.Bool("json", false, "output JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}

	var requestText string
	if *fileFlag != "" {
		data, err := os.ReadFile(*fileFlag)
		if err != nil {
			return fmt.Errorf("read request file: %w", err)
		}
		requestText = string(data)
	} else {
		requestText = strings.TrimSpace(strings.Join(fs.Args(), " "))
		if requestText == "" {
			return errors.New("usage: aproc request [--root PATH] [--config PATH] [--file PATH] [--json] \"request\"")
		}
	}

	_, paths, err := loadRuntimeConfig(*rootFlag, *configFlag)
	if err != nil {
		return err
	}

	request, err := createRequest(paths, requestText)
	if err != nil {
		return err
	}

	if *jsonFlag {
		return printRequestJSON(request)
	}
	return printRequestHuman(request)
}

func runStatusCommand(args []string) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	rootFlag := fs.String("root", ".", "AgentOS root path")
	configFlag := fs.String("config", "./config.yaml", "runner config path")
	watchFlag := fs.Bool("watch", false, "watch for changes")
	if err := fs.Parse(args); err != nil {
		return err
	}

	requestID := strings.TrimSpace(fs.Arg(0))
	if requestID == "" {
		return errors.New("usage: aproc status [--root PATH] [--config PATH] [--watch] <request-id>")
	}

	_, paths, err := loadRuntimeConfig(*rootFlag, *configFlag)
	if err != nil {
		return err
	}

	if *watchFlag {
		return watchStatus(paths, requestID)
	}

	return printStatus(paths, requestID)
}

func runAnswerCommand(args []string) error {
	fs := flag.NewFlagSet("answer", flag.ContinueOnError)
	rootFlag := fs.String("root", ".", "AgentOS root path")
	configFlag := fs.String("config", "./config.yaml", "runner config path")
	fileFlag := fs.String("file", "", "read answer from file")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if len(fs.Args()) < 1 {
		return errors.New("usage: aproc answer [--root PATH] [--config PATH] [--file PATH] <request-id> [answer]")
	}

	requestID := strings.TrimSpace(fs.Arg(0))
	if requestID == "" {
		return errors.New("request-id is required")
	}

	var answer string
	if *fileFlag != "" {
		data, err := os.ReadFile(*fileFlag)
		if err != nil {
			return fmt.Errorf("read answer file: %w", err)
		}
		answer = string(data)
	} else {
		answer = strings.TrimSpace(strings.Join(fs.Args()[1:], " "))
		if answer == "" {
			return errors.New("answer text is required")
		}
	}

	_, paths, err := loadRuntimeConfig(*rootFlag, *configFlag)
	if err != nil {
		return err
	}

	request, err := answerRequest(paths, requestID, answer)
	if err != nil {
		return err
	}

	return printRequestHuman(request)
}

func runInspectCommand(args []string) error {
	fs := flag.NewFlagSet("inspect", flag.ContinueOnError)
	rootFlag := fs.String("root", ".", "AgentOS root path")
	configFlag := fs.String("config", "./config.yaml", "runner config path")
	jsonFlag := fs.Bool("json", false, "output JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}

	requestID := strings.TrimSpace(fs.Arg(0))
	if requestID == "" {
		return errors.New("usage: aproc inspect [--root PATH] [--config PATH] [--json] <request-id>")
	}

	_, paths, err := loadRuntimeConfig(*rootFlag, *configFlag)
	if err != nil {
		return err
	}

	return printInspect(paths, requestID, *jsonFlag)
}

func runLogsCommand(args []string) error {
	fs := flag.NewFlagSet("logs", flag.ContinueOnError)
	rootFlag := fs.String("root", ".", "AgentOS root path")
	configFlag := fs.String("config", "./config.yaml", "runner config path")
	streamFlag := fs.String("stream", "stdout", "log stream: stdout or stderr")
	lastFlag := fs.Int("tail", 40, "number of log lines to show")
	if err := fs.Parse(args); err != nil {
		return err
	}

	requestID := strings.TrimSpace(fs.Arg(0))
	if requestID == "" {
		return errors.New("usage: aproc logs [--root PATH] [--config PATH] [--stream stdout|stderr] [--tail N] <request-id>")
	}

	_, paths, err := loadRuntimeConfig(*rootFlag, *configFlag)
	if err != nil {
		return err
	}

	return printLogs(paths, requestID, *streamFlag, *lastFlag)
}

func runDebugCommand(args []string) error {
	fs := flag.NewFlagSet("debug", flag.ContinueOnError)
	rootFlag := fs.String("root", ".", "AgentOS root path")
	configFlag := fs.String("config", "./config.yaml", "runner config path")
	if err := fs.Parse(args); err != nil {
		return err
	}

	requestID := strings.TrimSpace(fs.Arg(0))
	if requestID == "" {
		return errors.New("usage: aproc debug [--root PATH] [--config PATH] <request-id>")
	}

	_, paths, err := loadRuntimeConfig(*rootFlag, *configFlag)
	if err != nil {
		return err
	}

	return printDebug(paths, requestID)
}

func runListCommand(args []string) error {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	rootFlag := fs.String("root", ".", "AgentOS root path")
	configFlag := fs.String("config", "./config.yaml", "runner config path")
	jsonFlag := fs.Bool("json", false, "output JSON")
	limitFlag := fs.Int("n", 10, "number of recent requests to show")
	if err := fs.Parse(args); err != nil {
		return err
	}

	_, paths, err := loadRuntimeConfig(*rootFlag, *configFlag)
	if err != nil {
		return err
	}

	requests, err := listRequests(paths)
	if err != nil {
		return err
	}

	sort.Slice(requests, func(i, j int) bool {
		return requests[i].CreatedAt > requests[j].CreatedAt
	})

	if *jsonFlag {
		return printRequestsJSON(requests, *limitFlag)
	}
	return printRequestsHuman(requests, *limitFlag)
}

func printRequestHuman(request *RequestState) error {
	fmt.Printf("Request: %s\n", request.RequestID)
	fmt.Printf("Status: %s\n", request.Status)
	fmt.Printf("Summary: %s\n", request.Summary)
	if request.Status == StatusNeedsClarification {
		fmt.Printf("Questions pending: %d\n", len(request.Questions))
		for i, q := range request.Questions {
			fmt.Printf("  Q%d: %s\n", i+1, q)
		}
		fmt.Printf("Next: Run `aproc answer %s \"<your answer>\"`\n", request.RequestID)
	} else if request.Status == StatusRunning {
		if request.TaskID != "" {
			fmt.Printf("Task ID: %s\n", request.TaskID)
		}
		fmt.Printf("Next: Run `aproc status %s` for progress\n", request.RequestID)
	} else if request.Status == StatusCompleted {
		fmt.Printf("Next: Run `aproc result %s` for details\n", request.RequestID)
	}
	return nil
}

func printRequestJSON(request *RequestState) error {
	return nil
}

func printStatus(paths *RuntimePaths, requestID string) error {
	request, err := loadRequest(paths, requestID)
	if err != nil {
		return err
	}

	fmt.Printf("Request: %s\n", request.RequestID)
	fmt.Printf("Status: %s\n", request.Status)
	fmt.Printf("Summary: %s\n", request.Summary)

	if request.Status == StatusNeedsClarification {
		fmt.Println()
		for i, q := range request.Questions {
			fmt.Printf("Q%d: %s\n", i+1, q)
		}
		fmt.Printf("\nNext: Run `aproc answer %s \"<your answer>\"`\n", request.RequestID)
	} else if request.Status == StatusRunning && request.TaskID != "" {
		fmt.Printf("\nTask ID: %s\n", request.TaskID)
		summary, _, err := loadRunSummaryByRunID(paths, request.TaskID)
		if err == nil {
			fmt.Printf("Started: %s\n", summary.StartedAt)
			if summary.FinishedAt != "" {
				fmt.Printf("Finished: %s\n", summary.FinishedAt)
				if summary.DurationMS > 0 {
					fmt.Printf("Duration: %dms\n", summary.DurationMS)
				}
			}
			if len(summary.Events) > 0 {
				fmt.Println("\nRecent events:")
				for _, event := range summary.Events[max(0, len(summary.Events)-3):] {
					fmt.Printf("  - %s\n", event)
				}
			}
		}
		fmt.Printf("\nNext: Run `aproc status %s` for progress\n", request.RequestID)
	}

	return nil
}

func watchStatus(paths *RuntimePaths, requestID string) error {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		if err := printStatus(paths, requestID); err != nil {
			return err
		}
		
		request, err := loadRequest(paths, requestID)
		if err != nil {
			return err
		}
		
		if request.Status == StatusCompleted || request.Status == StatusFailed || request.Status == StatusCancelled {
			return nil
		}
		
		select {
		case <-ticker.C:
			continue
		}
	}
}

func printInspect(paths *RuntimePaths, requestID string, jsonFlag bool) error {
	request, err := loadRequest(paths, requestID)
	if err != nil {
		return err
	}

	fmt.Printf("Request ID: %s\n", request.RequestID)
	fmt.Printf("Status: %s\n", request.Status)
	fmt.Printf("Summary: %s\n", request.Summary)
	fmt.Printf("Created: %s\n", request.CreatedAt)
	fmt.Printf("Updated: %s\n", request.UpdatedAt)

	if request.TaskID != "" {
		fmt.Printf("\nTask ID: %s\n", request.TaskID)
		summary, _, err := loadRunSummaryByRunID(paths, request.TaskID)
		if err == nil {
			fmt.Printf("\nRun Summary:\n")
			fmt.Printf("  Status: %s\n", summary.Status)
			fmt.Printf("  Started: %s\n", summary.StartedAt)
			if summary.FinishedAt != "" {
				fmt.Printf("  Finished: %s\n", summary.FinishedAt)
			}
			if summary.DurationMS > 0 {
				fmt.Printf("  Duration: %dms\n", summary.DurationMS)
			}
			if summary.ResultPath != "" {
				fmt.Printf("  Result: %s\n", summary.ResultPath)
			}
			if summary.Error != "" {
				fmt.Printf("  Error: %s\n", summary.Error)
			}
			if len(summary.Events) > 0 {
				fmt.Println("\n  Events:")
				for _, event := range summary.Events {
					fmt.Printf("    - %s\n", event)
				}
			}
		}
	}

	if len(request.Questions) > 0 {
		fmt.Println("\nClarification Questions:")
		for i, q := range request.Questions {
			fmt.Printf("  %d. %s\n", i+1, q)
		}
	}

	if len(request.Answers) > 0 {
		fmt.Println("\nAnswers:")
		for i, a := range request.Answers {
			fmt.Printf("  %d. %s\n", i+1, a)
		}
	}

	return nil
}

func printLogs(paths *RuntimePaths, requestID, stream string, last int) error {
	request, err := loadRequest(paths, requestID)
	if err != nil {
		return err
	}

	if request.TaskID == "" {
		return fmt.Errorf("no task associated with request %s", requestID)
	}

	summary, _, err := loadRunSummaryByRunID(paths, request.TaskID)
	if err != nil {
		return err
	}

	logPath := summary.StdoutPath
	if strings.EqualFold(strings.TrimSpace(stream), "stderr") {
		logPath = summary.StderrPath
	}
	if strings.TrimSpace(logPath) == "" {
		return fmt.Errorf("no %s log path recorded for request %s", stream, requestID)
	}

	lines, err := readLastLines(logPath, last)
	if err != nil {
		return err
	}
	
	fmt.Printf("=== %s log for request %s (last %d lines) ===\n", stream, requestID, last)
	fmt.Println("(Note: This is low-level debugging output. For a summary, use `aproc inspect`)")
	fmt.Println()
	for _, line := range lines {
		fmt.Println(line)
	}

	return nil
}

func printDebug(paths *RuntimePaths, requestID string) error {
	fmt.Printf("=== Debug Info for Request %s ===\n", requestID)
	fmt.Println("(Warning: Debug output contains internal identifiers and filesystem locations)")
	fmt.Println()

	projectKey := ""
	if paths.hiddenRuntime {
		projectKey = paths.projectKey
		fmt.Printf("Hidden Runtime: Enabled\n")
		fmt.Printf("Project Key: %s\n", projectKey)
		fmt.Printf("State Root: %s\n", paths.stateRoot)
		fmt.Printf("Project Root: %s\n", paths.projectRoot())
	} else {
		fmt.Printf("Hidden Runtime: Disabled\n")
		fmt.Printf("AgentOS Root: %s\n", paths.agentOSRoot)
	}

	fmt.Printf("\nRuntime Paths:\n")
	fmt.Printf("  Queue Dir: %s\n", paths.QueueDir())
	fmt.Printf("  Active Runs Dir: %s\n", paths.ActiveRunsDir())
	fmt.Printf("  Completed Runs Dir: %s\n", paths.CompletedRunsDir())
	fmt.Printf("  Outputs Dir: %s\n", paths.OutputsDir())

	if projectKey != "" {
		fmt.Printf("\nFilesystem Locations:\n")
		fmt.Printf("  ~/.aproc/projects/%s/tasks/queue/\n", projectKey)
		fmt.Printf("  ~/.aproc/projects/%s/runs/active/\n", projectKey)
		fmt.Printf("  ~/.aproc/projects/%s/runs/completed/\n", projectKey)
		fmt.Printf("  ~/.aproc/projects/%s/outputs/results/\n", projectKey)
	}

	request, err := loadRequest(paths, requestID)
	if err != nil {
		return err
	}

	fmt.Printf("\nRequest State:\n")
	fmt.Printf("  ID: %s\n", request.RequestID)
	fmt.Printf("  Status: %s\n", request.Status)
	fmt.Printf("  Task ID: %s\n", request.TaskID)
	fmt.Printf("  Run ID: %s\n", request.RunID)
	if request.TaskID != "" {
		fmt.Printf("  Task Path: %s\n", paths.QueueTaskPath(request.TaskID))
	}

	return nil
}

func printRequestsHuman(requests []*RequestState, limit int) error {
	if len(requests) == 0 {
		fmt.Println("No requests found.")
		return nil
	}

	if limit > len(requests) {
		limit = len(requests)
	}

	fmt.Printf("Recent %d requests:\n", limit)
	fmt.Println()

	for i := 0; i < limit; i++ {
		req := requests[i]
		fmt.Printf("%s [%s] %s\n", req.RequestID, req.Status, req.Summary)
	}

	return nil
}

func printRequestsJSON(requests []*RequestState, limit int) error {
	return nil
}

