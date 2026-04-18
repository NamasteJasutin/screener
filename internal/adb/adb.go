package adb

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/NamasteJasutin/screener/internal/core"
	"github.com/NamasteJasutin/screener/internal/platform"
)

type ParsedDevice struct {
	Serial string
	State  string
	Attrs  map[string]string
}

func ParseDevicesLong(output string) []ParsedDevice {
	lines := strings.Split(output, "\n")
	devices := make([]ParsedDevice, 0)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "List of devices") || strings.HasPrefix(line, "*") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 1 {
			continue
		}
		d := ParsedDevice{Serial: fields[0], Attrs: map[string]string{}}
		for i := 1; i < len(fields); i++ {
			token := fields[i]
			if d.State == "" && !strings.ContainsAny(token, ":=") {
				d.State = token
				continue
			}
			if key, value, ok := parseAttrToken(token); ok {
				d.Attrs[key] = value
			}
		}
		if d.State == "" {
			d.State = "unknown"
		}
		devices = append(devices, d)
	}
	return devices
}

func parseAttrToken(token string) (string, string, bool) {
	parts := strings.SplitN(token, ":", 2)
	if len(parts) == 2 {
		return parts[0], parts[1], true
	}
	parts = strings.SplitN(token, "=", 2)
	if len(parts) == 2 {
		return parts[0], parts[1], true
	}
	return "", "", false
}

// ── Failure classification ────────────────────────────────────────────────────

type FailureReason string

const (
	FailureNone           FailureReason = "none"
	FailureCanceled       FailureReason = "canceled"
	FailureADBMissing     FailureReason = "adb_missing"
	FailureNoDevice       FailureReason = "no_device"
	FailureUnauthorized   FailureReason = "unauthorized"
	FailureAuthMismatch   FailureReason = "auth_mismatch" // stale cert / fingerprint changed
	FailureOffline        FailureReason = "offline"
	FailureRefused        FailureReason = "refused"
	FailureTimeout        FailureReason = "timeout"
	FailureNoRoute        FailureReason = "no_route"
	FailureStaleEndpoint  FailureReason = "stale_endpoint"
	FailureUnsupported    FailureReason = "unsupported_feature"
	FailureInvalid        FailureReason = "invalid_profile"
	FailureTerminalRender FailureReason = "terminal_rendering_failure"
	FailureNoDisplay      FailureReason = "no_display_server"
	FailureDeviceNotFound  FailureReason = "device_not_found"   // serial specified but not attached
	FailureMultipleDevices FailureReason = "multiple_devices"   // ambiguous — need -s flag
	FailureProtocol        FailureReason = "protocol_fault"     // ADB wire protocol error
	FailurePermissions     FailureReason = "insufficient_permissions" // Linux udev / root needed
	FailureVersionMismatch FailureReason = "version_mismatch"   // adb client/server version skew
	FailureConnectionReset FailureReason = "connection_reset"   // TCP RST mid-session
	FailureUnknown        FailureReason = "unknown"
)

