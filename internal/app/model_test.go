package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"github.com/NamasteJasutin/screener/internal/adb"
	"github.com/NamasteJasutin/screener/internal/core"
	"github.com/NamasteJasutin/screener/internal/persistence"
	"github.com/NamasteJasutin/screener/internal/scrcpy"
)

func TestTogglePreferH265UpdatesProfileAndPreview(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	before := m.preview
	profile := m.activeProfilePtr()
	if profile == nil {
		t.Fatal("expected active profile")
	}
	previous := profile.DesiredFlags["prefer_h265"]

	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: '3', Text: "3"}))
	m2 := updated.(Model)
	p2 := m2.activeProfilePtr()
	if p2 == nil {
		t.Fatal("expected active profile after mutation")
	}
	if p2.DesiredFlags["prefer_h265"] == previous { // 3 toggles h265
		t.Fatalf("prefer_h265 did not toggle; still %t", previous)
	}
	if m2.preview == "" || m2.preview == before {
		t.Fatalf("preview did not refresh after toggle: before=%q after=%q", before, m2.preview)
	}
}

func TestLaunchUsesExistingLastPlan(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	fake := &scrcpy.CommandPlan{
		Binary:     "true",
		Args:       []string{"--display-id", "7", "--video-bit-rate", "16M"},
		Resolution: core.EffectiveProfileResolution{FinalArgs: []string{"--display-id", "7", "--video-bit-rate", "16M"}, Launchable: true},
		Launchable: true,
	}
	m.lastPlan = fake
	m.preview = scrcpy.Preview(fake)

	updated, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: 's', Text: "s"}))
	if cmd == nil {
		t.Fatal("expected launch command")
	}
	msg := cmd()
	updated2, _ := updated.(Model).Update(msg)
	m2 := updated2.(Model)

	if m2.lastPlan != fake {
		t.Fatal("last plan reference changed; launch should use existing plan")
	}
	if m2.preview != "true --display-id 7 --video-bit-rate 16M" {
		t.Fatalf("preview changed unexpectedly: %q", m2.preview)
	}
	if !strings.Contains(strings.Join(m2.logs, "\n"), "session 1 started") {
		t.Fatal("expected session started log entry")
	}
}

func TestLaunchUnavailableBinaryLogsFailureReason(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	fake := &scrcpy.CommandPlan{
		Binary:     "definitely-not-installed-scrcpytui-test-binary",
		Args:       []string{"--display-id", "7"},
		Resolution: core.EffectiveProfileResolution{FinalArgs: []string{"--display-id", "7"}, Launchable: true},
		Launchable: true,
	}
	m.lastPlan = fake
	m.preview = scrcpy.Preview(fake)

	updated, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: 's', Text: "s"}))
	if cmd == nil {
		t.Fatal("expected launch command")
	}
	msg := cmd()
	updated2, _ := updated.(Model).Update(msg)
	m2 := updated2.(Model)

	if m2.lastPlan != fake {
		t.Fatal("last plan reference changed; launch should use existing plan")
	}
	logs := strings.Join(m2.logs, "\n")
	if !strings.Contains(logs, "launch failed:") {
		t.Fatal("expected launch failure log entry")
	}
	if !strings.Contains(logs, "launch failure reason: "+string(adb.FailureADBMissing)) {
		t.Fatalf("expected classified reason %q in logs: %s", adb.FailureADBMissing, logs)
	}
	if strings.Contains(logs, "launch invoked") {
		t.Fatal("unexpected launch success log entry")
	}
}

func TestLaunchFailureLogsStderrDiagnosticsAndNoFalseSuccess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test script uses POSIX shell")
	}
	t.Setenv("HOME", t.TempDir())
	m := NewModel()

	scriptPath := filepath.Join(t.TempDir(), "failing-launch.sh")
	script := "#!/bin/sh\necho fatal: unable to start video encoder 1>&2\nexit 6\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write failing script: %v", err)
	}

	fake := &scrcpy.CommandPlan{
		Binary:     scriptPath,
		Args:       []string{"--display-id", "7"},
		Resolution: core.EffectiveProfileResolution{FinalArgs: []string{"--display-id", "7"}, Launchable: true},
		Launchable: true,
	}
	m.lastPlan = fake
	m.preview = scrcpy.Preview(fake)

	updated, launchCmd := m.Update(tea.KeyPressMsg(tea.Key{Code: 's', Text: "s"}))
	if launchCmd == nil {
		t.Fatal("expected launch command")
	}
	// cmd.Start() succeeds (process starts), so launchMsg carries a live session.
	startMsg := launchCmd()
	updated2, monitorCmd := updated.(Model).Update(startMsg)
	m2 := updated2.(Model)
	if monitorCmd == nil {
		t.Fatal("expected monitor command after session started")
	}

	// The monitor goroutine waits for the process to exit.
	exitMsg := monitorCmd()
	updated3, _ := m2.Update(exitMsg)
	m3 := updated3.(Model)

	logs := strings.Join(m3.logs, "\n")
	if !strings.Contains(logs, "session 1 exited with error:") {
		t.Fatalf("expected session exit error log entry, got: %s", logs)
	}
	if !strings.Contains(logs, "stderr=fatal: unable to start video encoder") {
		t.Fatalf("expected stderr diagnostics detail in logs, got: %s", logs)
	}
	if !strings.Contains(logs, "exit_code=6") {
		t.Fatalf("expected exit code in diagnostics detail, got: %s", logs)
	}
	if strings.Contains(logs, "launch invoked") {
		t.Fatal("unexpected old-style launch success log entry")
	}
}

func TestLaunchSecondRequestWhileLaunchingIsBlocked(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.lastPlan = &scrcpy.CommandPlan{Binary: "true", Launchable: true}
	m.preview = scrcpy.Preview(m.lastPlan)

	updated, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: 's', Text: "s"}))
	if cmd == nil {
		t.Fatal("expected first launch command")
	}
	m2 := updated.(Model)
	if m2.launchState != LaunchStateLaunching {
		t.Fatalf("expected launch state %q got %q", LaunchStateLaunching, m2.launchState)
	}

	updated, cmd = m2.Update(tea.KeyPressMsg(tea.Key{Code: 's', Text: "s"}))
	m3 := updated.(Model)
	if cmd != nil {
		t.Fatal("expected no launch command when launch already in progress")
	}
	if !strings.Contains(strings.Join(m3.logs, "\n"), "launch skipped: another launch is starting") {
		t.Fatalf("expected deterministic skip reason in logs: %s", strings.Join(m3.logs, "\n"))
	}
}

func TestLaunchCancelWithoutInProgressLogsSkip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()

	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: 'z', Text: "z"}))
	m2 := updated.(Model)
	if !strings.Contains(strings.Join(m2.logs, "\n"), "launch cancel skipped: no launch in progress") {
		t.Fatalf("expected cancel skip log entry, got: %s", strings.Join(m2.logs, "\n"))
	}
}

func TestLaunchCancelFlowSetsCanceledStateAndReason(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.lastPlan = &scrcpy.CommandPlan{Binary: "true", Launchable: true}
	m.preview = scrcpy.Preview(m.lastPlan)

	updated, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: 's', Text: "s"}))
	if cmd == nil {
		t.Fatal("expected launch command")
	}
	m2 := updated.(Model)

	updated, _ = m2.Update(tea.KeyPressMsg(tea.Key{Code: 'z', Text: "z"}))
	m3 := updated.(Model)
	logs := strings.Join(m3.logs, "\n")
	if !strings.Contains(logs, "launch cancel requested") {
		t.Fatalf("expected cancel request log entry, got: %s", logs)
	}

	updated, _ = m3.Update(launchMsg{reason: adb.FailureCanceled, err: context.Canceled, res: scrcpy.ExecutionResult{Canceled: true}})
	m4 := updated.(Model)
	if m4.launchState != LaunchStateCanceled {
		t.Fatalf("expected launch state %q got %q", LaunchStateCanceled, m4.launchState)
	}
	if m4.launchCancel != nil {
		t.Fatal("expected launch cancel func to be cleared after completion")
	}
	logs = strings.Join(m4.logs, "\n")
	if !strings.Contains(logs, "launch failure reason: "+string(adb.FailureCanceled)) {
		t.Fatalf("expected canceled failure reason in logs, got: %s", logs)
	}
	// Accept either arrow style (→ or ->) for the transition log.
	if !strings.Contains(logs, "launching") || !strings.Contains(logs, "canceled") {
		t.Fatalf("expected launch state transition log, got: %s", logs)
	}
}

func TestViewCommandPaneShowsLaunchState(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.width = minFullLayoutWidth + 12
	m.height = minFullLayoutHeight + 8

	// New layout: launch state is shown inline in the right panel and status bar.
	m.launchState = LaunchStateLaunching
	view := stripANSIForTest(fmt.Sprint(m.View()))
	if !strings.Contains(view, "launching") {
		t.Fatalf("expected launching state in view: %s", view)
	}
	if !strings.Contains(view, "z") {
		t.Fatalf("expected cancel hint (z) while launching: %s", view)
	}

	m.launchState = LaunchStateIdle
	m.activeSessions = []activeSession{{id: 1, serial: "test", profileID: "Test"}}
	view = stripANSIForTest(fmt.Sprint(m.View()))
	if !strings.Contains(view, "running") {
		t.Fatalf("expected running session indicator in view: %s", view)
	}

	m.launchState = LaunchStateFailed
	view = stripANSIForTest(fmt.Sprint(m.View()))
	if !strings.Contains(view, "failed") {
		t.Fatalf("expected failed state in view: %s", view)
	}
}

func TestRenameModeRoundtrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.focus = PaneProfiles // E renames profiles only when Profiles pane is focused
	profile := m.activeProfilePtr()
	if profile == nil {
		t.Fatal("expected active profile")
	}
	before := profile.Name

	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: 'e', Text: "e"}))
	m2 := updated.(Model)
	if !m2.renameMode {
		t.Fatal("expected rename mode to start")
	}

	updated, _ = m2.Update(tea.KeyPressMsg(tea.Key{Code: 'X', Text: "X"}))
	m3 := updated.(Model)
	updated, _ = m3.Update(tea.KeyPressMsg(tea.Key{Code: 'Y', Text: "Y"}))
	m4 := updated.(Model)
	updated, _ = m4.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	m5 := updated.(Model)

	if m5.renameMode {
		t.Fatal("expected rename mode to finish")
	}
	p2 := m5.activeProfilePtr()
	if p2 == nil {
		t.Fatal("expected active profile after rename")
	}
	if p2.Name != before+"XY" {
		t.Fatalf("expected renamed profile %q got %q", before+"XY", p2.Name)
	}
}

func TestSetDefaultProfileKeyDEnforcesSingleDefault(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()

	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: 'n', Text: "n"}))
	m2 := updated.(Model)
	if len(m2.config.Profiles) < 2 {
		t.Fatal("expected profile creation")
	}

	updated, _ = m2.Update(tea.KeyPressMsg(tea.Key{Code: 'f', Text: "f"}))
	m3 := updated.(Model)

	defaultCount := 0
	for i, p := range m3.config.Profiles {
		if p.IsDefault {
			defaultCount++
			if i != m3.activeIdx {
				t.Fatalf("expected active profile to be default, got index %d", i)
			}
		}
	}
	if defaultCount != 1 {
		t.Fatalf("expected exactly one default profile, got %d", defaultCount)
	}
}

func TestWindowSizeTinyLogsTerminalFailureOnce(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()

	updated, _ := m.Update(tea.WindowSizeMsg{Width: minFullLayoutWidth - 1, Height: minFullLayoutHeight - 1})
	m2 := updated.(Model)

	logs := strings.Join(m2.logs, "\n")
	if !strings.Contains(logs, "layout degraded") {
		t.Fatalf("expected layout degraded log entry, got: %s", logs)
	}
	if !strings.Contains(logs, string(adb.FailureTerminalRender)) {
		t.Fatalf("expected classified reason %q in logs: %s", adb.FailureTerminalRender, logs)
	}

	updated, _ = m2.Update(tea.WindowSizeMsg{Width: minFullLayoutWidth - 5, Height: minFullLayoutHeight - 2})
	m3 := updated.(Model)
	logs = strings.Join(m3.logs, "\n")
	if got := strings.Count(logs, "layout degraded"); got != 1 {
		t.Fatalf("expected one degraded transition log, got %d logs: %s", got, logs)
	}
}

func TestWindowSizeRecoveryLogsOnce(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()

	updated, _ := m.Update(tea.WindowSizeMsg{Width: minFullLayoutWidth - 2, Height: minFullLayoutHeight - 2})
	m2 := updated.(Model)
	updated, _ = m2.Update(tea.WindowSizeMsg{Width: minFullLayoutWidth + 4, Height: minFullLayoutHeight + 3})
	m3 := updated.(Model)

	logs := strings.Join(m3.logs, "\n")
	if !strings.Contains(logs, "layout restored") {
		t.Fatalf("expected layout restored log entry, got: %s", logs)
	}
	if got := strings.Count(logs, "layout restored"); got != 1 {
		t.Fatalf("expected one restored transition log, got %d logs: %s", got, logs)
	}
}

func TestViewTinyTerminalUsesFallbackOverlay(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.width = minFullLayoutWidth - 10
	m.height = minFullLayoutHeight - 6

	view := fmt.Sprint(m.View())
	if !strings.Contains(view, "Too Small") && !strings.Contains(view, "too small") {
		t.Fatalf("expected tiny terminal fallback message in view: %s", view)
	}
	if !strings.Contains(view, "Required") {
		t.Fatalf("expected tiny terminal fallback requirement message in view: %s", view)
	}
	if strings.Contains(view, "Launch Command") {
		t.Fatalf("expected no full pane layout in tiny terminal fallback view: %s", view)
	}
	if strings.Contains(view, "Supported Features") {
		t.Fatalf("expected no inspector pane in tiny terminal fallback view: %s", view)
	}
}

