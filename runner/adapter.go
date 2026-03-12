package main

import (
	"fmt"
	"os"
	"strings"
)

func LoadAdapter(adapterPath string) (string, error) {
	data, err := os.ReadFile(adapterPath)
	if err != nil {
		return "", fmt.Errorf("read adapter: %w", err)
	}

	lines := strings.Split(string(data), "\n")
	inAgentic := false
	inCodeBlock := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if !inAgentic {
			if strings.EqualFold(trimmed, "## agentic") {
				inAgentic = true
			}
			continue
		}

		if strings.HasPrefix(trimmed, "## ") && !inCodeBlock {
			return "", fmt.Errorf("agentic code block not found in %s", adapterPath)
		}

		if !inCodeBlock {
			if strings.HasPrefix(trimmed, "```") {
				inCodeBlock = true
			}
			continue
		}

		if strings.HasPrefix(trimmed, "```") {
			return "", fmt.Errorf("agentic code block is empty in %s", adapterPath)
		}

		return trimmed, nil
	}

	if inAgentic {
		return "", fmt.Errorf("agentic code block not found in %s", adapterPath)
	}

	return "", fmt.Errorf("agentic section not found in %s", adapterPath)
}

func BuildCommand(template, prompt string) string {
	quoted := shellQuote(prompt)
	command := strings.ReplaceAll(template, "\"{prompt}\"", quoted)
	command = strings.ReplaceAll(command, "'{prompt}'", quoted)
	command = strings.ReplaceAll(command, "{prompt}", quoted)
	return command
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