func ClassifyFailure(err error) FailureReason {
	if err == nil {
		return FailureNone
	}
	if errors.Is(err, context.Canceled) {
		return FailureCanceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return FailureTimeout
	}
	msg := strings.ToLower(err.Error())
	switch {
	// Match only established cancellation idioms — NOT bare substrings — to avoid
	// misclassifying unrelated messages that happen to contain "canceled" (e.g.
	// "reconnect after task canceled by server").
	case strings.Contains(msg, "context canceled"),
		strings.Contains(msg, "context cancelled"),
		strings.Contains(msg, "operation canceled"),
		strings.Contains(msg, "operation cancelled"),
		strings.Contains(msg, "canceled=true"):
		return FailureCanceled
	case (strings.Contains(msg, "executable file not found") || strings.Contains(msg, "command not found")) &&
		(strings.Contains(msg, "adb") || strings.Contains(msg, "scrcpy")):
		return FailureADBMissing
	case strings.Contains(msg, "context deadline exceeded"):
		return FailureTimeout
	case strings.Contains(msg, "no devices/emulators found"):
		return FailureNoDevice
	// Auth mismatch: IP changed (e.g. local→Tailscale), device was re-imaged, or
	// ADB keys were wiped. Must be checked before the generic "unauthorized" case.
	case strings.Contains(msg, "failed to authenticate"),
		strings.Contains(msg, "authentication failed"),
		strings.Contains(msg, "failed to pair") && strings.Contains(msg, "authenticate"):
		return FailureAuthMismatch
	case strings.Contains(msg, "unauthorized"):
		return FailureUnauthorized
	case strings.Contains(msg, "offline"):
		return FailureOffline
	case strings.Contains(msg, "connection refused"), strings.Contains(msg, "refused"):
		return FailureRefused
	case strings.Contains(msg, "timeout"), strings.Contains(msg, "timed out"):
		return FailureTimeout
	case strings.Contains(msg, "no route to host"):
		return FailureNoRoute
	case strings.Contains(msg, "stale endpoint"):
		return FailureStaleEndpoint
	case strings.Contains(msg, "no available video device"),
		strings.Contains(msg, "could not initialize sdl"),
		strings.Contains(msg, "sdl_getdisplays"),
		strings.Contains(msg, "display server"),
		strings.Contains(msg, "xdg_session_type"):
		return FailureNoDisplay
	case strings.Contains(msg, "terminal too small"),
		strings.Contains(msg, "terminal size"),
		strings.Contains(msg, "window size"),
		strings.Contains(msg, "rendering failure"),
		strings.Contains(msg, "terminal rendering"):
		return FailureTerminalRender
	case strings.Contains(msg, "not found") && (strings.Contains(msg, "device '") || strings.Contains(msg, "no such device")):
		return FailureDeviceNotFound
	case strings.Contains(msg, "more than one device"), strings.Contains(msg, "more than one emulator"):
		return FailureMultipleDevices
	case strings.Contains(msg, "protocol fault"), strings.Contains(msg, "protocol error"):
		return FailureProtocol
	case strings.Contains(msg, "insufficient permissions"), strings.Contains(msg, "udev rules"), strings.Contains(msg, "permission denied") && strings.Contains(msg, "usb"):
		return FailurePermissions
	case strings.Contains(msg, "version") && (strings.Contains(msg, "doesn't match") || strings.Contains(msg, "does not match") || strings.Contains(msg, "mismatch")):
		return FailureVersionMismatch
	case strings.Contains(msg, "connection reset"), strings.Contains(msg, "wsaeconnreset"), strings.Contains(msg, "econnreset"):
		return FailureConnectionReset
	case strings.Contains(msg, "launch blocked by resolution"), strings.Contains(msg, "unsupported"):
		return FailureUnsupported
	case strings.Contains(msg, "invalid profile"), strings.Contains(msg, "empty command"), strings.Contains(msg, "nil command"), strings.Contains(msg, "conflict"):
		return FailureInvalid
	default:
		return FailureUnknown
	}
}

// ── Pairing & connect ─────────────────────────────────────────────────────────

// PairResult holds the outcome of an adb pair operation.
type PairResult struct {
	Success bool
	Output  string
}

// Pair runs "adb pair hostPort code" to pair with a device via wireless debugging.
func Pair(ctx context.Context, hostPort, code string) (PairResult, error) {
	if !platform.IsAvailable("adb") {
		return PairResult{}, errors.New("exec: \"adb\": executable file not found in $PATH")
	}
	args := []string{"pair", hostPort}
	if strings.TrimSpace(code) != "" {
		args = append(args, strings.TrimSpace(code))
	}
	res, err := platform.RunCommandDetailed(ctx, "adb", args...)
	combined := strings.TrimSpace(res.Stdout)
	if s := strings.TrimSpace(res.Stderr); s != "" {
		if combined != "" {
			combined += " | "
		}
		combined += s
	}
	if err != nil {
		return PairResult{Output: combined}, fmt.Errorf("adb pair: %w", err)
	}
	lc := strings.ToLower(combined)
	success := strings.Contains(lc, "successfully paired") || strings.Contains(lc, "paired to")
	return PairResult{Success: success, Output: combined}, nil
}

