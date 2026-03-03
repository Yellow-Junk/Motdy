package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
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

// Version variables populated by ldflags during build
var (
	Version   = "v0.2.0"
	BuildTime = "unknown"
	GitCommit = "None / build outside repository"
)

// Config defines the structure of our JSON configuration file
type Config struct {
	TemplatePath    string                       `json:"template"`
	OutputPath      string                       `json:"output"`
	Commands        map[string]string            `json:"commands"`
	WeekdayCommands map[string]map[string]string `json:"weekday_commands"`
	DynamicCommands map[string]DynamicCommand    `json:"dynamic_commands"`
}

// getDefaultConfigPath determines the default configuration file path to use.
// It checks the MOTDY_CONFIG environment variable, then ~/.config/motdy/config.json,
// and finally falls back to /etc/motdy/config.json.
func getDefaultConfigPath() string {
	if envConfig := os.Getenv("MOTDY_CONFIG"); envConfig != "" {
		return envConfig
	}

	if home, err := os.UserHomeDir(); err == nil {
		userConfig := filepath.Join(home, ".config", "motdy", "config.json")
		if _, err := os.Stat(userConfig); err == nil {
			return userConfig
		}
	}

	return "/etc/motdy/config.json"
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatalf("Error: %v", err)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("motdy", flag.ContinueOnError)

	// Define default values
	defaultConfig := getDefaultConfigPath()

	// Define CLI flags
	configFile := fs.String("config", defaultConfig, "Path to the configuration file (env MOTDY_CONFIG)")
	templateOverride := fs.String("template", os.Getenv("MOTDY_TEMPLATE"), "Override template path (env MOTDY_TEMPLATE)")
	outputOverride := fs.String("output", os.Getenv("MOTDY_OUTPUT"), "Override output path (env MOTDY_OUTPUT)")

	// Installation flags
	installFlag := fs.Bool("install", false, "Install motdy, default config, template, and setup cron job")
	installBin := fs.String("install-bin", "~/.local/bin/motdy", "Path to install the binary")
	installConfig := fs.String("install-config", "~/.config/motdy/config.json", "Path to install the default config")
	installTemplate := fs.String("install-template", "~/.config/motdy/template.txt", "Path to install the default template")
	installSchedule := fs.String("schedule", "@hourly", "Cron schedule expression for motdy")
	forceFlag := fs.Bool("force", false, "Force overwrite of existing files during installation")
	versionFlag := fs.Bool("version", false, "Print version information and exit")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *versionFlag {
		fmt.Printf("Motdy Version: %s\n", Version)
		fmt.Printf("Build Time:    %s\n", BuildTime)
		fmt.Printf("Git Commit:    %s\n", GitCommit)
		return nil
	}

	if *installFlag {
		return runInstall(*installBin, *installConfig, *installTemplate, *installSchedule, *forceFlag)
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

// promptOverwrite asks the user if they want to overwrite an existing file
func promptOverwrite(filePath string) bool {
	fmt.Printf("File %s already exists. Overwrite? [y/N]: ", filePath)
	var response string
	if _, err := fmt.Scanln(&response); err != nil {
		return false
	}
	response = strings.ToLower(strings.TrimSpace(response))
	return response == "y" || response == "yes"
}

// checkAndHandleExistingFile returns true if we should proceed with the file, false if we should skip
func checkAndHandleExistingFile(filePath string, force bool) bool {
	if _, err := os.Stat(filePath); err == nil {
		if force {
			return true
		}
		return promptOverwrite(filePath)
	}
	return true // File doesn't exist, proceed
}

// expandPath expands the tilde (~) in paths to the user's home directory
func expandPath(path string) (string, error) {
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("could not get user home directory: %v", err)
		}
		return filepath.Join(home, path[1:]), nil
	}
	return path, nil
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	sourceFileStat, err := os.Stat(src)
	if err != nil {
		return err
	}

	if !sourceFileStat.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", src)
	}

	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = source.Close() }()

	// Create destination directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = destination.Close() }()

	if _, err := io.Copy(destination, source); err != nil {
		return err
	}

	// Preserve permissions, especially executable bit for the binary
	return os.Chmod(dst, sourceFileStat.Mode())
}