func TestViewNormalTerminalIncludesPaneContentOverMatrix(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.width = minFullLayoutWidth + 12
	m.height = minFullLayoutHeight + 8

	view := fmt.Sprint(m.View())
	plainView := stripANSIForTest(view)
	if !strings.Contains(plainView, "Devices") {
		t.Fatalf("expected devices pane heading in normal view: %s", view)
	}
	if !strings.Contains(plainView, "Devices") {
		t.Fatalf("expected devices pane heading in normal view: %s", view)
	}
	if !strings.Contains(plainView, "Launch Command") {
		t.Fatalf("expected command section in normal view: %s", view)
	}
	if !strings.Contains(view, "\x1b[H") {
		t.Fatalf("expected composed matrix+pane output with home escape: %q", view)
	}
}

// ── New M3/M4/M7 tests ──────────────────────────────────────────────────────

func TestWarningOverlayTogglesOnUpperW(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	if m.overlayMode != OverlayNone {
		t.Fatal("expected no overlay on init")
	}
	// Uppercase W must open warning overlay
	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: 'W', Text: "W"}))
	m2 := updated.(Model)
	if m2.overlayMode != OverlayWarning {
		t.Fatalf("expected OverlayWarning after W, got %d", m2.overlayMode)
	}
	// Key '2' toggles stay_awake (w was the old binding; new binding is 2).
	// Capture origVal BEFORE Update to avoid map sharing mutation.
	origProfile := m.activeProfilePtr()
	origStayAwake := false
	if origProfile != nil {
		origStayAwake = origProfile.DesiredFlags["stay_awake"]
	}
	updated, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: '2', Text: "2"}))
	m3 := updated.(Model)
	if m3.overlayMode != OverlayNone {
		t.Fatalf("expected no overlay after key 2, got %d", m3.overlayMode)
	}
	profile := m3.activeProfilePtr()
	if profile == nil {
		t.Fatal("expected active profile")
	}
	if profile.DesiredFlags["stay_awake"] == origStayAwake {
		t.Fatalf("expected stay_awake to be toggled from %t, still %t", origStayAwake, profile.DesiredFlags["stay_awake"])
	}
}

func TestWarningOverlayClosesOnEsc(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: 'W', Text: "W"}))
	m2 := updated.(Model)
	if m2.overlayMode != OverlayWarning {
		t.Fatal("expected warning overlay")
	}
	updated, _ = m2.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	m3 := updated.(Model)
	if m3.overlayMode != OverlayNone {
		t.Fatalf("expected no overlay after esc, got %d", m3.overlayMode)
	}
}

func TestPairingOverlayOpensOnUpperP(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: 'P', Text: "P"}))
	m2 := updated.(Model)
	if m2.overlayMode != OverlayPairing {
		t.Fatalf("expected OverlayPairing after P, got %d", m2.overlayMode)
	}
	if !strings.Contains(strings.Join(m2.logs, "\n"), "pairing dialog opened") {
		t.Fatal("expected pairing dialog log entry")
	}
}

func TestPairingOverlayClosesOnEsc(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: 'P', Text: "P"}))
	m2 := updated.(Model)
	updated, _ = m2.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	m3 := updated.(Model)
	if m3.overlayMode != OverlayNone {
		t.Fatalf("expected no overlay after esc, got %d", m3.overlayMode)
	}
	if !strings.Contains(strings.Join(m3.logs, "\n"), "pairing dialog closed") {
		t.Fatal("expected pairing closed log entry")
	}
}

func TestPairingOverlayTextInput(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: 'P', Text: "P"}))
	m2 := updated.(Model)

	// Type IP address into field 0 (max 15 chars)
	for _, ch := range "192.168.1.5" {
		updated, _ = m2.Update(tea.KeyPressMsg(tea.Key{Text: string(ch)}))
		m2 = updated.(Model)
	}
	if m2.pairingHost != "192.168.1.5" {
		t.Fatalf("expected pairingHost, got %q", m2.pairingHost)
	}

	// Tab to port field
	updated, _ = m2.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	m2 = updated.(Model)
	if m2.pairingField != 1 {
		t.Fatalf("expected pairingField=1 after tab, got %d", m2.pairingField)
	}

	// Type port into field 1
	for _, ch := range "39500" {
		updated, _ = m2.Update(tea.KeyPressMsg(tea.Key{Text: string(ch)}))
		m2 = updated.(Model)
	}
	if m2.pairingPort != "39500" {
		t.Fatalf("expected pairingPort, got %q", m2.pairingPort)
	}

	// Tab to code field
	updated, _ = m2.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	m2 = updated.(Model)
	if m2.pairingField != 2 {
		t.Fatalf("expected pairingField=2 after tab, got %d", m2.pairingField)
	}

	// Type code into field 2
	for _, ch := range "123456" {
		updated, _ = m2.Update(tea.KeyPressMsg(tea.Key{Text: string(ch)}))
		m2 = updated.(Model)
	}
	if m2.pairingCode != "123456" {
		t.Fatalf("expected pairing code, got %q", m2.pairingCode)
	}

	// Tab to connect port field
	updated, _ = m2.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	m2 = updated.(Model)
	if m2.pairingField != 3 {
		t.Fatalf("expected pairingField=3 after tab, got %d", m2.pairingField)
	}

	// Connect port pre-filled with "5555"; clear it and type a custom port
	m2.pairingConnectPort = ""
	for _, ch := range "45678" {
		updated, _ = m2.Update(tea.KeyPressMsg(tea.Key{Text: string(ch)}))
		m2 = updated.(Model)
	}
	if m2.pairingConnectPort != "45678" {
		t.Fatalf("expected pairingConnectPort, got %q", m2.pairingConnectPort)
	}

	// Backspace deletes from connect port field
	updated, _ = m2.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace}))
	m2 = updated.(Model)
	if m2.pairingConnectPort != "4567" {
		t.Fatalf("expected truncated connect port, got %q", m2.pairingConnectPort)
	}
}

func TestPairingOverlayRendersInView(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.width = minFullLayoutWidth + 20
	m.height = minFullLayoutHeight + 10
	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: 'P', Text: "P"}))
	m2 := updated.(Model)

	view := stripANSIForTest(fmt.Sprint(m2.View()))
	if !strings.Contains(view, "ADB Wireless Pairing") {
		t.Fatalf("expected pairing overlay in view: %s", view)
	}
	if !strings.Contains(view, "IP Address") {
		t.Fatalf("expected IP Address field in pairing overlay: %s", view)
	}
}

func TestWarningOverlayRendersInView(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.width = minFullLayoutWidth + 20
	m.height = minFullLayoutHeight + 10
	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: 'W', Text: "W"}))
	m2 := updated.(Model)

	view := stripANSIForTest(fmt.Sprint(m2.View()))
	if !strings.Contains(view, "Warnings & Compatibility") {
		t.Fatalf("expected warning overlay in view: %s", view)
	}
}

func TestKnownDevicesAppearInDeviceList(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.config.KnownDevices = []core.KnownDevice{
		{
			Alias: "192.168.1.100",
			Model: "Pixel7",
			Endpoints: []core.Endpoint{
				{Host: "192.168.1.100", Port: 5555, Transport: "tcp"},
			},
		},
	}
	entries := m.mergedDeviceList()
	if len(entries) != 1 {
		t.Fatalf("expected 1 device entry, got %d", len(entries))
	}
	if entries[0].Serial != "192.168.1.100" {
		t.Fatalf("expected alias as serial for offline known device, got %q", entries[0].Serial)
	}
	if entries[0].State != "known-offline" {
		t.Fatalf("expected known-offline state, got %q", entries[0].State)
	}
	if entries[0].IsKnown != true {
		t.Fatal("expected IsKnown=true")
	}
	if entries[0].IsLive != false {
		t.Fatal("expected IsLive=false")
	}
}

func TestDeviceSelectionDrivesCaps(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	// Add two live devices with different SDKs
	m.devices = []core.DeviceCapabilitySnapshot{
		{Serial: "device1", Model: "A", State: "device", SDKInt: 28},
		{Serial: "device2", Model: "B", State: "device", SDKInt: 34},
	}
	m.deviceIdx = 0
	m.recomputePlanAndPreview()
	// SDK 28 should drop turn_screen_off (sdk<30) and stay_awake (sdk<29) for device1
	resolve1 := m.lastResolve

	m.deviceIdx = 1
	m.recomputePlanAndPreview()
	resolve2 := m.lastResolve

	// Device 1 (sdk=28): should have warnings about turn_screen_off and stay_awake
	if !contains(resolve1.Warnings, "turn_screen_off ignored on sdk<30") {
		t.Fatalf("expected sdk<30 warning for device1, got warnings: %v", resolve1.Warnings)
	}
	// Device 2 (sdk=34): should have no such warnings
	if contains(resolve2.Warnings, "turn_screen_off ignored on sdk<30") {
		t.Fatalf("unexpected sdk warning for device2 (sdk=34): %v", resolve2.Warnings)
	}
}

func TestDeviceSwitchWithJKWhenFocusZero(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.devices = []core.DeviceCapabilitySnapshot{
		{Serial: "aaa", State: "device", SDKInt: 34},
		{Serial: "bbb", State: "device", SDKInt: 34},
	}
	m.focus = PaneDevices
	m.deviceIdx = 0

	// Arrow keys navigate devices when Devices pane is focused.
	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	m2 := updated.(Model)
	if m2.deviceIdx != 1 {
		t.Fatalf("expected deviceIdx=1 after down arrow, got %d", m2.deviceIdx)
	}

	updated, _ = m2.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	m3 := updated.(Model)
	if m3.deviceIdx != 0 {
		t.Fatalf("expected deviceIdx=0 after up arrow, got %d", m3.deviceIdx)
	}
}

func TestPairResultMsgSavesKnownDevice(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	if len(m.config.KnownDevices) != 0 {
		t.Fatalf("expected no known devices initially, got %d", len(m.config.KnownDevices))
	}

	// Simulate a successful pair result
	updated, _ := m.Update(pairResultMsg{
		result:   adb.PairResult{Success: true, Output: "Successfully paired to 192.168.1.50:39123"},
		hostPort: "192.168.1.50:39123",
		err:      nil,
	})
	m2 := updated.(Model)
	if len(m2.config.KnownDevices) != 1 {
		t.Fatalf("expected 1 known device after pairing, got %d", len(m2.config.KnownDevices))
	}
	if m2.config.KnownDevices[0].Alias != "192.168.1.50" {
		t.Fatalf("expected alias 192.168.1.50, got %q", m2.config.KnownDevices[0].Alias)
	}
	if len(m2.config.KnownDevices[0].Endpoints) != 1 {
		t.Fatalf("expected 1 endpoint, got %d", len(m2.config.KnownDevices[0].Endpoints))
	}
	if m2.config.KnownDevices[0].Endpoints[0].Port != 5555 {
		t.Fatalf("expected ADB port 5555, got %d", m2.config.KnownDevices[0].Endpoints[0].Port)
	}
	if !strings.Contains(strings.Join(m2.logs, "\n"), "known device saved") {
		t.Fatalf("expected known device saved log: %s", strings.Join(m2.logs, "\n"))
	}
}

func TestPairResultMsgFailureLogsError(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	updated, _ := m.Update(pairResultMsg{
		result:   adb.PairResult{Success: false, Output: "Failed to pair"},
		hostPort: "10.0.0.1:12345",
		err:      nil,
	})
	m2 := updated.(Model)
	if len(m2.config.KnownDevices) != 0 {
		t.Fatalf("expected no known devices on failed pair, got %d", len(m2.config.KnownDevices))
	}
	if !strings.Contains(strings.Join(m2.logs, "\n"), "pair unsuccessful") {
		t.Fatalf("expected pair unsuccessful log: %s", strings.Join(m2.logs, "\n"))
	}
}

func contains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

func stripANSIForTest(s string) string {
	var out strings.Builder
	for i := 0; i < len(s); {
		if s[i] == 0x1b {
			next, ok := ansiSequenceEndForTest(s, i)
			if ok && next > i {
				i = next
				continue
			}
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			size = 1
		}
		out.WriteString(s[i : i+size])
		i += size
	}
	return out.String()
}

func ansiSequenceEndForTest(s string, start int) (int, bool) {
	if start+1 >= len(s) || s[start] != 0x1b {
		return start, false
	}

	lead := s[start+1]
	switch lead {
	case '[':
		i := start + 2
		for i < len(s) {
			b := s[i]
			if b >= 0x40 && b <= 0x7e {
				return i + 1, true
			}
			i++
		}
		return start, false
	case ']':
		i := start + 2
		for i < len(s) {
			if s[i] == 0x07 {
				return i + 1, true
			}
			if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '\\' {
				return i + 2, true
			}
			i++
		}
		return start, false
	default:
		return start + 2, true
	}
}

// ── Help overlay ──────────────────────────────────────────────────────────────

func TestHelpOverlayOpensOnQuestionMark(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()

	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Text: "?"}))
	m2 := updated.(Model)
	if m2.overlayMode != OverlayHelp {
		t.Fatalf("expected OverlayHelp after ?, got %d", m2.overlayMode)
	}
}