// Disconnect runs "adb disconnect hostPort" to remove a TCP device from ADB's registry.
// Safe to call on a device that is already disconnected.
func Disconnect(ctx context.Context, hostPort string) error {
	if !platform.IsAvailable("adb") {
		return errors.New("exec: \"adb\": executable file not found in $PATH")
	}
	_, err := platform.RunCommand(ctx, "adb", "disconnect", hostPort)
	return err
}

// Connect runs "adb connect hostPort" to establish a TCP ADB connection after pairing.
func Connect(ctx context.Context, hostPort string) (string, error) {
	if !platform.IsAvailable("adb") {
		return "", errors.New("exec: \"adb\": executable file not found in $PATH")
	}
	out, err := platform.RunCommand(ctx, "adb", "connect", hostPort)
	if err != nil {
		return out, fmt.Errorf("adb connect: %w", err)
	}
	lc := strings.ToLower(strings.TrimSpace(out))
	if strings.Contains(lc, "failed") || strings.Contains(lc, "error:") || strings.Contains(lc, "cannot connect") {
		return out, fmt.Errorf("adb connect: %s", out)
	}
	return out, nil
}

// ── Discovery ─────────────────────────────────────────────────────────────────

// Discover enumerates attached devices and probes real capabilities for
// devices in the "device" (authorised) state. Unauthorised / offline entries
// are included with conservative (all-false) capability flags so the UI can
// still show them. An empty result is never an error; use the returned error
// only to detect adb binary failures.
func Discover(ctx context.Context) ([]core.DeviceCapabilitySnapshot, error) {
	if !platform.IsAvailable("adb") {
		return nil, errors.New("exec: \"adb\": executable file not found in $PATH")
	}
	out, err := platform.RunCommand(ctx, "adb", "devices", "-l")
	if err != nil {
		return nil, fmt.Errorf("adb discovery failed: %w", err)
	}
	parsed := ParseDevicesLong(out)

	result := make([]core.DeviceCapabilitySnapshot, len(parsed))
	for i, d := range parsed {
		result[i] = buildBaseSnapshot(d)
	}

	// Probe authorised devices in parallel with a tight per-device timeout.
	var wg sync.WaitGroup
	for i, d := range parsed {
		if d.State != "device" {
			continue
		}
		wg.Add(1)
		go func(idx int, dev ParsedDevice) {
			defer wg.Done()
			probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			if probed, err := ProbeCapabilities(probeCtx, dev.Serial); err == nil {
				result[idx] = probed
			}
		}(i, d)
	}
	wg.Wait()

	// Return empty slice — not an error — when no devices are present.
	return result, nil
}