// runInstall handles the installation process
func runInstall(binPath, configPath, templatePath, schedule string, force bool) error {
	fmt.Println("Starting motdy installation...")

	// 1. Expand paths
	expandedBin, err := expandPath(binPath)
	if err != nil {
		return err
	}
	expandedConfig, err := expandPath(configPath)
	if err != nil {
		return err
	}
	expandedTemplate, err := expandPath(templatePath)
	if err != nil {
		return err
	}

	// 2. Install binary
	currentExec, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get current executable path: %v", err)
	}

	// Resolve absolute paths to compare them accurately
	absCurrentExec, _ := filepath.Abs(currentExec)
	absExpandedBin, _ := filepath.Abs(expandedBin)

	if absCurrentExec == absExpandedBin {
		fmt.Printf("Binary is already running from the target location (%s). Skipping binary installation.\n", expandedBin)
	} else if checkAndHandleExistingFile(expandedBin, force) {
		fmt.Printf("Installing binary to: %s\n", expandedBin)
		// Specifically handle "text file busy" by removing the old binary first
		if _, err := os.Stat(expandedBin); err == nil {
			if err := os.Remove(expandedBin); err != nil {
				return fmt.Errorf("failed to remove existing binary (could be running): %v", err)
			}
		}
		if err := copyFile(currentExec, expandedBin); err != nil {
			return fmt.Errorf("failed to install binary: %v", err)
		}
	} else {
		fmt.Printf("Skipping binary installation to: %s\n", expandedBin)
	}

	// 3. Create default config
	if checkAndHandleExistingFile(expandedConfig, force) {
		fmt.Printf("Creating default config at: %s\n", expandedConfig)
		if err := os.MkdirAll(filepath.Dir(expandedConfig), 0755); err != nil {
			return fmt.Errorf("failed to create config directory: %v", err)
		}

		defaultConfigContent := fmt.Sprintf(`{
  "template": "%s",
  "output": "~/.motdy.txt",
  "commands": {
    "Hostname": "hostname -f",
    "PrivateIp": "ip -o addr show scope global | awk '$2 != \"lo\" {sub(/\\/.*/, \"\", $4); out=out $2\": \"$4\", \"} END {sub(/, $/, \"\", out); print out}'",
    "PublicIp": "curl api.ipify.org",
    "OSRelease": "cat /etc/os-release | grep '^PRETTY_NAME=' | cut -d'=' -f2 | tr -d '\"'",
    "Kernel": "uname -r",
    "Uptime": "uptime -p | sed 's/up //'",
    "Date": "date '+%%A, %%B %%d, %%Y'",
    "Time": "date '+%%H:%%M:%%S %%Z'",
    "LoadAvg": "awk '{print $1, $2, $3}' /proc/loadavg",
    "MemoryUsage": "free -m | awk 'NR==2{printf \"%%.2f%%%% (%%.2fGB / %%.2fGB)\", $3*100/$2, $3/1024, $2/1024}'",
    "DiskUsage": "df -h / | awk '$NF==\"/\"{printf \"%%s / %%s (%%s)\", $3, $2, $5}'",
    "LoggedInUsers": "who | wc -l",
    "RandomQuote": "curl -s https://api.quotable.io/quotes/random 2>/dev/null | jq -r '.[0].content' || echo 'Stay positive!'"
  },
  "dynamic_commands": {
    "UpdatesAvailable": {
      "switch_cmd": "grep '^ID=' /etc/os-release | cut -d'=' -f2 | tr -d '\"'",
      "cases": {
        "ubuntu": "apt-get -s upgrade 2>/dev/null | grep -P '^\\d+ upgraded' || echo '0'",
        "debian": "apt-get -s upgrade 2>/dev/null | grep -P '^\\d+ upgraded' || echo '0'",
        "fedora": "dnf check-update -q | grep -v '^$' | wc -l || echo '0'"
      },
      "default": "echo 'Unknown'"
    },
    "Containers": {
      "switch_cmd": "if command -v podman >/dev/null 2>&1; then echo podman; elif command -v docker >/dev/null 2>&1; then echo docker; else echo none; fi",
      "cases": {
        "podman": "podman ps -q 2>/dev/null | wc -l",
        "docker": "docker ps -q 2>/dev/null | wc -l"
      },
      "default": "echo 'No container engine running'"
    }
  },
  "weekday_commands": {
    "Monday": {
      "DailyTask": "echo 'Time to review weekly goals!'"
    },
    "Tuesday": {
      "DailyTask": "echo 'Run database backups today.'"
    },
    "Wednesday": {
      "DailyTask": "echo 'Remember to update Neovim plugins!'",
      "UpdateCommand": "echo 'nvim --headless \"+Lazy! sync\" +qa'"
    },
    "Thursday": {
      "DailyTask": "echo 'Check error logs and monitoring dashboards.'"
    },
    "Friday": {
      "DailyTask": "echo 'Merge outstanding pull requests before the weekend!'"
    },
    "Saturday": {
      "DailyTask": "echo 'Enjoy the weekend!'"
    },
    "Sunday": {
      "DailyTask": "echo 'Prepare for the upcoming week.'"
    }
  }
}`, expandedTemplate)

		if err := os.WriteFile(expandedConfig, []byte(defaultConfigContent), 0644); err != nil {
			return fmt.Errorf("failed to write default config: %v", err)
		}
	} else {
		fmt.Printf("Skipping config creation at: %s\n", expandedConfig)
	}

	// 4. Create default template
	if checkAndHandleExistingFile(expandedTemplate, force) {
		fmt.Printf("Creating default template at: %s\n", expandedTemplate)
		if err := os.MkdirAll(filepath.Dir(expandedTemplate), 0755); err != nil {
			return fmt.Errorf("failed to create template directory: %v", err)
		}

		defaultTemplateContent := `===========================================================================
                      Welcome to {{.Hostname}}!
===========================================================================

  Date:          {{.Date}}
  Time:          {{.Time}}
  OS Release:    {{.OSRelease}}
  Kernel:        {{.Kernel}}
  Uptime:        {{.Uptime}}

  -- Network Information --------------------------------------------------
  Private IP:    {{.PrivateIp}}
  Public IP:     {{.PublicIp}}

  -- System Status --------------------------------------------------------
  Load Average:  {{.LoadAvg}}
  Memory Usage:  {{.MemoryUsage}}
  Disk Usage:    {{.DiskUsage}}
  Logged in:     {{.LoggedInUsers}} users

  -- Application Status ---------------------------------------------------
  Running Containers: {{.Containers}}
  Updates:            {{.UpdatesAvailable}}

  -- Daily Focus ----------------------------------------------------------
  {{.DailyTask}}{{if .UpdateCommand}}
  Updates Command: {{.UpdateCommand}}{{end}}

===========================================================================
  "Good luck!"
===========================================================================
`
		if err := os.WriteFile(expandedTemplate, []byte(defaultTemplateContent), 0644); err != nil {
			return fmt.Errorf("failed to write default template: %v", err)
		}
	} else {
		fmt.Printf("Skipping template creation at: %s\n", expandedTemplate)
	}

	// 5. Setup Cron Job
	fmt.Println("Setting up cron job...")
	if err := setupCronJob(expandedBin, expandedConfig, expandedTemplate, schedule); err != nil {
		return fmt.Errorf("failed to setup cron job: %v", err)
	}

	fmt.Println("\nInstallation complete! ✅")
	fmt.Printf("Cron job scheduled as: %s\n", schedule)
	return nil
}

// setupCronJob adds a cron job for motdy
func setupCronJob(binPath, configPath, templatePath, schedule string) error {
	// Construct the command to run in cron
	cronCmd := fmt.Sprintf("%s -config %s -template %s", binPath, configPath, templatePath)
	cronLine := fmt.Sprintf("%s %s", schedule, cronCmd)

	// Get current crontab
	cmd := exec.Command("crontab", "-l")
	out, err := cmd.Output()

	currentCron := ""
	if err == nil {
		currentCron = string(out)
	}

	// Check if motdy is already in crontab
	if strings.Contains(currentCron, binPath) {
		fmt.Println("Cron job already exists for motdy, skipping addition.")
		return nil
	}

	// Append our new cron job
	newCron := currentCron
	if newCron != "" && !strings.HasSuffix(newCron, "\n") {
		newCron += "\n"
	}
	newCron += cronLine + "\n"

	// Install the new crontab
	installCmd := exec.Command("crontab", "-")
	installCmd.Stdin = strings.NewReader(newCron)

	if err := installCmd.Run(); err != nil {
		return fmt.Errorf("failed to install new crontab: %v", err)
	}

	return nil
}
