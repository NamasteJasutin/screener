package doctor

import "runtime"

// Tool describes a required external binary and how to install it per package manager.
type Tool struct {
	Name     string
	Packages map[string]string // pm name → package name
}

// requiredTools returns the list of tools screener needs, with per-PM package names.
func requiredTools() []Tool {
	return []Tool{
		{
			Name: "adb",
			Packages: map[string]string{
				"winget":       "Google.PlatformTools",
				"choco":        "adb",
				"scoop":        "adb",
				"brew":         "android-platform-tools",
				"port":         "android-platform-tools",
				"dnf":          "android-tools",
				"apt":          "adb",
				"pacman":       "android-tools",
				"zypper":       "android-tools",
				"apk":          "android-tools",
				"xbps-install": "android-tools",
			},
		},
		{
			Name: "scrcpy",
			Packages: map[string]string{
				"winget":       "Genymobile.scrcpy",
				"choco":        "scrcpy",
				"scoop":        "scrcpy",
				"brew":         "scrcpy",
				"dnf":          "scrcpy",
				"apt":          "scrcpy",
				"pacman":       "scrcpy",
				"zypper":       "scrcpy",
				"apk":          "scrcpy",
				"xbps-install": "scrcpy",
			},
		},
	}
}

// platformToolsURL returns the Google platform-tools download URL for the current OS.
func platformToolsURL() string {
	switch runtime.GOOS {
	case "windows":
		return "https://dl.google.com/android/repository/platform-tools-latest-windows.zip"
	case "darwin":
		return "https://dl.google.com/android/repository/platform-tools-latest-darwin.zip"
	default:
		return "https://dl.google.com/android/repository/platform-tools-latest-linux.zip"
	}
}