// ProbeCapabilities queries a live authorised device for its real capability
// snapshot using a single batched adb shell invocation. It also collects
// battery level, internal/external storage, WiFi IP, and Tailscale IP.
func ProbeCapabilities(ctx context.Context, serial string) (core.DeviceCapabilitySnapshot, error) {
	if !platform.IsAvailable("adb") {
		return core.DeviceCapabilitySnapshot{}, errors.New("exec: \"adb\": executable file not found in $PATH")
	}
	// One shell call: newline-separated key=value output.
	// All sub-commands use 2>/dev/null so missing features don't abort the script.
	script := "echo sdk=$(getprop ro.build.version.sdk 2>/dev/null); " +
		"echo model=$(getprop ro.product.model 2>/dev/null); " +
		"echo manufacturer=$(getprop ro.manufacturer 2>/dev/null); " +
		"echo release=$(getprop ro.build.version.release 2>/dev/null); " +
		"echo brand=$(getprop ro.product.brand 2>/dev/null); " +
		"_dn=$(settings get global device_name 2>/dev/null); " +
		"[ \"$_dn\" = 'null' ] || [ -z \"$_dn\" ] && _dn=$(getprop net.hostname 2>/dev/null); " +
		"echo device_name=$_dn; " +
		"echo battery=$(cat /sys/class/power_supply/battery/capacity 2>/dev/null); " +
		"echo int_storage=$(df /data 2>/dev/null | awk 'END{print $2\":\"$4}'); " +
		"echo ext_storage=$(df /sdcard 2>/dev/null | awk 'END{print $2\":\"$4}'); " +
		"echo wifi_ip=$(ip -4 addr show wlan0 2>/dev/null | awk '/inet /{gsub(\"/.*\",\"\",$2); print $2; exit}'); " +
		"echo ts_ip=$(ip -4 addr show tailscale0 2>/dev/null | awk '/inet /{gsub(\"/.*\",\"\",$2); print $2; exit}')"

	out, err := platform.RunCommand(ctx, "adb", "-s", serial, "shell", script)
	if err != nil {
		return core.DeviceCapabilitySnapshot{}, fmt.Errorf("probe capabilities %s: %w", serial, err)
	}

	props := parseKeyValueLines(out)
	sdkInt, _ := strconv.Atoi(strings.TrimSpace(props["sdk"]))
	model := strings.TrimSpace(props["model"])
	manufacturer := strings.TrimSpace(props["manufacturer"])
	release := strings.TrimSpace(props["release"])
	deviceName := strings.TrimSpace(props["device_name"])
	if deviceName == "null" {
		deviceName = "" // settings returns literal "null" when unset
	}
	usb := isUSBSerial(serial)

	batteryLevel, _ := strconv.Atoi(strings.TrimSpace(props["battery"]))
	intTotal, intFree := parseStoragePair(props["int_storage"])
	extTotal, extFree := parseStoragePair(props["ext_storage"])
	wifiIP := strings.TrimSpace(props["wifi_ip"])
	tailscaleIP := strings.TrimSpace(props["ts_ip"])

	return core.DeviceCapabilitySnapshot{
		Serial:          serial,
		State:           "device",
		Model:           model,
		Manufacturer:    manufacturer,
		DeviceName:      deviceName,
		AndroidRelease:  release,
		SDKInt:          sdkInt,
		BatteryLevel:    batteryLevel,
		StorageTotal:    intTotal,
		StorageFree:     intFree,
		ExtStorageTotal: extTotal,
		ExtStorageFree:  extFree,
		LocalWiFiIP:     wifiIP,
		TailscaleIP:     tailscaleIP,
		// Capability heuristics derived from Android SDK level.
		// Conservative: assume unsupported when SDK is unknown (0).
		// These match the minimum Android versions documented by scrcpy.
		SupportsAudio:          sdkInt >= 29, // Android 10+
		SupportsCamera:         sdkInt >= 31, // Android 12+
		SupportsVirtualDisplay: sdkInt >= 29, // Android 10+
		SupportsH265:           sdkInt >= 29, // Most Android 10+ devices
		SupportsGamepadUHID:    sdkInt >= 29 && platform.UHIDAvailable(),
		SupportsGamepadAOA:     usb, // AOA requires USB transport
	}, nil
}

// parseStoragePair parses "total_1k:avail_1k" from df output into bytes.
// Returns (0, 0) on any parse failure so missing SD card doesn't surface errors.
func parseStoragePair(raw string) (total, free int64) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == ":" {
		return 0, 0
	}
	parts := strings.SplitN(raw, ":", 2)
	if len(parts) != 2 {
		return 0, 0
	}
	t, err1 := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
	f, err2 := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
	if err1 != nil || err2 != nil {
		return 0, 0
	}
	return t * 1024, f * 1024
}

// buildBaseSnapshot returns a conservative snapshot for a device that has not
// been probed (e.g. unauthorized or offline state). All capability flags are
// false so the resolver never enables unsupported features by default.
func buildBaseSnapshot(d ParsedDevice) core.DeviceCapabilitySnapshot {
	model := strings.ReplaceAll(d.Attrs["model"], "_", " ")
	return core.DeviceCapabilitySnapshot{
		Serial: d.Serial,
		State:  d.State,
		Model:  model,
		// SDKInt = 0 → all capability heuristics evaluate to false.
	}
}

// parseKeyValueLines parses "key=value\n..." output into a map.
func parseKeyValueLines(s string) map[string]string {
	out := make(map[string]string)
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		kv := strings.SplitN(line, "=", 2)
		if len(kv) == 2 {
			out[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
		}
	}
	return out
}

// isUSBSerial returns true when the serial is a USB device serial (no colon).
// TCP/IP device serials have the form "host:port", e.g. "192.168.1.1:5555".
func isUSBSerial(serial string) bool {
	return !strings.Contains(serial, ":")
}
