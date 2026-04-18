package main

import (
	"flag"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"screener/internal/app"
	"screener/internal/persistence"
)

// Injected at build time via -ldflags "-X main.Version=... -X main.CommitHash=... -X main.BuildDate=..."
var (
	Version    = "dev"
	CommitHash = "unknown"
	BuildDate  = ""
)

func versionString() string {
	if BuildDate != "" {
		return fmt.Sprintf("screener %s (%s, %s)", Version, CommitHash, BuildDate)
	}
	return fmt.Sprintf("screener %s (%s)", Version, CommitHash)
}

func main() {
	var (
		showVersion bool
		configPath  string
		logPath     string
		debug       bool
	)

	flag.BoolVar(&showVersion, "version", false, "print version and exit")
	flag.StringVar(&configPath, "config", "", "path to config file (default: ~/.config/screener/config.json)")
	flag.StringVar(&logPath, "log", "", "path to log file (default: ~/.local/state/screener/screener.log)")
	flag.BoolVar(&debug, "debug", false, "enable debug logging to /tmp/screener-debug.log")
	flag.Usage = func() {
		fmt.Fprintf(os.Stdout, `screener — terminal UI for ADB + scrcpy device management

Usage:
  screener [flags]

Flags:
  --version          print version and exit
  --config <path>    config file path (default: ~/.config/screener/config.json)
  --log    <path>    log file path    (default: ~/.local/state/screener/screener.log)
  --debug            write Bubble Tea debug log to /tmp/screener-debug.log

First run:
  Press B for the setup guide (ADB wireless + Tailscale).
  Press P to pair a device, Enter to launch scrcpy.
  Press ? for full key reference.

Config:  %s
Log:     %s
`, persistence.DefaultConfigPath(), persistence.DefaultLogPath())
	}
	flag.Parse()

	if showVersion {
		fmt.Println(versionString())
		os.Exit(0)
	}

	opts := app.ModelOptions{
		ConfigPath: configPath,
		LogPath:    logPath,
		Version:    versionString(),
	}

	model := app.NewModelWithOpts(opts)

	if debug {
		_, _ = tea.LogToFile("/tmp/screener-debug.log", "debug")
	}

	p := tea.NewProgram(model)
	finalModel, err := p.Run()
	// Flush and close the log file regardless of exit path.
	if am, ok := finalModel.(app.Model); ok {
		am.Cleanup()
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "screener failed: %v\n", err)
		os.Exit(1)
	}
}
