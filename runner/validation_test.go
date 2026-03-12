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
	agentOSRoot := t.TempDir()
	task := Task{
		TaskID:      "T-001",
		QualityGate: []string{"echo 'Checking quality...'"},
	}

	// Should execute "echo" successfully
	err := ValidateQualityGate(task, agentOSRoot)
	assert.NoError(t, err)

	// Failing command
	taskFail := Task{
		TaskID:      "T-002",
		QualityGate: []string{"exit 1"},
	}
	err = ValidateQualityGate(taskFail, agentOSRoot)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "quality gate command \"exit 1\" failed")
}

func TestValidateQualityGate_Disabled(t *testing.T) {
	agentOSRoot := t.TempDir()
	task := Task{
		TaskID:      "T-001",
		QualityGate: []string{}, // empty QualityGate
	}

	// No error because condition is empty
	err := ValidateQualityGate(task, agentOSRoot)
	assert.NoError(t, err)
}

func TestValidateOutputPaths_AllExpectedPathsExist(t *testing.T) {
	agentOSRoot := t.TempDir()
	runDir := t.TempDir()

	rootFile := filepath.Join(agentOSRoot, "outputs", "result.md")
	assert.NoError(t, os.MkdirAll(filepath.Dir(rootFile), 0o755))
	assert.NoError(t, os.WriteFile(rootFile, []byte("ok"), 0o644))

	runSubdir := filepath.Join(runDir, "reports")
	assert.NoError(t, os.MkdirAll(runSubdir, 0o755))

	task := Task{
		TaskID: "T-OUT-001",
		Output: "outputs/result.md, reports",
	}

	err := ValidateOutputPaths(task, agentOSRoot, runDir)
	assert.NoError(t, err)
}

func TestValidateOutputPaths_MissingPathReturnsExpectedError(t *testing.T) {
	agentOSRoot := t.TempDir()
	runDir := t.TempDir()
	task := Task{
		TaskID: "T-OUT-002",
		Output: "outputs/result.md, reports/missing.txt",
	}

	err := ValidateOutputPaths(task, agentOSRoot, runDir)
	assert.EqualError(t, err, "Output missing: outputs/result.md")
}

func TestValidateOutputPaths_UsesSecondMissingPathAfterFirstExists(t *testing.T) {
	agentOSRoot := t.TempDir()
	runDir := t.TempDir()

	rootFile := filepath.Join(agentOSRoot, "outputs", "result.md")
	assert.NoError(t, os.MkdirAll(filepath.Dir(rootFile), 0o755))
	assert.NoError(t, os.WriteFile(rootFile, []byte("ok"), 0o644))

	task := Task{
		TaskID: "T-OUT-003",
		Output: "outputs/result.md, reports/missing.txt",
	}

	err := ValidateOutputPaths(task, agentOSRoot, runDir)
	assert.EqualError(t, err, "Output missing: reports/missing.txt")
}

func TestValidateOutputPaths_AcceptsPathRelativeToRunDirectory(t *testing.T) {
	agentOSRoot := t.TempDir()
	runDir := t.TempDir()

	runOutput := filepath.Join(runDir, "reports", "result.md")
	assert.NoError(t, os.MkdirAll(filepath.Dir(runOutput), 0o755))
	assert.NoError(t, os.WriteFile(runOutput, []byte("ok"), 0o644))

	task := Task{
		TaskID: "T-OUT-004",
		Output: "reports/result.md",
	}

	err := ValidateOutputPaths(task, agentOSRoot, runDir)
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

func TestValidateOutputPaths_MultipleCommaSeparatedOutputs(t *testing.T) {
	agentOSRoot := t.TempDir()
	runDir := t.TempDir()

	// Create root outputs
	out1 := filepath.Join(agentOSRoot, "outputs/file1.txt")
	assert.NoError(t, os.MkdirAll(filepath.Dir(out1), 0o755))
	assert.NoError(t, os.WriteFile(out1, []byte("data1"), 0o644))

	// Create run directory outputs
	out2 := filepath.Join(runDir, "out/file2.txt")
	assert.NoError(t, os.MkdirAll(filepath.Dir(out2), 0o755))
	assert.NoError(t, os.WriteFile(out2, []byte("data2"), 0o644))

	task := Task{
		TaskID: "T-MULTI-OUT",
		Output: "outputs/file1.txt, out/file2.txt, outputs/missing.txt",
	}

	// 1. One missing -> error
	err := ValidateOutputPaths(task, agentOSRoot, runDir)
	assert.EqualError(t, err, "Output missing: outputs/missing.txt")

	// 2. All present -> success
	out3 := filepath.Join(agentOSRoot, "outputs/missing.txt")
	assert.NoError(t, os.WriteFile(out3, []byte("data3"), 0o644))

	err = ValidateOutputPaths(task, agentOSRoot, runDir)
	assert.NoError(t, err)
}

func TestValidateOutputPaths(t *testing.T) {
	agentOSRoot := t.TempDir()
	runDir := t.TempDir()

	task := Task{
		Output: "path/to/output1.txt, out/output2.txt",
	}

	// 1. Both missing -> error
	err := ValidateOutputPaths(task, agentOSRoot, runDir)
	assert.Error(t, err)
	assert.Equal(t, "Output missing: path/to/output1.txt", err.Error())

	// 2. output1.txt exists in AgentOS root, output2.txt missing -> error
	output1Path := filepath.Join(agentOSRoot, "path/to/output1.txt")
	err = os.MkdirAll(filepath.Dir(output1Path), 0o755)
	assert.NoError(t, err)
	err = os.WriteFile(output1Path, []byte("data"), 0o644)
	assert.NoError(t, err)

	err = ValidateOutputPaths(task, agentOSRoot, runDir)
	assert.Error(t, err)
	assert.Equal(t, "Output missing: out/output2.txt", err.Error())

	// 3. Both exist (one in root, one in runDir) -> success
	output2Path := filepath.Join(runDir, "out/output2.txt")
	err = os.MkdirAll(filepath.Dir(output2Path), 0o755)
	assert.NoError(t, err)
	err = os.WriteFile(output2Path, []byte("data"), 0o644)
	assert.NoError(t, err)

	err = ValidateOutputPaths(task, agentOSRoot, runDir)
	assert.NoError(t, err)
}