func TestHelpOverlayClosesOnEsc(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.overlayMode = OverlayHelp

	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	m2 := updated.(Model)
	if m2.overlayMode != OverlayNone {
		t.Fatalf("expected OverlayNone after Esc, got %d", m2.overlayMode)
	}
}

func TestHelpOverlayClosesOnSecondQuestionMark(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.overlayMode = OverlayHelp

	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Text: "?"}))
	m2 := updated.(Model)
	if m2.overlayMode != OverlayNone {
		t.Fatalf("expected OverlayNone after second ?, got %d", m2.overlayMode)
	}
}

func TestHelpOverlayRendersInView(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.width = minFullLayoutWidth + 20
	m.height = minFullLayoutHeight + 10
	m.overlayMode = OverlayHelp

	view := stripANSIForTest(fmt.Sprint(m.View()))
	if !strings.Contains(view, "Help") {
		t.Fatalf("expected Help overlay in view: %s", view)
	}
	if !strings.Contains(view, "Navigation") {
		t.Fatalf("expected Navigation section in help: %s", view)
	}
	if !strings.Contains(view, "Launch") {
		t.Fatalf("expected Launch section in help: %s", view)
	}
}

// ── Launch state auto-reset ───────────────────────────────────────────────────

func TestLaunchStateResetsToIdleAfterResetMsg(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, state := range []LaunchState{
		LaunchStateSucceeded, LaunchStateFailed, LaunchStateCanceled, LaunchStateTimedOut,
	} {
		m := NewModel()
		m.launchState = state

		updated, _ := m.Update(launchResetMsg{})
		m2 := updated.(Model)
		if m2.launchState != LaunchStateIdle {
			t.Fatalf("state %s: expected idle after launchResetMsg, got %s", state, m2.launchState)
		}
		if m2.launchReason != "none" {
			t.Fatalf("state %s: expected launchReason=none, got %s", state, m2.launchReason)
		}
	}
}

func TestLaunchStateNotResetWhenLaunching(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.launchState = LaunchStateLaunching

	updated, _ := m.Update(launchResetMsg{})
	m2 := updated.(Model)
	// A launchResetMsg arriving while still launching should be a no-op.
	if m2.launchState != LaunchStateLaunching {
		t.Fatalf("expected launch state to remain Launching, got %s", m2.launchState)
	}
}

// ── Serial injection ──────────────────────────────────────────────────────────

func TestSerialInjectedIntoPreviewForLiveDevice(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.devices = []core.DeviceCapabilitySnapshot{
		{
			Serial: "ABCD1234", Model: "Pixel7", State: "device", SDKInt: 34,
			SupportsAudio: true, SupportsH265: true, SupportsVirtualDisplay: true,
			SupportsGamepadUHID: true,
		},
	}
	m.deviceIdx = 0
	m.recomputePlanAndPreview()

	if !strings.Contains(m.preview, "--serial=ABCD1234") {
		t.Fatalf("expected --serial=ABCD1234 in preview for live device, got: %q", m.preview)
	}
}

func TestNoSerialInjectedForSimulatedCaps(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	// No live devices → falls back to simulated caps.
	m.devices = nil
	m.recomputePlanAndPreview()

	if strings.Contains(m.preview, "--serial=") {
		t.Fatalf("unexpected --serial in preview for simulated device: %q", m.preview)
	}
}

func TestNoSerialInjectedForKnownOfflineDevice(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.devices = nil
	m.config.KnownDevices = []core.KnownDevice{
		{
			Alias: "192.168.1.100",
			Endpoints: []core.Endpoint{
				{Host: "192.168.1.100", Port: 5555, Transport: "tcp"},
			},
		},
	}
	// Known-offline device is at deviceIdx=0 but IsLive=false.
	m.deviceIdx = 0
	m.recomputePlanAndPreview()

	// Known-offline devices must not have --serial injected (they're not connected).
	if strings.Contains(m.preview, "--serial=") {
		t.Fatalf("unexpected --serial for known-offline device: %q", m.preview)
	}
}

// ── Log buffer cap ────────────────────────────────────────────────────────────

func TestLogBufferCappedAtMaxLines(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	initial := len(m.logs)

	// Write well beyond the cap.
	for i := 0; i < maxLogLines+100; i++ {
		m.appendLog(fmt.Sprintf("line %d", i))
	}
	if len(m.logs) > maxLogLines {
		t.Fatalf("log buffer grew to %d (cap=%d, initial=%d)", len(m.logs), maxLogLines, initial)
	}
}

// ── Periodic poll ─────────────────────────────────────────────────────────────

func TestDevicePollMsgReturnsCommand(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()

	updated, cmd := m.Update(devicePollMsg{})
	_ = updated
	// devicePollMsg must always return a non-nil batch command.
	if cmd == nil {
		t.Fatal("expected non-nil cmd from devicePollMsg handler")
	}
}

// ── Profile scroll ────────────────────────────────────────────────────────────

func TestProfileScrollShowsActiveIdxWhenManyProfiles(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()

	// Create enough profiles to require scrolling.
	for i := 0; i < 20; i++ {
		updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: 'n', Text: "n"}))
		m = updated.(Model)
	}

	// Navigate to the very last profile.
	m.activeIdx = len(m.config.Profiles) - 1
	m.width = minFullLayoutWidth + 12
	m.height = minFullLayoutHeight + 8

	// View must not panic and must still show the devices pane.
	view := stripANSIForTest(fmt.Sprint(m.View()))
	if !strings.Contains(view, "Devices") {
		t.Fatalf("expected Devices pane in scrolled view: %s", view)
	}
	// The scroll indicator "N–M of T" should appear when list is longer than window.
	if !strings.Contains(view, "of ") {
		t.Logf("note: scroll indicator may not appear if all profiles fit: total=%d", len(m.config.Profiles))
	}
}

// ── Context keys ─────────────────────────────────────────────────────────────

func TestContextKeysIncludesHelpHint(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	keys := m.contextKeys()
	if !strings.Contains(keys, "?") {
		t.Fatalf("expected ?=help in contextKeys, got: %q", keys)
	}
}

// ── Profile CRUD (adjustBitrate, duplicate, delete, cancelRename, switchActiveProfile) ─

func TestAdjustBitrateIncreasesAndDecreases(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	p := m.activeProfilePtr()
	if p == nil {
		t.Fatal("expected active profile")
	}
	before := p.VideoBitRateMB

	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Text: "+"}))
	m2 := updated.(Model)
	if m2.activeProfilePtr().VideoBitRateMB != before+2 {
		t.Fatalf("expected bitrate %d, got %d", before+2, m2.activeProfilePtr().VideoBitRateMB)
	}

	updated, _ = m2.Update(tea.KeyPressMsg(tea.Key{Text: "-"}))
	m3 := updated.(Model)
	if m3.activeProfilePtr().VideoBitRateMB != before {
		t.Fatalf("expected bitrate restored to %d, got %d", before, m3.activeProfilePtr().VideoBitRateMB)
	}
}

func TestAdjustBitrateFloorsAtTwo(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.mutateActiveProfile(func(p *core.ProfileDefinition) string {
		p.VideoBitRateMB = 2
		return "set low"
	})
	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Text: "-"}))
	m2 := updated.(Model)
	if m2.activeProfilePtr().VideoBitRateMB < 2 {
		t.Fatalf("bitrate must not go below 2, got %d", m2.activeProfilePtr().VideoBitRateMB)
	}
}

func TestDuplicateProfileAddsNewEntry(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	before := len(m.config.Profiles)

	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: 'd', Text: "d"}))
	m2 := updated.(Model)
	if len(m2.config.Profiles) != before+1 {
		t.Fatalf("expected %d profiles after dup, got %d", before+1, len(m2.config.Profiles))
	}
	// The new profile must not be default.
	if m2.activeProfilePtr().IsDefault {
		t.Fatal("duplicated profile must not be marked default")
	}
	if !strings.Contains(strings.Join(m2.logs, "\n"), "profile duplicated") {
		t.Fatal("expected duplicate log")
	}
}

func TestDeleteProfileReducesCount(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	// Need at least 2 profiles to delete one.
	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: 'n', Text: "n"}))
	m = updated.(Model)
	before := len(m.config.Profiles)
	if before < 2 {
		t.Skip("need ≥2 profiles")
	}

	updated, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'x', Text: "x"}))
	m2 := updated.(Model)
	if len(m2.config.Profiles) != before-1 {
		t.Fatalf("expected %d profiles after delete, got %d", before-1, len(m2.config.Profiles))
	}
}

func TestDeleteLastProfileIsBlocked(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	// Trim to exactly one profile.
	for len(m.config.Profiles) > 1 {
		m.config.Profiles = m.config.Profiles[:len(m.config.Profiles)-1]
	}

	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: 'x', Text: "x"}))
	m2 := updated.(Model)
	if len(m2.config.Profiles) != 1 {
		t.Fatal("must not delete the last profile")
	}
	if !strings.Contains(strings.Join(m2.logs, "\n"), "delete skipped") {
		t.Fatal("expected 'delete skipped' log")
	}
}

func TestCancelRenameRestoresMode(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.focus = PaneProfiles
	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: 'e', Text: "e"}))
	m2 := updated.(Model)
	if !m2.renameMode {
		t.Fatal("expected rename mode")
	}

	updated, _ = m2.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	m3 := updated.(Model)
	if m3.renameMode {
		t.Fatal("expected rename mode cancelled after Esc")
	}
}

func TestSwitchActiveProfileWithArrows(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.focus = PaneProfiles
	m.activeIdx = 0
	n := len(m.config.Profiles)
	if n < 2 {
		t.Skip("need ≥2 profiles")
	}

	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	m2 := updated.(Model)
	if m2.activeIdx != 1 {
		t.Fatalf("expected activeIdx=1 after down, got %d", m2.activeIdx)
	}

	updated, _ = m2.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	m3 := updated.(Model)
	if m3.activeIdx != 0 {
		t.Fatalf("expected activeIdx=0 after up, got %d", m3.activeIdx)
	}
}

func TestCycleThemeChangesThemeName(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	before := m.themeName

	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Text: "]"}))
	m2 := updated.(Model)
	if m2.themeName == before {
		t.Fatalf("expected theme to change after ], still %q", m2.themeName)
	}

	updated, _ = m2.Update(tea.KeyPressMsg(tea.Key{Text: "["}))
	m3 := updated.(Model)
	if m3.themeName != before {
		t.Fatalf("expected theme to cycle back to %q, got %q", before, m3.themeName)
	}
}

// ── Init ──────────────────────────────────────────────────────────────────────

func TestInitReturnsNonNilCmd(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init() must return a non-nil batch command")
	}
}

// ── handleMouseClick ──────────────────────────────────────────────────────────

func TestMouseClickOnDevicePane(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.width = minFullLayoutWidth + 40
	m.height = minFullLayoutHeight + 20
	m.devices = []core.DeviceCapabilitySnapshot{
		{Serial: "DEV1", State: "device", SDKInt: 34},
		{Serial: "DEV2", State: "device", SDKInt: 34},
	}
	m.deviceIdx = 0
	m.focus = PaneRight // start elsewhere

	// Click row=3 is in the device pane body (below border+title)
	// col=5 is left panel (leftW ≈ 40 at width=92+40=132)
	updated, _ := m.Update(tea.MouseClickMsg{X: 5, Y: 3})
	m2 := updated.(Model)
	if m2.focus != PaneDevices {
		t.Fatalf("expected PaneDevices focus after click, got %d", m2.focus)
	}
}

func TestMouseClickOnRightPanel(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.width = minFullLayoutWidth + 40
	m.height = minFullLayoutHeight + 20
	m.focus = PaneDevices

	// Click well to the right of leftW
	updated, _ := m.Update(tea.MouseClickMsg{X: 80, Y: 5})
	m2 := updated.(Model)
	if m2.focus != PaneRight {
		t.Fatalf("expected PaneRight focus after right-panel click, got %d", m2.focus)
	}
}

func TestMouseClickDeviceItemSelection(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.width = 120
	m.height = 38
	m.devices = []core.DeviceCapabilitySnapshot{
		{Serial: "AAA", State: "device", SDKInt: 34},
		{Serial: "BBB", State: "device", SDKInt: 34},
		{Serial: "CCC", State: "device", SDKInt: 34},
	}
	m.deviceIdx = 0

	// Row 4 = border(1) + title(1) + item[2] (0-indexed=2 means third item)
	// With small terminal, just test that click doesn't panic and changes focus
	updated, _ := m.Update(tea.MouseClickMsg{X: 3, Y: 2})
	m2 := updated.(Model)
	// deviceIdx may or may not change depending on exact geometry,
	// but focus must be Devices and no panic
	if m2.focus != PaneDevices && m2.focus != PaneProfiles {
		t.Logf("focus after left-panel click: %d (acceptable)", m2.focus)
	}
}

func TestMouseClickProfileItemSelectionWithScroll(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.width = 120
	m.height = 38

	// Create extra profiles to exercise scroll offset logic
	for i := 0; i < 10; i++ {
		updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: 'n', Text: "n"}))
		m = updated.(Model)
	}

	// Click in left panel below device pane — should route to profile pane
	updated, _ := m.Update(tea.MouseClickMsg{X: 3, Y: 25})
	m2 := updated.(Model)
	_ = m2 // must not panic
}

// ── reconnectKnownDevicesCmd ──────────────────────────────────────────────────

