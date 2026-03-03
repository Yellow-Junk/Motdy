# Motdy

Motdy is a lightweight, and zero-dependency Go application that generates a dynamic Message of the Day (MOTD) for Linux systems. It allows you to configure shell commands or scripts, execute them, and use their output as variables within a Go text template to generate your final MOTD file.

My main reason for it was to have an easy way to remind myself for certain commands I should run, since I mostly forget to update plugins, packages, etc. So I wanted something that would show me (in my face) commands I should run without me having to remember A) the command itself and B) to update things.

It's nothing special, there are probably easier and handier ways to do this but this suited my needs.

## Features

- **Dynamic Content**: Run any shell command or script and capture its output.
- **Templating Engine**: Uses Go's built-in `text/template` engine to format the final output.
- **Configurable**: Easily map command outputs to template variables via a simple JSON configuration.
- **Lightweight**: Compiled as a statically linked binary with no external dependencies.

## Installation & Compilation

You can compile Motdy from source using Go. It is recommended to build it as a statically linked binary. It also supports compiling with versioning information.

```bash
# Initialize the Go module
go mod init motdy

# Set version variables
VERSION="v0.2.0"
BUILD_TIME=$(date -u +'%Y-%m-%dT%H:%M:%SZ')
GIT_COMMIT=$(git rev-parse HEAD 2>/dev/null || echo "None / build outside repository")

# Build the static binary with versioning
CGO_ENABLED=0 go build -ldflags="-s -w -X main.Version=$VERSION -X main.BuildTime=$BUILD_TIME -X 'main.GitCommit=$GIT_COMMIT'" -o motdy main.go
```

Motdy includes a built-in installation feature that will copy the binary, create default configuration files, and set up a cron job for you. 

```bash
# Run the built-in installer
./motdy --install
```

By default, the installer will:

1. Copy the binary to `~/.local/bin/motdy`
2. Create a default config at `~/.config/motdy/config.json`
3. Create a default template at `~/.config/motdy/template.txt`
4. Add an `@hourly` cron job to your user's crontab

You can customize the installation using command line flags (see the Command Line Arguments section).

Alternatively, you can manually move the `motdy` binary to a suitable location in your PATH, such as `/usr/local/bin/`.

```bash
sudo mv motdy /usr/local/bin/
sudo chmod +x /usr/local/bin/motdy
```

## Configuration

The application searches for its configuration file in the following order:

1. Path specified by the `MOTDY_CONFIG` environment variable (if set).
2. User-specific configuration at `~/.config/motdy/config.json`.
3. System-wide configuration at `/etc/motdy/config.json`.

You will need to create this directory and file. If you used the `--install` flag, a default configuration has already been created at `~/.config/motdy/config.json`.

```bash
mkdir -p ~/.config/motdy
```

### `config.json`

Create `/etc/motdy/config.json` with the following structure:

```json
{
  "template": "/etc/motdy/template.txt",
  "output": "/etc/motd",
  "commands": {
    "Hostname": "hostname -f",
    "Uptime": "uptime -p",
    "Kernel": "uname -r",
    "LoadAvg": "cat /proc/loadavg | awk '{print $1, $2, $3}'"
  },
  "weekday_commands": {
    "Wednesday": {
      "UpdateCommand": "echo 'nvim --headless \"+Lazy! sync\" +qa'"
    }
  }
}
```

- **`template`**: The absolute path to your MOTD Go template file.
- **`output`**: The absolute path where the final MOTD will be written (usually `/etc/motd`).
- **`commands`**: A key-value map where the key is the variable name to be used in the template, and the value is the shell command to execute. Commands are executed using `sh -c`, so pipes and other shell features are supported. These run every time.
- **`weekday_commands`**: (Optional) A map where the key is the day of the week (e.g., `Monday`, `Tuesday`, etc. - must be Title Case) and the value is another key-value map of commands. These commands are *only* executed and available to the template on that specific day.
- **`dynamic_commands`**: (Optional) A map of commands that are executed based on the evaluation of a switch condition. This allows for OS-specific or environment-specific logic.

#### Dynamic Commands Structure

Each entry in `dynamic_commands` takes the following format:

```json
"VariableName": {
  "switch_cmd": "command to determine switch value",
  "switch_var": "existing variable name to use as switch (alternative to switch_cmd)",
  "cases": {
    "expected_value_1": "command to run if matched",
    "expected_value_2": "command to run if matched"
  },
  "default": "command to run if no cases match"
}
```

- Only **one** of `switch_cmd` or `switch_var` should be provided. `switch_cmd` runs a fresh shell command, while `switch_var` reuses the output of a command already executed in the `commands` block.

#### Example: OS-Specific Updates

```json
"UpdatesAvailable": {
  "switch_cmd": "grep '^ID=' /etc/os-release | cut -d'=' -f2 | tr -d '\"'",
  "cases": {
    "ubuntu": "apt-get -s upgrade 2>/dev/null | grep -P '^\\d+ upgraded' || echo '0'",
    "fedora": "dnf check-update -q | grep -v '^$' | wc -l || echo '0'"
  },
  "default": "echo 'Unknown OS'"
}
```

### `template.txt`

Create your template file (e.g., `/etc/motdy/template.txt`). You can use the keys defined in the `commands`, `weekday_commands`, and `dynamic_commands` sections of your config as variables in the template using Go's `{{.VariableName}}` syntax.

You can use Go template's `if` statements to conditionally render sections based on whether a weekday command was executed:

```text
Welcome to {{.Hostname}}!

System Information:
-------------------
OS Kernel:  {{.Kernel}}
Uptime:     {{.Uptime}}
Load Avg:   {{.LoadAvg}}

{{if .UpdateCommand}}
Wednesday Maintenance:
----------------------
Run: {{.UpdateCommand}}
{{end}}
```

## Usage

Once configured, simply run the binary to generate the MOTD:

```bash
sudo motdy
```

### Command Line Arguments and Environment Variables

You can override the default paths using command line arguments or environment variables. This is useful for testing or running multiple instances.

| Flag | Environment Variable | Default | Description |
| --- | --- | --- | --- |
| `-config` | `MOTDY_CONFIG` | Auto-detected | Path to the JSON configuration file. |
| `-template` | `MOTDY_TEMPLATE` | (from config) | Override the template path defined in the config. |
| `-output` | `MOTDY_OUTPUT` | (from config) | Override the output path defined in the config. |
| `-install` | - | `false` | Install motdy, default config, template, and setup cron job. |
| `-install-bin` | - | `~/.local/bin/motdy` | Path to install the binary. |
| `-install-config` | - | `~/.config/motdy/config.json` | Path to install the default config. |
| `-install-template` | - | `~/.config/motdy/template.txt` | Path to install the default template. |
| `-schedule` | - | `@hourly` | Cron schedule expression for motdy. |
| `-force` | - | `false` | Force overwrite of existing files during installation. |
| `-version` | - | `false` | Print version information and exit. |

**Example using flags:**

```bash
motdy -config ./test-config.json -template ./test-template.txt -output ./test-motd
```

**Example using environment variables:**

```bash
MOTDY_CONFIG=./test-config.json motdy
```

### Automation

To keep your MOTD updated, you can run `motdy` periodically using cron or a systemd timer.

**Cron Example (runs every 5 minutes):**

```bash
*/5 * * * * /usr/local/bin/motdy
```

## Error Handling

If a command fails to execute, its output variable in the template will be gracefully set to `[Error running command]`, preventing the entire MOTD generation from failing.

## License

[MIT License](LICENSE)

---
