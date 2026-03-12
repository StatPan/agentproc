package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type IntakeSession struct {
	ID                string   `json:"id"`
	Request           string   `json:"request"`
	Questions         []string `json:"questions"`
	Answers           []string `json:"answers"`
	Status            string   `json:"status"`
	CreatedAt         string   `json:"created_at"`
	TaskID            string   `json:"task_id,omitempty"`
	GeneratedTaskPath string   `json:"generated_task_path,omitempty"`
}

func runIntakeCommand(args []string) error {
	fs := flag.NewFlagSet("intake", flag.ContinueOnError)
	rootFlag := fs.String("root", ".", "AgentOS root path")
	configFlag := fs.String("config", "./config.yaml", "runner config path")
	if err := fs.Parse(args); err != nil {
		return err
	}

	request := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if request == "" {
		return errors.New("usage: aproc intake [--root PATH] [--config PATH] \"request\"")
	}

	_, paths, err := loadRuntimeConfig(*rootFlag, *configFlag)
	if err != nil {
		return err
	}

	intakeDir := paths.IntakeSessionsDir()
	if err := os.MkdirAll(intakeDir, 0o755); err != nil {
		return fmt.Errorf("mkdir intake dir: %w", err)
	}

	session := &IntakeSession{
		ID:        newIntakeSessionID(),
		Request:   request,
		Questions: deriveClarifyingQuestions(request),
		Status:    "needs_input",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}

	if len(session.Questions) == 0 {
		taskPath, taskID, err := createTaskFromIntake(paths, request, nil)
		if err != nil {
			return err
		}
		session.Status = "ready"
		session.TaskID = taskID
		session.GeneratedTaskPath = taskPath
		if err := saveIntakeSession(intakeDir, session); err != nil {
			return err
		}
		fmt.Printf("READY %s\n", session.ID)
		fmt.Printf("TASK %s\n", taskID)
		fmt.Printf("PATH %s\n", taskPath)
		return nil
	}

	if err := saveIntakeSession(intakeDir, session); err != nil {
		return err
	}

	fmt.Printf("NEEDS_INPUT %s\n", session.ID)
	for i, question := range session.Questions {
		fmt.Printf("Q%d %s\n", i+1, question)
	}
	return nil
}

func runReplyCommand(args []string) error {
	fs := flag.NewFlagSet("reply", flag.ContinueOnError)
	rootFlag := fs.String("root", ".", "AgentOS root path")
	configFlag := fs.String("config", "./config.yaml", "runner config path")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if len(fs.Args()) < 2 {
		return errors.New("usage: aproc reply [--root PATH] [--config PATH] <session-id> \"answer\"")
	}

	sessionID := strings.TrimSpace(fs.Arg(0))
	answer := strings.TrimSpace(strings.Join(fs.Args()[1:], " "))
	if sessionID == "" || answer == "" {
		return errors.New("reply requires session id and answer")
	}

	_, paths, err := loadRuntimeConfig(*rootFlag, *configFlag)
	if err != nil {
		return err
	}

	intakeDir := paths.IntakeSessionsDir()
	session, sessionPath, err := loadIntakeSession(intakeDir, sessionID)
	if err != nil {
		return err
	}

	if session.Status == "ready" {
		fmt.Printf("READY %s\n", session.ID)
		fmt.Printf("TASK %s\n", session.TaskID)
		fmt.Printf("PATH %s\n", session.GeneratedTaskPath)
		return nil
	}

	session.Answers = append(session.Answers, answer)
	if len(session.Answers) < len(session.Questions) {
		if err := saveIntakeSessionFile(sessionPath, session); err != nil {
			return err
		}
		fmt.Printf("NEEDS_INPUT %s\n", session.ID)
		fmt.Printf("Q%d %s\n", len(session.Answers)+1, session.Questions[len(session.Answers)])
		return nil
	}

	taskPath, taskID, err := createTaskFromIntake(paths, session.Request, session.Answers)
	if err != nil {
		return err
	}

	session.Status = "ready"
	session.TaskID = taskID
	session.GeneratedTaskPath = taskPath
	if err := saveIntakeSessionFile(sessionPath, session); err != nil {
		return err
	}

	fmt.Printf("READY %s\n", session.ID)
	fmt.Printf("TASK %s\n", taskID)
	fmt.Printf("PATH %s\n", taskPath)
	return nil
}