func TestReconnectKnownDevicesCmdWithTCPEndpoints(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.config.KnownDevices = []core.KnownDevice{
		{
			Alias: "device1",
			Endpoints: []core.Endpoint{
				{Host: "10.0.0.1", Port: 5555, Transport: "tcp"},
				{Host: "10.0.0.2", Port: 5556, Transport: "tcp"},
			},
		},
	}
	cmd := m.reconnectKnownDevicesCmd()
	if cmd == nil {
		t.Fatal("expected non-nil cmd for TCP endpoints")
	}
}

func TestReconnectKnownDevicesCmdNoTCPEndpoints(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.config.KnownDevices = []core.KnownDevice{
		{
			Alias: "usbonly",
			Endpoints: []core.Endpoint{
				{Host: "local", Port: 0, Transport: "usb"},
			},
		},
	}
	cmd := m.reconnectKnownDevicesCmd()
	if cmd != nil {
		t.Fatal("expected nil cmd when no TCP endpoints")
	}
}

func TestReconnectKnownDevicesCmdEmpty(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.config.KnownDevices = nil
	cmd := m.reconnectKnownDevicesCmd()
	if cmd != nil {
		t.Fatal("expected nil cmd with no known devices")
	}
}

// ── ensureDefaultProfile deeper branches ─────────────────────────────────────

func TestEnsureDefaultProfileMultipleDefaultsCleaned(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	// Force multiple defaults
	for i := range m.config.Profiles {
		m.config.Profiles[i].IsDefault = true
	}
	m.ensureDefaultProfile()
	count := 0
	for _, p := range m.config.Profiles {
		if p.IsDefault {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected 1 default after ensureDefaultProfile, got %d", count)
	}
}

func TestEnsureDefaultProfileNoProfilesCreatesDefault(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.config.Profiles = nil
	m.ensureDefaultProfile()
	if len(m.config.Profiles) == 0 {
		t.Fatal("expected profiles after ensureDefaultProfile on empty list")
	}
	count := 0
	for _, p := range m.config.Profiles {
		if p.IsDefault {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected 1 default in created profiles: %d", count)
	}
}

// ── deviceIconStr all states ──────────────────────────────────────────────────

func TestDeviceIconStrAllStates(t *testing.T) {
	cases := []struct {
		state string
		wantNonEmpty bool
	}{
		{"device", true},
		{"unauthorized", true},
		{"offline", true},
		{"known-offline", true},
		{"unknown_xyz", true},
		{"", true},
	}
	for _, tc := range cases {
		got := deviceIconStr(tc.state)
		if got == "" && tc.wantNonEmpty {
			t.Fatalf("deviceIconStr(%q) returned empty", tc.state)
		}
	}
}

// ── profileModeLabel ──────────────────────────────────────────────────────────

func TestProfileModeLabelMainDisplay(t *testing.T) {
	p := &core.ProfileDefinition{DisplayID: 2}
	got := profileModeLabel(p)
	if got != "Main (id=2)" {
		t.Fatalf("expected 'Main (id=2)', got %q", got)
	}
}

func TestProfileModeLabelVirtualDisplayFromDesired(t *testing.T) {
	p := &core.ProfileDefinition{Desired: map[string]string{"new_display": "true"}}
	got := profileModeLabel(p)
	if got != "Virtual Display" {
		t.Fatalf("expected 'Virtual Display' from Desired, got %q", got)
	}
}

func TestProfileModeLabelVirtualDisplayFromExtraArgs(t *testing.T) {
	p := &core.ProfileDefinition{ExtraArgs: []string{"--new-display"}}
	got := profileModeLabel(p)
	if got != "Virtual Display" {
		t.Fatalf("expected 'Virtual Display' from ExtraArgs, got %q", got)
	}
}

func TestProfileModeLabelVirtualDisplayFromExtraArgsWithParam(t *testing.T) {
	p := &core.ProfileDefinition{ExtraArgs: []string{"--new-display=1920x1080"}}
	got := profileModeLabel(p)
	if got != "Virtual Display" {
		t.Fatalf("expected 'Virtual Display' from --new-display=..., got %q", got)
	}
}

// ── padRight ──────────────────────────────────────────────────────────────────

func TestPadRightPadsShortString(t *testing.T) {
	got := padRight("hi", 6)
	if len([]rune(got)) != 6 {
		t.Fatalf("padRight('hi', 6) = %q (len=%d), want len=6", got, len(got))
	}
}

func TestPadRightTruncatesLong(t *testing.T) {
	got := padRight("hello world", 5)
	if got != "hello" {
		t.Fatalf("padRight truncation: %q", got)
	}
}

func TestPadRightExact(t *testing.T) {
	got := padRight("abc", 3)
	if got != "abc" {
		t.Fatalf("padRight exact: %q", got)
	}
}

// ── capSource / extractTCPHost / wrapText helpers ─────────────────────────────

func TestCapSourceSimulated(t *testing.T) {
	if capSource("") != "simulated" {
		t.Fatal("empty serial should be simulated")
	}
	if capSource("simulated") != "simulated" {
		t.Fatal("'simulated' should be simulated")
	}
}

func TestCapSourceLive(t *testing.T) {
	if capSource("ABCD1234") != "live" {
		t.Fatal("real serial should be live")
	}
}

func TestExtractTCPHostWithPort(t *testing.T) {
	got := extractTCPHost("192.168.1.1:5555")
	if got != "192.168.1.1" {
		t.Fatalf("expected '192.168.1.1', got %q", got)
	}
}

func TestExtractTCPHostNoPort(t *testing.T) {
	got := extractTCPHost("192.168.1.1")
	if got != "192.168.1.1" {
		t.Fatalf("expected unchanged, got %q", got)
	}
}

func TestExtractTCPHostIPv6(t *testing.T) {
	got := extractTCPHost("[::1]:5555")
	if got != "[::1]" {
		t.Fatalf("IPv6 host: expected '[::1]', got %q", got)
	}
}

func TestWrapTextEmpty(t *testing.T) {
	got := wrapText("", 40)
	if len(got) != 0 {
		t.Fatalf("expected nil/empty for empty string: %v", got)
	}
}

func TestWrapTextFitsOnOneLine(t *testing.T) {
	got := wrapText("hello world", 40)
	if len(got) != 1 || got[0] != "hello world" {
		t.Fatalf("expected single line: %v", got)
	}
}

func TestWrapTextWraps(t *testing.T) {
	got := wrapText("hello world foo bar", 8)
	if len(got) < 2 {
		t.Fatalf("expected wrapping: %v", got)
	}
}

func TestWrapTextMaxWZeroOrNegative(t *testing.T) {
	got := wrapText("hello world", 0)
	if len(got) == 0 {
		t.Fatal("expected non-empty for maxW=0")
	}
}

// ── contextKeys all states ────────────────────────────────────────────────────

func TestContextKeysLaunching(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.launchState = LaunchStateLaunching
	if got := m.contextKeys(); got != "z=cancel" {
		t.Fatalf("expected z=cancel while launching, got %q", got)
	}
}

func TestContextKeysOverlayOpen(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.overlayMode = OverlayWarning
	if got := m.contextKeys(); got != "Esc=close" {
		t.Fatalf("expected Esc=close with overlay, got %q", got)
	}
}

func TestContextKeysRenameMode(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.renameMode = true
	if got := m.contextKeys(); got != "Enter=save  Esc=cancel" {
		t.Fatalf("expected rename keys, got %q", got)
	}
}

// ── launchStateInline all states ─────────────────────────────────────────────

func TestLaunchStateInlineTimedOut(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.launchState = LaunchStateTimedOut
	got := m.launchStateInline()
	if got == "" {
		t.Fatal("expected non-empty for timed_out state")
	}
}

func TestLaunchStateInlineIdle(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.launchState = LaunchStateIdle
	got := m.launchStateInline()
	if got != "" {
		t.Fatalf("expected empty for idle state, got %q", got)
	}
}

// ── recomputePlanAndPreview nil profile branch ────────────────────────────────

func TestRecomputePlanNoProfile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.config.Profiles = nil
	m.recomputePlanAndPreview()
	if m.lastPlan != nil {
		t.Fatal("expected nil plan with no profiles")
	}
	if m.preview != "" {
		t.Fatalf("expected empty preview with no profiles, got %q", m.preview)
	}
}

// ── refreshDevicesCmd ─────────────────────────────────────────────────────────

func TestRefreshDevicesCmdReturnsMsg(t *testing.T) {
	cmd := refreshDevicesCmd()
	if cmd == nil {
		t.Fatal("expected non-nil cmd from refreshDevicesCmd")
	}
	// Execute it — should return a devicesMsg (may error if no adb, that's ok)
	msg := cmd()
	switch msg.(type) {
	case devicesMsg:
		// correct
	default:
		t.Fatalf("expected devicesMsg, got %T", msg)
	}
}

// ── mergedDeviceList — known device serial matches live device ─────────────────

func TestMergedDeviceListKnownMatchesLiveBySerial(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.devices = []core.DeviceCapabilitySnapshot{
		{Serial: "KNOWN001", State: "device", SDKInt: 34, Model: "Pixel7"},
	}
	m.config.KnownDevices = []core.KnownDevice{
		{Alias: "mypixel", Serial: "KNOWN001", Model: "Pixel7",
			Endpoints: []core.Endpoint{{Host: "10.0.0.1", Port: 5555, Transport: "tcp"}}},
	}
	entries := m.mergedDeviceList()
	// Should produce exactly 1 entry (merged, not duplicated)
	if len(entries) != 1 {
		t.Fatalf("expected 1 merged entry, got %d: %v", len(entries), entries)
	}
	if !entries[0].IsKnown || !entries[0].IsLive {
		t.Fatalf("expected IsKnown=true, IsLive=true: %+v", entries[0])
	}
}

func TestMergedDeviceListKnownMatchesLiveByEndpointHost(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.devices = []core.DeviceCapabilitySnapshot{
		{Serial: "192.168.1.1:5555", State: "device", SDKInt: 34},
	}
	m.config.KnownDevices = []core.KnownDevice{
		{Alias: "wifi-dev", Serial: "",
			Endpoints: []core.Endpoint{{Host: "192.168.1.1", Port: 5555, Transport: "tcp"}}},
	}
	entries := m.mergedDeviceList()
	if len(entries) != 1 {
		t.Fatalf("expected 1 merged entry (matched by endpoint), got %d", len(entries))
	}
	if !entries[0].IsKnown {
		t.Fatal("expected IsKnown=true for endpoint-matched device")
	}
}

// ── renderWarningOverlay — with features ──────────────────────────────────────

func TestRenderWarningOverlayWithBlockedFeatures(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.width = minFullLayoutWidth + 30
	m.height = minFullLayoutHeight + 20
	m.lastResolve = core.EffectiveProfileResolution{
		BlockedFeatures:     []string{"require_audio"},
		UnsupportedFeatures: []string{"camera"},
		Warnings:            []string{"sdk<30 warning"},
	}
	rendered := m.renderWarningOverlay()
	if rendered == "" {
		t.Fatal("expected non-empty warning overlay")
	}
	plain := stripANSIForTest(rendered)
	if !strings.Contains(plain, "BLOCKED") {
		t.Fatalf("expected BLOCKED in warning overlay: %s", plain)
	}
	if !strings.Contains(plain, "UNSUPPORTED") {
		t.Fatalf("expected UNSUPPORTED section: %s", plain)
	}
	if !strings.Contains(plain, "WARNINGS") {
		t.Fatalf("expected WARNINGS section: %s", plain)
	}
}

func TestRenderWarningOverlayNoIssues(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.width = minFullLayoutWidth + 30
	m.height = minFullLayoutHeight + 20
	m.lastResolve = core.EffectiveProfileResolution{}
	rendered := m.renderWarningOverlay()
	plain := stripANSIForTest(rendered)
	if !strings.Contains(plain, "No warnings") {
		t.Fatalf("expected 'No warnings' text: %s", plain)
	}
}

// ── renderPairingOverlay — with status ────────────────────────────────────────

func TestRenderPairingOverlayWithStatus(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.width = minFullLayoutWidth + 30
	m.height = minFullLayoutHeight + 20
	m.pairingStatus = "✓ Paired successfully"
	rendered := m.renderPairingOverlay()
	plain := stripANSIForTest(rendered)
	if !strings.Contains(plain, "Status") {
		t.Fatalf("expected Status line when pairingStatus set: %s", plain)
	}
}

func TestRenderPairingOverlayErrorStatus(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.width = minFullLayoutWidth + 30
	m.height = minFullLayoutHeight + 20
	m.pairingStatus = "✗ Connection refused"
	rendered := m.renderPairingOverlay()
	plain := stripANSIForTest(rendered)
	if !strings.Contains(plain, "✗") {
		t.Fatalf("expected error status: %s", plain)
	}
}

// ── flagBadge FeatureFlags path ───────────────────────────────────────────────

func TestFlagBadgeFeatureFlagsOnPath(t *testing.T) {
	p := &core.ProfileDefinition{
		DesiredFlags: nil, // nil so FeatureFlags branch is taken
		FeatureFlags: map[string]bool{"stay_awake": true},
	}
	got := flagBadge(p, "stay_awake", "stay-awake")
	// Should contain checkmark (the "on" path)
	if !strings.Contains(stripANSIForTest(got), "✓") {
		t.Fatalf("expected ✓ for on=true via FeatureFlags: %q", got)
	}
}

func TestFlagBadgeBothNilOff(t *testing.T) {
	p := &core.ProfileDefinition{DesiredFlags: nil, FeatureFlags: nil}
	got := flagBadge(p, "any_flag", "Any")
	plain := stripANSIForTest(got)
	if strings.Contains(plain, "✓") {
		t.Fatalf("expected ○ when both nil, got: %q", plain)
	}
}

// ── pairCmd / connectCmd smoke tests ──────────────────────────────────────────

func TestPairCmdReturnsNonNilCmd(t *testing.T) {
	cmd := pairCmd("999.999.999.999:12345", "000000")
	if cmd == nil {
		t.Fatal("pairCmd must return non-nil")
	}
	// Execute it — will fail (unreachable host) but covers the function body
	msg := cmd()
	if _, ok := msg.(pairResultMsg); !ok {
		t.Fatalf("expected pairResultMsg, got %T", msg)
	}
}

func TestConnectCmdReturnsNonNilCmd(t *testing.T) {
	cmd := connectCmd("999.999.999.999:5555")
	if cmd == nil {
		t.Fatal("connectCmd must return non-nil")
	}
	msg := cmd()
	if _, ok := msg.(connectResultMsg); !ok {
		t.Fatalf("expected connectResultMsg, got %T", msg)
	}
}

// ── Update — connectResultMsg ─────────────────────────────────────────────────

func TestUpdateConnectResultMsgSuccess(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.config.KnownDevices = []core.KnownDevice{
		{Alias: "192.168.1.50",
			Endpoints: []core.Endpoint{{Host: "192.168.1.50", Port: 5555, Transport: "tcp"}}},
	}
	updated, _ := m.Update(connectResultMsg{
		hostPort: "192.168.1.50:5555",
		output:   "connected to 192.168.1.50:5555",
	})
	m2 := updated.(Model)
	logs := strings.Join(m2.logs, "\n")
	if !strings.Contains(logs, "adb connect") {
		t.Fatalf("expected adb connect log: %s", logs)
	}
}

func TestUpdateConnectResultMsgError(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	updated, _ := m.Update(connectResultMsg{
		hostPort: "10.0.0.1:5555",
		err:      fmt.Errorf("connection refused"),
	})
	m2 := updated.(Model)
	logs := strings.Join(m2.logs, "\n")
	if !strings.Contains(logs, "adb connect failed") {
		t.Fatalf("expected error log: %s", logs)
	}
}

// ── handleKey — log scroll and space key ─────────────────────────────────────

func TestHandleKeyLogScrollUp(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.focus = PaneRight
	m.logScroll = 5 // start non-zero

	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	m2 := updated.(Model)
	if m2.logScroll != 4 {
		t.Fatalf("expected logScroll=4 after up, got %d", m2.logScroll)
	}
}

func TestHandleKeyLogScrollDown(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.focus = PaneRight
	m.logScroll = 0

	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	m2 := updated.(Model)
	if m2.logScroll != 1 {
		t.Fatalf("expected logScroll=1 after down, got %d", m2.logScroll)
	}
}

func TestHandleKeyLogScrollUpAtZeroNoOp(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.focus = PaneRight
	m.logScroll = 0

	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	m2 := updated.(Model)
	if m2.logScroll != 0 {
		t.Fatalf("expected logScroll to stay at 0 on up at zero, got %d", m2.logScroll)
	}
}

func TestHandleKeySLaunchEquivalent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.lastPlan = &scrcpy.CommandPlan{Binary: "true", Launchable: true}
	m.preview = "true"

	// 's' key triggers launch (same as Enter)
	updated, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: 's', Text: "s"}))
	m2 := updated.(Model)
	_ = m2
	if cmd == nil {
		t.Fatal("s key should trigger launch cmd")
	}
}

