package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadAdapterAndBuildCommand(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	adapterPath := filepath.Join(dir, "gemini.md")
	content := "# gemini\n\n## agentic\n```\ngemini -m gemini-3-flash-preview -y -p \"{prompt}\"\n```\n"

	if err := os.WriteFile(adapterPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write adapter: %v", err)
	}

	template, err := LoadAdapter(adapterPath)
	if err != nil {
		t.Fatalf("LoadAdapter returned error: %v", err)
	}

	if want := `gemini -m gemini-3-flash-preview -y -p "{prompt}"`; template != want {
		t.Fatalf("unexpected template: got %q want %q", template, want)
	}

	command := BuildCommand(template, "read files")
	if !strings.Contains(command, "'read files'") {
		t.Fatalf("prompt was not injected: %q", command)
	}
	if strings.Contains(command, "{prompt}") {
		t.Fatalf("placeholder still present: %q", command)
	}
}

func TestBuildCommandShellQuotesPrompt(t *testing.T) {
	t.Parallel()

	template := `gemini -p "{prompt}"`
	command := BuildCommand(template, `line 1 "quoted" and it's fine`)

	if want := `gemini -p 'line 1 "quoted" and it'"'"'s fine'`; command != want {
		t.Fatalf("unexpected command: got %q want %q", command, want)
	}
}

func TestLoadAdapterReturnsErrorWhenCodeBlockMissing(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	adapterPath := filepath.Join(dir, "broken.md")
	content := "# broken\n\n## agentic\n설명만 있고 코드블록은 없음\n"

	if err := os.WriteFile(adapterPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write adapter: %v", err)
	}

	_, err := LoadAdapter(adapterPath)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "code block not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}
