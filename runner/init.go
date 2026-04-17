package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func runInitCommand(args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	forceFlag := fs.Bool("force", false, "force reinstall even if already initialized")
	if err := fs.Parse(args); err != nil {
		return err
	}

	return initAssets(*forceFlag)
}

func initAssets(force bool) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home directory: %w", err)
	}

	aprocDir := filepath.Join(homeDir, ".aproc")
	if err := os.MkdirAll(aprocDir, 0o755); err != nil {
		return fmt.Errorf("create .aproc dir: %w", err)
	}

	requiredDirs := []string{
		filepath.Join(aprocDir, "config"),
		filepath.Join(aprocDir, "assets", "roles"),
		filepath.Join(aprocDir, "assets", "adapters"),
		filepath.Join(aprocDir, "projects"),
		filepath.Join(aprocDir, "logs", "cli"),
		filepath.Join(aprocDir, "logs", "maintenance"),
		filepath.Join(aprocDir, "cache", "downloads"),
		filepath.Join(aprocDir, "cache", "model-responses"),
		filepath.Join(aprocDir, "cache", "content-addressed"),
		filepath.Join(aprocDir, "tmp", "sessions"),
		filepath.Join(aprocDir, "gc"),
	}

	for _, dir := range requiredDirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create directory %s: %w", dir, err)
		}
	}

	if err := copyAssets(aprocDir, force); err != nil {
		return err
	}

	fmt.Printf("aproc initialized successfully at %s\n", aprocDir)
	return nil
}

func copyAssets(aprocDir string, force bool) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	agentosRoot := findAgentOSRoot(cwd)
	if agentosRoot == "" {
		return fmt.Errorf("could not find AgentOS root directory")
	}

	assetsDir := filepath.Join(aprocDir, "assets")
	rolesDir := filepath.Join(assetsDir, "roles")
	adaptersDir := filepath.Join(assetsDir, "adapters")

	sourceRolesDir := filepath.Join(agentosRoot, "roles")
	sourceAdaptersDir := filepath.Join(agentosRoot, "adapters")

	if err := copyDirContents(sourceRolesDir, rolesDir, force); err != nil {
		return fmt.Errorf("copy roles: %w", err)
	}

	if err := os.WriteFile(filepath.Join(assetsDir, "AGENTS.md"), []byte("placeholder"), 0o644); err != nil {
		return fmt.Errorf("copy AGENTS.md: %w", err)
	}

	if err := copyDirContents(sourceAdaptersDir, adaptersDir, force); err != nil {
		return fmt.Errorf("copy adapters: %w", err)
	}

	fmt.Printf("Assets copied from %s to %s\n", agentosRoot, assetsDir)
	return nil
}

func findAgentOSRoot(start string) string {
	current := start
	for {
		if hasMarker(current) {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return start
}

func hasMarker(dir string) bool {
	for _, marker := range []string{".git", "AGENTS.md"} {
		if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
			return true
		}
	}
	return false
}

func copyDirContents(src, dst string, force bool) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		_, err := os.Stat(dstPath)
		if err == nil && !force {
			continue
		}
		if err != nil && !os.IsNotExist(err) {
			return err
		}

		data, err := os.ReadFile(srcPath)
		if err != nil {
			return err
		}

		if err := os.WriteFile(dstPath, data, 0o644); err != nil {
			return err
		}
	}

	return nil
}