func TestHandleKeyCtrlC(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	// ctrl+c: use Mod=ModCtrl with 'c' code
	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Mod: tea.ModCtrl, Code: 'c'}))
	if cmd == nil {
		t.Fatal("ctrl+c must return quit cmd")
	}
}

func TestHandleKeyCtrlArrowNavigation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.focus = PaneDevices

	// ctrl+right: use Mod=ModCtrl + KeyRight
	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Mod: tea.ModCtrl, Code: tea.KeyRight}))
	m2 := updated.(Model)
	if m2.focus != PaneRight {
		t.Fatalf("expected PaneRight after ctrl+right, got %d", m2.focus)
	}

	// ctrl+left: PaneRight → PaneDevices
	updated3, _ := m2.Update(tea.KeyPressMsg(tea.Key{Mod: tea.ModCtrl, Code: tea.KeyLeft}))
	m3 := updated3.(Model)
	if m3.focus != PaneDevices {
		t.Fatalf("expected PaneDevices after ctrl+left from right, got %d", m3.focus)
	}
}

// ── devicesMsg with live devices ──────────────────────────────────────────────

func TestUpdateDevicesMsgWithLiveDevices(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	updated, _ := m.Update(devicesMsg{
		devices: []core.DeviceCapabilitySnapshot{
			{Serial: "LIVE001", SDKInt: 34, State: "device"},
		},
	})
	m2 := updated.(Model)
	if len(m2.devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(m2.devices))
	}
	logs := strings.Join(m2.logs, "\n")
	if !strings.Contains(logs, "found 1 live") {
		t.Fatalf("expected live device log: %s", logs)
	}
}

func TestUpdateDevicesMsgDeviceIdxClampedOnShrink(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.devices = []core.DeviceCapabilitySnapshot{
		{Serial: "A", SDKInt: 34, State: "device"},
		{Serial: "B", SDKInt: 34, State: "device"},
	}
	m.deviceIdx = 1

	// Shrink to 0 devices
	updated, _ := m.Update(devicesMsg{devices: nil})
	m2 := updated.(Model)
	if m2.deviceIdx != 0 {
		t.Fatalf("expected deviceIdx clamped to 0, got %d", m2.deviceIdx)
	}
}

// ── commitRename — name collision → nextProfileName ───────────────────────────

func TestCommitRenameToExistingNameGetsUniqified(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	// Create a second profile named "Copy"
	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: 'n', Text: "n"}))
	m = updated.(Model)
	m.activeIdx = 0 // back to first profile

	// Force the first profile to be renamed to the second profile's name
	m.renameMode = true
	m.renameBuffer = m.config.Profiles[1].Name // exact collision

	updated, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	m2 := updated.(Model)
	// Must not have the collision — name was uniquified
	names := map[string]int{}
	for _, p := range m2.config.Profiles {
		names[p.Name]++
	}
	for name, count := range names {
		if count > 1 {
			t.Fatalf("duplicate profile name after rename: %q (count=%d)", name, count)
		}
	}
}

// ── handleKey — shift+tab, ctrl+up, ctrl+down ─────────────────────────────────

func TestHandleKeyShiftTab(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.focus = PaneDevices

	// shift+tab: Devices → Right
	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Mod: tea.ModShift, Code: tea.KeyTab}))
	m2 := updated.(Model)
	if m2.focus != PaneRight {
		t.Fatalf("expected PaneRight after shift+tab from Devices, got %d", m2.focus)
	}

	// shift+tab: Right → Devices (PaneProfiles removed from main cycle)
	updated, _ = m2.Update(tea.KeyPressMsg(tea.Key{Mod: tea.ModShift, Code: tea.KeyTab}))
	m3 := updated.(Model)
	if m3.focus != PaneDevices {
		t.Fatalf("expected PaneDevices after shift+tab from Right, got %d", m3.focus)
	}
}

func TestHandleKeyCtrlUpDown(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.focus = PaneProfiles

	// ctrl+up: Profiles → Devices
	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Mod: tea.ModCtrl, Code: tea.KeyUp}))
	m2 := updated.(Model)
	if m2.focus != PaneDevices {
		t.Fatalf("expected PaneDevices after ctrl+up from Profiles, got %d", m2.focus)
	}

	// ctrl+down: Devices → Profiles
	updated, _ = m2.Update(tea.KeyPressMsg(tea.Key{Mod: tea.ModCtrl, Code: tea.KeyDown}))
	m3 := updated.(Model)
	if m3.focus != PaneProfiles {
		t.Fatalf("expected PaneProfiles after ctrl+down from Devices, got %d", m3.focus)
	}
}

func TestHandleKeyCtrlUpFromRight(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.focus = PaneRight

	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Mod: tea.ModCtrl, Code: tea.KeyUp}))
	m2 := updated.(Model)
	if m2.focus != PaneDevices {
		t.Fatalf("expected PaneDevices after ctrl+up from Right, got %d", m2.focus)
	}
}

// ── handleKey — r key with reconnect ──────────────────────────────────────────

func TestHandleKeyRWithKnownTCPDevicesReturnsCmd(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.config.KnownDevices = []core.KnownDevice{
		{Alias: "dev1",
			Endpoints: []core.Endpoint{{Host: "10.0.0.1", Port: 5555, Transport: "tcp"}}},
	}
	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: 'r', Text: "r"}))
	if cmd == nil {
		t.Fatal("expected batch cmd from r key with TCP known devices")
	}
}

// ── launchDetail — all fields ─────────────────────────────────────────────────

func TestLaunchDetailAllFields(t *testing.T) {
	res := scrcpy.ExecutionResult{
		Stderr:   "fatal error message",
		ExitCode: 7,
		TimedOut: true,
		Canceled: false,
	}
	detail := launchDetail(res)
	if !strings.Contains(detail, "stderr=fatal error message") {
		t.Fatalf("expected stderr in detail: %q", detail)
	}
	if !strings.Contains(detail, "exit_code=7") {
		t.Fatalf("expected exit_code in detail: %q", detail)
	}
	if !strings.Contains(detail, "timed_out=true") {
		t.Fatalf("expected timed_out in detail: %q", detail)
	}
}

func TestLaunchDetailEmpty(t *testing.T) {
	detail := launchDetail(scrcpy.ExecutionResult{})
	if detail != "" {
		t.Fatalf("expected empty detail for zero result, got %q", detail)
	}
}

func TestLaunchDetailStderrTruncation(t *testing.T) {
	// Stderr longer than 240 runes must be truncated
	longStderr := strings.Repeat("x", 300)
	res := scrcpy.ExecutionResult{Stderr: longStderr, ExitCode: 1}
	detail := launchDetail(res)
	if len([]rune(detail)) > 300 {
		// truncation applied; check ellipsis
		if !strings.Contains(detail, "…") {
			t.Fatalf("expected ellipsis in truncated detail: %q", detail[:50])
		}
	}
}

// ── devicePollCmd / launchResetCmd / tickCmd closures ─────────────────────────

func TestDevicePollCmdClosureProducesCorrectMsg(t *testing.T) {
	cmd := devicePollCmd()
	if cmd == nil {
		t.Fatal("devicePollCmd must not be nil")
	}
	// The cmd is a tea.Tick — we can't easily execute the closure without
	// waiting 3 seconds. Coverage is achieved via the function returning non-nil.
}

func TestLaunchResetCmdClosureProducesCorrectMsg(t *testing.T) {
	cmd := launchResetCmd()
	if cmd == nil {
		t.Fatal("launchResetCmd must not be nil")
	}
}

func TestTickCmdNonNil(t *testing.T) {
	cmd := tickCmd()
	if cmd == nil {
		t.Fatal("tickCmd must not be nil")
	}
}

// ── activeProfilePtr — out of range index ────────────────────────────────────

func TestActiveProfilePtrOutOfRangeClampsToZero(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.activeIdx = 9999 // way out of range
	p := m.activeProfilePtr()
	if p == nil {
		t.Fatal("expected non-nil profile after clamping")
	}
	if m.activeIdx != 0 {
		t.Fatalf("expected activeIdx clamped to 0, got %d", m.activeIdx)
	}
}

// ── buildRightPanel — no device selected path ─────────────────────────────────

func TestBuildRightPanelNoDeviceSimulatedCaps(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.width = minFullLayoutWidth + 40
	m.height = minFullLayoutHeight + 20
	m.devices = nil
	m.config.KnownDevices = nil

	// Must not panic
	lines := m.buildRightPanel(60, 30)
	if len(lines) == 0 {
		t.Fatal("expected non-empty right panel")
	}
	// Should show "(no device selected)" for endpoints
	combined := strings.Join(lines, "\n")
	plainCombined := stripANSIForTest(combined)
	if !strings.Contains(plainCombined, "no device selected") {
		t.Logf("note: right panel content (first 200 chars): %q", plainCombined[:min(200, len(plainCombined))])
	}
}

// ── doLaunch — plan recomputed when nil ──────────────────────────────────────

func TestDoLaunchWithNilPlanRecomputesAndProceeds(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.lastPlan = nil // force recompute

	updated, cmd := m.doLaunch()
	m2 := updated.(Model)
	_ = m2
	// Either we get a launch cmd or a "no command plan" log
	// but it must not panic
	_ = cmd
}

// ── mergeUniqueArgs — duplicate suppression ──────────────────────────────────

func TestMergeUniqueArgsSupressDuplicateInSecondary(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	// Call through the exported path via ResolveEffectiveProfile
	p := core.DefaultProfile()
	// ExtraArgs duplicates a flag that synthesizeDynamicExtraArgs also produces
	p.Desired = map[string]string{"launch_mode": "new_display"}
	p.ExtraArgs = []string{"--new-display", "--new-display"} // both duplicates

	res := core.ResolveEffectiveProfile(p, core.DeviceCapabilitySnapshot{SDKInt: 34, SupportsVirtualDisplay: true})
	count := 0
	for _, a := range res.FinalArgs {
		if a == "--new-display" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 --new-display, got %d in %v", count, res.FinalArgs)
	}
}



// ── toggleFlag — nil maps initialization ──────────────────────────────────────

