package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	AgentOSRoot string       `yaml:"agentos_root"`
	Layers      LayerConfig  `yaml:"layers"`
	Runner      RunnerConfig `yaml:"runner"`
}

type LayerConfig struct {
	Subprocess string `yaml:"subprocess"`
	Thread     string `yaml:"thread"`
}

type RunnerConfig struct {
	MaxConcurrent        int           `yaml:"max_concurrent"`
	PollInterval         time.Duration `yaml:"poll_interval"`
	Mode                 string        `yaml:"mode"`
	HiddenRuntime        bool          `yaml:"hidden_runtime"`
	DefaultRetryCount    int           `yaml:"default_retry_count"`
	QualityGateEnabled   bool          `yaml:"quality_gate_enabled"`
	ThreadFallbackModels []string      `yaml:"thread_fallback_models"`
}

type rawConfig struct {
	AgentOSRoot string          `yaml:"agentos_root"`
	Layers      LayerConfig     `yaml:"layers"`
	Runner      rawRunnerConfig `yaml:"runner"`
}

type rawRunnerConfig struct {
	MaxConcurrent        int      `yaml:"max_concurrent"`
	PollInterval         string   `yaml:"poll_interval"`
	Mode                 string   `yaml:"mode"`
	HiddenRuntime        bool     `yaml:"hidden_runtime"`
	DefaultRetryCount    int      `yaml:"default_retry_count"`
	QualityGateEnabled   bool     `yaml:"quality_gate_enabled"`
	ThreadFallbackModels []string `yaml:"thread_fallback_models"`
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var raw rawConfig
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse config yaml: %w", err)
	}

	pollInterval := 5 * time.Second
	if strings.TrimSpace(raw.Runner.PollInterval) != "" {
		pollInterval, err = time.ParseDuration(raw.Runner.PollInterval)
		if err != nil {
			return nil, fmt.Errorf("parse runner.poll_interval: %w", err)
		}
	}

	maxConcurrent := raw.Runner.MaxConcurrent
	if maxConcurrent == 0 {
		maxConcurrent = runtime.NumCPU()
	}

	cfg := &Config{
		AgentOSRoot: raw.AgentOSRoot,
		Layers:      raw.Layers,
		Runner: RunnerConfig{
			MaxConcurrent:        maxConcurrent,
			PollInterval:         pollInterval,
			Mode:                 raw.Runner.Mode,
			HiddenRuntime:        raw.Runner.HiddenRuntime,
			DefaultRetryCount:    raw.Runner.DefaultRetryCount,
			QualityGateEnabled:   raw.Runner.QualityGateEnabled,
			ThreadFallbackModels: append([]string(nil), raw.Runner.ThreadFallbackModels...),
		},
	}

	return cfg, nil
}

func (cfg *Config) RuntimePaths() *RuntimePaths {
	return NewRuntimePaths(cfg.AgentOSRoot, cfg.Runner.HiddenRuntime)
}

func defaultConfig() *Config {
	return &Config{
		Layers: LayerConfig{},
		Runner: RunnerConfig{
			MaxConcurrent: runtime.NumCPU(),
			PollInterval:  5 * time.Second,
			Mode:          "daemon",
		},
	}
}

func loadProjectConfig(projectRoot, configArg string) (*Config, string, error) {
	candidates, err := resolveConfigCandidates(projectRoot, configArg)
	if err != nil {
		return nil, "", err
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			cfg, err := LoadConfig(candidate)
			if err != nil {
				return nil, "", err
			}
			return cfg, candidate, nil
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, "", fmt.Errorf("stat config: %w", err)
		}
	}

	if isImplicitConfigPath(configArg) {
		return defaultConfig(), "", nil
	}

	return nil, "", fmt.Errorf("read config: %w", os.ErrNotExist)
}

func isImplicitConfigPath(path string) bool {
	cleaned := filepath.Clean(strings.TrimSpace(path))
	return cleaned == "config.yaml" || cleaned == ".aproc.yml"
}

func resolveConfigCandidates(projectRoot, configArg string) ([]string, error) {
	if filepath.IsAbs(configArg) {
		return []string{filepath.Clean(configArg)}, nil
	}

	if isImplicitConfigPath(configArg) {
		return []string{
			filepath.Join(projectRoot, "config.yaml"),
			filepath.Join(projectRoot, "runner", "config.yaml"),
			filepath.Join(projectRoot, ".aproc.yml"),
		}, nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("get working directory: %w", err)
	}

	return []string{
		filepath.Clean(filepath.Join(cwd, configArg)),
		filepath.Clean(filepath.Join(projectRoot, configArg)),
	}, nil
}
