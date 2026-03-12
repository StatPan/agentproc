package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ValidateTaskMarker checks if out/result.md exists and contains a PASS/FAIL state.
func ValidateTaskMarker(taskID, runDir string) error {
	resultPath := filepath.Join(runDir, "out", "result.md")
	content, err := os.ReadFile(resultPath)
	if err != nil {
		return fmt.Errorf("failed to read result marker: %w", err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, "PASS") && !strings.Contains(contentStr, "FAIL") {
		return fmt.Errorf("result.md does not contain PASS or FAIL state")
	}

	return nil
}

// ValidateDoneCondition checks if the task's DoneConditions are met.
// For now, it simply ensures the out directory contains expected outputs.
func ValidateDoneCondition(task Task, runDir string) error {
	if len(task.DoneCondition) == 0 {
		return nil // No specific conditions to validate
	}

	outDir := filepath.Join(runDir, "out")
	if _, err := os.Stat(outDir); os.IsNotExist(err) {
		return fmt.Errorf("out directory does not exist for task %s", task.TaskID)
	}

	// This is a basic implementation. In reality, it might parse condition strings
	// or check specific files. For now, we assume if out/ exists, we at least have output.
	return nil
}

// ValidateQualityGate checks if Quality Gate conditions are met.
func ValidateQualityGate(task Task, runDir string) error {
	if len(task.QualityGate) == 0 {
		return nil
	}

	// Example logic: just ensuring the directory structure exists for quality gate outputs
	outDir := filepath.Join(runDir, "out")
	if _, err := os.Stat(outDir); os.IsNotExist(err) {
		return fmt.Errorf("out directory does not exist, quality gate failed")
	}

	return nil
}

// ValidateRetryCount checks if the current retry count exceeds the allowed limit.
func ValidateRetryCount(task Task, retryCount int) error {
	if task.RetryCount <= 0 {
		return nil // 0 means unlimited
	}

	if retryCount >= task.RetryCount {
		return fmt.Errorf("max retry count (%d) reached", task.RetryCount)
	}

	return nil
}

// ValidateThreadWrapperUsage checks if out/thread-evidence/invoke-thread exists and is executable.
func ValidateThreadWrapperUsage(runDir string) error {
	wrapperPath := filepath.Join(runDir, "out", "thread-evidence", "invoke-thread")
	info, err := os.Stat(wrapperPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("thread wrapper %s does not exist", wrapperPath)
		}
		return fmt.Errorf("error accessing thread wrapper: %w", err)
	}

	// Check if executable (for Linux/Unix)
	if info.Mode()&0111 == 0 {
		return fmt.Errorf("thread wrapper %s is not executable", wrapperPath)
	}

	return nil
}
