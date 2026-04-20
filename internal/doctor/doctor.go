package doctor

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/NamasteJasutin/screener/internal/persistence"
)

// DoctorOptions controls the Run behaviour.
type DoctorOptions struct {
	ConfigPath string
	BinDir     string    // where to install downloaded binaries; default: ~/.screener/bin
	DryRun     bool
	Out        io.Writer // nil = os.Stdout
}

// Run performs the full doctor check: tools, config, PATH.
func Run(opts DoctorOptions) error {
	if opts.Out == nil {
		opts.Out = os.Stdout
	}
	if opts.ConfigPath == "" {
		opts.ConfigPath = persistence.DefaultConfigPath()
	}
	if opts.BinDir == "" {
		opts.BinDir = defaultBinDir()
	}

	fmt.Fprintln(opts.Out, "screener doctor — system check")
	fmt.Fprintln(opts.Out, "")

	pms := DetectPMs()
	tools := requiredTools()

	allOK := true
	for _, tool := range tools {
		st := CheckTool(tool.Name)
		if st.Found {
			fmt.Fprintf(opts.Out, "  ✓  %-10s %s  (%s)\n", tool.Name, st.Version, st.Path)
			continue
		}

		allOK = false
		fmt.Fprintf(opts.Out, "  ✗  %-10s not found in PATH\n", tool.Name)

		if opts.DryRun {
			fmt.Fprintf(opts.Out, "       (dry-run: skipping install)\n")
			printManualHint(tool.Name, opts.Out)
			continue
		}

		if tryInstall(tool, pms, opts.BinDir, opts.Out) {
			allOK = true
		} else {
			printManualHint(tool.Name, opts.Out)
		}
	}

	// ── Config ────────────────────────────────────────────────────────────────
	fmt.Fprintln(opts.Out, "")
	cs := CheckAndRepairConfig(opts.ConfigPath, opts.DryRun)
	switch {
	case cs.Fatal != "":
		allOK = false
		fmt.Fprintf(opts.Out, "  ✗  config     %s\n", cs.Fatal)
	case len(cs.Repairs) > 0:
		fmt.Fprintf(opts.Out, "  ~  config     repaired  (%s)\n", opts.ConfigPath)
		for _, r := range cs.Repairs {
			fmt.Fprintf(opts.Out, "       • %s\n", r)
		}
	default:
		fmt.Fprintf(opts.Out, "  ✓  config     ok  (%s)\n", opts.ConfigPath)
	}

	fmt.Fprintln(opts.Out, "")
	if allOK {
		fmt.Fprintln(opts.Out, "All checks passed. screener is ready.")
	} else {
		fmt.Fprintln(opts.Out, "Some issues could not be resolved automatically. See hints above.")
	}
	return nil
}

// tryInstall tries package managers then falls back to a direct download.
// Returns true when the binary is usable afterward.
func tryInstall(tool Tool, pms []PackageManager, binDir string, out io.Writer) bool {
	for _, pm := range pms {
		if _, ok := tool.Packages[pm.Name]; !ok {
			continue
		}
		fmt.Fprintf(out, "       installing via %s...\n", pm.Name)
		if err := InstallViaPM(pm, tool, out); err == nil {
			if st := CheckTool(tool.Name); st.Found {
				fmt.Fprintf(out, "       ✓ installed %s %s\n", tool.Name, st.Version)
				return true
			}
		}
	}

	// No package manager worked — try direct download.
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		fmt.Fprintf(out, "       ✗ cannot create bin dir %s: %v\n", binDir, err)
		return false
	}

	fmt.Fprintf(out, "       downloading %s → %s ...\n", tool.Name, binDir)
	var dlErr error
	switch tool.Name {
	case "adb":
		dlErr = DownloadADB(binDir)
	case "scrcpy":
		dlErr = DownloadScrcpy(binDir)
	default:
		fmt.Fprintf(out, "       no direct download available for %s\n", tool.Name)
		return false
	}
	if dlErr != nil {
		fmt.Fprintf(out, "       ✗ download failed: %v\n", dlErr)
		return false
	}

	// Make sure binDir is on PATH so the tool is found in future sessions.
	if modified, err := EnsureOnPath(binDir); err != nil {
		fmt.Fprintf(out, "       warn: could not update PATH: %v\n", err)
	} else if modified {
		fmt.Fprintf(out, "       added %s to PATH — restart your shell for it to take effect\n", binDir)
	}

	// Confirm the binary landed where expected.
	name := tool.Name
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if _, err := os.Stat(filepath.Join(binDir, name)); err == nil {
		fmt.Fprintf(out, "       ✓ downloaded %s to %s\n", tool.Name, binDir)
		return true
	}
	return false
}

func printManualHint(toolName string, out io.Writer) {
	switch toolName {
	case "adb":
		switch runtime.GOOS {
		case "windows":
			fmt.Fprintln(out, "       → winget install Google.PlatformTools")
			fmt.Fprintln(out, "         or https://developer.android.com/tools/releases/platform-tools")
		case "darwin":
			fmt.Fprintln(out, "       → brew install android-platform-tools")
		default:
			if available("dnf") {
				fmt.Fprintln(out, "       → sudo dnf install android-tools")
			} else if available("apt") {
				fmt.Fprintln(out, "       → sudo apt install adb")
			} else if available("pacman") {
				fmt.Fprintln(out, "       → sudo pacman -S android-tools")
			} else {
				fmt.Fprintln(out, "       → install android-sdk-platform-tools from your package manager")
			}
		}
	case "scrcpy":
		switch runtime.GOOS {
		case "windows":
			fmt.Fprintln(out, "       → winget install Genymobile.scrcpy")
			fmt.Fprintln(out, "         or https://github.com/Genymobile/scrcpy/releases")
		case "darwin":
			fmt.Fprintln(out, "       → brew install scrcpy")
		default:
			if available("dnf") {
				fmt.Fprintln(out, "       → sudo dnf install scrcpy")
			} else if available("apt") {
				fmt.Fprintln(out, "       → sudo apt install scrcpy")
			} else if available("pacman") {
				fmt.Fprintln(out, "       → sudo pacman -S scrcpy")
			} else {
				fmt.Fprintln(out, "       → https://github.com/Genymobile/scrcpy/releases")
			}
		}
	}
}

func defaultBinDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".screener", "bin")
}