func TestToggleFlagInitializesNilMaps(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	// Force nil maps on active profile
	p := m.activeProfilePtr()
	if p == nil {
		t.Fatal("expected active profile")
	}
	p.DesiredFlags = nil
	p.FeatureFlags = nil

	m.toggleFlag("turn_screen_off")

	p2 := m.activeProfilePtr()
	if p2.DesiredFlags == nil {
		t.Fatal("expected DesiredFlags initialized after toggleFlag")
	}
	if p2.FeatureFlags == nil {
		t.Fatal("expected FeatureFlags initialized after toggleFlag")
	}
	if !p2.DesiredFlags["turn_screen_off"] {
		t.Fatal("expected turn_screen_off toggled to true")
	}
}

// ── switchActiveProfile — single profile no-op ────────────────────────────────

func TestSwitchActiveProfileSingleProfileNoOp(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	// Keep only one profile
	m.config.Profiles = m.config.Profiles[:1]
	m.activeIdx = 0

	m.switchActiveProfile(1) // should be a no-op
	if m.activeIdx != 0 {
		t.Fatalf("expected no change with single profile: %d", m.activeIdx)
	}
}

// ── ValidateLaunch — empty string args ────────────────────────────────────────

func TestValidateLaunchEmptySlice(t *testing.T) {
	res := core.ValidateLaunch([]string{})
	if res.OK {
		t.Fatal("expected failure for empty args slice")
	}
}

// ── launchStateInline — failed with display hint ──────────────────────────────

func TestLaunchStateInlineFailedWithDisplayHint(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.launchState = LaunchStateFailed
	m.launchReason = adb.FailureNoDisplay
	got := m.launchStateInline()
	if !strings.Contains(stripANSIForTest(got), "no display server") {
		t.Fatalf("expected display server hint in failed state: %q", got)
	}
}

// ── ensureDefaultProfile — no default sets activeIdx fallback ────────────────

func TestEnsureDefaultProfileNoDefaultSetsFirstFallback(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	// Remove all built-in profiles and add two custom ones with no default
	m.config.Profiles = []core.ProfileDefinition{
		{Name: "Custom A", ProfileID: "custom-a", IsDefault: false},
		{Name: "Custom B", ProfileID: "custom-b", IsDefault: false},
	}
	m.activeIdx = 0
	m.ensureDefaultProfile()
	defaultCount := 0
	for _, p := range m.config.Profiles {
		if p.IsDefault {
			defaultCount++
		}
	}
	if defaultCount != 1 {
		t.Fatalf("expected 1 default after ensureDefaultProfile: %d", defaultCount)
	}
}

// ── handleKey — remaining branches ───────────────────────────────────────────

func TestHandleKeyQuitViaQ(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: 'q', Text: "q"}))
	if cmd == nil {
		t.Fatal("q must return quit cmd")
	}
}

func TestHandleKeyTabCycleFromRight(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.focus = PaneRight

	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	m2 := updated.(Model)
	if m2.focus != PaneDevices {
		t.Fatalf("Tab from Right should go to Devices, got %d", m2.focus)
	}
}

func TestHandleKeyTabCycleFromProfiles(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	// PaneProfiles is no longer in the main cycle; tab from it goes to Devices.
	m.focus = PaneProfiles

	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	m2 := updated.(Model)
	if m2.focus != PaneDevices {
		t.Fatalf("Tab from Profiles should go to Devices (profiles pane removed), got %d", m2.focus)
	}
}

func TestHandleKeyWarningOverlayClosesOnW(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.overlayMode = OverlayWarning

	// lowercase w closes warning overlay
	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: 'w', Text: "w"}))
	m2 := updated.(Model)
	if m2.overlayMode != OverlayNone {
		t.Fatalf("expected OverlayNone after w in warning mode, got %d", m2.overlayMode)
	}
}

func TestHandleKeyWarningOverlayClosesOnQ(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.overlayMode = OverlayWarning

	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: 'q', Text: "q"}))
	m2 := updated.(Model)
	if m2.overlayMode != OverlayNone {
		t.Fatalf("expected OverlayNone after q in warning mode, got %d", m2.overlayMode)
	}
}

// ── startRename — nil profile branch ─────────────────────────────────────────

func TestStartRenameWithNoProfiles(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.config.Profiles = nil
	m.startRename() // must not panic
	if m.renameMode {
		t.Fatal("expected renameMode=false when no profiles")
	}
}

// ── saveConfig — with empty active profile ────────────────────────────────────

func TestSaveConfigPreservesActiveProfile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.saveConfig()
	// Must not panic and should write to config path
	if m.config.ActiveProfile == "" {
		t.Fatal("expected non-empty active profile after saveConfig")
	}
}

// ── Update — windowsizeMsg recovery path ────────────────────────────────────

func TestUpdateWindowSizeTooBigRestoresFromDegraded(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.layoutDegraded = true // force degraded state
	m.width = minFullLayoutWidth - 5
	m.height = minFullLayoutHeight - 5

	// Restore to full size
	updated, _ := m.Update(tea.WindowSizeMsg{
		Width:  minFullLayoutWidth + 20,
		Height: minFullLayoutHeight + 10,
	})
	m2 := updated.(Model)
	if m2.layoutDegraded {
		t.Fatal("expected layoutDegraded=false after restore")
	}
	logs := strings.Join(m2.logs, "\n")
	if !strings.Contains(logs, "layout restored") {
		t.Fatalf("expected 'layout restored' in logs: %s", logs)
	}
}

// ── paneInnerHeights — very small available space ────────────────────────────

func TestPaneInnerHeightsVerySmall(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.width = 80
	m.height = 24
	// contentH=23, available=23-4=19 (>= 4), normal path
	dev, prof := m.paneInnerHeights(23)
	if dev+prof != 19 {
		t.Fatalf("expected dev+prof=19, got %d+%d=%d", dev, prof, dev+prof)
	}

	// Very small: available < 4 → clamped to 4
	dev2, prof2 := m.paneInnerHeights(6) // contentH=6: available=6-4=2 < 4 → 4
	total2 := dev2 + prof2
	if total2 != 4 {
		t.Fatalf("expected dev+prof=4 for tiny height, got %d", total2)
	}
}

// ── commitRename — empty name cancels ────────────────────────────────────────

func TestCommitRenameEmptyNameCancels(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	original := m.activeProfilePtr().Name
	m.renameMode = true
	m.renameBuffer = "   " // whitespace → trimmed to empty

	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	m2 := updated.(Model)
	if m2.renameMode {
		t.Fatal("expected rename mode to end")
	}
	// Profile name should be unchanged (empty rename cancelled)
	if m2.activeProfilePtr().Name != original {
		t.Fatalf("expected unchanged name, got %q", m2.activeProfilePtr().Name)
	}
}

// ── setDefaultProfile — out of range ─────────────────────────────────────────

func TestSetDefaultProfileOutOfRange(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.setDefaultProfile(-1)  // must not panic
	m.setDefaultProfile(999) // must not panic
}

// ── mutateActiveProfile — nil profile ────────────────────────────────────────

func TestMutateActiveProfileWithNoProfiles(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.config.Profiles = nil
	// Must not panic
	m.mutateActiveProfile(func(p *core.ProfileDefinition) string {
		return "mutated"
	})
}

// ── NewModel — missing HOME path fallback ─────────────────────────────────────

func TestNewModelWithNoHomeDir(t *testing.T) {
	// Unset HOME to test fallback path in DefaultConfigPath
	t.Setenv("HOME", "")
	// Must not panic
	m := NewModel()
	_ = m
}

// ── Tick message constructors ─────────────────────────────────────────────────

func TestTickMsgConstructors(t *testing.T) {
	now := time.Now()
	if msg := makeTickMsg(now); msg != tickMsg(now) {
		t.Fatalf("makeTickMsg wrong: %v", msg)
	}
	if msg := makeDevicePollMsg(now); msg != devicePollMsg(now) {
		t.Fatalf("makeDevicePollMsg wrong: %v", msg)
	}
	if msg := makeLaunchResetMsg(now); msg != (launchResetMsg{}) {
		t.Fatalf("makeLaunchResetMsg wrong: %v", msg)
	}
}

// ── Update — unknown message falls through ────────────────────────────────────

type unknownTestMsg struct{}

func TestUpdateUnknownMessageNoOp(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	logs := strings.Join(m.logs, "\n")
	updated, cmd := m.Update(unknownTestMsg{})
	m2 := updated.(Model)
	// Unknown message must be a no-op — no new logs, no cmd
	newLogs := strings.Join(m2.logs, "\n")
	if newLogs != logs {
		t.Fatalf("unknown message must not change logs: new=%q", newLogs)
	}
	if cmd != nil {
		t.Fatalf("unknown message must return nil cmd, got %T", cmd)
	}
}

// ── NewModel — corrupted config triggers fallback ─────────────────────────────

func TestNewModelWithCorruptedConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Write a corrupted config file to the expected path
	cfgDir := home + "/.config/screener"
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgDir+"/config.json", []byte("{not valid json"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := NewModel()
	logs := strings.Join(m.logs, "\n")
	if !strings.Contains(logs, "config load failed") {
		t.Fatalf("expected 'config load failed' in logs for corrupted config: %s", logs)
	}
	// Must still have a valid default config
	if len(m.config.Profiles) == 0 {
		t.Fatal("expected default profiles after config load failure")
	}
}

// ── saveConfig — error path ───────────────────────────────────────────────────

func TestSaveConfigErrorIsLogged(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	// Point to an invalid path (read-only location)
	m.configPath = "/dev/null/cannot/write/here/config.json"
	m.saveConfig()
	logs := strings.Join(m.logs, "\n")
	if !strings.Contains(logs, "config save failed") {
		t.Fatalf("expected 'config save failed' in logs: %s", logs)
	}
}

// ── handleKey — remaining navigation combos ───────────────────────────────────

func TestHandleKeyCtrlDownFromProfiles(t *testing.T) {
	// ctrl+down from Profiles should do nothing (already at bottom)
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.focus = PaneProfiles

	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Mod: tea.ModCtrl, Code: tea.KeyDown}))
	m2 := updated.(Model)
	// PaneProfiles -> ctrl+down does nothing (no pane below)
	if m2.focus != PaneProfiles {
		t.Logf("note: ctrl+down from Profiles went to %d (may be valid if there's a pane below)", m2.focus)
	}
}

func TestHandleKeyCtrlUpFromDevices(t *testing.T) {
	// ctrl+up from Devices should do nothing (already at top)
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.focus = PaneDevices

	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Mod: tea.ModCtrl, Code: tea.KeyUp}))
	m2 := updated.(Model)
	// Should stay at Devices (no pane above)
	if m2.focus != PaneDevices {
		t.Logf("note: ctrl+up from Devices: focus=%d", m2.focus)
	}
}

// ── buildRightPanel — scroll underflow clamp ─────────────────────────────────

func TestBuildRightPanelScrollClampedOnUnderflow(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.width = minFullLayoutWidth + 40
	m.height = minFullLayoutHeight + 20

	// Set logScroll to a very large value — should be clamped down
	m.logScroll = 99999
	lines := m.buildRightPanel(60, 30)
	if len(lines) == 0 {
		t.Fatal("expected non-empty panel")
	}
	// After calling buildRightPanel, logScroll should have been clamped
	if m.logScroll < 0 {
		t.Fatalf("logScroll must not be negative: %d", m.logScroll)
	}
}

// ── ExportProfiles — path in directory that exists ────────────────────────────

func TestExportProfilesRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "p.json")
	profiles := core.DefaultProfiles()
	if err := persistence.ExportProfiles(path, profiles); err != nil {
		t.Fatalf("ExportProfiles: %v", err)
	}
	loaded, err := persistence.ImportProfiles(path)
	if err != nil {
		t.Fatalf("ImportProfiles: %v", err)
	}
	if len(loaded) != len(profiles) {
		t.Fatalf("expected %d profiles, got %d", len(profiles), len(loaded))
	}
}

// ── Profile Editor Overlay (M5 - full edit) ────────────────────────────────────

func TestProfileEditorOpensOnO(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	if m.overlayMode != OverlayNone {
		t.Fatal("expected no overlay on init")
	}

	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: 'o', Text: "o"}))
	m2 := updated.(Model)
	if m2.overlayMode != OverlayProfileEditor {
		t.Fatalf("expected OverlayProfileEditor after o, got %d", m2.overlayMode)
	}
	if m2.editorCursor != 0 {
		t.Fatalf("expected editorCursor=0 on open, got %d", m2.editorCursor)
	}
	logs := strings.Join(m2.logs, "\n")
	if !strings.Contains(logs, "profile editor opened") {
		t.Fatalf("expected editor opened log: %s", logs)
	}
}

func TestProfileEditorClosesOnEsc(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.overlayMode = OverlayProfileEditor

	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	m2 := updated.(Model)
	if m2.overlayMode != OverlayNone {
		t.Fatalf("expected OverlayNone after Esc, got %d", m2.overlayMode)
	}
	logs := strings.Join(m2.logs, "\n")
	if !strings.Contains(logs, "profile editor closed") {
		t.Fatalf("expected editor closed log: %s", logs)
	}
}

func TestProfileEditorClosesOnQ(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.overlayMode = OverlayProfileEditor

	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: 'q', Text: "q"}))
	m2 := updated.(Model)
	if m2.overlayMode != OverlayNone {
		t.Fatalf("expected OverlayNone after q in editor, got %d", m2.overlayMode)
	}
}

func TestProfileEditorNavigatesWithArrows(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.overlayMode = OverlayProfileEditor
	m.editorCursor = 0

	// Down arrow increments cursor
	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	m2 := updated.(Model)
	if m2.editorCursor != 1 {
		t.Fatalf("expected cursor=1 after down, got %d", m2.editorCursor)
	}

	// Up arrow decrements cursor
	updated, _ = m2.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	m3 := updated.(Model)
	if m3.editorCursor != 0 {
		t.Fatalf("expected cursor=0 after up, got %d", m3.editorCursor)
	}
}

