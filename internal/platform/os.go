// Package platform provides OS detection and subprocess execution helpers.
package platform

import (
	"os"
	"runtime"
)

// GOOS values used throughout the codebase.
const (
	OSLinux   = "linux"
	OSDarwin  = "darwin"
	OSWindows = "windows"
)

// IsLinux returns true when running on Linux.
func IsLinux() bool { return runtime.GOOS == OSLinux }

// IsMacOS returns true when running on macOS / Darwin.
func IsMacOS() bool { return runtime.GOOS == OSDarwin }

// IsWindows returns true when running on Windows.
func IsWindows() bool { return runtime.GOOS == OSWindows }

// CurrentOS returns the runtime.GOOS value.
func CurrentOS() string { return runtime.GOOS }

// ── scrcpy flag-prefix note ───────────────────────────────────────────────────
//
// scrcpy uses GNU-style "--flag" syntax on ALL platforms (Linux, macOS, Windows).
// There is NO need for a "if Windows use '/' else use '--'" separator switch.
// scrcpy is a C application with its own arg parser — it ignores OS conventions.
//
// Platform-specific feature gaps (NOT syntax gaps):
//   Linux only:   --v4l2-sink, --v4l2-buffer, --keyboard=uhid, --mouse=uhid,
//                 --gamepad=uhid  (these need the UHID kernel module)
//   macOS only:   --render-driver=metal
//   Windows only: --render-driver=direct3d
//   USB only:     --keyboard=aoa, --mouse=aoa, --gamepad=aoa, --otg
//
// UHIDAvailable returns true when the UHID-based input modes are supported.
// On Linux this is always true (kernel module is present on modern distros).
// On macOS/Windows the UHID kernel module is not available.
func UHIDAvailable() bool { return IsLinux() }

// V4L2Available returns true when V4L2 loopback devices can be used.
func V4L2Available() bool { return IsLinux() }

// MetalAvailable returns true when the Metal render driver is available.
func MetalAvailable() bool { return IsMacOS() }

// Direct3DAvailable returns true when the Direct3D render driver is available.
func Direct3DAvailable() bool { return IsWindows() }

// ── Display / windowing detection ─────────────────────────────────────────────

// HasDisplay returns true when scrcpy can open a window on this machine.
// Windows and macOS use native windowing APIs — no env vars required.
// Linux requires DISPLAY (X11) or WAYLAND_DISPLAY (Wayland) to be set.
func HasDisplay() bool {
	if IsWindows() || IsMacOS() {
		return true
	}
	return os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != ""
}

// DisplayDescription returns a short human-readable string describing the
// active display environment, or an empty string when none is available.
func DisplayDescription() string {
	if IsWindows() {
		return "Win32 (native)"
	}
	if IsMacOS() {
		return "macOS native"
	}
	if d := os.Getenv("WAYLAND_DISPLAY"); d != "" {
		return "Wayland:" + d
	}
	if d := os.Getenv("DISPLAY"); d != "" {
		return "X11:" + d
	}
	return ""
}

// NoDisplayWarning returns a one-line warning for the log when no display
// server is available.  Empty string on Windows/macOS (always have display).
func NoDisplayWarning() string {
	if IsWindows() || IsMacOS() {
		return ""
	}
	return "warn: DISPLAY and WAYLAND_DISPLAY unset — scrcpy cannot open window"
}

// NoDisplayHint returns a one-line actionable hint matching the current OS,
// or an empty string when no hint is needed.
func NoDisplayHint() string {
	switch {
	case IsWindows():
		return ""
	case IsMacOS():
		return ""
	default:
		return "hint: run Screener from a desktop terminal emulator (Kitty, Gnome Terminal…)"
	}
}

// NoDisplayUIWarning returns the in-pane warning line for the right panel,
// or an empty string when no warning is needed.
func NoDisplayUIWarning() string {
	if IsWindows() || IsMacOS() {
		return ""
	}
	return "⚠ No display server (DISPLAY/WAYLAND_DISPLAY unset)"
}

// NoDisplayUIHint returns the in-pane hint line for the right panel,
// or an empty string when no hint is needed.
func NoDisplayUIHint() string {
	switch {
	case IsWindows():
		return ""
	case IsMacOS():
		return ""
	default:
		return "Run from a desktop terminal (Kitty, Gnome Terminal…)"
	}
}

// LaunchEnvDescription returns a short log string describing the display
// environment at launch time.
func LaunchEnvDescription() string {
	if IsWindows() {
		return "display=Win32"
	}
	if IsMacOS() {
		return "display=macOS-native"
	}
	disp := os.Getenv("DISPLAY")
	wayland := os.Getenv("WAYLAND_DISPLAY")
	sessionType := os.Getenv("XDG_SESSION_TYPE")
	return "DISPLAY=" + disp + " WAYLAND_DISPLAY=" + wayland + " XDG_SESSION_TYPE=" + sessionType
}

// UnsupportedOptionIDs returns a set of scrcpy option IDs that cannot be used
// on the current OS. The map value is a human-readable reason.
func UnsupportedOptionIDs() map[string]string {
	out := make(map[string]string)
	if !IsLinux() {
		out["v4l2_sink"] = "V4L2 loopback is Linux-only"
		out["v4l2_buffer"] = "V4L2 loopback is Linux-only"
	}
	if !UHIDAvailable() {
		out["keyboard_uhid"] = "UHID kernel module is Linux-only (use sdk or aoa instead)"
		out["mouse_uhid"] = "UHID kernel module is Linux-only"
		out["gamepad_uhid"] = "UHID kernel module is Linux-only"
	}
	return out
}
