package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"text/template"
	"time"
)

// DynamicCommand defines a command that executes based on a switch condition
type DynamicCommand struct {
	SwitchCmd string            `json:"switch_cmd,omitempty"`
	SwitchVar string            `json:"switch_var,omitempty"`
	Cases     map[string]string `json:"cases"`
	Default   string            `json:"default,omitempty"`
}

// Config defines the structure of our JSON configuration file
type Config struct {
	TemplatePath    string                       `json:"template"`
	OutputPath      string                       `json:"output"`
	Commands        map[string]string            `json:"commands"`
	WeekdayCommands map[string]map[string]string `json:"weekday_commands"`
	DynamicCommands map[string]DynamicCommand    `json:"dynamic_commands"`
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatalf("Error: %v", err)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("motdy", flag.ContinueOnError)

	// Define default values
	defaultConfig := "/etc/motdy/config.json"

	// Check environment variables first
	if envConfig := os.Getenv("MOTDY_CONFIG"); envConfig != "" {
		defaultConfig = envConfig
	}

	// Define CLI flags
	configFile := fs.String("config", defaultConfig, "Path to the configuration file (env MOTDY_CONFIG)")
	templateOverride := fs.String("template", os.Getenv("MOTDY_TEMPLATE"), "Override template path (env MOTDY_TEMPLATE)")
	outputOverride := fs.String("output", os.Getenv("MOTDY_OUTPUT"), "Override output path (env MOTDY_OUTPUT)")

	if err := fs.Parse(args); err != nil {
		return err
	}

	configData, err := os.ReadFile(*configFile)
	if err != nil {
		return fmt.Errorf("failed to read config: %v", err)
	}

	var config Config
	if err := json.Unmarshal(configData, &config); err != nil {
		return fmt.Errorf("failed to parse config: %v", err)
	}

	// Apply overrides
	if *templateOverride != "" {
		config.TemplatePath = *templateOverride
	}
	if *outputOverride != "" {
		config.OutputPath = *outputOverride
	}

	// Check required paths
	if config.TemplatePath == "" {
		return fmt.Errorf("template path is not specified in config, env, or args")
	}
	if config.OutputPath == "" {
		return fmt.Errorf("output path is not specified in config, env, or args")
	}

	// This map will hold the outputs of our commands
	vars := make(map[string]string)

	// Helper function to execute and store commands
	executeCommand := func(name, cmdStr string) {
		cmd := exec.Command("sh", "-c", cmdStr)
		out, err := cmd.Output()
		if err != nil {
			vars[name] = "[Error running command]"
			return
		}
		vars[name] = strings.TrimSpace(string(out))
	}

	// Execute general commands
	for name, cmdStr := range config.Commands {
		executeCommand(name, cmdStr)
	}

	// Execute weekday-specific commands
	currentWeekday := time.Now().Weekday().String()
	if dayCommands, exists := config.WeekdayCommands[currentWeekday]; exists {
		for name, cmdStr := range dayCommands {
			executeCommand(name, cmdStr)
		}
	}

	// Execute dynamic commands
	for name, dynCmd := range config.DynamicCommands {
		switchValue := ""

		// Determine the switch value
		if dynCmd.SwitchCmd != "" {
			cmd := exec.Command("sh", "-c", dynCmd.SwitchCmd)
			if out, err := cmd.Output(); err == nil {
				switchValue = strings.TrimSpace(string(out))
			}
		} else if dynCmd.SwitchVar != "" {
			if val, exists := vars[dynCmd.SwitchVar]; exists {
				switchValue = val
			}
		}

		// Execute based on matched case, or default
		if targetCmd, matched := dynCmd.Cases[switchValue]; matched {
			executeCommand(name, targetCmd)
		} else if dynCmd.Default != "" {
			executeCommand(name, dynCmd.Default)
		}
	}

	// Load the Jinja-like template
	tmpl, err := template.ParseFiles(config.TemplatePath)
	if err != nil {
		return fmt.Errorf("failed to parse template: %v", err)
	}

	// Render the template with our variables
	var rendered bytes.Buffer
	if err := tmpl.Execute(&rendered, vars); err != nil {
		return fmt.Errorf("failed to execute template: %v", err)
	}

	// Write the final output to /etc/motd
	if err := os.WriteFile(config.OutputPath, rendered.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write output MOTD: %v", err)
	}

	return nil
}
