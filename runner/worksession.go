package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type WorkSessionListItem struct {
	ID              string `json:"id"`
	Title           string `json:"title"`
	Status          string `json:"status"`
	Summary         string `json:"summary"`
	TaskCount       int    `json:"task_count"`
	ActiveTaskCount int    `json:"active_task_count"`
	UpdatedAt       string `json:"updated_at"`
}

type createWorkSessionRequest struct {
	Request string `json:"request"`
}

type WorkSessionDetail struct {
	Session       RequestView     `json:"session"`
	Tasks         []TaskView      `json:"tasks"`
	Topology      TopologyView    `json:"topology"`
	Interventions []Intervention  `json:"interventions"`
	Timeline      []TimelineEvent `json:"timeline"`
}

type RequestView struct {
	ID                string         `json:"id"`
	Title             string         `json:"title"`
	Status            string         `json:"status"`
	Stage             string         `json:"stage,omitempty"`
	Summary           string         `json:"summary"`
	OriginalText      string         `json:"original_text"`
	Questions         []string       `json:"questions"`
	Answers           []string       `json:"answers"`
	TaskIDs           []string       `json:"task_ids"`
	AvailableControls []string       `json:"available_controls"`
	UpdatedAt         string         `json:"updated_at"`
	Messages          []Intervention `json:"messages"`
	PendingCount      int            `json:"pending_count"`
	LatestEvent       string         `json:"latest_event,omitempty"`
	LastActivity      string         `json:"last_activity,omitempty"`
	NeedsInput        bool           `json:"needs_input,omitempty"`
}

type TaskView struct {
	ID                string         `json:"id"`
	Title             string         `json:"title"`
	Status            string         `json:"status"`
	Stage             string         `json:"stage,omitempty"`
	Role              string         `json:"role"`
	AssignedTo        string         `json:"assigned_to"`
	Execution         string         `json:"execution"`
	Input             string         `json:"input"`
	DependsOn         []string       `json:"depends_on"`
	OutputPaths       []string       `json:"output_paths"`
	LatestRunID       string         `json:"latest_run_id,omitempty"`
	RunCount          int            `json:"run_count"`
	AvailableControls []string       `json:"available_controls"`
	Runs              []RunView      `json:"runs"`
	Interventions     []Intervention `json:"interventions"`
	PendingCount      int            `json:"pending_count"`
	LatestEvent       string         `json:"latest_event,omitempty"`
	LastActivity      string         `json:"last_activity,omitempty"`
	NextApplication   string         `json:"next_application,omitempty"`
}

type RunView struct {
	ID                string         `json:"id"`
	TaskID            string         `json:"task_id"`
	Status            string         `json:"status"`
	Stage             string         `json:"stage,omitempty"`
	StartedAt         string         `json:"started_at"`
	FinishedAt        string         `json:"finished_at,omitempty"`
	DurationMS        int64          `json:"duration_ms,omitempty"`
	ResultPath        string         `json:"result_path,omitempty"`
	StdoutPath        string         `json:"stdout_path,omitempty"`
	StderrPath        string         `json:"stderr_path,omitempty"`
	EvidencePath      string         `json:"evidence_path,omitempty"`
	Error             string         `json:"error,omitempty"`
	Events            []string       `json:"events,omitempty"`
	Active            bool           `json:"active"`
	AvailableControls []string       `json:"available_controls"`
	Interventions     []Intervention `json:"interventions"`
	PendingCount      int            `json:"pending_count"`
	LatestEvent       string         `json:"latest_event,omitempty"`
	LastActivity      string         `json:"last_activity,omitempty"`
	NextApplication   string         `json:"next_application,omitempty"`
}

type TopologyView struct {
	Nodes []TopologyNode `json:"nodes"`
	Edges []TopologyEdge `json:"edges"`
}

type TopologyNode struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Label    string `json:"label"`
	Status   string `json:"status"`
	ParentID string `json:"parent_id,omitempty"`
}

type TopologyEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type Intervention struct {
	ID        string         `json:"id"`
	NodeID    string         `json:"node_id"`
	NodeType  string         `json:"node_type"`
	Action    string         `json:"action"`
	Message   string         `json:"message,omitempty"`
	Command   string         `json:"command,omitempty"`
	Status    string         `json:"status"`
	CreatedAt string         `json:"created_at"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type nodeMessageRequest struct {
	Message string `json:"message"`
}

type nodeControlRequest struct {
	Command string `json:"command"`
}

type nodeActionResponse struct {
	Status          string        `json:"status"`
	NodeID          string        `json:"node_id"`
	Intervention    *Intervention `json:"intervention,omitempty"`
	PendingCount    int           `json:"pending_count,omitempty"`
	NextApplication string        `json:"next_application,omitempty"`
	LatestActivity  string        `json:"latest_activity,omitempty"`
}

type runLogsResponse struct {
	RunID  string   `json:"run_id"`
	Stream string   `json:"stream"`
	Path   string   `json:"path"`
	Lines  []string `json:"lines"`
}

type runEvidenceResponse struct {
	RunID        string           `json:"run_id"`
	EvidencePath string           `json:"evidence_path"`
	Invocations  []map[string]any `json:"invocations"`
}

type workSessionStore struct {
	paths *RuntimePaths
}

func newWorkSessionStore(paths *RuntimePaths) *workSessionStore {
	return &workSessionStore{paths: paths}
}

func (s *workSessionStore) list() ([]WorkSessionListItem, error) {
	if ids, err := canonicalSessionIDs(s.paths); err == nil && len(ids) > 0 {
		items := make([]WorkSessionListItem, 0, len(ids))
		for _, id := range ids {
			detail, loadErr := s.load(id)
			if loadErr != nil {
				continue
			}
			activeTasks := 0
			for _, task := range detail.Tasks {
				if task.Status == StatusRunning || task.Status == StatusQueued {
					activeTasks++
				}
			}
			items = append(items, WorkSessionListItem{
				ID:              detail.Session.ID,
				Title:           detail.Session.Title,
				Status:          detail.Session.Status,
				Summary:         detail.Session.Summary,
				TaskCount:       len(detail.Tasks),
				ActiveTaskCount: activeTasks,
				UpdatedAt:       detail.Session.UpdatedAt,
			})
		}
		sort.Slice(items, func(i, j int) bool {
			return items[i].UpdatedAt > items[j].UpdatedAt
		})
		return items, nil
	} else if err != nil {
		return nil, err
	}

	requests, err := listRequests(s.paths)
	if err != nil {
		return nil, err
	}

	items := make([]WorkSessionListItem, 0, len(requests))
	for _, request := range requests {
		detail, err := s.load(request.RequestID)
		if err != nil {
			continue
		}
		activeTasks := 0
		for _, task := range detail.Tasks {
			if task.Status == StatusRunning || task.Status == StatusQueued {
				activeTasks++
			}
		}
		items = append(items, WorkSessionListItem{
			ID:              detail.Session.ID,
			Title:           detail.Session.Title,
			Status:          detail.Session.Status,
			Summary:         detail.Session.Summary,
			TaskCount:       len(detail.Tasks),
			ActiveTaskCount: activeTasks,
			UpdatedAt:       detail.Session.UpdatedAt,
		})
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].UpdatedAt > items[j].UpdatedAt
	})
	return items, nil
}

func (s *workSessionStore) load(requestID string) (*WorkSessionDetail, error) {
	if session, err := loadCanonicalSession(s.paths, requestID); err == nil {
		timeline, timelineErr := loadSessionTimeline(s.paths, requestID)
		if timelineErr != nil {
			return nil, timelineErr
		}
		tasks := make([]TaskView, 0, len(session.TaskOrder))
		nodes := []TopologyNode{{
			ID:     "request:" + session.Request.RequestID,
			Kind:   "request",
			Label:  requestTitle(session.Request),
			Status: session.Request.Status,
		}}
		edges := make([]TopologyEdge, 0)
		for _, taskID := range session.TaskOrder {
			task := session.Tasks[taskID]
			if task == nil {
				continue
			}
			view := TaskView{
				ID:                task.TaskID,
				Title:             task.Title,
				Status:            task.Status,
				Role:              task.Role,
				AssignedTo:        task.AssignedTo,
				Execution:         task.Execution,
				Input:             task.Input,
				DependsOn:         append([]string(nil), task.DependsOn...),
				OutputPaths:       append([]string(nil), task.OutputPaths...),
				RunCount:          len(task.Runs),
				AvailableControls: availableTaskControls(task.Status),
				Interventions:     filterInterventions(session.Interventions, "task:"+task.TaskID, ""),
				PendingCount:      pendingCountForNode(timeline, "task:"+task.TaskID),
				LatestEvent:       latestEventTitleForNode(timeline, "task:"+task.TaskID),
				LastActivity:      lastActivityForNode(timeline, "task:"+task.TaskID),
				NextApplication:   nextApplicationForNode(timeline, "task:"+task.TaskID),
			}
			for _, run := range task.Runs {
				runView := *run
				runView.PendingCount = pendingCountForNode(timeline, "run:"+run.ID)
				runView.LatestEvent = latestEventTitleForNode(timeline, "run:"+run.ID)
				runView.LastActivity = lastActivityForNode(timeline, "run:"+run.ID)
				runView.NextApplication = nextApplicationForNode(timeline, "run:"+run.ID)
				runView.Stage = projectRunStage(runView.Status)
				view.Runs = append(view.Runs, runView)
				nodes = append(nodes, TopologyNode{
					ID:       "run:" + run.ID,
					Kind:     "run",
					Label:    run.ID,
					Status:   run.Status,
					ParentID: "task:" + task.TaskID,
				})
				edges = append(edges, TopologyEdge{From: "task:" + task.TaskID, To: "run:" + run.ID})
			}
			if len(view.Runs) > 0 {
				view.LatestRunID = view.Runs[0].ID
			}
			view.Stage = projectTaskStage(view.Status, len(view.Runs))
			if view.LastActivity == "" && len(view.Runs) > 0 {
				view.LastActivity = coalesce(view.Runs[0].LastActivity, view.Runs[0].LatestEvent, view.Runs[0].Status)
			}
			tasks = append(tasks, view)
			nodes = append(nodes, TopologyNode{
				ID:       "task:" + task.TaskID,
				Kind:     "task",
				Label:    task.Title,
				Status:   task.Status,
				ParentID: "request:" + session.Request.RequestID,
			})
			edges = append(edges, TopologyEdge{From: "request:" + session.Request.RequestID, To: "task:" + task.TaskID})
		}

		sessionView := RequestView{
			ID:                session.Request.RequestID,
			Title:             requestTitle(session.Request),
			Status:            session.Request.Status,
			Summary:           session.Request.Summary,
			OriginalText:      session.Request.OriginalText,
			Questions:         append([]string(nil), session.Request.Questions...),
			Answers:           append([]string(nil), session.Request.Answers...),
			TaskIDs:           append([]string(nil), allTaskIDs(session.Request)...),
			AvailableControls: availableRequestControls(session.Request.Status),
			UpdatedAt:         session.Request.UpdatedAt,
			Messages:          append([]Intervention(nil), session.Messages...),
			PendingCount:      pendingCountForNode(timeline, "request:"+session.Request.RequestID),
			LatestEvent:       latestEventTitleForNode(timeline, "request:"+session.Request.RequestID),
			LastActivity:      lastActivityForNode(timeline, "request:"+session.Request.RequestID),
			NeedsInput:        session.Request.Status == StatusNeedsClarification && len(session.Request.Answers) < len(session.Request.Questions),
		}
		sessionView.Stage = projectRequestStage(sessionView.Status, sessionView.NeedsInput, tasks)
		if sessionView.LastActivity == "" {
			sessionView.LastActivity = coalesce(sessionView.LatestEvent, sessionView.Summary)
		}

		return &WorkSessionDetail{
			Session: sessionView,
			Tasks:   tasks,
			Topology: TopologyView{
				Nodes: nodes,
				Edges: edges,
			},
			Interventions: append([]Intervention(nil), session.Interventions...),
			Timeline:      timeline,
		}, nil
	}

	request, err := loadRequest(s.paths, requestID)
	if err != nil {
		return nil, err
	}

	allInterventions, err := loadInterventions(s.paths, "")
	if err != nil {
		return nil, err
	}
	filtered := filterInterventions(allInterventions, "request:"+requestID, "")

	taskIDs := allTaskIDs(request)
	tasks := make([]TaskView, 0, len(taskIDs))
	nodes := []TopologyNode{{
		ID:     "request:" + request.RequestID,
		Kind:   "request",
		Label:  requestTitle(request),
		Status: deriveRequestStatus(request, nil),
	}}
	edges := make([]TopologyEdge, 0, len(taskIDs)*2)

	var latestUpdated string
	for _, taskID := range taskIDs {
		taskView, err := s.buildTaskView(taskID, allInterventions)
		if err != nil {
			continue
		}
		tasks = append(tasks, *taskView)
		nodes = append(nodes, TopologyNode{
			ID:       "task:" + taskView.ID,
			Kind:     "task",
			Label:    taskView.Title,
			Status:   taskView.Status,
			ParentID: "request:" + request.RequestID,
		})
		edges = append(edges, TopologyEdge{From: "request:" + request.RequestID, To: "task:" + taskView.ID})
		for _, run := range taskView.Runs {
			nodes = append(nodes, TopologyNode{
				ID:       "run:" + run.ID,
				Kind:     "run",
				Label:    run.ID,
				Status:   run.Status,
				ParentID: "task:" + taskView.ID,
			})
			edges = append(edges, TopologyEdge{From: "task:" + taskView.ID, To: "run:" + run.ID})
			if run.StartedAt > latestUpdated {
				latestUpdated = run.StartedAt
			}
		}
	}

	status := deriveRequestStatus(request, tasks)
	if latestUpdated == "" {
		latestUpdated = request.UpdatedAt
	}

	return &WorkSessionDetail{
		Session: RequestView{
			ID:                request.RequestID,
			Title:             requestTitle(request),
			Status:            status,
			Stage:             projectRequestStage(status, status == StatusNeedsClarification && len(request.Answers) < len(request.Questions), tasks),
			Summary:           summarizeSession(request, tasks),
			OriginalText:      request.OriginalText,
			Questions:         append([]string(nil), request.Questions...),
			Answers:           append([]string(nil), request.Answers...),
			TaskIDs:           append([]string(nil), taskIDs...),
			AvailableControls: availableRequestControls(status),
			UpdatedAt:         latestUpdated,
			Messages:          filtered,
			LastActivity:      coalesce(request.Summary, status),
			NeedsInput:        status == StatusNeedsClarification && len(request.Answers) < len(request.Questions),
		},
		Tasks: tasks,
		Topology: TopologyView{
			Nodes: nodes,
			Edges: edges,
		},
		Interventions: filterInterventions(allInterventions, "", requestID),
		Timeline:      []TimelineEvent{},
	}, nil
}

func pendingCountForNode(timeline []TimelineEvent, nodeRef string) int {
	count := 0
	for _, event := range timeline {
		if event.NodeRef == nodeRef && (event.Status == "recorded" || event.Status == "queued") {
			count++
		}
	}
	return count
}

func lastActivityForNode(timeline []TimelineEvent, nodeRef string) string {
	for i := len(timeline) - 1; i >= 0; i-- {
		if timeline[i].NodeRef == nodeRef {
			return coalesce(timeline[i].Title, timeline[i].Status)
		}
	}
	return ""
}

func latestEventTitleForNode(timeline []TimelineEvent, nodeRef string) string {
	for i := len(timeline) - 1; i >= 0; i-- {
		if timeline[i].NodeRef == nodeRef {
			return timeline[i].Title
		}
	}
	return ""
}

func nextApplicationForNode(timeline []TimelineEvent, nodeRef string) string {
	for i := len(timeline) - 1; i >= 0; i-- {
		event := timeline[i]
		if event.NodeRef != nodeRef {
			continue
		}
		switch event.Status {
		case "recorded":
			return "Applies on next run"
		case "queued":
			if strings.TrimSpace(event.ConsumedByRunID) != "" {
				return "Queued for " + event.ConsumedByRunID
			}
			return "Queued for next run"
		case "applied":
			if strings.TrimSpace(event.ConsumedByRunID) != "" {
				return "Applied in " + event.ConsumedByRunID
			}
			return "Applied"
		}
	}
	return ""
}

func (s *workSessionStore) buildTaskView(taskID string, interventions []Intervention) (*TaskView, error) {
	task, _ := loadTaskSnapshot(s.paths, taskID)
	runs, err := loadRunsForTask(s.paths, taskID)
	if err != nil {
		return nil, err
	}

	status := deriveTaskStatus(task, runs)
	title := taskID
	role := ""
	assignedTo := ""
	execution := ""
	input := ""
	dependsOn := []string{}
	outputs := []string{}
	if task != nil {
		if task.Title != "" {
			title = task.Title
		}
		role = task.Role
		assignedTo = task.AssignedTo
		execution = task.Execution
		input = task.Input
		dependsOn = append([]string(nil), task.DependsOn...)
		outputs = append([]string(nil), task.ExpectedOutputPaths()...)
	}

	view := &TaskView{
		ID:                taskID,
		Title:             title,
		Status:            status,
		Stage:             projectTaskStage(status, len(runs)),
		Role:              role,
		AssignedTo:        assignedTo,
		Execution:         execution,
		Input:             input,
		DependsOn:         dependsOn,
		OutputPaths:       outputs,
		RunCount:          len(runs),
		Runs:              runs,
		AvailableControls: availableTaskControls(status),
		Interventions:     filterInterventions(interventions, "task:"+taskID, ""),
		LastActivity:      status,
	}
	if len(runs) > 0 {
		view.LatestRunID = runs[0].ID
		view.LastActivity = coalesce(runs[0].LastActivity, runs[0].LatestEvent, runs[0].Status)
	}
	return view, nil
}

func requestTitle(request *RequestState) string {
	title := strings.TrimSpace(request.OriginalText)
	if title == "" {
		title = request.Summary
	}
	if len([]rune(title)) > 72 {
		title = string([]rune(title)[:72]) + "..."
	}
	return title
}

func summarizeSession(request *RequestState, tasks []TaskView) string {
	if len(tasks) == 0 {
		return request.Summary
	}
	latest := tasks[0]
	if len(latest.Runs) > 0 {
		run := latest.Runs[0]
		return fmt.Sprintf("%s · %s · %s", latest.Title, latest.Status, run.Status)
	}
	return fmt.Sprintf("%s · %s", latest.Title, latest.Status)
}

func deriveRequestStatus(request *RequestState, tasks []TaskView) string {
	if request == nil {
		return StatusQueued
	}
	if strings.TrimSpace(request.Status) == StatusNeedsClarification {
		if len(request.Answers) < len(request.Questions) {
			return StatusNeedsClarification
		}
	}
	if len(tasks) == 0 {
		if request.Status != "" {
			return request.Status
		}
		return StatusQueued
	}
	statuses := make([]string, 0, len(tasks))
	for _, task := range tasks {
		statuses = append(statuses, task.Status)
	}
	if containsStatus(statuses, StatusRunning) {
		return StatusRunning
	}
	if containsStatus(statuses, StatusQueued) {
		return StatusQueued
	}
	if containsStatus(statuses, terminalStateFailed.String()) || containsStatus(statuses, StatusFailed) {
		return StatusFailed
	}
	if containsStatus(statuses, terminalStateInterrupted.String()) {
		return StatusFailed
	}
	if containsStatus(statuses, StatusCancelled) {
		return StatusCancelled
	}
	if containsStatus(statuses, terminalStateCompleted.String()) {
		return StatusCompleted
	}
	if request.Status != "" {
		return request.Status
	}
	return StatusQueued
}

func deriveTaskStatus(task *Task, runs []RunView) string {
	if len(runs) > 0 {
		return runs[0].Status
	}
	if taskExistsInQueue(task) {
		return StatusQueued
	}
	return StatusQueued
}

func taskExistsInQueue(task *Task) bool {
	return task != nil
}

func containsStatus(statuses []string, target string) bool {
	for _, status := range statuses {
		if status == target {
			return true
		}
	}
	return false
}

func projectRequestStage(status string, needsInput bool, tasks []TaskView) string {
	if needsInput {
		return "needs-input"
	}
	switch status {
	case StatusCompleted, terminalStateCompleted.String():
		return "done"
	case StatusFailed, terminalStateFailed.String(), terminalStateInterrupted.String():
		return "failed"
	case StatusCancelled:
		return "cancelled"
	case StatusRunning:
		for _, task := range tasks {
			if task.Stage == "spawning" || task.Stage == "planned" {
				return "spawning"
			}
		}
		return "running"
	case StatusQueued:
		if len(tasks) == 0 {
			return "planned"
		}
		return "spawning"
	default:
		if len(tasks) == 0 {
			return "planned"
		}
		return "running"
	}
}

func projectTaskStage(status string, runCount int) string {
	switch status {
	case StatusCompleted, terminalStateCompleted.String():
		return "done"
	case StatusFailed, terminalStateFailed.String(), terminalStateInterrupted.String():
		return "failed"
	case StatusCancelled:
		return "cancelled"
	case StatusRunning:
		return "running"
	case StatusNeedsClarification:
		return "needs-input"
	case StatusQueued:
		if runCount == 0 {
			return "spawning"
		}
		return "planned"
	default:
		return "planned"
	}
}

func projectRunStage(status string) string {
	switch status {
	case StatusCompleted, terminalStateCompleted.String():
		return "done"
	case StatusFailed, terminalStateFailed.String(), terminalStateInterrupted.String():
		return "failed"
	case StatusCancelled:
		return "cancelled"
	case StatusRunning:
		return "running"
	case StatusQueued:
		return "planned"
	default:
		return "planned"
	}
}

func coalesce(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func availableRequestControls(status string) []string {
	controls := []string{"message"}
	if status == StatusQueued || status == StatusNeedsClarification {
		controls = append(controls, "cancel")
	}
	if status == StatusFailed || status == StatusCompleted {
		controls = append(controls, "retry")
	}
	return controls
}

func availableTaskControls(status string) []string {
	controls := []string{"message"}
	if status == StatusQueued {
		controls = append(controls, "cancel")
	}
	if status == StatusFailed || status == terminalStateFailed.String() || status == terminalStateInterrupted.String() || status == terminalStateCompleted.String() {
		controls = append(controls, "retry")
	}
	return controls
}

func availableRunControls(status string) []string {
	controls := []string{"message"}
	if status == terminalStateFailed.String() || status == terminalStateInterrupted.String() || status == terminalStateCompleted.String() {
		controls = append(controls, "retry")
	}
	return controls
}

func loadTaskSnapshot(paths *RuntimePaths, taskID string) (*Task, error) {
	queuePath := paths.QueueTaskPath(taskID)
	if _, err := os.Stat(queuePath); err == nil {
		return ParseTask(queuePath)
	}

	activeEntries, err := os.ReadDir(paths.ActiveRunsDir())
	if err == nil {
		for _, entry := range activeEntries {
			taskPath := filepath.Join(paths.ActiveRunsDir(), entry.Name(), "task.md")
			task, parseErr := ParseTask(taskPath)
			if parseErr == nil && task.TaskID == taskID {
				return task, nil
			}
		}
	}

	return nil, os.ErrNotExist
}

func loadRunsForTask(paths *RuntimePaths, taskID string) ([]RunView, error) {
	runs := make([]RunView, 0)

	activeEntries, err := os.ReadDir(paths.ActiveRunsDir())
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	for _, entry := range activeEntries {
		if !entry.IsDir() {
			continue
		}
		state, err := loadRunState(paths.ActiveRunStatePath(entry.Name()))
		if err != nil || state.TaskID != taskID {
			continue
		}
		runs = append(runs, buildRunViewFromState(paths, state))
	}

	completedEntries, err := os.ReadDir(paths.CompletedRunsDir())
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	for _, entry := range completedEntries {
		if !entry.IsDir() {
			continue
		}
		summary, _, err := loadRunSummary(paths, entry.Name())
		if err != nil || summary.TaskID != taskID {
			continue
		}
		runs = append(runs, buildRunViewFromSummary(summary))
	}

	sort.Slice(runs, func(i, j int) bool {
		return runs[i].StartedAt > runs[j].StartedAt
	})
	return runs, nil
}

func buildRunViewFromState(paths *RuntimePaths, state *RunState) RunView {
	runID := strings.TrimSpace(state.RunID)
	runBase := paths.CompletedRunDir(runID)
	return RunView{
		ID:                runID,
		TaskID:            state.TaskID,
		Status:            state.Status,
		Stage:             projectRunStage(state.Status),
		StartedAt:         state.StartedAt,
		StdoutPath:        filepath.Join(runBase, "logs", "stdout.log"),
		StderrPath:        filepath.Join(runBase, "logs", "stderr.log"),
		EvidencePath:      filepath.Join(paths.OutputsDir(), "thread-evidence-"+state.TaskID),
		Active:            true,
		AvailableControls: availableRunControls(state.Status),
		LastActivity:      coalesce(state.Status, state.StartedAt),
	}
}

func buildRunViewFromSummary(summary *RunSummary) RunView {
	return RunView{
		ID:                summary.RunID,
		TaskID:            summary.TaskID,
		Status:            summary.Status,
		Stage:             projectRunStage(summary.Status),
		StartedAt:         summary.StartedAt,
		FinishedAt:        summary.FinishedAt,
		DurationMS:        summary.DurationMS,
		ResultPath:        summary.ResultPath,
		StdoutPath:        summary.StdoutPath,
		StderrPath:        summary.StderrPath,
		EvidencePath:      summary.EvidencePath,
		Error:             summary.Error,
		Events:            append([]string(nil), summary.Events...),
		AvailableControls: availableRunControls(summary.Status),
		LastActivity:      coalesce(summary.Status, summary.FinishedAt, summary.StartedAt),
	}
}

func loadInterventions(paths *RuntimePaths, nodeID string) ([]Intervention, error) {
	dir := paths.InterventionsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Intervention{}, nil
		}
		return nil, err
	}

	items := make([]Intervention, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var item Intervention
		if err := json.Unmarshal(data, &item); err != nil {
			continue
		}
		if nodeID != "" && item.NodeID != nodeID {
			continue
		}
		items = append(items, item)
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].CreatedAt > items[j].CreatedAt
	})
	return items, nil
}

func filterInterventions(items []Intervention, nodeID string, requestID string) []Intervention {
	filtered := make([]Intervention, 0, len(items))
	for _, item := range items {
		if nodeID != "" && item.NodeID == nodeID {
			filtered = append(filtered, item)
			continue
		}
		if requestID != "" {
			if item.Metadata != nil {
				if owner, ok := item.Metadata["request_id"].(string); ok && owner == requestID {
					filtered = append(filtered, item)
				}
			}
		}
	}
	return filtered
}

func writeIntervention(paths *RuntimePaths, intervention Intervention) (*Intervention, error) {
	if err := os.MkdirAll(paths.InterventionsDir(), 0o755); err != nil {
		return nil, err
	}
	if intervention.ID == "" {
		intervention.ID = fmt.Sprintf("iv-%d", time.Now().UTC().UnixNano())
	}
	if intervention.CreatedAt == "" {
		intervention.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	path := filepath.Join(paths.InterventionsDir(), intervention.ID+".json")
	data, err := json.MarshalIndent(intervention, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return nil, err
	}
	return &intervention, nil
}

func parseNodeID(nodeID string) (string, string, error) {
	parts := strings.SplitN(strings.TrimSpace(nodeID), ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid node id %q", nodeID)
	}
	return parts[0], parts[1], nil
}

func resolveRequestForNode(paths *RuntimePaths, nodeType, id string) (*RequestState, error) {
	requests, err := listRequests(paths)
	if err != nil {
		return nil, err
	}
	for _, request := range requests {
		if nodeType == "request" && request.RequestID == id {
			return request, nil
		}
		for _, taskID := range allTaskIDs(request) {
			if nodeType == "task" && taskID == id {
				return request, nil
			}
			if nodeType == "run" {
				runs, runErr := loadRunsForTask(paths, taskID)
				if runErr != nil {
					continue
				}
				for _, run := range runs {
					if run.ID == id {
						return request, nil
					}
				}
			}
		}
	}
	return nil, os.ErrNotExist
}

func handleNodeMessage(paths *RuntimePaths, nodeID string, message string) (*Intervention, error) {
	nodeType, id, err := parseNodeID(nodeID)
	if err != nil {
		return nil, err
	}
	request, err := resolveRequestForNode(paths, nodeType, id)
	if err != nil {
		return nil, err
	}
	if err := ensureCanonicalSessionSeed(paths, request); err != nil {
		return nil, err
	}

	if nodeType == "request" && request.Status == StatusNeedsClarification && len(request.Answers) < len(request.Questions) {
		if _, err := answerRequest(paths, request.RequestID, message); err != nil {
			return nil, err
		}
	} else {
		if err := appendCanonicalEvent(paths, request.RequestID, eventNodeMessageRecorded, nodeType, id, nodeMessagePayload{
			Message: strings.TrimSpace(message),
		}); err != nil {
			return nil, err
		}
	}

	return &Intervention{
		NodeID:   nodeID,
		NodeType: nodeType,
		Action:   "message",
		Message:  strings.TrimSpace(message),
		Status:   "recorded",
		Metadata: map[string]any{
			"request_id": request.RequestID,
		},
	}, nil
}

func handleNodeControl(paths *RuntimePaths, nodeID string, command string) (*Intervention, error) {
	nodeType, id, err := parseNodeID(nodeID)
	if err != nil {
		return nil, err
	}
	request, err := resolveRequestForNode(paths, nodeType, id)
	if err != nil {
		return nil, err
	}
	if err := ensureCanonicalSessionSeed(paths, request); err != nil {
		return nil, err
	}

	command = strings.TrimSpace(command)
	switch command {
	case "retry":
		if err := appendCanonicalEvent(paths, request.RequestID, eventControlApplied, nodeType, id, controlPayload{
			Command: command,
		}); err != nil {
			return nil, err
		}
		_, taskID, err := createTaskFromRequest(paths, request)
		if err != nil {
			return nil, err
		}
		updated, loadErr := loadRequest(paths, request.RequestID)
		if loadErr == nil {
			request = updated
			if strings.TrimSpace(request.TaskID) == "" {
				setCurrentTask(request, taskID)
			}
		}
	case "cancel":
		if err := appendCanonicalEvent(paths, request.RequestID, eventControlApplied, nodeType, id, controlPayload{
			Command: command,
		}); err != nil {
			return nil, err
		}
		if err := appendCanonicalEvent(paths, request.RequestID, eventSessionCancelled, "request", request.RequestID, map[string]string{
			"reason": "cancelled from UI",
		}); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported command %q", command)
	}

	return &Intervention{
		NodeID:   nodeID,
		NodeType: nodeType,
		Action:   "control",
		Command:  command,
		Status:   "applied",
		Metadata: map[string]any{
			"request_id": request.RequestID,
		},
	}, nil
}

func loadRunView(paths *RuntimePaths, runID string) (*RunView, error) {
	activeStatePath := paths.ActiveRunStatePath(runID)
	if state, err := loadRunState(activeStatePath); err == nil {
		view := buildRunViewFromState(paths, state)
		return &view, nil
	}
	summary, _, err := loadRunSummary(paths, runID)
	if err != nil {
		return nil, err
	}
	view := buildRunViewFromSummary(summary)
	return &view, nil
}

func loadRunEvidence(paths *RuntimePaths, runID string) (*runEvidenceResponse, error) {
	runView, err := loadRunView(paths, runID)
	if err != nil {
		return nil, err
	}
	evidencePath := strings.TrimSpace(runView.EvidencePath)
	if evidencePath == "" {
		return &runEvidenceResponse{RunID: runID, EvidencePath: "", Invocations: []map[string]any{}}, nil
	}

	file, err := os.Open(filepath.Join(evidencePath, "invocations.jsonl"))
	if err != nil {
		if os.IsNotExist(err) {
			return &runEvidenceResponse{RunID: runID, EvidencePath: evidencePath, Invocations: []map[string]any{}}, nil
		}
		return nil, err
	}
	defer file.Close()

	invocations := make([]map[string]any, 0)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var item map[string]any
		if err := json.Unmarshal([]byte(line), &item); err != nil {
			continue
		}
		invocations = append(invocations, item)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return &runEvidenceResponse{
		RunID:        runID,
		EvidencePath: evidencePath,
		Invocations:  invocations,
	}, nil
}

func loadRunLogs(paths *RuntimePaths, runID, stream string, last int) (*runLogsResponse, error) {
	runView, err := loadRunView(paths, runID)
	if err != nil {
		return nil, err
	}
	logPath := runView.StdoutPath
	if strings.EqualFold(stream, "stderr") {
		logPath = runView.StderrPath
		stream = "stderr"
	} else {
		stream = "stdout"
	}
	if strings.TrimSpace(logPath) == "" {
		return nil, errors.New("log path unavailable")
	}
	lines, err := readLastLines(logPath, last)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return &runLogsResponse{
		RunID:  runID,
		Stream: stream,
		Path:   logPath,
		Lines:  lines,
	}, nil
}

func (s terminalState) String() string {
	return string(s)
}
