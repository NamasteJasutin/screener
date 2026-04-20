package doctor

import (
	"os/exec"
	"runtime"
)

// PackageManager describes an OS package manager and how to invoke it.
type PackageManager struct {
	Name      string
	Binary    string
	NeedsSudo bool
	args      func(pkg string) []string
}

// InstallArgs returns the full argument slice to install pkg.
func (pm PackageManager) InstallArgs(pkg string) []string { return pm.args(pkg) }

// DetectPMs returns all available package managers in priority order for the
// current OS. The first entry is the preferred one; fall back through the list.
func DetectPMs() []PackageManager {
	switch runtime.GOOS {
	case "windows":
		return detectWindows()
	case "darwin":
		return detectDarwin()
	default:
		return detectLinux()
	}
}

func available(binary string) bool {
	_, err := exec.LookPath(binary)
	return err == nil
}

func detectWindows() []PackageManager {
	var pms []PackageManager
	if available("winget.exe") {
		pms = append(pms, PackageManager{
			Name:   "winget",
			Binary: "winget.exe",
			args:   func(pkg string) []string { return []string{"install", "--id", pkg, "-e", "--silent"} },
		})
	}
	if available("choco.exe") {
		pms = append(pms, PackageManager{
			Name:   "choco",
			Binary: "choco.exe",
			args:   func(pkg string) []string { return []string{"install", pkg, "-y"} },
		})
	}
	if available("scoop.cmd") || available("scoop") {
		bin := "scoop.cmd"
		if !available("scoop.cmd") {
			bin = "scoop"
		}
		pms = append(pms, PackageManager{
			Name:   "scoop",
			Binary: bin,
			args:   func(pkg string) []string { return []string{"install", pkg} },
		})
	}
	return pms
}

func detectDarwin() []PackageManager {
	var pms []PackageManager
	if available("brew") {
		pms = append(pms, PackageManager{
			Name:   "brew",
			Binary: "brew",
			args:   func(pkg string) []string { return []string{"install", pkg} },
		})
	}
	if available("port") {
		pms = append(pms, PackageManager{
			Name:      "port",
			Binary:    "port",
			NeedsSudo: true,
			args:      func(pkg string) []string { return []string{"install", pkg} },
		})
	}
	return pms
}

func detectLinux() []PackageManager {
	var pms []PackageManager

	// Homebrew on Linux (some power users)
	if available("brew") {
		pms = append(pms, PackageManager{
			Name:   "brew",
			Binary: "brew",
			args:   func(pkg string) []string { return []string{"install", pkg} },
		})
	}
	if available("dnf") {
		pms = append(pms, PackageManager{
			Name:      "dnf",
			Binary:    "dnf",
			NeedsSudo: true,
			args:      func(pkg string) []string { return []string{"install", "-y", pkg} },
		})
	}
	if available("apt") {
		pms = append(pms, PackageManager{
			Name:      "apt",
			Binary:    "apt",
			NeedsSudo: true,
			args:      func(pkg string) []string { return []string{"install", "-y", pkg} },
		})
	}
	if available("pacman") {
		pms = append(pms, PackageManager{
			Name:      "pacman",
			Binary:    "pacman",
			NeedsSudo: true,
			args:      func(pkg string) []string { return []string{"-S", "--noconfirm", pkg} },
		})
	}
	if available("zypper") {
		pms = append(pms, PackageManager{
			Name:      "zypper",
			Binary:    "zypper",
			NeedsSudo: true,
			args:      func(pkg string) []string { return []string{"install", "-y", pkg} },
		})
	}
	if available("apk") {
		pms = append(pms, PackageManager{
			Name:      "apk",
			Binary:    "apk",
			NeedsSudo: true,
			args:      func(pkg string) []string { return []string{"add", pkg} },
		})
	}
	if available("xbps-install") {
		pms = append(pms, PackageManager{
			Name:      "xbps-install",
			Binary:    "xbps-install",
			NeedsSudo: true,
			args:      func(pkg string) []string { return []string{"-y", pkg} },
		})
	}
	return pms
}