func deriveClarifyingQuestions(request string) []string {
	trimmed := strings.TrimSpace(strings.ToLower(request))
	questions := make([]string, 0, 3)

	pathLike := regexp.MustCompile("([a-zA-Z0-9_./-]+\\.[a-zA-Z0-9]+|/[a-zA-Z0-9_./-]+|`[^`]+`)")
	if !pathLike.MatchString(request) {
		questions = append(questions, "어느 파일, 디렉터리, 또는 컴포넌트를 기준으로 작업할까요?")
	}

	if !containsAny(trimmed, []string{"수정", "구현", "작성", "점검", "검토", "분석", "fix", "implement", "write", "review", "audit"}) {
		questions = append(questions, "원하는 작업 형태를 알려주세요. 예: 점검만, 수정 포함, 문서 작성")
	}

	if !containsAny(trimmed, []string{"테스트", "검증", "확인", "test", "verify", "validation"}) {
		questions = append(questions, "검증 범위를 알려주세요. 예: 테스트 포함, 코드 변경 없이 분석만")
	}

	if len(questions) > 3 {
		return questions[:3]
	}
	return questions
}

func containsAny(s string, candidates []string) bool {
	for _, candidate := range candidates {
		if strings.Contains(s, candidate) {
			return true
		}
	}
	return false
}

func newIntakeSessionID() string {
	return "I-" + time.Now().UTC().Format("20060102-150405.000000000")
}

func nextAutoTaskID(queueDir string) (string, error) {
	entries, err := os.ReadDir(queueDir)
	if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("read queue dir: %w", err)
	}

	maxID := 0
	re := regexp.MustCompile(`^T-AUTO-(\d+)\.md$`)
	for _, entry := range entries {
		matches := re.FindStringSubmatch(entry.Name())
		if len(matches) != 2 {
			continue
		}
		var n int
		fmt.Sscanf(matches[1], "%d", &n)
		if n > maxID {
			maxID = n
		}
	}

	return fmt.Sprintf("T-AUTO-%03d", maxID+1), nil
}

func createTaskFromIntake(paths *RuntimePaths, request string, answers []string) (string, string, error) {
	queueDir := paths.QueueDir()
	if err := os.MkdirAll(queueDir, 0o755); err != nil {
		return "", "", fmt.Errorf("mkdir queue dir: %w", err)
	}

	taskID, err := nextAutoTaskID(queueDir)
	if err != nil {
		return "", "", err
	}

	title := request
	if len([]rune(title)) > 60 {
		title = string([]rune(title)[:60])
	}

	input := strings.TrimSpace(request)
	if len(answers) > 0 {
		input += "\n\n## Intake Answers\n"
		for i, answer := range answers {
			input += fmt.Sprintf("%d. %s\n", i+1, answer)
		}
	}

	taskBody := fmt.Sprintf(`## Task ID: %s

## Title: %s

## Depends On: []

## Execution: parallel

## Role: designer

## Design Ref: none

## Assigned To: %s

## Input

%s

## Output

`+"`outputs/result-%s.md`"+`

## Done Condition

- [ ] 요청이 구조화된 태스크 입력으로 정리됨
- [ ] 결과 마커가 outputs/ 하위에 기록됨

## Quality Gate

- [ ] 컨텍스트 누락 없이 실행 가능한 수준으로 작성됨

## Timeout: 30m

## Retry Count: 0
`, taskID, sanitizeSingleLine(title), defaultAssignedTo(), input, taskID)

	taskPath := paths.QueueTaskPath(taskID)
	if err := os.WriteFile(taskPath, []byte(taskBody), 0o644); err != nil {
		return "", "", fmt.Errorf("write task file: %w", err)
	}

	return taskPath, taskID, nil
}

func sanitizeSingleLine(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.TrimSpace(s)
}

func defaultAssignedTo() string {
	return "codex"
}

func saveIntakeSession(dir string, session *IntakeSession) error {
	return saveIntakeSessionFile(filepath.Join(dir, session.ID+".json"), session)
}

func saveIntakeSessionFile(path string, session *IntakeSession) error {
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal intake session: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write intake session: %w", err)
	}
	return nil
}

func loadIntakeSession(dir, sessionID string) (*IntakeSession, string, error) {
	path := filepath.Join(dir, sessionID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("read intake session: %w", err)
	}

	var session IntakeSession
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, "", fmt.Errorf("parse intake session: %w", err)
	}
	return &session, path, nil
}