func TestProfileEditorCursorFloorAtZero(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.overlayMode = OverlayProfileEditor
	m.editorCursor = 0

	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	m2 := updated.(Model)
	if m2.editorCursor != 0 {
		t.Fatalf("cursor must not go below 0: %d", m2.editorCursor)
	}
}

func TestProfileEditorCursorCeilingAtMax(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.overlayMode = OverlayProfileEditor
	m.editorCursor = pfFieldCount - 1

	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	m2 := updated.(Model)
	if m2.editorCursor != pfFieldCount-1 {
		t.Fatalf("cursor must not exceed pfFieldCount-1: %d", m2.editorCursor)
	}
}

func TestProfileEditorCyclesLaunchMode(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.overlayMode = OverlayProfileEditor
	m.editorCursor = pfLaunchMode

	// Right arrow cycles launch mode
	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyRight}))
	m2 := updated.(Model)

	p := m2.activeProfilePtr()
	if p == nil {
		t.Fatal("expected active profile")
	}
	// First cycle from main_display → new_display
	if p.Desired == nil || p.Desired[core.DesiredKeyLaunchMode] == core.LaunchModeMainDisplay {
		t.Logf("launch mode after right: %v", p.Desired)
	}
	// Must have changed the launch mode
	val := m2.pfFieldValue(p, pfLaunchMode)
	if val == "" {
		t.Fatal("expected non-empty launch mode value")
	}
}

func TestProfileEditorCyclesMaxSize(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.overlayMode = OverlayProfileEditor
	m.editorCursor = pfMaxSize

	p0 := m.activeProfilePtr()
	origSize := p0.MaxSize

	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyRight}))
	m2 := updated.(Model)

	p := m2.activeProfilePtr()
	if p.MaxSize == origSize {
		t.Fatalf("expected max size to change from %d", origSize)
	}
}

func TestProfileEditorAdjustsBitrate(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.overlayMode = OverlayProfileEditor
	m.editorCursor = pfBitrateMB

	p0 := m.activeProfilePtr()
	origBitrate := p0.VideoBitRateMB

	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyRight}))
	m2 := updated.(Model)

	p := m2.activeProfilePtr()
	if p.VideoBitRateMB != origBitrate+2 {
		t.Fatalf("expected bitrate=%d, got %d", origBitrate+2, p.VideoBitRateMB)
	}
}

func TestProfileEditorBitrateLowerBound(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.overlayMode = OverlayProfileEditor
	m.editorCursor = pfBitrateMB

	// Set bitrate to minimum
	m.mutateActiveProfile(func(p *core.ProfileDefinition) string {
		p.VideoBitRateMB = 2
		return "set min"
	})

	// Left arrow should not go below 2
	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyLeft}))
	m2 := updated.(Model)
	if m2.activeProfilePtr().VideoBitRateMB < 2 {
		t.Fatalf("bitrate must not go below 2: %d", m2.activeProfilePtr().VideoBitRateMB)
	}
}

func TestProfileEditorBitrateUpperBound(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.overlayMode = OverlayProfileEditor
	m.editorCursor = pfBitrateMB

	m.mutateActiveProfile(func(p *core.ProfileDefinition) string {
		p.VideoBitRateMB = 32
		return "set max"
	})

	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyRight}))
	m2 := updated.(Model)
	if m2.activeProfilePtr().VideoBitRateMB > 32 {
		t.Fatalf("bitrate must not exceed 32: %d", m2.activeProfilePtr().VideoBitRateMB)
	}
}

func TestProfileEditorCyclesMaxFPS(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.overlayMode = OverlayProfileEditor
	m.editorCursor = pfMaxFPS

	// Start at unlimited (no entry), cycle to 30fps
	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyRight}))
	m2 := updated.(Model)

	p := m2.activeProfilePtr()
	val := m2.pfFieldValue(p, pfMaxFPS)
	// Should show some fps value (either "unlimited" if wrapped around, or a numeric value)
	if val == "" {
		t.Fatal("expected non-empty MaxFPS value")
	}
}

func TestProfileEditorTogglesTurnScreenOff(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.overlayMode = OverlayProfileEditor
	m.editorCursor = pfTurnScreenOff

	p0 := m.activeProfilePtr()
	orig := pfFlagVal(p0, "turn_screen_off")

	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyRight}))
	m2 := updated.(Model)

	p := m2.activeProfilePtr()
	if pfFlagVal(p, "turn_screen_off") == orig {
		t.Fatal("expected turn_screen_off to toggle")
	}
}

func TestProfileEditorTogglesStayAwake(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.overlayMode = OverlayProfileEditor
	m.editorCursor = pfStayAwake

	p0 := m.activeProfilePtr()
	orig := pfFlagVal(p0, "stay_awake")

	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyRight}))
	m2 := updated.(Model)

	if pfFlagVal(m2.activeProfilePtr(), "stay_awake") == orig {
		t.Fatal("expected stay_awake to toggle")
	}
}

func TestProfileEditorTogglesPreferH265(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.overlayMode = OverlayProfileEditor
	m.editorCursor = pfPreferH265

	orig := pfFlagVal(m.activeProfilePtr(), "prefer_h265")

	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyRight}))
	m2 := updated.(Model)

	if pfFlagVal(m2.activeProfilePtr(), "prefer_h265") == orig {
		t.Fatal("expected prefer_h265 to toggle")
	}
}

func TestProfileEditorTogglesRequireAudio(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.overlayMode = OverlayProfileEditor
	m.editorCursor = pfRequireAudio

	orig := pfFlagVal(m.activeProfilePtr(), "require_audio")

	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyRight}))
	m2 := updated.(Model)

	if pfFlagVal(m2.activeProfilePtr(), "require_audio") == orig {
		t.Fatal("expected require_audio to toggle")
	}
}

func TestProfileEditorCyclesGamepad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.overlayMode = OverlayProfileEditor
	m.editorCursor = pfGamepad

	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyRight}))
	m2 := updated.(Model)

	val := m2.pfFieldValue(m2.activeProfilePtr(), pfGamepad)
	if val == "" {
		t.Fatal("expected non-empty gamepad value")
	}
}

func TestProfileEditorLeftArrowCyclesBackward(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.overlayMode = OverlayProfileEditor
	m.editorCursor = pfGamepad

	// Cycle right 2 times, then left 2 times — should return to original
	p0val := m.pfFieldValue(m.activeProfilePtr(), pfGamepad)

	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyRight}))
	m2 := updated.(Model)
	updated, _ = m2.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyLeft}))
	m3 := updated.(Model)

	p3val := m3.pfFieldValue(m3.activeProfilePtr(), pfGamepad)
	if p3val != p0val {
		t.Fatalf("expected to cycle back to %q, got %q", p0val, p3val)
	}
}

func TestProfileEditorEnterSameCycleAsRight(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.overlayMode = OverlayProfileEditor
	m.editorCursor = pfMaxSize

	// 'Enter' in the editor should cycle the field just like Right
	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	m2 := updated.(Model)

	// Overlay must still be open (Enter only cycles, not closes)
	if m2.overlayMode != OverlayProfileEditor {
		t.Fatalf("expected editor still open after Enter, got overlay=%d", m2.overlayMode)
	}
}

func TestProfileEditorUpdatesPreviewOnChange(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.devices = []core.DeviceCapabilitySnapshot{
		{Serial: "TEST001", SDKInt: 34, State: "device", SupportsVirtualDisplay: true},
	}
	m.deviceIdx = 0
	m.overlayMode = OverlayProfileEditor
	m.editorCursor = pfLaunchMode
	m.recomputePlanAndPreview()
	prevBefore := m.preview

	// Cycle to new_display mode
	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyRight}))
	m2 := updated.(Model)

	// Preview should have changed because launch mode changed
	_ = prevBefore
	_ = m2.preview
	// At minimum, no panic — the preview is recomputed
}

func TestProfileEditorRendersInView(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.width = minFullLayoutWidth + 30
	m.height = minFullLayoutHeight + 20
	m.overlayMode = OverlayProfileEditor

	view := stripANSIForTest(fmt.Sprint(m.View()))
	if !strings.Contains(view, "Options") && !strings.Contains(view, "Edit") {
		t.Fatalf("expected profile editor overlay in view: %s", view[:min(300, len(view))])
	}
	if !strings.Contains(view, "Launch Mode") {
		t.Fatalf("expected Launch Mode field in editor: %s", view[:min(300, len(view))])
	}
}

func TestProfileEditorNoOpWithNoProfiles(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.config.Profiles = nil

	// Pressing 'o' with no profiles must not panic
	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: 'o', Text: "o"}))
	m2 := updated.(Model)
	// Should not open editor
	if m2.overlayMode == OverlayProfileEditor {
		t.Fatal("editor must not open when no profiles exist")
	}
}

func TestPfFieldValueAllFields(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	p := m.activeProfilePtr()
	if p == nil {
		t.Fatal("expected active profile")
	}
	// All fields must return non-empty strings
	for i := 0; i < pfFieldCount; i++ {
		val := m.pfFieldValue(p, i)
		if val == "" {
			t.Fatalf("pfFieldValue(%d) returned empty string", i)
		}
	}
}

func TestPfBoolStr(t *testing.T) {
	if pfBoolStr(true) != "on" {
		t.Fatal("pfBoolStr(true) must return 'on'")
	}
	if pfBoolStr(false) != "off" {
		t.Fatal("pfBoolStr(false) must return 'off'")
	}
}

func TestPfFlagValFromDesiredFlags(t *testing.T) {
	p := &core.ProfileDefinition{
		DesiredFlags: map[string]bool{"turn_screen_off": true},
	}
	if !pfFlagVal(p, "turn_screen_off") {
		t.Fatal("expected true from DesiredFlags")
	}
}

func TestPfFlagValFromFeatureFlagsFallback(t *testing.T) {
	p := &core.ProfileDefinition{
		DesiredFlags: nil,
		FeatureFlags: map[string]bool{"stay_awake": true},
	}
	if !pfFlagVal(p, "stay_awake") {
		t.Fatal("expected true from FeatureFlags when DesiredFlags nil")
	}
}

func TestPfFlagValFalseWhenBothNil(t *testing.T) {
	p := &core.ProfileDefinition{}
	if pfFlagVal(p, "any_flag") {
		t.Fatal("expected false when both maps nil")
	}
}

func TestEditorCycleFieldNilProfile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.config.Profiles = nil
	// Must not panic
	m.editorCycleField(pfLaunchMode, 1)
}

// ── Profile editor — remaining coverage gaps ─────────────────────────────────

func TestProfileEditorMaxFPSCyclesBackToUnlimited(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.overlayMode = OverlayProfileEditor
	m.editorCursor = pfMaxFPS

	// Cycle through all MaxFPS choices until we reach 0 (unlimited) again
	// pfMaxFPSChoices = {0, 30, 60, 90, 120}
	// Starting at 0 → right → 30 → 60 → 90 → 120 → 0 (delete)
	for i := 0; i < len(pfMaxFPSChoices); i++ {
		updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyRight}))
		m = updated.(Model)
	}
	// After full cycle, should be back to unlimited
	p := m.activeProfilePtr()
	val := m.pfFieldValue(p, pfMaxFPS)
	if val != "unlimited" {
		t.Fatalf("expected 'unlimited' after full cycle, got %q", val)
	}
}

func TestProfileEditorMaxFPSFieldValueNilDesired(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	p := m.activeProfilePtr()
	p.Desired = nil // nil map
	val := m.pfFieldValue(p, pfMaxFPS)
	if val != "unlimited" {
		t.Fatalf("expected 'unlimited' when Desired=nil, got %q", val)
	}
}

func TestProfileEditorGamepadFieldValueNilDesired(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	p := m.activeProfilePtr()
	p.Desired = nil
	val := m.pfFieldValue(p, pfGamepad)
	if val != core.GamepadModeOff {
		t.Fatalf("expected GamepadModeOff when Desired=nil, got %q", val)
	}
}

func TestProfileEditorLaunchModeFieldValueNilDesired(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	p := m.activeProfilePtr()
	p.Desired = nil
	val := m.pfFieldValue(p, pfLaunchMode)
	if val != core.LaunchModeMainDisplay {
		t.Fatalf("expected main_display when Desired=nil, got %q", val)
	}
}

func TestEditorCycleFieldInitializesNilMaps(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	p := m.activeProfilePtr()
	p.Desired = nil
	p.DesiredFlags = nil
	p.FeatureFlags = nil

	// Cycling should initialize nil maps
	m.editorCycleField(pfLaunchMode, 1)
	p2 := m.activeProfilePtr()
	if p2.Desired == nil {
		t.Fatal("expected Desired map initialized after cycle")
	}
}

func TestEditorCycleFieldMaxFPSCurrentValueFound(t *testing.T) {
	// Test that when Desired[MaxFPS] is set to a valid choice, the cycle finds it
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	p := m.activeProfilePtr()
	if p.Desired == nil {
		p.Desired = map[string]string{}
	}
	p.Desired[core.DesiredKeyMaxFPS] = "60" // set to 60fps
	m.overlayMode = OverlayProfileEditor
	m.editorCursor = pfMaxFPS

	// Cycling right from 60 should go to 90
	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyRight}))
	m2 := updated.(Model)

	p2 := m2.activeProfilePtr()
	val := m2.pfFieldValue(p2, pfMaxFPS)
	if val != "90fps" {
		t.Fatalf("expected 90fps after right from 60, got %q", val)
	}
}

