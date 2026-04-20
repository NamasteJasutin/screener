package doctor

import (
	"os/exec"
	"regexp"
	"strings"
)

// ToolStatus is the result of probing a single required binary.
type ToolStatus struct {
	Name      string
	Found     bool
	Path      string
	Version   string
	Installed bool   // set to true if doctor installed it during this run
	ErrMsg    string // non-empty when Found=false or probe failed
}

var versionRe = regexp.MustCompile(`\d+[\.\d]*`)

// CheckTool probes for a binary by name and attempts to read its version.
func CheckTool(name string) ToolStatus {
	path, err := exec.LookPath(name)
	if err != nil {
		return ToolStatus{Name: name, Found: false, ErrMsg: "not found in PATH"}
	}
	version := probeVersion(name, path)
	return ToolStatus{Name: name, Found: true, Path: path, Version: version}
}

// probeVersion runs a version flag and returns the first version-like string found.
func probeVersion(name, path string) string {
	flags := versionFlags(name)
	for _, flag := range flags {
		out, err := exec.Command(path, flag).CombinedOutput()
		if err != nil && len(out) == 0 {
			continue
		}
		lines := strings.SplitN(strings.TrimSpace(string(out)), "\n", 3)
		for _, line := range lines {
			if m := versionRe.FindString(line); m != "" {
				return m
			}
		}
	}
	return "unknown"
}

func versionFlags(name string) []string {
	switch name {
	case "adb":
		return []string{"version"}
	case "scrcpy":
		return []string{"--version", "-v"}
	default:
		return []string{"--version", "-version", "version"}
	}
}
