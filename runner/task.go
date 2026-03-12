package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type Task struct {
	TaskID        string
	Title         string
	DependsOn     []string
	Execution     string
	Role          string
	DesignRef     string
	AssignedTo    string
	Input         string
	Output        string
	DoneCondition []string
	QualityGate   []string
	Timeout       string
	RetryCount    int
}

func (t Task) ExpectedOutputPaths() []string {
	if strings.TrimSpace(t.Output) == "" {
		return nil
	}

	parts := strings.Split(t.Output, ",")
	paths := make([]string, 0, len(parts))
	for _, part := range parts {
		path := strings.TrimSpace(part)
		if path != "" {
			paths = append(paths, path)
		}
	}

	return paths
}

func ParseTask(path string) (*Task, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open task file: %w", err)
	}
	defer file.Close()

	task := &Task{
		DependsOn: []string{},
	}

	scanner := bufio.NewScanner(file)
	var currentKey string
	var outputLines []string
	var inputLines []string

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "## ") {
			key, value, hasValue := parseHeaderLine(line)
			currentKey = key

			switch key {
			case "Task ID":
				task.TaskID = value
			case "Title":
				task.Title = value
			case "Depends On":
				task.DependsOn = parseDependsOn(value)
			case "Execution":
				task.Execution = value
			case "Role":
				task.Role = value
			case "Design Ref":
				task.DesignRef = value
			case "Assigned To":
				task.AssignedTo = value
			case "Retry Count", "RetryCount":
				if value != "" {
					retryCount, err := strconv.Atoi(strings.Split(value, " ")[0])
					if err != nil {
						return nil, fmt.Errorf("parse retry count: %w", err)
					}
					task.RetryCount = retryCount
				}
			case "Timeout":
				if value != "" {
					task.Timeout = strings.Split(value, " ")[0]
				}
			case "Output":
				if hasValue && value != "" {
					outputLines = append(outputLines, value)
				}
			}

			continue
		}

		switch currentKey {
		case "Output":
			if trimmed := strings.TrimSpace(line); trimmed != "" {
				outputLines = append(outputLines, trimmed)
			}
		case "Input":
			inputLines = append(inputLines, line)
		case "QualityGate":
			if item := parseListItem(line); item != "" {
				task.QualityGate = append(task.QualityGate, item)
			}
		case "Done Condition":
			if item := parseListItem(line); item != "" {
				task.DoneCondition = append(task.DoneCondition, item)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan task file: %w", err)
	}

	task.Output = parseOutput(outputLines)
	task.Input = strings.TrimSpace(strings.Join(inputLines, "\n"))

	return task, nil
}

func LoadQueue(queueDir string) ([]*Task, error) {
	entries, err := os.ReadDir(queueDir)
	if err != nil {
		return nil, fmt.Errorf("read queue dir: %w", err)
	}

	var mdFiles []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		mdFiles = append(mdFiles, filepath.Join(queueDir, entry.Name()))
	}

	sort.Strings(mdFiles)

	tasks := make([]*Task, 0, len(mdFiles))
	for _, path := range mdFiles {
		task, err := ParseTask(path)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		tasks = append(tasks, task)
	}

	return tasks, nil
}

func parseHeaderLine(line string) (key string, value string, hasValue bool) {
	body := strings.TrimSpace(strings.TrimPrefix(line, "## "))
	parts := strings.SplitN(body, ":", 2)
	if len(parts) == 1 {
		return strings.TrimSpace(parts[0]), "", false
	}

	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), true
}

func parseDependsOn(value string) []string {
	trimmed := strings.TrimSpace(value)
	trimmed = strings.TrimPrefix(trimmed, "[")
	trimmed = strings.TrimSuffix(trimmed, "]")
	trimmed = strings.TrimSpace(trimmed)
	if trimmed == "" {
		return []string{}
	}

	parts := strings.Split(trimmed, ",")
	dependsOn := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item != "" {
			dependsOn = append(dependsOn, item)
		}
	}

	return dependsOn
}

func parseOutput(lines []string) string {
	var paths []string
	for _, line := range lines {
		start := 0
		for {
			s := strings.Index(line[start:], "`")
			if s == -1 {
				break
			}
			s += start
			e := strings.Index(line[s+1:], "`")
			if e == -1 {
				break
			}
			e += s + 1
			path := strings.TrimSpace(line[s+1 : e])
			if path != "" {
				paths = append(paths, path)
			}
			start = e + 1
		}
	}

	if len(paths) == 0 {
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}
			if strings.HasPrefix(trimmed, "- ") {
				trimmed = strings.TrimPrefix(trimmed, "- ")
			}
			parts := strings.Split(trimmed, ",")
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if p != "" {
					paths = append(paths, p)
				}
			}
		}
	}

	return strings.Join(paths, ", ")
}

func parseListItem(line string) string {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "- ") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
}
