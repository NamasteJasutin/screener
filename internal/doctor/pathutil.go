package doctor

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// EnsureOnPath ensures dir is present in the user's PATH. Returns true if any
// shell rc file (or Windows registry) was modified.
func EnsureOnPath(dir string) (bool, error) {
	if runtime.GOOS == "windows" {
		return ensureOnPathWindows(dir)
	}
	return ensureOnPathUnix(dir)
}

// rcFiles returns shell rc files to update on Unix systems.
func rcFiles() []string {
	home, _ := os.UserHomeDir()
	candidates := []string{
		filepath.Join(home, ".bashrc"),
		filepath.Join(home, ".zshrc"),
		filepath.Join(home, ".profile"),
	}
	fish := filepath.Join(home, ".config", "fish", "config.fish")
	if _, err := os.Stat(fish); err == nil {
		candidates = append(candidates, fish)
	}
	return candidates
}

func ensureOnPathUnix(dir string) (bool, error) {
	exportLine := fmt.Sprintf(`export PATH="%s:$PATH"`, dir)
	fishLine := fmt.Sprintf(`set -gx PATH "%s" $PATH`, dir)

	modified := false
	for _, rc := range rcFiles() {
		isFish := strings.HasSuffix(rc, "config.fish")
		line := exportLine
		if isFish {
			line = fishLine
		}

		// Skip if the dir is already mentioned in this file.
		if fileContains(rc, dir) {
			continue
		}

		f, err := os.OpenFile(rc, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			continue // skip unwritable files silently
		}
		_, werr := fmt.Fprintf(f, "\n# added by screener doctor\n%s\n", line)
		f.Close()
		if werr == nil {
			modified = true
		}
	}
	return modified, nil
}

func ensureOnPathWindows(dir string) (bool, error) {
	// Read current user PATH via `reg query`
	out, err := exec.Command("reg", "query",
		`HKCU\Environment`, "/v", "PATH").CombinedOutput()
	if err != nil {
		// PATH key may not exist yet
	}
	current := ""
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, "PATH") && strings.Contains(line, "REG_") {
			parts := strings.SplitN(line, "    ", 4)
			if len(parts) == 4 {
				current = strings.TrimSpace(parts[3])
			}
		}
	}

	if strings.Contains(strings.ToLower(current), strings.ToLower(dir)) {
		return false, nil
	}

	newPath := dir
	if current != "" {
		newPath = dir + ";" + current
	}
	if err := exec.Command("setx", "PATH", newPath).Run(); err != nil {
		return false, fmt.Errorf("setx PATH: %w", err)
	}
	return true, nil
}

func fileContains(path, substr string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if strings.Contains(sc.Text(), substr) {
			return true
		}
	}
	return false
}
