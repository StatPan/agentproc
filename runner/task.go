package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan task file: %w", err)
	}

	task.Output = parseFirstOutputPath(outputLines)
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

func parseFirstOutputPath(lines []string) string {
	for _, line := range lines {
		if start := strings.Index(line, "`"); start >= 0 {
			rest := line[start+1:]
			if end := strings.Index(rest, "`"); end >= 0 {
				return rest[:end]
			}
		}
	}

	return ""
}