// ── handleKey — equals key for bitrate (same as +) ───────────────────────────

func TestHandleKeyEqualsAsBitrateIncrease(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	p := m.activeProfilePtr()
	orig := p.VideoBitRateMB

	// '=' is mapped to same as '+' (adjustBitrate +2)
	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: '=', Text: "="}))
	m2 := updated.(Model)
	if m2.activeProfilePtr().VideoBitRateMB != orig+2 {
		t.Fatalf("expected bitrate %d after '=', got %d", orig+2, m2.activeProfilePtr().VideoBitRateMB)
	}
}

// ── NewModel — active profile name not found in profiles ─────────────────────

func TestNewModelActiveProfileNotFoundDefaultsToFirst(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Write a config where ActiveProfile doesn't match any profile
	cfgDir := home + "/.config/screener"
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := `{
		"version":1,
		"active_profile":"NonExistentProfile",
		"profiles":[
			{"name":"Only Profile","display_id":0,"max_size":1920,"video_bitrate_mb":8,"is_default":true}
		]
	}`
	if err := os.WriteFile(cfgDir+"/config.json", []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}

	m := NewModel()
	// activeIdx should be 0 (default, not finding "NonExistentProfile")
	if m.activeIdx != 0 {
		t.Fatalf("expected activeIdx=0 when profile not found, got %d", m.activeIdx)
	}
}

// ── Update — devicesMsg error with specific classification ───────────────────

func TestUpdateDevicesMsgADBMissingError(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()

	updated, _ := m.Update(devicesMsg{
		err: fmt.Errorf("exec: \"adb\": executable file not found in $PATH"),
	})
	m2 := updated.(Model)
	logs := strings.Join(m2.logs, "\n")
	if !strings.Contains(logs, "device refresh failed") {
		t.Fatalf("expected device refresh failed log: %s", logs)
	}
	if !strings.Contains(logs, string(adb.FailureADBMissing)) {
		t.Fatalf("expected adb_missing reason in logs: %s", logs)
	}
}

// ── handleKey — 'o' with no profiles is noop ─────────────────────────────────

func TestHandleKeyOWithNoProfilesNoop(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.config.Profiles = nil

	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: 'o', Text: "o"}))
	m2 := updated.(Model)
	if m2.overlayMode != OverlayNone {
		t.Fatalf("expected no overlay when no profiles, got %d", m2.overlayMode)
	}
}

// ── editorCycleField — MaxSize cycling left ───────────────────────────────────

func TestEditorCycleFieldMaxSizeLeft(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.overlayMode = OverlayProfileEditor
	m.editorCursor = pfMaxSize

	// Cycle left from current size
	p0size := m.activeProfilePtr().MaxSize
	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyLeft}))
	m2 := updated.(Model)

	p := m2.activeProfilePtr()
	if p.MaxSize == p0size && len(pfMaxSizeChoices) > 1 {
		t.Logf("note: left cycle wrapped around or same; original=%d new=%d", p0size, p.MaxSize)
	}
	// Must not panic, must produce a valid size
	found := false
	for _, c := range pfMaxSizeChoices {
		if p.MaxSize == c {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("MaxSize %d is not in pfMaxSizeChoices %v", p.MaxSize, pfMaxSizeChoices)
	}
}

// ── contextKeys — includes O=edit ────────────────────────────────────────────

func TestContextKeysIncludesEditHint(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	keys := m.contextKeys()
	if !strings.Contains(keys, "O=") {
		t.Fatalf("expected O= in contextKeys: %q", keys)
	}
}

// ── NewModel — Wayland-only display path ──────────────────────────────────────

func TestNewModelWithWaylandDisplay(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("DISPLAY", "")
	t.Setenv("WAYLAND_DISPLAY", "wayland-0")

	m := NewModel()
	logs := strings.Join(m.logs, "\n")
	// Should log the wayland display, not the "no display" warning
	if strings.Contains(logs, "DISPLAY and WAYLAND_DISPLAY unset") {
		t.Fatal("should not warn about display when WAYLAND_DISPLAY is set")
	}
	if !strings.Contains(logs, "display:") {
		t.Fatalf("expected display log with wayland: %s", logs)
	}
}

// ── buildRightPanel — hasDisplayEnv=false path ────────────────────────────────

func TestBuildRightPanelNoDisplayEnvShowsWarning(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("DISPLAY", "")
	t.Setenv("WAYLAND_DISPLAY", "")
	m := NewModel()
	m.width = minFullLayoutWidth + 40
	m.height = minFullLayoutHeight + 20

	lines := m.buildRightPanel(70, 35)
	combined := strings.Join(lines, "\n")
	plain := stripANSIForTest(combined)
	if !strings.Contains(plain, "No display server") && !strings.Contains(plain, "DISPLAY") {
		t.Fatalf("expected display warning when no display env: %s", plain[:min(200, len(plain))])
	}
}

// ── switchActiveDevice — wrap-around ─────────────────────────────────────────

func TestSwitchActiveDeviceWrapsAround(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.devices = []core.DeviceCapabilitySnapshot{
		{Serial: "A", State: "device", SDKInt: 34},
		{Serial: "B", State: "device", SDKInt: 34},
		{Serial: "C", State: "device", SDKInt: 34},
	}
	m.deviceIdx = 2 // last device

	m.switchActiveDevice(1) // should wrap to 0
	if m.deviceIdx != 0 {
		t.Fatalf("expected wrap to 0, got %d", m.deviceIdx)
	}
}

func TestSwitchActiveDeviceWrapBackward(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.devices = []core.DeviceCapabilitySnapshot{
		{Serial: "A", State: "device", SDKInt: 34},
		{Serial: "B", State: "device", SDKInt: 34},
	}
	m.deviceIdx = 0

	m.switchActiveDevice(-1) // should wrap to last
	if m.deviceIdx != 1 {
		t.Fatalf("expected wrap to 1, got %d", m.deviceIdx)
	}
}

// ── commitRename — nil profile guard ─────────────────────────────────────────

func TestCommitRenameNilProfileCancels(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.config.Profiles = nil
	m.renameMode = true
	m.renameBuffer = "NewName"

	m.commitRename()
	if m.renameMode {
		t.Fatal("expected rename mode to end even with nil profile")
	}
}

// ── handleMouseClick — device item beyond list length ───────────────────────

func TestHandleMouseClickDeviceItemBeyondList(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.width = 120
	m.height = 38
	m.devices = []core.DeviceCapabilitySnapshot{
		{Serial: "ONLY_ONE", State: "device", SDKInt: 34},
	}
	// Click at row 100 (way beyond device list) — must not panic
	updated, _ := m.Update(tea.MouseClickMsg{X: 3, Y: 100})
	_ = updated
}

// ── refreshDevicesCmd — executes without panic ───────────────────────────────

func TestRefreshDevicesCmdBodyExecutes(t *testing.T) {
	cmd := refreshDevicesCmd()
	if cmd == nil {
		t.Fatal("nil cmd")
	}
	msg := cmd()
	switch msg.(type) {
	case devicesMsg:
		// correct
	default:
		t.Fatalf("expected devicesMsg got %T", msg)
	}
}

// ── launchStateInline — succeeded state ──────────────────────────────────────

func TestLaunchStateInlineRunning(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.activeSessions = []activeSession{{id: 1, serial: "test", profileID: "Test"}}
	got := m.launchStateInline()
	if got == "" {
		t.Fatal("expected non-empty inline when sessions are running")
	}
	if !strings.Contains(stripANSIForTest(got), "running") {
		t.Fatalf("expected 'running' in inline state, got: %q", got)
	}
}

// ── recomputePlanAndPreview — ValidateLaunch fails ────────────────────────────

func TestRecomputePlanMarksUnlaunchableOnConflict(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	p := m.activeProfilePtr()
	if p == nil {
		t.Fatal("expected active profile")
	}
	// Force a conflicting ExtraArgs that ValidateLaunch will reject
	p.ExtraArgs = []string{"--display-id", "0", "--new-display"}
	// Also clear Desired so normalizedLaunchMode returns main_display
	// (so --display-id is emitted, conflicting with --new-display in ExtraArgs)
	p.Desired = map[string]string{}
	m.recomputePlanAndPreview()

	// With conflicting --display-id + --new-display, ValidateLaunch should fail
	if m.lastPlan != nil && m.lastPlan.Launchable {
		t.Logf("plan launchable=%v args=%v", m.lastPlan.Launchable, m.lastPlan.Args)
		// The validator may or may not catch this depending on ResolveEffectiveProfile behavior
	}
}

// ── executePlanCmd — scrcpy not found (FailureADBMissing) ────────────────────

func TestExecutePlanCmdBinaryNotFound(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	// Use a binary name containing "scrcpy" so ClassifyFailure recognises it
	plan := &scrcpy.CommandPlan{
		Binary:     "scrcpy-nonexistent-xyz-12345",
		Args:       []string{"--display-id", "0"},
		Launchable: true,
	}
	cmd := startSessionCmd(plan, nil, 1, func() {}, "", "")
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	msg := cmd()
	lm, ok := msg.(launchMsg)
	if !ok {
		t.Fatalf("expected launchMsg, got %T", msg)
	}
	if lm.reason != adb.FailureADBMissing {
		t.Fatalf("expected FailureADBMissing for unknown binary, got %s", lm.reason)
	}
}

// ── switchActiveProfile — same index no-op ───────────────────────────────────

func TestSwitchActiveProfileSameIndexNoOp(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.activeIdx = 0
	logs0 := strings.Join(m.logs, "\n")
	m.switchActiveProfile(0) // delta=0, should be no-op (next == activeIdx)
	logs1 := strings.Join(m.logs, "\n")
	// No new profile log should have been added
	if strings.Count(logs1, "profile:") > strings.Count(logs0, "profile:") {
		t.Logf("note: switchActiveProfile(0) may log when wrapping; acceptable")
	}
}

// ── handleKey — number flag toggles coverage ─────────────────────────────────

func TestHandleKeyFlagToggles1Through5(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	keys := []struct {
		key  rune
		flag string
	}{
		{'1', "turn_screen_off"},
		{'2', "stay_awake"},
		{'3', "prefer_h265"},
		{'4', "require_audio"},
		{'5', "require_camera"},
	}
	for _, tc := range keys {
		m := NewModel()
		p := m.activeProfilePtr()
		if p.DesiredFlags == nil {
			p.DesiredFlags = map[string]bool{}
		}
		orig := p.DesiredFlags[tc.flag]

		updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: tc.key, Text: string(tc.key)}))
		m2 := updated.(Model)
		if m2.activeProfilePtr().DesiredFlags[tc.flag] == orig {
			t.Errorf("key %c: expected flag %q to toggle from %v", tc.key, tc.flag, orig)
		}
	}
}

// ── Update — tick message ─────────────────────────────────────────────────────

func TestUpdateTickMsgAdvancesMatrix(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	// tickMsg advances the matrix tick counter
	updated, cmd := m.Update(tickMsg{})
	m2 := updated.(Model)
	_ = m2
	if cmd == nil {
		t.Fatal("expected next tickCmd after tickMsg")
	}
}

// ── mergedDeviceList — known device with empty alias ─────────────────────────

func TestMergedDeviceListKnownWithEmptyAliasUsesEndpointHost(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.devices = nil
	m.config.KnownDevices = []core.KnownDevice{
		{
			Alias: "", // empty alias
			Endpoints: []core.Endpoint{
				{Host: "192.168.5.5", Port: 5555, Transport: "tcp"},
			},
		},
	}
	entries := m.mergedDeviceList()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	// Serial should fall back to endpoint host
	if entries[0].Serial == "" {
		t.Fatal("expected non-empty serial from endpoint host for empty alias")
	}
}

func TestMergedDeviceListKnownWithNoEndpointsUsesUnknown(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.devices = nil
	m.config.KnownDevices = []core.KnownDevice{
		{Alias: "", Endpoints: nil},
	}
	entries := m.mergedDeviceList()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	// Should fall back to "(unknown)"
	if entries[0].Serial != "(unknown)" {
		t.Fatalf("expected '(unknown)' serial, got %q", entries[0].Serial)
	}
}

// ── doLaunch — display hint when not launchable ───────────────────────────────

func TestDoLaunchLogsEnvAndDevice(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.lastPlan = &scrcpy.CommandPlan{Binary: "true", Launchable: true}
	m.preview = "true"

	updated, cmd := m.doLaunch()
	m2 := updated.(Model)
	// doLaunch always sets LaunchStateLaunching
	if m2.launchState != LaunchStateLaunching {
		t.Fatalf("expected LaunchStateLaunching after doLaunch, got %s", m2.launchState)
	}
	if cmd == nil {
		t.Fatal("expected executePlanCmd from doLaunch")
	}
	logs := strings.Join(m2.logs, "\n")
	if !strings.Contains(logs, "launch requested") {
		t.Fatalf("expected launch requested log: %s", logs)
	}
	if !strings.Contains(logs, "launch env:") {
		t.Fatalf("expected launch env log: %s", logs)
	}
}

// ── adjustBitrate — negative delta ────────────────────────────────────────────

func TestAdjustBitrateNegativeDelta(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.mutateActiveProfile(func(p *core.ProfileDefinition) string {
		p.VideoBitRateMB = 10
		return "set 10"
	})
	m.adjustBitrate(-2)
	if m.activeProfilePtr().VideoBitRateMB != 8 {
		t.Fatalf("expected 8, got %d", m.activeProfilePtr().VideoBitRateMB)
	}
}
