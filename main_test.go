package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun_Success(t *testing.T) {
	// Create a temporary directory for our test files
	tempDir := t.TempDir()

	// 1. Create a dummy template file
	templatePath := filepath.Join(tempDir, "motd.tmpl")
	templateContent := "Hello {{.User}}! Today is a good day."
	if err := os.WriteFile(templatePath, []byte(templateContent), 0644); err != nil {
		t.Fatalf("Failed to write template: %v", err)
	}

	// 2. Create a dummy output file path
	outputPath := filepath.Join(tempDir, "motd.out")

	// 3. Create a dummy config file
	configPath := filepath.Join(tempDir, "config.json")
	configContent := `{
		"template": "` + strings.ReplaceAll(templatePath, "\\", "\\\\") + `",
		"output": "` + strings.ReplaceAll(outputPath, "\\", "\\\\") + `",
		"commands": {
			"User": "echo 'TestUser'"
		}
	}`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	// Run the application
	args := []string{"-config", configPath}
	if err := run(args); err != nil {
		t.Fatalf("run() failed unexpectedly: %v", err)
	}

	// Check the output
	outBytes, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	outStr := string(outBytes)
	expected := "Hello TestUser! Today is a good day."
	if outStr != expected {
		t.Errorf("Expected output %q, got %q", expected, outStr)
	}
}

func TestRun_MissingConfig(t *testing.T) {
	args := []string{"-config", "/nonexistent/path/to/config.json"}
	err := run(args)
	if err == nil {
		t.Errorf("Expected an error for missing config, got nil")
	}
}

func TestRun_MissingTemplate(t *testing.T) {
	tempDir := t.TempDir()
	outputPath := filepath.Join(tempDir, "motd.out")
	configPath := filepath.Join(tempDir, "config.json")

	configContent := `{
		"output": "` + strings.ReplaceAll(outputPath, "\\", "\\\\") + `"
	}`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	args := []string{"-config", configPath}
	err := run(args)
	if err == nil {
		t.Errorf("Expected an error for missing template, got nil")
	}
}
