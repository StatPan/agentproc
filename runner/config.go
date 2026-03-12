package main

import (
	"fmt"
	"os"
	"runtime"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	AgentOSRoot string       `yaml:"agentos_root"`
	Layers      LayerConfig  `yaml:"layers"`
	Runner      RunnerConfig `yaml:"runner"`
}

type LayerConfig struct {
	Process    string `yaml:"process"`
	Subprocess string `yaml:"subprocess"`
	Thread     string `yaml:"thread"`
}

type RunnerConfig struct {
	MaxConcurrent      int           `yaml:"max_concurrent"`
	PollInterval       time.Duration `yaml:"poll_interval"`
	Mode               string        `yaml:"mode"`
	DefaultRetryCount  int           `yaml:"default_retry_count"`
	QualityGateEnabled bool          `yaml:"quality_gate_enabled"`
}

type rawConfig struct {
	AgentOSRoot string          `yaml:"agentos_root"`
	Layers      LayerConfig     `yaml:"layers"`
	Runner      rawRunnerConfig `yaml:"runner"`
}

type rawRunnerConfig struct {
	MaxConcurrent      int    `yaml:"max_concurrent"`
	PollInterval       string `yaml:"poll_interval"`
	Mode               string `yaml:"mode"`
	DefaultRetryCount  int    `yaml:"default_retry_count"`
	QualityGateEnabled bool   `yaml:"quality_gate_enabled"`
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

	pollInterval, err := time.ParseDuration(raw.Runner.PollInterval)
	if err != nil {
		return nil, fmt.Errorf("parse runner.poll_interval: %w", err)
	}

	maxConcurrent := raw.Runner.MaxConcurrent
	if maxConcurrent == 0 {
		maxConcurrent = runtime.NumCPU()
	}

	cfg := &Config{
		AgentOSRoot: raw.AgentOSRoot,
		Layers:      raw.Layers,
		Runner: RunnerConfig{
			MaxConcurrent:      maxConcurrent,
			PollInterval:       pollInterval,
			Mode:               raw.Runner.Mode,
			DefaultRetryCount:  raw.Runner.DefaultRetryCount,
			QualityGateEnabled: raw.Runner.QualityGateEnabled,
		},
	}

	return cfg, nil
}
