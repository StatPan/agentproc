package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateTaskMarker_PassCase(t *testing.T) {
	runDir := t.TempDir()
	outDir := filepath.Join(runDir, "out")
	err := os.MkdirAll(outDir, 0o755)
	assert.NoError(t, err)

	resultPath := filepath.Join(outDir, "result.md")
	err = os.WriteFile(resultPath, []byte("# Result\n\n- Status: PASS\n"), 0o644)
	assert.NoError(t, err)

	err = ValidateTaskMarker("T-001", runDir)
	assert.NoError(t, err)
}

func TestValidateTaskMarker_FailCase(t *testing.T) {
	runDir := t.TempDir()

	// 1. result.md does not exist -> error
	err := ValidateTaskMarker("T-001", runDir)
	assert.Error(t, err)

	// 2. result.md exists but invalid state -> error
	outDir := filepath.Join(runDir, "out")
	err = os.MkdirAll(outDir, 0o755)
	assert.NoError(t, err)

	resultPath := filepath.Join(outDir, "result.md")
	err = os.WriteFile(resultPath, []byte("Just some random text"), 0o644)
	assert.NoError(t, err)

	err = ValidateTaskMarker("T-001", runDir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "does not contain PASS or FAIL")
}

func TestValidateDoneCondition_MissingCondition(t *testing.T) {
	runDir := t.TempDir()
	task := Task{TaskID: "T-001"} // Empty DoneCondition

	err := ValidateDoneCondition(task, runDir)
	assert.NoError(t, err)
}

func TestValidateDoneCondition_Satisfied(t *testing.T) {
	runDir := t.TempDir()
	task := Task{
		TaskID:        "T-001",
		DoneCondition: []string{"Must do something"},
	}

	// For our basic implementation, it checks if outDir exists
	// First, missing out/ dir -> error
	err := ValidateDoneCondition(task, runDir)
	assert.Error(t, err)

	// Now with out/ dir -> satisfied (for our current logic)
	outDir := filepath.Join(runDir, "out")
	err = os.MkdirAll(outDir, 0o755)
	assert.NoError(t, err)

	err = ValidateDoneCondition(task, runDir)
	assert.NoError(t, err)
}

func TestValidateQualityGate_Enabled(t *testing.T) {
	runDir := t.TempDir()
	task := Task{
		TaskID:      "T-001",
		QualityGate: []string{"No lint errors"},
	}

	// Missing outDir -> error
	err := ValidateQualityGate(task, runDir)
	assert.Error(t, err)

	// With outDir -> success
	outDir := filepath.Join(runDir, "out")
	err = os.MkdirAll(outDir, 0o755)
	assert.NoError(t, err)

	err = ValidateQualityGate(task, runDir)
	assert.NoError(t, err)
}

func TestValidateQualityGate_Disabled(t *testing.T) {
	runDir := t.TempDir()
	task := Task{
		TaskID:      "T-001",
		QualityGate: []string{}, // disabled means it might be empty or Config.QualityGateEnabled=false
	}

	// No error because condition is empty
	err := ValidateQualityGate(task, runDir)
	assert.NoError(t, err)
}

func TestValidateRetryCount(t *testing.T) {
	// RetryCount = 0 (Unlimited)
	taskUnlimited := Task{RetryCount: 0}
	err := ValidateRetryCount(taskUnlimited, 5)
	assert.NoError(t, err)

	taskLimited := Task{RetryCount: 3}
	// Less than max
	err = ValidateRetryCount(taskLimited, 1)
	assert.NoError(t, err)
	err = ValidateRetryCount(taskLimited, 2)
	assert.NoError(t, err)

	// Equal to or greater than max
	err = ValidateRetryCount(taskLimited, 3)
	assert.Error(t, err)
	err = ValidateRetryCount(taskLimited, 4)
	assert.Error(t, err)
}

func TestValidateThreadWrapperUsage(t *testing.T) {
	runDir := t.TempDir()

	// Missing invoke-thread -> error
	err := ValidateThreadWrapperUsage(runDir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "does not exist")

	evidenceDir := filepath.Join(runDir, "out", "thread-evidence")
	err = os.MkdirAll(evidenceDir, 0o755)
	assert.NoError(t, err)

	wrapperPath := filepath.Join(evidenceDir, "invoke-thread")

	// Not executable -> error
	err = os.WriteFile(wrapperPath, []byte("#!/bin/bash\necho ok"), 0o644)
	assert.NoError(t, err)

	err = ValidateThreadWrapperUsage(runDir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not executable")

	// Executable -> success
	err = os.Chmod(wrapperPath, 0o755)
	assert.NoError(t, err)

	err = ValidateThreadWrapperUsage(runDir)
	assert.NoError(t, err)
}
