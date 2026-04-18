package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/NamasteJasutin/screener/internal/adb"
	"github.com/NamasteJasutin/screener/internal/core"
	"github.com/NamasteJasutin/screener/internal/persistence"
	"github.com/NamasteJasutin/screener/internal/platform"
	"github.com/NamasteJasutin/screener/internal/render"
	"github.com/NamasteJasutin/screener/internal/scrcpy"
	"github.com/NamasteJasutin/screener/internal/ui"
)

// ── Overlay / focus enums ─────────────────────────────────────────────────────

type OverlayMode int

const (
	OverlayNone          OverlayMode = iota
	OverlayWarning                   // W
	OverlayPairing                   // P (shift)
	OverlayHelp                      // ?
	OverlayProfileEditor             // O
	OverlayOnboarding                // B
	OverlayDeviceEditor              // E (on Devices pane)
	OverlayNicknameEntry             // shown after successful pairing to name the device
	OverlayPortUpdate                // shown when adb connect is refused — port likely changed
	OverlayConfirmDelete             // two-step delete confirmation for devices and profiles
	OverlayProfilePicker             // p — assign a profile to the selected device
)

// FocusedPane controls which panel receives arrow-key navigation.
type FocusedPane int

const (
	PaneDevices  FocusedPane = iota // left-top
	PaneProfiles                    // left-bottom
	PaneRight                       // right panel (read-only scroll)
)

// ── Message types ─────────────────────────────────────────────────────────────

type tickMsg      time.Time
type devicePollMsg time.Time
type launchResetMsg struct{}

type devicesMsg struct {
	devices []core.DeviceCapabilitySnapshot
	err     error
}

type sessionID int

type activeSession struct {
	id        sessionID
	serial    string
	profileID string
	startedAt time.Time
	cancel    context.CancelFunc
}

type launchMsg struct {
	reason    adb.FailureReason
	err       error
	res       scrcpy.ExecutionResult
	// Populated on successful detached start:
	sid       sessionID
	session   *scrcpy.Session
	cancel    context.CancelFunc
	serial    string
	profileID string
}

type sessionExitedMsg struct {
	id  sessionID
	res scrcpy.ExecutionResult
	err error
}

type pairResultMsg struct {
	result   adb.PairResult
	hostPort string
	err      error
}

type connectResultMsg struct {
	hostPort string
	output   string
	err      error
}

type disconnectResultMsg struct {
	hostPort string
}

// ── Launch state ──────────────────────────────────────────────────────────────

type LaunchState string

const (
	LaunchStateIdle      LaunchState = "idle"
	LaunchStateLaunching LaunchState = "launching"
	LaunchStateSucceeded LaunchState = "succeeded"
	LaunchStateFailed    LaunchState = "failed"
	LaunchStateCanceled  LaunchState = "canceled"
	LaunchStateTimedOut  LaunchState = "timed_out"
)

// ── Device entry ──────────────────────────────────────────────────────────────

type DeviceEntry struct {
	Serial  string
	Model   string
	State   string
	SDKInt  int
	IsLive  bool
	IsKnown bool
	Known   *core.KnownDevice
	Caps    core.DeviceCapabilitySnapshot
}

// ── Model ─────────────────────────────────────────────────────────────────────

// ModelOptions configures paths and build metadata passed in from main.
type ModelOptions struct {
	ConfigPath string // overrides XDG default when non-empty
	LogPath    string // overrides XDG default when non-empty
	Version    string // build-time version string for display in help overlay
}

type Model struct {
	width          int
	height         int
	layoutDegraded bool
	focus          FocusedPane
	activeIdx      int // selected profile
	deviceIdx      int // selected device
	logScroll      int // right-panel log scroll offset
	renameMode     bool
	renameBuffer   string
	overlayMode    OverlayMode
	editorCursor   int // focused field index in profile editor overlay
	pairingField       int
	pairingHost        string
	pairingPort        string
	pairingCode        string
	pairingConnectPort string
	pairingStatus      string
	deviceEditorField  int
	deviceEditorBuffer []string // nickname, alias, ep0.host, ep0.port, ep1.host, ep1.port, ...
	nicknameEntryHost  string   // IP of device being nicknamed after pairing
	nicknameInput      string
	portUpdateHost     string // IP of device whose connect port needs updating
	portUpdateInput    string
	portUpdateStatus   string
	deleteTarget       string // display name of item pending deletion
	deleteType         string // "device" or "profile"
	deleteChoice       int    // 0=No (default), 1=Yes
	profilePickerIdx   int    // selected profile index in the profile picker overlay
	onboardingStep  int
	themeName       string
	version        string
	configPath     string
	logPath        string
	logFile        *os.File // kept open for the process lifetime; avoids per-write syscall overhead
	config         persistence.Config
	devices        []core.DeviceCapabilitySnapshot
	logs           []string
	preview        string
	lastPlan       *scrcpy.CommandPlan
	launchState       LaunchState
	launchReason      adb.FailureReason
	launchCancel      context.CancelFunc
	activeSessions    []activeSession
	nextSID           sessionID
	pendingAutoLaunch bool // true while waiting for adb connect to complete before launching
	matrix            *render.MatrixRain
	lastResolve       core.EffectiveProfileResolution
}

const (
	minFullLayoutWidth  = 60
	minFullLayoutHeight = 14
	defaultTheme        = "Matrix"
)

// ── Profile editor field index constants ─────────────────────────────────────

const (
	pfLaunchMode    = 0
	pfMaxSize        = 1
	pfBitrateMB      = 2
	pfMaxFPS         = 3
	pfTurnScreenOff  = 4
	pfStayAwake      = 5
	pfPreferH265     = 6
	pfRequireAudio   = 7
	pfGamepad        = 8
	pfFieldCount     = 9
)

var pfLabels = [pfFieldCount]string{
	"Launch Mode",
	"Max Size",
	"Video Bitrate",
	"Max FPS",
	"Turn Screen Off",
	"Stay Awake",
	"Prefer H265",
	"Require Audio",
	"Gamepad Mode",
}

var pfLaunchModeChoices = []string{
	core.LaunchModeMainDisplay,
	core.LaunchModeNewDisplay,
	core.LaunchModeDexVirtual,
	core.LaunchModeCamera,
}
var pfMaxSizeChoices = []int{0, 720, 1080, 1440, 1920}
var pfMaxFPSChoices = []int{0, 30, 60, 90, 120}
var pfGamepadChoices = []string{core.GamepadModeOff, core.GamepadModeUHID, core.GamepadModeAOA}

// ── Init ──────────────────────────────────────────────────────────────────────

// NewModel returns a Model using default XDG config and log paths.
func NewModel() Model { return NewModelWithOpts(ModelOptions{}) }

// NewModelWithOpts returns a Model using paths and metadata from opts.
// Empty opts fields fall back to XDG defaults.
func NewModelWithOpts(opts ModelOptions) Model {
	configPath := opts.ConfigPath
	if configPath == "" {
		configPath = persistence.DefaultConfigPath()
	}
	logPath := opts.LogPath
	if logPath == "" {
		logPath = persistence.DefaultLogPath()
	}

	// Open log file first so directory errors can be written to it.
	_ = os.MkdirAll(filepath.Dir(logPath), 0o755)
	var logFile *os.File
	if lf, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
		logFile = lf
	} else {
		fmt.Fprintf(os.Stderr, "warn: cannot open log file %s: %v\n", logPath, err)
	}

	cfg, err := persistence.Load(configPath)
	fallback := false
	if err != nil {
		cfg = persistence.DefaultConfig()
		fallback = true
	}

	// Apply persisted theme or default.
	themeName := cfg.Theme
	if themeName == "" {
		themeName = defaultTheme
	}
	t := ui.FindTheme(themeName)
	ui.SetTheme(t)

	m := Model{
		width:       120,
		height:      38,
		focus:       PaneDevices,
		configPath:  configPath,
		logPath:     logPath,
		logFile:     logFile,
		config:      cfg,
		themeName:   themeName,
		version:     opts.Version,
		matrix:      render.NewMatrixRain(120, 38),
		launchState: LaunchStateIdle,
	}
	m.matrix.SetPalette(t.MatrixPalette())
	m.matrix.SetBackground(t.PaneBg)
	m.ensureDefaultProfile()

	for i, p := range m.config.Profiles {
		if p.Name == m.config.ActiveProfile {
			m.activeIdx = i
			break
		}
	}

	if fallback {
		m.appendLog("config load failed; using defaults")
	}
	// Log any config-dir creation error now that log file is open.
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		m.appendLog("warn: could not create config dir: " + err.Error())
	}

	ver := opts.Version
	if ver == "" {
		ver = "dev"
	}
	m.appendLog("screener initialized  version=" + ver + "  theme=" + themeName)

	if !platform.HasDisplay() {
		if w := platform.NoDisplayWarning(); w != "" {
			m.appendLog(w)
		}
		if h := platform.NoDisplayHint(); h != "" {
			m.appendLog(h)
		}
	} else {
		m.appendLog("display: " + platform.DisplayDescription() + "  scrcpy will open window")
	}

	// Preflight: warn immediately if required external binaries are absent.
	if !platform.IsAvailable("adb") {
		m.appendLog("warn: adb not found in PATH — device discovery disabled")
		m.appendLog("hint: install Android SDK Platform-Tools and add to PATH")
	}
	if !platform.IsAvailable("scrcpy") {
		m.appendLog("warn: scrcpy not found in PATH — launch unavailable")
		m.appendLog("hint: install from https://github.com/Genymobile/scrcpy")
	}

	m.recomputePlanAndPreview()
	return m
}

// Cleanup flushes and closes the log file. Call after the Bubble Tea program exits.
func (m Model) Cleanup() {
	if m.logFile != nil {
		_ = m.logFile.Sync()
		_ = m.logFile.Close()
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(tickCmd(), devicePollCmd(), refreshDevicesCmd())
}

// ── Update ────────────────────────────────────────────────────────────────────

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = v.Width, v.Height
		m.matrix.Resize(v.Width, v.Height)
		tooSmall := m.isTerminalTooSmall()
		if tooSmall && !m.layoutDegraded {
			m.layoutDegraded = true
			m.appendLog(fmt.Sprintf("layout degraded %dx%d (min %dx%d) reason=%s",
				m.width, m.height, minFullLayoutWidth, minFullLayoutHeight, adb.FailureTerminalRender))
		}
		if !tooSmall && m.layoutDegraded {
			m.layoutDegraded = false
			m.appendLog(fmt.Sprintf("layout restored %dx%d", m.width, m.height))
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(v)

	case tea.MouseClickMsg:
		return m.handleMouseClick(v)

	case tickMsg:
		m.matrix.Tick()
		return m, tickCmd()

	case devicePollMsg:
		// Automatic background refresh; reschedule immediately.
		return m, tea.Batch(devicePollCmd(), refreshDevicesCmd())

	case launchResetMsg:
		// Auto-clear terminal launch state so operators don't see stale badges.
		switch m.launchState {
		case LaunchStateSucceeded, LaunchStateFailed, LaunchStateCanceled, LaunchStateTimedOut:
			m.setLaunchState(LaunchStateIdle)
			m.launchReason = adb.FailureNone
		}
		return m, nil

	case devicesMsg:
		if v.err != nil {
			reason := adb.ClassifyFailure(v.err)
			m.appendLog("device refresh failed: " + v.err.Error())
			m.appendLog("device refresh reason: " + string(reason))
			return m, nil
		}
		m.devices = v.devices
		// Sync live device IPs and metadata back to KnownDevice records.
		for _, d := range v.devices {
			for i := range m.config.KnownDevices {
				kd := &m.config.KnownDevices[i]
				matched := (kd.Serial == d.Serial && d.Serial != "") ||
					func() bool {
						for _, ep := range kd.Endpoints {
							if fmt.Sprintf("%s:%d", ep.Host, ep.Port) == d.Serial {
								return true
							}
						}
						return false
					}()
				if !matched {
					continue
				}
				kd.LastSeenAt = time.Now()
				if d.Model != "" {
					kd.Model = d.Model
				}
				if d.Manufacturer != "" {
					kd.Manufacturer = d.Manufacturer
				}
				if d.AndroidRelease != "" {
					kd.AndroidRelease = d.AndroidRelease
				}
				if d.DeviceName != "" {
					kd.DeviceName = d.DeviceName
				}
				if d.LocalWiFiIP != "" {
					kd.LocalWiFiIP = d.LocalWiFiIP
				}
				if d.TailscaleIP != "" {
					kd.TailscaleIP = d.TailscaleIP
					// Auto-register tailscale endpoint if not already present.
					hasTailscale := false
					for _, ep := range kd.Endpoints {
						if ep.Name == "Tailscale" || ep.Host == d.TailscaleIP {
							hasTailscale = true
							break
						}
					}
					if !hasTailscale {
						kd.Endpoints = append(kd.Endpoints, core.Endpoint{
							Host: d.TailscaleIP, Port: 5555,
							Transport: "tcp", Name: "Tailscale", Priority: 10,
						})
						m.appendLog("auto-registered tailscale endpoint: " + d.TailscaleIP)
					}
				}
			}
		}
		if entries := m.mergedDeviceList(); m.deviceIdx >= len(entries) {
			m.deviceIdx = 0
		}
		m.recomputePlanAndPreview()
		m.appendLog(fmt.Sprintf("found %d live / %d known device(s)",
			len(v.devices), len(m.config.KnownDevices)))

		// If a launch was requested while the device was offline, try now.
		if m.pendingAutoLaunch {
			m.pendingAutoLaunch = false
			entries := m.mergedDeviceList()
			if m.deviceIdx < len(entries) && entries[m.deviceIdx].IsLive {
				m.appendLog("auto-connect succeeded — launching")
				return m.doLaunch()
			}
			m.appendLog("auto-connect failed: device still offline")
			return m, nil
		}

		// Background reconnect: if no USB/TCP devices are visible, try known endpoints.
		if len(v.devices) == 0 {
			if cmd := m.reconnectKnownDevicesCmd(); cmd != nil {
				return m, cmd
			}
		}
		return m, nil

	case launchMsg:
		m.launchCancel = nil
		m.launchReason = v.reason
		if v.session != nil {
			// Process started successfully — track it and return to idle immediately
			// so the user can launch additional sessions.
			m.activeSessions = append(m.activeSessions, activeSession{
				id:        v.sid,
				serial:    v.serial,
				profileID: v.profileID,
				startedAt: time.Now(),
				cancel:    v.cancel,
			})
			m.setLaunchState(LaunchStateIdle)
			m.appendLog(fmt.Sprintf("session %d started (serial=%s profile=%s)", v.sid, v.serial, v.profileID))
			return m, monitorSessionCmd(v.sid, v.session)
		}
		// Process failed to start.
		if v.cancel != nil {
			v.cancel()
		}
		switch {
		case v.reason == adb.FailureCanceled || errors.Is(v.err, context.Canceled):
			m.setLaunchState(LaunchStateCanceled)
		case v.reason == adb.FailureTimeout || v.res.TimedOut:
			m.setLaunchState(LaunchStateTimedOut)
		default:
			m.setLaunchState(LaunchStateFailed)
		}
		m.appendLog("launch failed: " + v.err.Error())
		if d := launchDetail(v.res); d != "" {
			m.appendLog("launch detail: " + d)
		}
		m.appendLog("launch failure reason: " + string(v.reason))
		switch v.reason {
		case adb.FailureNoDisplay:
			if h := platform.NoDisplayHint(); h != "" {
				m.appendLog(h)
			} else {
				m.appendLog("hint: scrcpy could not open a window — check your display server")
			}
		case adb.FailureDeviceNotFound:
			m.appendLog("hint: device serial not found — try refreshing (R) or re-connecting")
		case adb.FailureMultipleDevices:
			m.appendLog("hint: multiple ADB devices attached — select a specific device")
		case adb.FailurePermissions:
			m.appendLog("hint: USB permission denied — check udev rules or run as root")
		case adb.FailureVersionMismatch:
			m.appendLog("hint: adb client/server version mismatch — restart adb server (adb kill-server)")
		case adb.FailureConnectionReset:
			m.appendLog("hint: connection reset — device may have gone to sleep or USB was unplugged")
		case adb.FailureProtocol:
			m.appendLog("hint: ADB protocol error — try adb kill-server && adb start-server")
		}
		return m, launchResetCmd()

	case sessionExitedMsg:
		for i, s := range m.activeSessions {
			if s.id == v.id {
				s.cancel()
				m.activeSessions = append(m.activeSessions[:i], m.activeSessions[i+1:]...)
				break
			}
		}
		if v.err != nil && adb.ClassifyFailure(v.err) != adb.FailureCanceled {
			m.appendLog(fmt.Sprintf("session %d exited with error: %v", v.id, v.err))
			if d := launchDetail(v.res); d != "" {
				m.appendLog("session exit detail: " + d)
			}
		} else {
			m.appendLog(fmt.Sprintf("session %d ended", v.id))
		}
		return m, nil

	case pairResultMsg:
		m.pairingStatus = ""
		if v.err != nil {
			reason := adb.ClassifyFailure(v.err)
			if reason == adb.FailureAuthMismatch {
				tcpHost := extractTCPHost(v.hostPort)
				cmds := m.purgeStaleDevice(tcpHost)
				m.pairingStatus = "✗ Fingerprint mismatch — old entry removed. Re-pair from Wireless Debugging."
				m.appendLog("auth mismatch pairing " + v.hostPort + ": stale device purged")
				return m, tea.Batch(cmds...)
			}
			m.pairingStatus = "✗ " + v.err.Error()
			m.appendLog("pair failed " + v.hostPort + ": " + v.err.Error())
			return m, nil
		}
		if !v.result.Success {
			// adb pair exits 0 but prints a failure message — check output for auth issues.
			outLow := strings.ToLower(v.result.Output)
			if strings.Contains(outLow, "failed to authenticate") ||
				strings.Contains(outLow, "authentication failed") {
				tcpHost := extractTCPHost(v.hostPort)
				cmds := m.purgeStaleDevice(tcpHost)
				m.pairingStatus = "✗ Fingerprint mismatch — old entry removed. Re-pair from Wireless Debugging."
				m.appendLog("auth mismatch (output) pairing " + v.hostPort + ": stale device purged")
				return m, tea.Batch(cmds...)
			}
			m.pairingStatus = "✗ " + v.result.Output
			m.appendLog("pair unsuccessful " + v.hostPort + ": " + v.result.Output)
			return m, nil
		}
		m.pairingStatus = "✓ " + v.result.Output
		m.appendLog("pair succeeded " + v.hostPort + ": " + v.result.Output)
		tcpHost := extractTCPHost(v.hostPort)
		connectPort := strings.TrimSpace(m.pairingConnectPort)
		if connectPort == "" {
			connectPort = "5555"
		}
		connectHostPort := tcpHost + ":" + connectPort
		connectPortInt, _ := strconv.Atoi(connectPort)
		if connectPortInt == 0 {
			connectPortInt = 5555
		}
		known := core.KnownDevice{
			Alias:    tcpHost,
			PairedAt: time.Now(),
			Endpoints: []core.Endpoint{
				{Host: tcpHost, Port: connectPortInt, Transport: "tcp", Name: "ADB-TCP", Priority: 20},
			},
			Tags: []string{"wireless-debug"},
		}
		found := false
		for i, kd := range m.config.KnownDevices {
			if kd.Alias == known.Alias {
				m.config.KnownDevices[i] = known
				found = true
				break
			}
		}
		if !found {
			m.config.KnownDevices = append(m.config.KnownDevices, known)
		}
		m.saveConfig()
		m.appendLog("known device saved: " + known.Alias + " connect=" + connectHostPort)
		m.overlayMode = OverlayNicknameEntry
		m.nicknameEntryHost = tcpHost
		m.nicknameInput = ""
		return m, tea.Batch(connectCmd(connectHostPort), refreshDevicesCmd())

	case connectResultMsg:
		if v.err != nil {
			reason := adb.ClassifyFailure(v.err)
			if reason == adb.FailureAuthMismatch {
				tcpHost := extractTCPHost(v.hostPort)
				cmds := m.purgeStaleDevice(tcpHost)
				m.appendLog("auth mismatch connecting " + v.hostPort + ": stale device purged — use P to re-pair")
				if m.pendingAutoLaunch {
					m.pendingAutoLaunch = false
				}
				return m, tea.Batch(append(cmds, refreshDevicesCmd())...)
			}
			// adb connect can succeed at the TCP level but report auth failure in output.
			outLow := strings.ToLower(v.output)
			if strings.Contains(outLow, "failed to authenticate") ||
				strings.Contains(outLow, "device unauthorized") {
				tcpHost := extractTCPHost(v.hostPort)
				cmds := m.purgeStaleDevice(tcpHost)
				m.appendLog("auth mismatch (output) connecting " + v.hostPort + ": stale device purged — use P to re-pair")
				if m.pendingAutoLaunch {
					m.pendingAutoLaunch = false
				}
				return m, tea.Batch(append(cmds, refreshDevicesCmd())...)
			}
			// Connection refused = host is reachable but port changed (ADB restarts with a new port).
			if reason == adb.FailureRefused {
				tcpHost := extractTCPHost(v.hostPort)
				if m.isKnownDeviceHost(tcpHost) {
					m.appendLog("adb connect refused " + v.hostPort + ": port may have changed")
					if m.pendingAutoLaunch {
						m.pendingAutoLaunch = false
					}
					m.overlayMode = OverlayPortUpdate
					m.portUpdateHost = tcpHost
					m.portUpdateInput = ""
					m.portUpdateStatus = ""
					return m, nil
				}
			}
			m.appendLog("adb connect failed " + v.hostPort + ": " + v.err.Error())
			if m.pendingAutoLaunch {
				return m, refreshDevicesCmd()
			}
			return m, nil
		}
		m.appendLog("adb connect " + v.hostPort + " → " + v.output)
		for i, kd := range m.config.KnownDevices {
			if kd.Alias == extractTCPHost(v.hostPort) {
				m.config.KnownDevices[i].Serial = v.hostPort
				m.config.KnownDevices[i].LastSeenAt = time.Now()
				break
			}
		}
		m.saveConfig()
		return m, refreshDevicesCmd()

	case disconnectResultMsg:
		m.appendLog("adb disconnected " + v.hostPort)
	}
	return m, nil
}

// ── Key handler ───────────────────────────────────────────────────────────────

func (m Model) handleKey(v tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := v.String()
	s := strings.ToLower(key)

	// ctrl+c always quits.
	if s == "ctrl+c" {
		return m, tea.Quit
	}

	// ── Rename mode ───────────────────────────────────────────────────────────
	if m.renameMode {
		switch s {
		case "enter":
			m.commitRename()
		case "esc":
			m.cancelRename()
		case "backspace", "ctrl+h":
			if len(m.renameBuffer) > 0 {
				m.renameBuffer = m.renameBuffer[:len(m.renameBuffer)-1]
			}
		default:
			if isPrintableText(key) {
				m.renameBuffer += key
			}
		}
		return m, nil
	}

	// ── Pairing overlay ───────────────────────────────────────────────────────
	if m.overlayMode == OverlayPairing {
		switch s {
		case "esc":
			m.closePairingOverlay()
		case "tab":
			m.pairingField = (m.pairingField + 1) % 4
		case "shift+tab":
			m.pairingField = (m.pairingField + 3) % 4
		case "enter":
			if m.pairingStatus == "pairing…" {
				return m, nil
			}
			ip := strings.TrimSpace(m.pairingHost)
			pairPort := strings.TrimSpace(m.pairingPort)
			connectPort := strings.TrimSpace(m.pairingConnectPort)
			if ip == "" {
				m.pairingStatus = "! IP address required"
				m.pairingField = 0
				return m, nil
			}
			if pairPort == "" {
				m.pairingStatus = "! pairing port required"
				m.pairingField = 1
				return m, nil
			}
			if _, err := strconv.Atoi(pairPort); err != nil {
				m.pairingStatus = "! pairing port must be a number"
				m.pairingField = 1
				return m, nil
			}
			if connectPort == "" {
				m.pairingStatus = "! connect port required"
				m.pairingField = 3
				return m, nil
			}
			if _, err := strconv.Atoi(connectPort); err != nil {
				m.pairingStatus = "! connect port must be a number"
				m.pairingField = 3
				return m, nil
			}
			hostPort := ip + ":" + pairPort
			m.pairingStatus = "pairing…"
			return m, pairCmd(hostPort, strings.TrimSpace(m.pairingCode))
		case "backspace", "ctrl+h":
			switch m.pairingField {
			case 0:
				if len(m.pairingHost) > 0 {
					m.pairingHost = m.pairingHost[:len(m.pairingHost)-1]
				}
			case 1:
				if len(m.pairingPort) > 0 {
					m.pairingPort = m.pairingPort[:len(m.pairingPort)-1]
				}
			case 2:
				if len(m.pairingCode) > 0 {
					m.pairingCode = m.pairingCode[:len(m.pairingCode)-1]
				}
			case 3:
				if len(m.pairingConnectPort) > 0 {
					m.pairingConnectPort = m.pairingConnectPort[:len(m.pairingConnectPort)-1]
				}
			}
		default:
			if isPrintableText(key) {
				switch m.pairingField {
				case 0:
					if len([]rune(m.pairingHost)) < 15 {
						m.pairingHost += key
					}
				case 1:
					if len([]rune(m.pairingPort)) < 5 {
						m.pairingPort += key
					}
				case 2:
					if len([]rune(m.pairingCode)) < 6 {
						m.pairingCode += key
					}
				case 3:
					if len([]rune(m.pairingConnectPort)) < 5 {
						m.pairingConnectPort += key
					}
				}
			}
		}
		return m, nil
	}

	// ── Nickname entry overlay ────────────────────────────────────────────────
	if m.overlayMode == OverlayNicknameEntry {
		switch s {
		case "esc":
			m.overlayMode = OverlayNone
			m.appendLog("nickname skipped for " + m.nicknameEntryHost)
		case "enter":
			nickname := strings.TrimSpace(m.nicknameInput)
			for i := range m.config.KnownDevices {
				if m.config.KnownDevices[i].Alias == m.nicknameEntryHost {
					m.config.KnownDevices[i].Nickname = nickname
					break
				}
			}
			m.saveConfig()
			if nickname != "" {
				m.appendLog("nickname set: " + nickname + " for " + m.nicknameEntryHost)
			}
			m.overlayMode = OverlayNone
		case "backspace", "ctrl+h":
			if len(m.nicknameInput) > 0 {
				m.nicknameInput = string([]rune(m.nicknameInput)[:len([]rune(m.nicknameInput))-1])
			}
		default:
			if len(key) == 1 && key[0] >= 0x20 {
				if len([]rune(m.nicknameInput)) < 32 {
					m.nicknameInput += key
				}
			}
		}
		return m, nil
	}

	// ── Profile picker overlay ────────────────────────────────────────────────
	if m.overlayMode == OverlayProfilePicker {
		n := len(m.config.Profiles)
		switch s {
		case "esc", "q":
			m.overlayMode = OverlayNone
		case "up", "k":
			if m.profilePickerIdx > 0 {
				m.profilePickerIdx--
			}
		case "down", "j":
			if m.profilePickerIdx < n-1 {
				m.profilePickerIdx++
			}
		case "enter", " ":
			if m.profilePickerIdx >= 0 && m.profilePickerIdx < n {
				profileName := m.config.Profiles[m.profilePickerIdx].Name
				entries := m.mergedDeviceList()
				if m.deviceIdx < len(entries) && entries[m.deviceIdx].IsKnown && entries[m.deviceIdx].Known != nil {
					kd := entries[m.deviceIdx].Known
					for i := range m.config.KnownDevices {
						if &m.config.KnownDevices[i] == kd {
							m.config.KnownDevices[i].DefaultProfile = profileName
							break
						}
					}
					m.saveConfig()
					m.appendLog("device profile set: " + kd.DisplayName() + " → " + profileName)
				}
				m.activeIdx = m.profilePickerIdx
				m.recomputePlanAndPreview()
			}
			m.overlayMode = OverlayNone
		}
		return m, nil
	}

	// ── Port update overlay ───────────────────────────────────────────────────
	if m.overlayMode == OverlayPortUpdate {
		switch s {
		case "esc":
			m.overlayMode = OverlayNone
			m.appendLog("port update cancelled for " + m.portUpdateHost)
		case "enter":
			portStr := strings.TrimSpace(m.portUpdateInput)
			portInt, err := strconv.Atoi(portStr)
			if err != nil || portInt < 1 || portInt > 65535 {
				m.portUpdateStatus = "✗ Invalid port (1–65535)"
				return m, nil
			}
			// Update all TCP endpoints for this host in config.
			for i := range m.config.KnownDevices {
				kd := &m.config.KnownDevices[i]
				isHost := kd.Alias == m.portUpdateHost || kd.LocalWiFiIP == m.portUpdateHost
				for j := range kd.Endpoints {
					if kd.Endpoints[j].Host == m.portUpdateHost {
						isHost = true
					}
				}
				if !isHost {
					continue
				}
				for j := range kd.Endpoints {
					if kd.Endpoints[j].Host == m.portUpdateHost && kd.Endpoints[j].Transport == "tcp" {
						kd.Endpoints[j].Port = portInt
					}
				}
			}
			m.saveConfig()
			hostPort := fmt.Sprintf("%s:%d", m.portUpdateHost, portInt)
			m.appendLog("port updated: " + hostPort)
			m.overlayMode = OverlayNone
			m.pendingAutoLaunch = true
			return m, tea.Batch(connectCmd(hostPort), refreshDevicesCmd())
		case "backspace", "ctrl+h":
			if len(m.portUpdateInput) > 0 {
				m.portUpdateInput = m.portUpdateInput[:len(m.portUpdateInput)-1]
			}
		default:
			if len(key) == 1 && key[0] >= '0' && key[0] <= '9' {
				if len(m.portUpdateInput) < 5 {
					m.portUpdateInput += key
				}
			}
		}
		return m, nil
	}

	// ── Confirm delete overlay ────────────────────────────────────────────────
	if m.overlayMode == OverlayConfirmDelete {
		switch s {
		case "esc", "n":
			m.overlayMode = OverlayNone
			m.appendLog("delete cancelled: " + m.deleteTarget)
		case "left", "h":
			m.deleteChoice = 0
		case "right", "l":
			m.deleteChoice = 1
		case "enter", " ":
			if m.deleteChoice == 1 {
				switch m.deleteType {
				case "device":
					m.deleteSelectedDevice()
				case "profile":
					m.deleteActiveProfile()
				}
			}
			m.overlayMode = OverlayNone
		}
		return m, nil
	}

	// ── Warning overlay ───────────────────────────────────────────────────────
	if m.overlayMode == OverlayWarning {
		switch s {
		case "esc", "w", "q":
			m.overlayMode = OverlayNone
		}
		return m, nil
	}

	// ── Help overlay ──────────────────────────────────────────────────────────
	if m.overlayMode == OverlayHelp {
		switch s {
		case "esc", "q", "?":
			m.overlayMode = OverlayNone
		}
		return m, nil
	}

	// ── Onboarding overlay ────────────────────────────────────────────────────
	if m.overlayMode == OverlayOnboarding {
		const onboardingSteps = 5
		switch s {
		case "esc", "q", "b":
			m.overlayMode = OverlayNone
		case "right", "enter", "n":
			if m.onboardingStep < onboardingSteps-1 {
				m.onboardingStep++
			} else {
				m.overlayMode = OverlayNone
				m.openPairingOverlay()
			}
		case "left", "p":
			if m.onboardingStep > 0 {
				m.onboardingStep--
			}
		}
		return m, nil
	}

	// ── Device editor overlay ────────────────────────────────────────────────
	if m.overlayMode == OverlayDeviceEditor {
		fieldCount := len(m.deviceEditorBuffer)
		switch s {
		case "esc":
			m.overlayMode = OverlayNone
			m.appendLog("device editor cancelled")
		case "enter":
			m.saveDeviceEditor()
			m.overlayMode = OverlayNone
		case "tab", "down":
			if fieldCount > 0 {
				m.deviceEditorField = (m.deviceEditorField + 1) % fieldCount
			}
		case "shift+tab", "up":
			if fieldCount > 0 {
				m.deviceEditorField = (m.deviceEditorField + fieldCount - 1) % fieldCount
			}
		case "backspace", "ctrl+h":
			if m.deviceEditorField < fieldCount {
				v := m.deviceEditorBuffer[m.deviceEditorField]
				if len(v) > 0 {
					m.deviceEditorBuffer[m.deviceEditorField] = v[:len(v)-1]
				}
			}
		default:
			if isPrintableText(key) && m.deviceEditorField < fieldCount {
				// buf: [nickname=0, alias=1, ep0.host=2, ep0.port=3, ep1.host=4, ep1.port=5, ...]
				// Port fields are at indices 3, 5, 7… (odd, ≥3)
				isPort := m.deviceEditorField >= 3 && m.deviceEditorField%2 == 1
				v := m.deviceEditorBuffer[m.deviceEditorField]
				maxLen := 45
				switch {
				case isPort:
					maxLen = 5
				case m.deviceEditorField == 0: // nickname
					maxLen = 32
				case m.deviceEditorField == 1: // alias/IP
					maxLen = 45
				}
				if len([]rune(v)) < maxLen {
					m.deviceEditorBuffer[m.deviceEditorField] += key
				}
			}
		}
		return m, nil
	}

	// ── Profile editor overlay ───────────────────────────────────────────────
	if m.overlayMode == OverlayProfileEditor {
		switch s {
		case "esc", "q":
			m.overlayMode = OverlayNone
			m.appendLog("profile editor closed")
		case "up":
			if m.editorCursor > 0 {
				m.editorCursor--
			}
		case "down":
			if m.editorCursor < pfFieldCount-1 {
				m.editorCursor++
			}
		case "left":
			m.editorCycleField(m.editorCursor, -1)
		case "right", "enter", " ":
			m.editorCycleField(m.editorCursor, 1)
		}
		return m, nil
	}

	// ── Uppercase specials (before lowercasing switch) ─────────────────────────
	switch key {
	case "W":
		m.overlayMode = OverlayWarning
		return m, nil
	case "P":
		m.openPairingOverlay()
		return m, nil
	case "?":
		m.overlayMode = OverlayHelp
		return m, nil
	case "B":
		m.overlayMode = OverlayOnboarding
		m.onboardingStep = 0
		return m, nil
	case "D":
		if m.focus == PaneDevices {
			entries := m.mergedDeviceList()
			if m.deviceIdx < len(entries) && entries[m.deviceIdx].IsKnown && entries[m.deviceIdx].Known != nil {
				kd := entries[m.deviceIdx].Known
				m.deleteTarget = kd.DisplayName()
				m.deleteType = "device"
				m.deleteChoice = 0
				m.overlayMode = OverlayConfirmDelete
			}
		} else {
			if p := m.activeProfilePtr(); p != nil {
				m.deleteTarget = p.Name
				m.deleteType = "profile"
				m.deleteChoice = 0
				m.overlayMode = OverlayConfirmDelete
			}
		}
		return m, nil
	}

	// ── Universal navigation: Ctrl+Arrow moves between panes ─────────────────
	switch s {
	case "ctrl+left":
		if m.focus == PaneRight {
			m.focus = PaneDevices
		}
		return m, nil
	case "ctrl+right":
		m.focus = PaneRight
		return m, nil
	case "ctrl+up":
		if m.focus == PaneProfiles {
			m.focus = PaneDevices
		} else if m.focus == PaneRight {
			m.focus = PaneDevices
		}
		return m, nil
	case "ctrl+down":
		if m.focus == PaneDevices {
			m.focus = PaneProfiles
		}
		return m, nil
	case "tab":
		// Tab cycles: Devices → Right → Devices
		switch m.focus {
		case PaneDevices:
			m.focus = PaneRight
		default:
			m.focus = PaneDevices
		}
		return m, nil
	case "shift+tab":
		switch m.focus {
		case PaneDevices:
			m.focus = PaneRight
		default:
			m.focus = PaneDevices
		}
		return m, nil
	}

	// ── Arrow keys: navigate within focused pane ──────────────────────────────
	switch s {
	case "up":
		switch m.focus {
		case PaneDevices:
			m.switchActiveDevice(-1)
		case PaneProfiles:
			m.switchActiveProfile(-1)
		case PaneRight:
			if m.logScroll > 0 {
				m.logScroll--
			}
		}
		return m, nil
	case "down":
		switch m.focus {
		case PaneDevices:
			m.switchActiveDevice(1)
		case PaneProfiles:
			m.switchActiveProfile(1)
		case PaneRight:
			m.logScroll++
		}
		return m, nil
	}

	// ── Normal actions ────────────────────────────────────────────────────────
	switch s {
	case "q":
		return m, tea.Quit

	case "r":
		m.appendLog("refreshing devices")
		return m, tea.Batch(refreshDevicesCmd(), m.reconnectKnownDevicesCmd())

	case "enter", " ", "s":
		// S = Start scrcpy (same as Enter/Space)
		return m.doLaunch()

	case "e":
		if m.focus == PaneDevices {
			m.openDeviceEditor()
		} else {
			m.startRename()
		}
		return m, nil

	case "o":
		// O = Open full profile option editor
		if m.activeProfilePtr() != nil {
			m.overlayMode = OverlayProfileEditor
			m.editorCursor = 0
			m.appendLog("profile editor opened")
		}
		return m, nil

	case "z":
		if m.launchState == LaunchStateLaunching && m.launchCancel != nil {
			m.launchCancel()
			m.appendLog("launch cancel requested")
			return m, nil
		}
		m.appendLog("launch cancel skipped: no launch in progress")
		return m, nil

	case "n":
		m.createProfileFromDefault()
		return m, nil

	case "d":
		m.duplicateActiveProfile()
		return m, nil

	case "x":
		m.deleteActiveProfile()
		return m, nil

	case "p":
		m.openProfilePicker()
		return m, nil

	case "f":
		m.setDefaultProfile(m.activeIdx)
		return m, nil

	// Flag toggles for active profile
	case "1":
		m.toggleFlag("turn_screen_off")
		return m, nil
	case "2":
		m.toggleFlag("stay_awake")
		return m, nil
	case "3":
		m.toggleFlag("prefer_h265")
		return m, nil
	case "4":
		m.toggleFlag("require_audio")
		return m, nil
	case "5":
		m.toggleFlag("require_camera")
		return m, nil

	case "+", "=":
		m.adjustBitrate(2)
		return m, nil
	case "-":
		m.adjustBitrate(-2)
		return m, nil

	// Theme cycling: [ and ]
	case "[":
		m.cycleTheme(-1)
		return m, nil
	case "]":
		m.cycleTheme(1)
		return m, nil
	}

	return m, nil
}

// ── Mouse click handler ───────────────────────────────────────────────────────

func (m Model) handleMouseClick(v tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	col := v.X
	row := v.Y

	leftW, _, contentH := m.layoutDimensions()

	// Left panel clicks (devices pane)
	if col < leftW {
		devInnerH := contentH - ui.PaneFrameH
		devPaneOuter := devInnerH + ui.PaneFrameH
		if row >= 1 && row < 1+devPaneOuter {
			m.focus = PaneDevices
			// Item rows start at row 2 (border+title)
			itemRow := row - 2
			if itemRow >= 0 {
				entries := m.mergedDeviceList()
				if itemRow < len(entries) {
					m.deviceIdx = itemRow
					m.recomputePlanAndPreview()
				}
			}
		}
		return m, nil
	}

	// Right panel click → focus right
	if col >= leftW {
		m.focus = PaneRight
	}
	return m, nil
}

// ── View ──────────────────────────────────────────────────────────────────────

func (m Model) View() tea.View {
	matrix := m.matrix.View(m.height)

	// ── Too small ─────────────────────────────────────────────────────────────
	if m.isTerminalTooSmall() {
		lines := []string{
			"Terminal too small.",
			fmt.Sprintf("Current:  %dx%d", m.width, m.height),
			fmt.Sprintf("Required: %dx%d", minFullLayoutWidth, minFullLayoutHeight),
			"Resize to restore.",
		}
		pw := max(24, min(m.width-4, 40))
		ph := 8
		ox := max(1, (m.width-pw-ui.PaneFrameW)/2)
		oy := max(1, (m.height-ph-ui.PaneFrameH)/2)
		content := ui.RenderPane("⚠ Too Small", lines, pw, ph)
		return newView(render.Compose(m.width, m.height, matrix,
			render.OverlayBlock{X: ox, Y: oy, Content: content}))
	}

	leftW, rightW, contentH := m.layoutDimensions()

	// ── Left panel: devices only (full height) ────────────────────────────────
	leftInnerW := leftW - ui.PaneFrameW
	devInnerH := contentH - ui.PaneFrameH

	// maxLabelW: leave room for icon+space (2) + " [K]" marker (4) = 6
	devLabelW := max(leftInnerW-6, 6)
	devPane := ui.RenderPaneFocused(
		"Devices ("+itoa(len(m.mergedDeviceList()))+")",
		m.renderDeviceLines(m.mergedDeviceList(), devLabelW),
		leftInnerW, devInnerH,
		m.focus == PaneDevices,
	)
	leftPanel := devPane

	// ── Right panel ───────────────────────────────────────────────────────────
	rightLines := m.buildRightPanel(rightW-2, contentH)
	rightPane := lipgloss.NewStyle().
		Width(rightW).
		Height(contentH).
		Background(lipgloss.Color(ui.Active.PaneBg)).
		Foreground(lipgloss.Color(ui.Active.PaneFg)).
		Render(strings.Join(rightLines, "\n"))

	layout := lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, rightPane)

	// ── Log strip (2 lines above status bar) ─────────────────────────────────
	logStrip := m.renderLogStrip(m.width)

	// ── Status bar ────────────────────────────────────────────────────────────
	devLabel := "no device"
	entries := m.mergedDeviceList()
	if len(entries) > 0 && m.deviceIdx < len(entries) {
		e := entries[m.deviceIdx]
		devLabel = e.Serial
		if e.Model != "" && e.Model != "?" {
			devLabel += " (" + e.Model + ")"
		}
	}
	// Truncate long labels to prevent status bar overflow on narrow terminals.
	const maxDevLabelRunes = 32
	if r := []rune(devLabel); len(r) > maxDevLabelRunes {
		devLabel = string(r[:maxDevLabelRunes-1]) + "…"
	}
	profLabel := ""
	if p := m.activeProfilePtr(); p != nil {
		profLabel = p.Name
	}
	statusBar := ui.RenderStatusBar(m.width, devLabel, profLabel,
		string(m.launchState), m.contextKeys())

	foreground := layout + "\n" + logStrip + "\n" + statusBar

	// ── Overlays ──────────────────────────────────────────────────────────────
	overlays := []render.OverlayBlock{{X: 1, Y: 1, Content: foreground, SeeThrough: true}}
	switch m.overlayMode {
	case OverlayWarning:
		oc := m.renderWarningOverlay()
		overlays = append(overlays, centeredOverlay(oc, m.width, m.height))
	case OverlayPairing:
		oc := m.renderPairingOverlay()
		overlays = append(overlays, centeredOverlay(oc, m.width, m.height))
	case OverlayHelp:
		oc := m.renderHelpOverlay()
		overlays = append(overlays, centeredOverlay(oc, m.width, m.height))
	case OverlayProfileEditor:
		oc := m.renderProfileEditorOverlay()
		overlays = append(overlays, centeredOverlay(oc, m.width, m.height))
	case OverlayOnboarding:
		oc := m.renderOnboardingOverlay()
		overlays = append(overlays, centeredOverlay(oc, m.width, m.height))
	case OverlayDeviceEditor:
		oc := m.renderDeviceEditorOverlay()
		overlays = append(overlays, centeredOverlay(oc, m.width, m.height))
	case OverlayNicknameEntry:
		oc := m.renderNicknameEntryOverlay()
		overlays = append(overlays, centeredOverlay(oc, m.width, m.height))
	case OverlayPortUpdate:
		oc := m.renderPortUpdateOverlay()
		overlays = append(overlays, centeredOverlay(oc, m.width, m.height))
	case OverlayConfirmDelete:
		oc := m.renderConfirmDeleteOverlay()
		overlays = append(overlays, centeredOverlay(oc, m.width, m.height))
	case OverlayProfilePicker:
		oc := m.renderProfilePickerOverlay()
		overlays = append(overlays, centeredOverlay(oc, m.width, m.height))
	}

	v := tea.NewView(render.Compose(m.width, m.height, matrix, overlays...))
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion // enable mouse click reporting
	return v
}

// ── Layout helpers ────────────────────────────────────────────────────────────

// layoutDimensions returns (leftW, rightW, contentH) where
// leftW + rightW = m.width and contentH + 3 = m.height (2 log strip + 1 status bar).
// leftW and rightW are OUTER widths (including any border/padding that callers
// add themselves).
func (m *Model) layoutDimensions() (leftW, rightW, contentH int) {
	contentH = m.height - 3                   // 2 log strip + 1 status bar
	if contentH < 4 {
		contentH = 4
	}
	leftW = max(22, min(36, m.width*28/100))  // ~28% of terminal, clamped
	rightW = m.width - leftW
	return
}

// paneInnerHeights returns the INNER content heights for the device and
// profile panes such that their rendered outer heights sum to contentH.
// outer = inner + PaneFrameH, so innerDev + innerProf = contentH - 2*PaneFrameH.
func (m *Model) paneInnerHeights(contentH int) (devInner, profInner int) {
	available := contentH - 2*ui.PaneFrameH // total inner rows available
	if available < 4 {
		available = 4
	}
	devInner = max(3, available*2/5)
	profInner = available - devInner
	return
}

// centeredOverlay positions an overlay block at the center of the terminal.
func centeredOverlay(content string, termW, termH int) render.OverlayBlock {
	lines := strings.Split(content, "\n")
	contentW := 0
	for _, l := range lines {
		if w := len([]rune(ui.StripANSIExport(l))); w > contentW {
			contentW = w
		}
	}
	contentH := len(lines)
	ox := max(1, (termW-contentW)/2)
	oy := max(2, (termH-contentH)/2)
	return render.OverlayBlock{X: ox, Y: oy, Content: content}
}

// ── Right panel ───────────────────────────────────────────────────────────────

func (m *Model) buildRightPanel(width, height int) []string {
	lines := make([]string, 0, height)
	add := func(s string) { lines = append(lines, s) }
	hr := func(label string) {
		pad := max(0, width-len(label)-4)
		add(ui.StyleMuted("── " + label + " " + strings.Repeat("─", pad)))
	}

	t := ui.Active
	profile := m.activeProfilePtr()

	// ── Profile header ────────────────────────────────────────────────────────
	if profile != nil {
		defMark := ""
		if profile.IsDefault {
			defMark = "  " + t.GoodStyle().Render("[default]")
		}
		add(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(t.TitleFg)).
			Background(lipgloss.Color(t.PaneBg)).Render(profile.Name) + defMark)
		add("")
		add(fmt.Sprintf("  Mode %-18s  Max Size %-6s  Bitrate %-4s",
			ui.StyleInfo(profileModeLabel(profile)),
			ui.StyleInfo(fmt.Sprintf("%dpx", profile.MaxSize)),
			ui.StyleInfo(fmt.Sprintf("%dM", profile.VideoBitRateMB)),
		))
		add("  " + flagRow(profile))
		add("")
	} else {
		add(ui.StyleMuted("(no profile)"))
		add("")
	}

	// ── Compatibility ─────────────────────────────────────────────────────────
	caps := m.selectedDeviceCaps()
	deviceLine := ui.StyleMuted("(simulated caps)")
	if caps.Serial != "" && caps.Serial != "simulated" {
		deviceLine = ui.StyleMuted(caps.Serial + "  sdk=" + itoa(caps.SDKInt))
	}
	hr("Compatibility  " + deviceLine)

	if len(m.lastResolve.SupportedFeatures)+len(m.lastResolve.Warnings)+
		len(m.lastResolve.UnsupportedFeatures)+len(m.lastResolve.BlockedFeatures) == 0 {
		add("  " + ui.StyleMuted("none"))
	} else {
		for _, f := range m.lastResolve.SupportedFeatures {
			add("  " + ui.StyleGood("✓") + " " + f)
		}
		for _, f := range m.lastResolve.Warnings {
			add("  " + ui.StyleWarn("!") + " " + f)
		}
		for _, f := range m.lastResolve.UnsupportedFeatures {
			add("  " + ui.StyleErr("✗") + " " + f + "  " + ui.StyleMuted("(dropped)"))
		}
		for _, f := range m.lastResolve.BlockedFeatures {
			add("  " + ui.StyleErr("⊘") + " " + f + "  " + ui.StyleErr("BLOCKED"))
		}
	}
	add("")

	// ── Device Status (heartbeat data) ────────────────────────────────────────
	{
		devEntries := m.mergedDeviceList()
		if m.deviceIdx < len(devEntries) {
			e := devEntries[m.deviceIdx]
			if e.IsLive && e.Caps.Serial != "" && e.Caps.Serial != "simulated" {
				hr("Device Status")
				if e.Caps.BatteryLevel > 0 {
					add("  Battery    " + batteryBadge(e.Caps.BatteryLevel))
				}
				if e.Caps.LocalWiFiIP != "" {
					add("  WiFi IP    " + ui.StyleInfo(e.Caps.LocalWiFiIP))
				}
				if e.Caps.TailscaleIP != "" {
					add("  Tailscale  " + ui.StyleGood(e.Caps.TailscaleIP))
				}
				if e.Caps.StorageTotal > 0 {
					pct := int(100 * e.Caps.StorageFree / e.Caps.StorageTotal)
					storageStr := fmt.Sprintf("%s free / %s  (%d%%)", fmtBytes(e.Caps.StorageFree), fmtBytes(e.Caps.StorageTotal), pct)
					add("  Storage    " + ui.StyleInfo(storageStr))
				}
				if e.Caps.ExtStorageTotal > 0 {
					pct := int(100 * e.Caps.ExtStorageFree / e.Caps.ExtStorageTotal)
					sdStr := fmt.Sprintf("%s free / %s  (%d%%)", fmtBytes(e.Caps.ExtStorageFree), fmtBytes(e.Caps.ExtStorageTotal), pct)
					add("  SD Card    " + ui.StyleInfo(sdStr))
				} else if e.Caps.LocalWiFiIP == "" && e.Caps.TailscaleIP == "" && e.Caps.BatteryLevel == 0 {
					add("  " + ui.StyleMuted("(heartbeat pending…)"))
				}
				add("")
			}
		}
	}

	// ── Endpoints ───────────────────────────────────────────────────────────────
	hr("Endpoints")
	{
		devEntries := m.mergedDeviceList()
		var selEntry *DeviceEntry
		if m.deviceIdx < len(devEntries) {
			e := devEntries[m.deviceIdx]
			selEntry = &e
		}
		switch {
		case selEntry != nil && selEntry.IsKnown && selEntry.Known != nil && len(selEntry.Known.Endpoints) > 0:
			for _, ep := range selEntry.Known.Endpoints {
				tag := strings.ToUpper(ep.Transport)
				name := ep.Name
				if name == "" {
					name = ep.Transport
				}
				failStr := ""
				if ep.FailureCount > 0 {
					failStr = "  " + ui.StyleWarn(fmt.Sprintf("⚠%d", ep.FailureCount))
				}
				add(fmt.Sprintf("  [%s] %-8s %s:%d%s", tag, name, ep.Host, ep.Port, failStr))
			}
		case selEntry != nil && selEntry.IsKnown && selEntry.Known != nil:
			add("  " + ui.StyleMuted("(no stored endpoints)"))
		case selEntry != nil && selEntry.IsLive:
			transport := "USB"
			if strings.Contains(selEntry.Serial, ":") {
				transport = "TCP"
			}
			add("  [" + transport + "] " + ui.StyleInfo(selEntry.Serial) + ui.StyleMuted(" (live, not saved)"))
		default:
			add("  " + ui.StyleMuted("(no device selected)"))
		}
	}
	add("")

	// ── Launch command ────────────────────────────────────────────────────────
	hr("Launch Command  →  opens scrcpy window")
	if !platform.HasDisplay() {
		if w := platform.NoDisplayUIWarning(); w != "" {
			add("  " + ui.StyleWarn(w))
		}
		if h := platform.NoDisplayUIHint(); h != "" {
			add("  " + ui.StyleMuted("  "+h))
		}
		add("")
	}
	if m.preview != "" {
		for _, l := range wrapText("$ "+m.preview, width-2) {
			add("  " + ui.StyleInfo(l))
		}
	} else {
		add("  " + ui.StyleMuted("(none)"))
	}
	if m.lastPlan != nil && !m.lastPlan.Launchable {
		add("  " + ui.StyleErr("✗ NOT launchable — see compatibility"))
	}
	if st := m.launchStateInline(); st != "" {
		add("")
		add(st)
	}
	add("")

	// ── Logs ──────────────────────────────────────────────────────────────────
	hr("Logs")
	remaining := height - len(lines) - 1
	if remaining < 2 {
		remaining = 2
	}
	logLines := m.logs
	total := len(logLines)
	// Apply scroll offset (logScroll counts from end, 0 = most recent at bottom)
	start := total - remaining - m.logScroll
	if start < 0 {
		start = 0
		m.logScroll = total - remaining
		if m.logScroll < 0 {
			m.logScroll = 0
		}
	}
	end := start + remaining
	if end > total {
		end = total
	}
	visible := logLines[start:end]
	maxLogW := max(width-4, 8) // "  " prefix + 2 border chars
	for _, l := range visible {
		display := l
		// RFC3339 timestamps vary in length (UTC suffix = 1 char, ±HH:MM = 6 chars).
		// Locate the first space to split timestamp from message correctly.
		if idx := strings.IndexByte(l, ' '); idx >= 19 {
			ts := l[:idx]
			if len(ts) >= 19 {
				display = ts[11:19] + "  " + l[idx+1:]
			}
		}
		// Truncate before styling — long lines would wrap inside the pane and
		// consume multiple visible rows, displacing other log entries.
		display = truncateName(display, maxLogW)
		add("  " + ui.StyleMuted(display))
	}

	// Pad to height
	for len(lines) < height {
		lines = append(lines, "")
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	return lines
}

func (m *Model) launchStateInline() string {
	switch m.launchState {
	case LaunchStateLaunching:
		return ui.StyleWarn("  ⟳ launching…  z=cancel")
	case LaunchStateFailed:
		hint := ""
		if m.launchReason == adb.FailureNoDisplay {
			hint = "  →  no display server"
		}
		return ui.StyleErr("  ✗ failed: "+string(m.launchReason)) + ui.StyleMuted(hint)
	case LaunchStateCanceled:
		return ui.StyleMuted("  ⊘ canceled")
	case LaunchStateTimedOut:
		return ui.StyleErr("  ✗ timed out")
	}
	if n := len(m.activeSessions); n > 0 {
		label := "session"
		if n > 1 {
			label = "sessions"
		}
		return ui.StyleGood(fmt.Sprintf("  ● %d %s running", n, label))
	}
	return ""
}

// renderLogStrip renders 2 log lines as a thin bar spanning the terminal width.
func (m *Model) renderLogStrip(width int) string {
	maxW := max(width-4, 8)
	extract := func(l string) string {
		if idx := strings.IndexByte(l, ' '); idx >= 19 {
			ts := l[:idx]
			if len(ts) >= 19 {
				return ts[11:19] + "  " + l[idx+1:]
			}
		}
		return l
	}
	lines := make([]string, 2)
	total := len(m.logs)
	for i := 0; i < 2; i++ {
		idx := total - 2 + i
		if idx >= 0 && idx < total {
			lines[i] = truncateName(extract(m.logs[idx]), maxW)
		}
	}
	style := lipgloss.NewStyle().
		Width(width).
		Foreground(lipgloss.Color(ui.Active.Muted)).
		Background(lipgloss.Color(ui.Active.PaneBg))
	return style.Render(" "+lines[0]) + "\n" + style.Render(" "+lines[1])
}

// openProfilePicker opens the profile picker overlay, pre-selecting the
// device's current default profile if one is set.
func (m *Model) openProfilePicker() {
	m.profilePickerIdx = m.activeIdx
	entries := m.mergedDeviceList()
	if m.deviceIdx < len(entries) && entries[m.deviceIdx].IsKnown && entries[m.deviceIdx].Known != nil {
		dp := entries[m.deviceIdx].Known.DefaultProfile
		if dp != "" {
			for i, p := range m.config.Profiles {
				if p.Name == dp {
					m.profilePickerIdx = i
					break
				}
			}
		}
	}
	m.overlayMode = OverlayProfilePicker
}

func (m *Model) renderProfilePickerOverlay() string {
	w := min(58, max(40, m.width-12))
	innerW := w - ui.PaneFrameW
	t := ui.Active

	// Determine the device name for the title.
	deviceLabel := "(no device)"
	entries := m.mergedDeviceList()
	if m.deviceIdx < len(entries) {
		e := entries[m.deviceIdx]
		if e.IsKnown && e.Known != nil {
			deviceLabel = e.Known.DisplayName()
		} else {
			deviceLabel = e.Serial
		}
	}

	// Current default profile for the device.
	currentDefault := ""
	if m.deviceIdx < len(entries) && entries[m.deviceIdx].IsKnown && entries[m.deviceIdx].Known != nil {
		currentDefault = entries[m.deviceIdx].Known.DefaultProfile
	}

	lines := []string{""}
	for i, p := range m.config.Profiles {
		focused := i == m.profilePickerIdx
		marker := "  "
		name := truncateName(p.Name, innerW-10)
		suffix := ""
		if p.Name == currentDefault {
			suffix = " " + t.GoodStyle().Render("[assigned]")
		} else if p.IsDefault {
			suffix = " " + t.MutedStyle().Render("[default]")
		}
		if focused {
			marker = t.GoodStyle().Render("> ")
			lines = append(lines, marker+lipgloss.NewStyle().Bold(true).
				Foreground(lipgloss.Color(t.PaneFg)).Background(lipgloss.Color(t.PaneBg)).
				Render(name)+suffix)
		} else {
			lines = append(lines, marker+t.MutedStyle().Render(name)+suffix)
		}
	}
	lines = append(lines, "")
	lines = append(lines, ui.StyleMuted("  ↑↓=select  Enter=assign  Esc=cancel"))
	lines = append(lines, "")
	title := "★ Profile for: " + truncateName(deviceLabel, 24)
	return ui.RenderPane(title, lines, innerW, len(lines)+ui.PaneFrameH)
}

// ── Overlay renderers ─────────────────────────────────────────────────────────

func (m *Model) renderWarningOverlay() string {
	w := min(62, max(42, m.width-10))
	lines := []string{""}
	profileName := "(none)"
	if profile := m.activeProfilePtr(); profile != nil {
		profileName = profile.Name
	}
	lines = append(lines, "  Profile: "+profileName)
	entries := m.mergedDeviceList()
	if len(entries) > 0 && m.deviceIdx < len(entries) {
		e := entries[m.deviceIdx]
		lines = append(lines, fmt.Sprintf("  Device:  %s  sdk=%d", e.Serial, e.SDKInt))
	} else {
		lines = append(lines, "  Device:  (simulated caps)")
	}
	lines = append(lines, "")
	if len(m.lastResolve.BlockedFeatures) > 0 {
		lines = append(lines, "  "+ui.StyleErr("⊘ BLOCKED (prevents launch):"))
		for _, f := range m.lastResolve.BlockedFeatures {
			lines = append(lines, "      "+f)
		}
		lines = append(lines, "")
	}
	if len(m.lastResolve.UnsupportedFeatures) > 0 {
		lines = append(lines, "  "+ui.StyleErr("✗ UNSUPPORTED (dropped):"))
		for _, f := range m.lastResolve.UnsupportedFeatures {
			lines = append(lines, "      "+f)
		}
		lines = append(lines, "")
	}
	if len(m.lastResolve.Warnings) > 0 {
		lines = append(lines, "  "+ui.StyleWarn("! WARNINGS:"))
		for _, f := range m.lastResolve.Warnings {
			lines = append(lines, "      "+f)
		}
		lines = append(lines, "")
	}
	if len(m.lastResolve.BlockedFeatures)+len(m.lastResolve.UnsupportedFeatures)+
		len(m.lastResolve.Warnings) == 0 {
		lines = append(lines, "  "+ui.StyleGood("✓ No warnings — profile fully supported."))
		lines = append(lines, "")
	}
	lines = append(lines, ui.StyleMuted("  W / q / esc  to close"))
	lines = append(lines, "")
	h := len(lines) + 4
	return ui.RenderPane("* Warnings & Compatibility", lines, w-ui.PaneFrameW, h)
}

func (m *Model) renderPairingOverlay() string {
	w := min(58, max(46, m.width-12))

	// Fixed field widths: IP=15 (max IPv4/Tailscale), Port=5, Code=6.
	const ipW, portW, codeW = 15, 5, 6

	ipVal := m.pairingHost
	if m.pairingField == 0 {
		ipVal += "_"
	}
	portVal := m.pairingPort
	if m.pairingField == 1 {
		portVal += "_"
	}
	codeVal := m.pairingCode
	if m.pairingField == 2 {
		codeVal += "_"
	}
	connPortVal := m.pairingConnectPort
	if m.pairingField == 3 {
		connPortVal += "_"
	}

	fieldStyle := func(val string, maxW int, active bool) string {
		padded := "[" + padRight(val, maxW) + "]"
		if active {
			return lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color(ui.Active.Info)).
				Background(lipgloss.Color(ui.Active.PaneBg)).
				Render(padded)
		}
		return ui.StyleMuted(padded)
	}

	lines := []string{
		"",
		"  Android: Settings → Developer Options",
		"  → Wireless Debugging",
		"  Tap \"Pair device with pairing code\"",
		"",
		"  IP Address   " + fieldStyle(ipVal, ipW, m.pairingField == 0),
		"  Pair Port    " + fieldStyle(portVal, portW, m.pairingField == 1),
		"  Code         " + fieldStyle(codeVal, codeW, m.pairingField == 2),
		"",
		"  " + ui.StyleMuted("Connect port (shown on Wireless Debugging screen):"),
		"  Connect Port " + fieldStyle(connPortVal, portW, m.pairingField == 3),
		"",
		"  " + ui.StyleMuted("Tab=next   Shift+Tab=prev   Enter=pair   Esc=cancel"),
		"",
	}
	if m.pairingStatus != "" {
		col := ui.StyleGood
		if strings.HasPrefix(m.pairingStatus, "✗") || strings.HasPrefix(m.pairingStatus, "!") {
			col = ui.StyleErr
		}
		lines = append(lines, "  Status: "+col(m.pairingStatus))
		lines = append(lines, "")
	}
	h := len(lines) + 4
	return ui.RenderPane("* ADB Wireless Pairing", lines, w-ui.PaneFrameW, h)
}

func (m *Model) renderConfirmDeleteOverlay() string {
	w := min(52, max(40, m.width-16))
	innerW := w - ui.PaneFrameW

	noStyle := ui.StyleMuted("[ No ]")
	yesStyle := ui.StyleMuted("[ Yes ]")
	if m.deleteChoice == 0 {
		noStyle = ui.StyleWarn("[ No ]")
	} else {
		yesStyle = ui.StyleErr("[ Yes ]")
	}

	typeLabel := m.deleteType
	lines := []string{
		"",
		"  Permanently delete this " + typeLabel + "?",
		"",
		"  " + ui.StyleErr(m.deleteTarget),
		"",
		"  " + noStyle + "   " + yesStyle,
		"",
		"  " + ui.StyleMuted("← → to choose   Enter to confirm   Esc to cancel"),
		"",
	}
	return ui.RenderPane("⚠ Confirm Delete", lines, innerW, len(lines)+ui.PaneFrameH)
}

func (m *Model) renderNicknameEntryOverlay() string {
	w := min(56, max(44, m.width-14))
	innerW := w - ui.PaneFrameW

	// Find the device display name to show in the prompt.
	deviceLabel := m.nicknameEntryHost
	for i := range m.config.KnownDevices {
		if m.config.KnownDevices[i].Alias == m.nicknameEntryHost {
			if dn := m.config.KnownDevices[i].DeviceName; dn != "" {
				deviceLabel = dn
			} else if mod := m.config.KnownDevices[i].Model; mod != "" {
				deviceLabel = mod
			}
			break
		}
	}

	cursor := ui.StyleGood("|")
	inputLine := ui.StyleGood(m.nicknameInput) + cursor
	lines := []string{
		ui.StyleMuted("Device: ") + deviceLabel,
		"",
		"Give this device a nickname (optional).",
		"Leave empty to use the device name.",
		"",
		ui.StyleMuted("Nickname: ") + inputLine,
		"",
		ui.StyleMuted("Enter=save  Esc=skip"),
	}
	return ui.RenderPane("★ Name Device", lines, innerW, len(lines)+ui.PaneFrameH)
}

func (m *Model) renderPortUpdateOverlay() string {
	w := min(58, max(46, m.width-12))
	innerW := w - ui.PaneFrameW

	deviceLabel := m.portUpdateHost
	for i := range m.config.KnownDevices {
		kd := &m.config.KnownDevices[i]
		for _, ep := range kd.Endpoints {
			if ep.Host == m.portUpdateHost {
				deviceLabel = kd.DisplayName()
				break
			}
		}
	}

	cursor := ui.StyleGood("|")
	inputLine := ui.StyleGood(m.portUpdateInput) + cursor
	statusLine := ""
	if m.portUpdateStatus != "" {
		statusLine = ui.StyleErr(m.portUpdateStatus)
	}
	lines := []string{
		ui.StyleErr("Connection refused for: ") + deviceLabel,
		ui.StyleMuted("(" + m.portUpdateHost + ")"),
		"",
		"Wireless Debugging changes its connect port",
		"each time the phone reconnects to Wi-Fi.",
		"",
		"Open Wireless Debugging on the phone and",
		"enter the port shown next to the IP address.",
		"",
		ui.StyleMuted("New connect port: ") + inputLine,
	}
	if statusLine != "" {
		lines = append(lines, "", statusLine)
	}
	lines = append(lines, "", ui.StyleMuted("Enter=connect  Esc=cancel"))
	return ui.RenderPane("⚡ Update Connect Port", lines, innerW, len(lines)+ui.PaneFrameH)
}

func (m *Model) renderDeviceEditorOverlay() string {
	w := min(62, max(48, m.width-10))
	innerW := w - ui.PaneFrameW

	entries := m.mergedDeviceList()
	if m.deviceIdx >= len(entries) || !entries[m.deviceIdx].IsKnown {
		return ui.RenderPane("* Edit Device", []string{"No known device selected."}, innerW, 4)
	}
	kd := entries[m.deviceIdx].Known

	fieldStyle := func(val string, maxW int, active bool) string {
		padded := "[" + padRight(val, maxW) + "]"
		if active {
			return lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color(ui.Active.Info)).
				Background(lipgloss.Color(ui.Active.PaneBg)).
				Render(padded)
		}
		return ui.StyleMuted(padded)
	}

	buf := m.deviceEditorBuffer
	get := func(i int) string {
		if i < len(buf) {
			return buf[i]
		}
		return ""
	}
	cursor := func(i int) string {
		if m.deviceEditorField == i {
			return get(i) + "_"
		}
		return get(i)
	}

	// buf layout: [nickname, alias, ep0.host, ep0.port, ...]
	lines := []string{""}
	lines = append(lines, "  Nickname       "+fieldStyle(cursor(0), 28, m.deviceEditorField == 0))
	lines = append(lines, "  "+ui.StyleMuted("(empty = use device name or IP)"))
	lines = append(lines, "")
	lines = append(lines, "  Alias (IP)     "+fieldStyle(cursor(1), 28, m.deviceEditorField == 1))
	for i, ep := range kd.Endpoints {
		lines = append(lines, "")
		epLabel := ep.Name
		if epLabel == "" {
			epLabel = fmt.Sprintf("Endpoint %d", i+1)
		}
		lines = append(lines, "  "+ui.StyleMuted("── "+epLabel+" ──────────────────"))
		hostIdx := 2 + i*2
		portIdx := 3 + i*2
		lines = append(lines, "  Host          "+fieldStyle(cursor(hostIdx), 25, m.deviceEditorField == hostIdx))
		lines = append(lines, "  Port          "+fieldStyle(cursor(portIdx), 5, m.deviceEditorField == portIdx))
	}
	lines = append(lines, "")
	lines = append(lines, "  "+ui.StyleMuted("Tab/↑↓=navigate   Enter=save   Esc=cancel"))
	lines = append(lines, "")

	title := "* Edit Device"
	if len(buf) >= 2 && strings.TrimSpace(buf[0]) != "" {
		title = "* Edit: " + strings.TrimSpace(buf[0])
	} else if len(buf) >= 2 && strings.TrimSpace(buf[1]) != "" {
		title = "* Edit Device: " + strings.TrimSpace(buf[1])
	}
	return ui.RenderPane(title, lines, innerW, len(lines)+4)
}

func (m *Model) renderHelpOverlay() string {
	w := min(72, max(52, m.width-8))
	lines := []string{
		"",
		"  Navigation",
		"    Tab / Shift+Tab    cycle panes: Devices → Profiles → Panel",
		"    Ctrl+↑↓←→         jump to pane directly",
		"    ↑ / ↓             navigate list in focused pane",
		"    Mouse click       focus and select item",
		"",
		"  Launch",
		"    Enter / S         launch scrcpy session for selected device",
		"    z                 cancel in-progress launch",
		"    R                 refresh device list",
		"",
		"  Profiles",
		"    E  rename         O  edit options",
		"    N  new profile    d  duplicate",
		"    D  delete (safe)  F  set default",
		"    1  turn-screen-off    2  stay-awake",
		"    3  prefer-H265        4  require-audio",
		"    5  require-camera",
		"    +  bitrate +2M        -  bitrate −2M",
		"",
		"  Overlays",
		"    p  assign profile to device",
		"    W  warnings & compatibility details",
		"    P  ADB wireless pairing",
		"    B  first-time setup guide",
		"    ?  this help screen",
		"",
		"  Appearance",
		"    [  previous theme     ]  next theme",
		"",
		"    q  quit     Ctrl+C  force quit     Esc  close overlay",
		"",
	}
	if m.version != "" {
		lines = append(lines, ui.StyleMuted("    "+m.version))
		lines = append(lines, "")
	}
	h := len(lines) + 4
	return ui.RenderPane("? Help — Screener", lines, w-ui.PaneFrameW, h)
}

var onboardingPages = []struct {
	title string
	lines []string
}{
	{
		title: "Step 1/5 — Enable Developer Options",
		lines: []string{
			"",
			"  On your Android phone:",
			"",
			"  1. Open Settings → About phone",
			"  2. Tap \"Build number\" 7 times rapidly",
			"  3. You'll see: \"You are now a developer!\"",
			"",
			"  If you already have Developer Options, skip ahead.",
			"",
			"  ──────────────────────────────────────────────",
			"  → / Enter = next step     ← / P = back     Esc = close",
			"",
		},
	},
	{
		title: "Step 2/5 — Enable Wireless Debugging",
		lines: []string{
			"",
			"  On your Android phone (Android 11+ required):",
			"",
			"  1. Open Settings → Developer options",
			"  2. Scroll to find \"Wireless debugging\"",
			"  3. Toggle it ON — confirm the prompt",
			"  4. Your phone will show its IP address and port",
			"     e.g.  192.168.1.5:37123",
			"",
			"  Keep the Wireless debugging screen open for the next step.",
			"",
			"  ──────────────────────────────────────────────",
			"  → / Enter = next     ← = back     Esc = close",
			"",
		},
	},
	{
		title: "Step 3/5 — Get the Pairing Code",
		lines: []string{
			"",
			"  Still on the Wireless debugging screen:",
			"",
			"  1. Tap \"Pair device with pairing code\"",
			"  2. Note the pairing address, port, and 6-digit code",
			"     e.g.  Address: 192.168.1.5:44421   Code: 123456",
			"",
			"  The pairing port differs from the debugging port!",
			"  Use the pairing address for the P dialog (next).",
			"",
			"  ──────────────────────────────────────────────",
			"  → / Enter = next     ← = back     Esc = close",
			"",
		},
	},
	{
		title: "Step 4/5 — Pair in Screener",
		lines: []string{
			"",
			"  Back in Screener:",
			"",
			"  1. Press P to open the pairing dialog",
			"  2. Enter the pairing address and port shown on your phone",
			"     e.g.  192.168.1.5:44421",
			"  3. Enter the 6-digit pairing code",
			"  4. Press Enter — you'll see \"paired successfully\"",
			"",
			"  After pairing, Screener connects and saves the device.",
			"  Future sessions reconnect automatically.",
			"",
			"  ──────────────────────────────────────────────",
			"  → / Enter = open pairing     ← = back     Esc = close",
			"",
		},
	},
	{
		title: "Step 5/5 — Optional: Tailscale",
		lines: []string{
			"",
			"  To connect over Tailscale (away from home Wi-Fi):",
			"",
			"  1. Install the Tailscale app on your phone",
			"  2. Log in to the same tailnet as your laptop",
			"  3. Enable \"Accept routes\" in Tailscale settings",
			"",
			"  Once both devices are on the same tailnet:",
			"  • Screener auto-detects the tailscale0 IP (100.x.x.x)",
			"  • A \"Tailscale\" endpoint is added to your device record",
			"  • Connect from anywhere without home Wi-Fi",
			"",
			"  ──────────────────────────────────────────────",
			"  Enter = open pairing dialog     Esc = close",
			"",
		},
	},
}

func (m *Model) renderOnboardingOverlay() string {
	w := min(66, max(54, m.width-8))
	step := m.onboardingStep
	if step < 0 || step >= len(onboardingPages) {
		step = 0
	}
	page := onboardingPages[step]
	h := len(page.lines) + 4
	return ui.RenderPane(page.title, page.lines, w-ui.PaneFrameW, h)
}

func (m *Model) renderProfileEditorOverlay() string {
	w := min(66, max(48, m.width-8))
	p := m.activeProfilePtr()
	if p == nil {
		return ui.RenderPane("✎ Edit Profile", []string{"(no profile selected)"}, w-ui.PaneFrameW, 6)
	}

	lines := []string{""}
	for i := 0; i < pfFieldCount; i++ {
		val := m.pfFieldValue(p, i)
		focused := i == m.editorCursor
		hint := ""
		if focused {
			hint = ui.StyleMuted("  ←→")
		}
		marker := "  "
		styleVal := ui.StyleInfo
		if focused {
			marker = ui.Active.GoodStyle().Render("> ")
			styleVal = func(s string) string {
				return lipgloss.NewStyle().Bold(true).
					Foreground(lipgloss.Color(ui.Active.Info)).Render(s)
			}
		}
		row := fmt.Sprintf("%s%-18s %s%s", marker, pfLabels[i], styleVal(val), hint)
		lines = append(lines, row)
	}
	lines = append(lines, "")
	lines = append(lines, ui.StyleMuted("  ↑↓=field  ←→/Enter=change  Esc=close"))
	lines = append(lines, "")
	h := len(lines) + 4
	title := "✎ Options: " + truncateName(p.Name, 26)
	return ui.RenderPane(title, lines, w-ui.PaneFrameW, h)
}

// pfFieldValue returns the human-readable current value for a profile editor field.
func (m *Model) pfFieldValue(p *core.ProfileDefinition, field int) string {
	switch field {
	case pfLaunchMode:
		if p.Desired != nil {
			if v := p.Desired[core.DesiredKeyLaunchMode]; v != "" {
				return v
			}
		}
		return core.LaunchModeMainDisplay
	case pfMaxSize:
		if p.MaxSize == 0 {
			return "unlimited"
		}
		return itoa(p.MaxSize) + "px"
	case pfBitrateMB:
		return itoa(p.VideoBitRateMB) + "M"
	case pfMaxFPS:
		if p.Desired != nil {
			if fps := p.Desired[core.DesiredKeyMaxFPS]; fps != "" {
				return fps + "fps"
			}
		}
		return "unlimited"
	case pfTurnScreenOff:
		return pfBoolStr(pfFlagVal(p, "turn_screen_off"))
	case pfStayAwake:
		return pfBoolStr(pfFlagVal(p, "stay_awake"))
	case pfPreferH265:
		return pfBoolStr(pfFlagVal(p, "prefer_h265"))
	case pfRequireAudio:
		return pfBoolStr(pfFlagVal(p, "require_audio"))
	case pfGamepad:
		if p.Desired != nil {
			if g := p.Desired[core.DesiredKeyGamepad]; g != "" {
				return g
			}
		}
		return core.GamepadModeOff
	}
	return ""
}

// editorCycleField applies a +1 or -1 delta to the profile field at index,
// immediately applying and persisting the change.
func (m *Model) editorCycleField(field, delta int) {
	p := m.activeProfilePtr()
	if p == nil {
		return
	}
	if p.Desired == nil {
		p.Desired = map[string]string{}
	}
	if p.DesiredFlags == nil {
		p.DesiredFlags = map[string]bool{}
	}
	if p.FeatureFlags == nil {
		p.FeatureFlags = map[string]bool{}
	}

	switch field {
	case pfLaunchMode:
		cur := 0
		for i, c := range pfLaunchModeChoices {
			if c == p.Desired[core.DesiredKeyLaunchMode] {
				cur = i
				break
			}
		}
		next := (cur + delta + len(pfLaunchModeChoices)) % len(pfLaunchModeChoices)
		p.Desired[core.DesiredKeyLaunchMode] = pfLaunchModeChoices[next]

	case pfMaxSize:
		cur := 0
		for i, c := range pfMaxSizeChoices {
			if c == p.MaxSize {
				cur = i
				break
			}
		}
		next := (cur + delta + len(pfMaxSizeChoices)) % len(pfMaxSizeChoices)
		p.MaxSize = pfMaxSizeChoices[next]

	case pfBitrateMB:
		p.VideoBitRateMB += delta * 2
		if p.VideoBitRateMB < 2 {
			p.VideoBitRateMB = 2
		}
		if p.VideoBitRateMB > 32 {
			p.VideoBitRateMB = 32
		}

	case pfMaxFPS:
		cur := 0
		for i, c := range pfMaxFPSChoices {
			if fmt.Sprintf("%d", c) == p.Desired[core.DesiredKeyMaxFPS] {
				cur = i
				break
			}
		}
		next := (cur + delta + len(pfMaxFPSChoices)) % len(pfMaxFPSChoices)
		if pfMaxFPSChoices[next] == 0 {
			delete(p.Desired, core.DesiredKeyMaxFPS)
		} else {
			p.Desired[core.DesiredKeyMaxFPS] = fmt.Sprintf("%d", pfMaxFPSChoices[next])
		}

	case pfTurnScreenOff:
		cur := pfFlagVal(p, "turn_screen_off")
		p.DesiredFlags["turn_screen_off"] = !cur
		p.FeatureFlags["turn_screen_off"] = !cur

	case pfStayAwake:
		cur := pfFlagVal(p, "stay_awake")
		p.DesiredFlags["stay_awake"] = !cur
		p.FeatureFlags["stay_awake"] = !cur

	case pfPreferH265:
		cur := pfFlagVal(p, "prefer_h265")
		p.DesiredFlags["prefer_h265"] = !cur
		p.FeatureFlags["prefer_h265"] = !cur

	case pfRequireAudio:
		cur := pfFlagVal(p, "require_audio")
		p.DesiredFlags["require_audio"] = !cur
		p.FeatureFlags["require_audio"] = !cur

	case pfGamepad:
		cur := 0
		for i, c := range pfGamepadChoices {
			if c == p.Desired[core.DesiredKeyGamepad] {
				cur = i
				break
			}
		}
		next := (cur + delta + len(pfGamepadChoices)) % len(pfGamepadChoices)
		p.Desired[core.DesiredKeyGamepad] = pfGamepadChoices[next]
	}

	m.recomputePlanAndPreview()
	m.saveConfig()
	m.appendLog(fmt.Sprintf("editor field[%d] = %s", field, m.pfFieldValue(p, field)))
}

// pfFlagVal reads a boolean flag from DesiredFlags then FeatureFlags.
func pfFlagVal(p *core.ProfileDefinition, key string) bool {
	if p.DesiredFlags != nil {
		if v, ok := p.DesiredFlags[key]; ok {
			return v
		}
	}
	if p.FeatureFlags != nil {
		return p.FeatureFlags[key]
	}
	return false
}

func pfBoolStr(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

// ── Device / profile line renderers ──────────────────────────────────────────

func (m *Model) renderDeviceLines(entries []DeviceEntry, maxLabelW int) []string {
	if len(entries) == 0 {
		return []string{
			ui.StyleMuted("No devices."),
			"",
			ui.StyleMuted("P = pair  ↑↓ = navigate"),
		}
	}
	t := ui.Active
	lines := make([]string, 0, len(entries)+2)
	for i, e := range entries {
		focused := i == m.deviceIdx
		icon := deviceIconStr(e.State)
		var label string
		if e.IsKnown && e.Known != nil {
			label = e.Known.DisplayName()
		} else {
			label = e.Serial
			if e.Model != "" && e.Model != "?" {
				label += " · " + e.Model
			}
		}
		label = truncateName(label, maxLabelW)
		extra := ""
		if e.IsKnown && e.IsLive {
			extra = " " + t.GoodStyle().Render("[K]")
		} else if e.IsKnown {
			extra = " " + t.MutedStyle().Render("[K]")
		}
		// Show battery % and network badge for live devices.
		if e.IsLive && e.Caps.BatteryLevel > 0 {
			battStr := batteryBadge(e.Caps.BatteryLevel)
			extra = battStr + extra
		}
		if e.IsLive && e.Caps.TailscaleIP != "" {
			extra = " " + t.GoodStyle().Render("TS") + extra
		}
		if focused {
			lines = append(lines, t.GoodStyle().Render("> ")+icon+" "+
				lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(t.PaneFg)).
					Background(lipgloss.Color(t.PaneBg)).Render(label)+extra)
		} else {
			lines = append(lines, "  "+icon+" "+t.MutedStyle().Render(label)+extra)
		}
	}
	lines = append(lines, "")
	lines = append(lines, ui.StyleMuted("Enter=launch  p=profile  e=edit  D=del  r=refresh"))
	return lines
}

func (m *Model) renderProfileLines(scroll, itemRows, maxNameW int) []string {
	t := ui.Active
	n := len(m.config.Profiles)
	lines := make([]string, 0, itemRows+4)
	end := min(scroll+itemRows, n)
	for i := scroll; i < end; i++ {
		p := m.config.Profiles[i]
		focused := i == m.activeIdx
		name := truncateName(p.Name, maxNameW)
		defMark := ""
		if p.IsDefault {
			defMark = " " + t.MutedStyle().Render("[*]")
		}
		if focused {
			lines = append(lines,
				t.GoodStyle().Render("> ")+
					lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(t.PaneFg)).
						Background(lipgloss.Color(t.PaneBg)).Render(name)+defMark)
		} else {
			lines = append(lines, "  "+t.MutedStyle().Render(name)+defMark)
		}
	}
	// Scroll indicator when the profile list is longer than the visible window.
	if n > itemRows {
		indicator := fmt.Sprintf("  %d\u2013%d of %d", scroll+1, end, n)
		lines = append(lines, t.MutedStyle().Render(indicator))
	} else {
		lines = append(lines, "")
	}
	if m.renameMode {
		lines = append(lines, ui.StyleWarn("  rename> ")+m.renameBuffer+"_")
		lines = append(lines, ui.StyleMuted("  Enter=save  Esc=cancel"))
	} else {
		lines = append(lines, ui.StyleMuted("  E=rename  N=new  d=dup  D=delete  F=default"))
		lines = append(lines, ui.StyleMuted("  1-5=toggle  +/-=bitrate  [/]=theme"))
	}
	return lines
}

// ── Status bar helpers ────────────────────────────────────────────────────────

func (m *Model) contextKeys() string {
	if m.launchState == LaunchStateLaunching {
		return "z=cancel"
	}
	if m.overlayMode != OverlayNone {
		return "Esc=close"
	}
	if m.renameMode {
		return "Enter=save  Esc=cancel"
	}
	return "Enter=launch  p=profile  E=edit  O=opts  D=delete  P=pair  B=setup  ?=help  q=quit"
}

// ── Launch ────────────────────────────────────────────────────────────────────

func (m Model) doLaunch() (tea.Model, tea.Cmd) {
	if m.launchState == LaunchStateLaunching {
		m.appendLog("launch skipped: another launch is starting")
		return m, nil
	}
	if m.lastPlan == nil {
		m.recomputePlanAndPreview()
	}
	if m.lastPlan == nil {
		m.appendLog("launch skipped: no command plan")
		return m, nil
	}
	profileName := "<none>"
	if p := m.activeProfilePtr(); p != nil {
		profileName = p.Name
	}

	// Auto-connect: if the selected device is known but not currently live (no ADB
	// session), attempt adb connect for all its TCP endpoints before launching.
	if entries := m.mergedDeviceList(); m.deviceIdx < len(entries) {
		e := entries[m.deviceIdx]
		if !e.IsLive && e.IsKnown && e.Known != nil {
			var cmds []tea.Cmd
			for _, ep := range e.Known.Endpoints {
				if ep.Transport == "tcp" && ep.Host != "" {
					cmds = append(cmds, connectCmd(fmt.Sprintf("%s:%d", ep.Host, ep.Port)))
				}
			}
			if len(cmds) > 0 {
				m.pendingAutoLaunch = true
				m.appendLog("auto-connecting to " + e.Serial + " (profile=" + profileName + ")")
				return m, tea.Batch(cmds...)
			}
			m.appendLog("launch failed: " + e.Serial + " is offline and has no TCP endpoints")
			return m, nil
		}
	}

	caps := m.selectedDeviceCaps()
	m.appendLog("launch requested  profile=" + profileName)
	m.appendLog("launch command: " + m.preview)
	m.appendLog("launch env: " + platform.LaunchEnvDescription())
	m.appendLog(fmt.Sprintf("launch device: serial=%q sdk=%d source=%s",
		caps.Serial, caps.SDKInt, capSource(caps.Serial)))
	m.appendLog(fmt.Sprintf("launch diagnostics: warnings=%d unsupported=%d launchable=%t",
		len(m.lastResolve.Warnings), len(m.lastResolve.UnsupportedFeatures), m.lastPlan.Launchable))
	m.nextSID++
	sid := m.nextSID
	ctx, cancel := context.WithCancel(context.Background())
	m.launchCancel = cancel
	m.setLaunchState(LaunchStateLaunching)
	return m, startSessionCmd(m.lastPlan, ctx, sid, cancel, caps.Serial, profileName)
}

// ── Theme cycling ─────────────────────────────────────────────────────────────

func (m *Model) cycleTheme(delta int) {
	themes := ui.AllThemes()
	idx := 0
	for i, t := range themes {
		if t.Name == m.themeName {
			idx = i
			break
		}
	}
	next := (idx + delta + len(themes)) % len(themes)
	m.themeName = themes[next].Name
	t := themes[next]
	ui.SetTheme(t)
	m.matrix.SetPalette(t.MatrixPalette())
	m.matrix.SetBackground(t.PaneBg)
	m.config.Theme = m.themeName
	m.saveConfig()
	m.appendLog("theme: " + m.themeName)
}

// ── Device / caps model ───────────────────────────────────────────────────────

func (m *Model) mergedDeviceList() []DeviceEntry {
	entries := make([]DeviceEntry, 0, len(m.devices)+len(m.config.KnownDevices))
	for i, d := range m.devices {
		e := DeviceEntry{
			Serial: d.Serial, Model: d.Model, State: d.State,
			SDKInt: d.SDKInt, IsLive: true, Caps: m.devices[i],
		}
		for j := range m.config.KnownDevices {
			kd := &m.config.KnownDevices[j]
			if kd.Serial == d.Serial && d.Serial != "" {
				e.IsKnown, e.Known = true, kd
				break
			}
			for _, ep := range kd.Endpoints {
				if fmt.Sprintf("%s:%d", ep.Host, ep.Port) == d.Serial {
					e.IsKnown, e.Known = true, kd
					break
				}
			}
			if !e.IsKnown {
				// For TCP serials (host:port), also match by host alone — port changes
				// each time Wireless Debugging restarts, so an exact port match may be stale.
				liveHost := extractTCPHost(d.Serial)
				if liveHost != d.Serial { // d.Serial contained a colon → it's a TCP serial
					if kd.Alias == liveHost || kd.TailscaleIP == liveHost || kd.LocalWiFiIP == liveHost {
						e.IsKnown, e.Known = true, kd
					}
					if !e.IsKnown {
						for _, ep := range kd.Endpoints {
							if ep.Host == liveHost {
								e.IsKnown, e.Known = true, kd
								break
							}
						}
					}
				}
			}
			// USB serial: match by WiFi IP if both sides have it.
			if !e.IsKnown && kd.LocalWiFiIP != "" && kd.LocalWiFiIP == d.LocalWiFiIP {
				e.IsKnown, e.Known = true, kd
			}
			if e.IsKnown {
				break
			}
		}
		entries = append(entries, e)
	}
outer:
	for j := range m.config.KnownDevices {
		kd := &m.config.KnownDevices[j]
		for _, e := range entries {
			if (e.IsKnown && e.Known == kd) || (kd.Serial != "" && kd.Serial == e.Serial) {
				continue outer
			}
			if kd.LocalWiFiIP != "" && kd.LocalWiFiIP == e.Caps.LocalWiFiIP {
				continue outer
			}
			// TCP live device matched by host: don't also show as offline.
			liveHost := extractTCPHost(e.Serial)
			if liveHost != e.Serial {
				if kd.Alias == liveHost || kd.TailscaleIP == liveHost || kd.LocalWiFiIP == liveHost {
					continue outer
				}
				for _, ep := range kd.Endpoints {
					if ep.Host == liveHost {
						continue outer
					}
				}
			}
		}
		alias := kd.Alias
		if alias == "" {
			if len(kd.Endpoints) > 0 {
				alias = kd.Endpoints[0].Host
			} else {
				alias = "(unknown)"
			}
		}
		model := kd.Model
		if model == "" {
			model = "?"
		}
		entries = append(entries, DeviceEntry{
			Serial: alias, Model: model, State: "known-offline",
			SDKInt: 34, IsLive: false, IsKnown: true, Known: kd,
			Caps: core.DeviceCapabilitySnapshot{
				Serial: kd.Serial, Model: model, State: "offline",
				SDKInt: 34, SupportsH265: true, SupportsAudio: true,
				SupportsCamera: true, SupportsVirtualDisplay: true,
				SupportsGamepadUHID: true, SupportsGamepadAOA: true,
			},
		})
	}
	return entries
}

func (m *Model) selectedDeviceCaps() core.DeviceCapabilitySnapshot {
	entries := m.mergedDeviceList()
	if len(entries) == 0 || m.deviceIdx >= len(entries) {
		return simulatedCaps()
	}
	return entries[m.deviceIdx].Caps
}

func simulatedCaps() core.DeviceCapabilitySnapshot {
	return core.DeviceCapabilitySnapshot{
		Serial: "simulated", Model: "Simulated", State: "device", SDKInt: 34,
		SupportsH265: true, SupportsAudio: true, SupportsCamera: true,
		SupportsVirtualDisplay: true, SupportsGamepadUHID: true, SupportsGamepadAOA: true,
	}
}

// ── Profile helpers ───────────────────────────────────────────────────────────

func (m *Model) activeProfilePtr() *core.ProfileDefinition {
	if len(m.config.Profiles) == 0 {
		return nil
	}
	if m.activeIdx < 0 || m.activeIdx >= len(m.config.Profiles) {
		m.activeIdx = 0
	}
	return &m.config.Profiles[m.activeIdx]
}

func (m *Model) recomputePlanAndPreview() {
	profile := m.activeProfilePtr()
	if profile == nil {
		m.lastPlan, m.preview = nil, ""
		m.lastResolve = core.EffectiveProfileResolution{}
		return
	}
	m.config.ActiveProfile = profile.Name
	caps := m.selectedDeviceCaps()
	resolved := core.ResolveEffectiveProfile(*profile, caps)
	// Inject the serial so scrcpy targets the correct device when multiple are connected.
	targetSerial := ""
	if entries := m.mergedDeviceList(); m.deviceIdx < len(entries) {
		if e := entries[m.deviceIdx]; e.IsLive {
			targetSerial = e.Serial
		}
	}
	plan := scrcpy.BuildPlanFromResolution(resolved, targetSerial)
	if v := core.ValidateLaunch(plan.Args); !v.OK {
		plan.Launchable = false
		resolved.Warnings = append(resolved.Warnings, v.Errors...)
	}
	plan.Resolution = resolved
	m.lastResolve, m.lastPlan = resolved, plan
	m.preview = scrcpy.Preview(plan)
}

func (m *Model) saveConfig() {
	m.ensureDefaultProfile()
	if p := m.activeProfilePtr(); p != nil {
		m.config.ActiveProfile = p.Name
	}
	m.config.Theme = m.themeName
	if err := persistence.SaveAtomic(m.configPath, m.config); err != nil {
		m.appendLog("config save failed: " + err.Error())
	}
}

func (m *Model) mutateActiveProfile(mut func(*core.ProfileDefinition) string) {
	p := m.activeProfilePtr()
	if p == nil {
		return
	}
	msg := mut(p)
	m.recomputePlanAndPreview()
	m.saveConfig()
	m.appendLog(msg)
}

func (m *Model) toggleFlag(name string) {
	m.mutateActiveProfile(func(p *core.ProfileDefinition) string {
		if p.DesiredFlags == nil {
			p.DesiredFlags = map[string]bool{}
		}
		if p.FeatureFlags == nil {
			p.FeatureFlags = map[string]bool{}
		}
		next := !p.DesiredFlags[name]
		p.DesiredFlags[name] = next
		p.FeatureFlags[name] = next
		return fmt.Sprintf("%s=%t", name, next)
	})
}

func (m *Model) adjustBitrate(delta int) {
	m.mutateActiveProfile(func(p *core.ProfileDefinition) string {
		next := p.VideoBitRateMB + delta
		if next < 2 {
			next = 2
		}
		p.VideoBitRateMB = next
		if p.Desired == nil {
			p.Desired = map[string]string{}
		}
		p.Desired["video_bitrate_mb"] = itoa(next)
		return fmt.Sprintf("bitrate=%dM", next)
	})
}

func (m *Model) duplicateActiveProfile() {
	p := m.activeProfilePtr()
	if p == nil {
		return
	}
	cp := *p
	cp.Name = m.nextProfileName(p.Name + " Copy")
	cp.IsDefault = false
	cp.ProfileID = ""
	m.config.Profiles = append(m.config.Profiles, cp)
	m.activeIdx = len(m.config.Profiles) - 1
	m.ensureDefaultProfile()
	m.recomputePlanAndPreview()
	m.saveConfig()
	m.appendLog("profile duplicated: " + cp.Name)
}

func (m *Model) deleteSelectedDevice() {
	entries := m.mergedDeviceList()
	if m.deviceIdx >= len(entries) {
		return
	}
	e := entries[m.deviceIdx]
	if !e.IsKnown || e.Known == nil {
		m.appendLog("device delete skipped: not a known device")
		return
	}
	name := e.Known.DisplayName()
	for i := range m.config.KnownDevices {
		if &m.config.KnownDevices[i] == e.Known {
			m.config.KnownDevices = append(m.config.KnownDevices[:i], m.config.KnownDevices[i+1:]...)
			break
		}
	}
	newEntries := m.mergedDeviceList()
	if m.deviceIdx >= len(newEntries) {
		m.deviceIdx = max(0, len(newEntries)-1)
	}
	m.saveConfig()
	m.appendLog("device deleted: " + name)
}

func (m *Model) deleteActiveProfile() {
	if len(m.config.Profiles) <= 1 {
		m.appendLog("delete skipped: need at least one profile")
		return
	}
	name := m.config.Profiles[m.activeIdx].Name
	m.config.Profiles = append(m.config.Profiles[:m.activeIdx], m.config.Profiles[m.activeIdx+1:]...)
	if m.activeIdx >= len(m.config.Profiles) {
		m.activeIdx = len(m.config.Profiles) - 1
	}
	m.ensureDefaultProfile()
	m.recomputePlanAndPreview()
	m.saveConfig()
	m.appendLog("profile deleted: " + name)
}

func (m *Model) ensureDefaultProfile() {
	if len(m.config.Profiles) == 0 {
		p := core.DefaultProfile()
		m.config.Profiles = []core.ProfileDefinition{p}
		m.activeIdx = 0
		m.config.ActiveProfile = p.Name
		return
	}
	first := -1
	for i := range m.config.Profiles {
		if m.config.Profiles[i].IsDefault {
			if first == -1 {
				first = i
			} else {
				m.config.Profiles[i].IsDefault = false
			}
		}
	}
	if first == -1 {
		idx := m.activeIdx
		if idx < 0 || idx >= len(m.config.Profiles) {
			idx = 0
		}
		m.config.Profiles[idx].IsDefault = true
	}
}

func (m *Model) setDefaultProfile(index int) {
	if index < 0 || index >= len(m.config.Profiles) {
		return
	}
	for i := range m.config.Profiles {
		m.config.Profiles[i].IsDefault = i == index
	}
	m.recomputePlanAndPreview()
	m.saveConfig()
	m.appendLog("default profile: " + m.config.Profiles[index].Name)
}

func (m *Model) createProfileFromDefault() {
	p := core.DefaultProfile()
	p.Name = m.nextProfileName(p.Name)
	p.IsDefault = false
	p.ProfileID = ""
	m.config.Profiles = append(m.config.Profiles, p)
	m.activeIdx = len(m.config.Profiles) - 1
	m.ensureDefaultProfile()
	m.recomputePlanAndPreview()
	m.saveConfig()
	m.appendLog("profile created: " + p.Name)
}

func (m *Model) startRename() {
	p := m.activeProfilePtr()
	if p == nil {
		return
	}
	m.renameMode = true
	m.renameBuffer = p.Name
	m.appendLog("rename started")
}

func (m *Model) commitRename() {
	p := m.activeProfilePtr()
	if p == nil {
		m.cancelRename()
		return
	}
	name := strings.TrimSpace(m.renameBuffer)
	if name == "" {
		m.cancelRename()
		return
	}
	if name != p.Name && m.hasProfileName(name) {
		name = m.nextProfileName(name)
	}
	old := p.Name
	p.Name = name
	m.renameMode = false
	m.renameBuffer = ""
	m.recomputePlanAndPreview()
	m.saveConfig()
	m.appendLog("renamed: " + old + " → " + name)
}

func (m *Model) cancelRename() {
	m.renameMode = false
	m.renameBuffer = ""
}

func (m *Model) switchActiveProfile(delta int) {
	n := len(m.config.Profiles)
	if n <= 1 {
		return
	}
	next := (m.activeIdx + delta + n) % n
	if next == m.activeIdx {
		return
	}
	m.activeIdx = next
	m.recomputePlanAndPreview()
	m.saveConfig()
	m.appendLog("profile: " + m.config.Profiles[next].Name)
}

func (m *Model) switchActiveDevice(delta int) {
	entries := m.mergedDeviceList()
	if len(entries) == 0 {
		return
	}
	n := len(entries)
	next := (m.deviceIdx + delta + n) % n
	m.deviceIdx = next
	m.recomputePlanAndPreview()
	m.appendLog("device: " + entries[next].Serial)
}

func (m *Model) nextProfileName(base string) string {
	if !m.hasProfileName(base) {
		return base
	}
	for i := 2; ; i++ {
		c := fmt.Sprintf("%s %d", base, i)
		if !m.hasProfileName(c) {
			return c
		}
	}
}

func (m *Model) hasProfileName(name string) bool {
	for _, p := range m.config.Profiles {
		if p.Name == name {
			return true
		}
	}
	return false
}

// ── Device editor ─────────────────────────────────────────────────────────────

func (m *Model) openDeviceEditor() {
	entries := m.mergedDeviceList()
	if m.deviceIdx >= len(entries) {
		return
	}
	e := entries[m.deviceIdx]
	if !e.IsKnown || e.Known == nil {
		m.appendLog("device editor: selected device is not a known device")
		return
	}
	kd := e.Known
	// buf layout: [nickname, alias, ep0.host, ep0.port, ep1.host, ep1.port, ...]
	buf := []string{kd.Nickname, kd.Alias}
	for _, ep := range kd.Endpoints {
		buf = append(buf, ep.Host)
		buf = append(buf, strconv.Itoa(ep.Port))
	}
	m.deviceEditorBuffer = buf
	m.deviceEditorField = 0
	m.overlayMode = OverlayDeviceEditor
	m.appendLog("device editor opened: " + kd.Alias)
}

func (m *Model) saveDeviceEditor() {
	entries := m.mergedDeviceList()
	if m.deviceIdx >= len(entries) {
		return
	}
	e := entries[m.deviceIdx]
	if !e.IsKnown || e.Known == nil {
		return
	}
	// Find the KnownDevice by pointer identity within the config slice.
	kdIdx := -1
	for i := range m.config.KnownDevices {
		if &m.config.KnownDevices[i] == e.Known {
			kdIdx = i
			break
		}
	}
	if kdIdx < 0 {
		return
	}
	buf := m.deviceEditorBuffer
	if len(buf) < 2 {
		return
	}
	// buf layout: [nickname, alias, ep0.host, ep0.port, ...]
	m.config.KnownDevices[kdIdx].Nickname = strings.TrimSpace(buf[0])
	if alias := strings.TrimSpace(buf[1]); alias != "" {
		m.config.KnownDevices[kdIdx].Alias = alias
	}
	for i := range m.config.KnownDevices[kdIdx].Endpoints {
		hostIdx := 2 + i*2
		portIdx := 3 + i*2
		if hostIdx >= len(buf) {
			break
		}
		if host := strings.TrimSpace(buf[hostIdx]); host != "" {
			m.config.KnownDevices[kdIdx].Endpoints[i].Host = host
		}
		if portIdx < len(buf) {
			if p, err := strconv.Atoi(strings.TrimSpace(buf[portIdx])); err == nil && p > 0 {
				m.config.KnownDevices[kdIdx].Endpoints[i].Port = p
			}
		}
	}
	// Refresh the saved serial to match the first TCP endpoint's new host:port.
	for _, ep := range m.config.KnownDevices[kdIdx].Endpoints {
		if ep.Transport == "tcp" {
			m.config.KnownDevices[kdIdx].Serial = fmt.Sprintf("%s:%d", ep.Host, ep.Port)
			break
		}
	}
	m.saveConfig()
	m.appendLog("device saved: " + m.config.KnownDevices[kdIdx].Alias)
}

// ── Pairing ───────────────────────────────────────────────────────────────────

func (m *Model) openPairingOverlay() {
	m.overlayMode = OverlayPairing
	m.pairingField = 0
	m.pairingStatus = ""
	// Pre-fill connect port with 5555 if not already set from a prior session.
	if m.pairingConnectPort == "" {
		m.pairingConnectPort = "5555"
	}
	// Preserve all other fields from prior session so the user can retry without retyping.
	m.appendLog("pairing dialog opened")
}

func (m *Model) closePairingOverlay() {
	m.overlayMode = OverlayNone
	m.pairingStatus = ""
	m.appendLog("pairing dialog closed")
}

func (m *Model) reconnectKnownDevicesCmd() tea.Cmd {
	var cmds []tea.Cmd
	for _, kd := range m.config.KnownDevices {
		for _, ep := range kd.Endpoints {
			if ep.Transport == "tcp" && ep.Host != "" {
				cmds = append(cmds, connectCmd(fmt.Sprintf("%s:%d", ep.Host, ep.Port)))
			}
		}
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

// ── Commands ──────────────────────────────────────────────────────────────────

// Tick message constructors are named so the closure body is reachable by tests.
func makeTickMsg(t time.Time) tea.Msg        { return tickMsg(t) }
func makeDevicePollMsg(t time.Time) tea.Msg  { return devicePollMsg(t) }
func makeLaunchResetMsg(time.Time) tea.Msg   { return launchResetMsg{} }

func tickCmd() tea.Cmd {
	return tea.Tick(120*time.Millisecond, makeTickMsg)
}

// devicePollCmd fires every 10 seconds to trigger device re-discovery and
// heartbeat data (battery, storage, IPs) independently of the animation tick.
func devicePollCmd() tea.Cmd {
	return tea.Tick(10*time.Second, makeDevicePollMsg)
}

// launchResetCmd returns a one-shot timer that resets a terminal launch state
// (succeeded/failed/canceled) back to idle so operators don't see stale badges.
func launchResetCmd() tea.Cmd {
	return tea.Tick(6*time.Second, makeLaunchResetMsg)
}

func refreshDevicesCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		devices, err := adb.Discover(ctx)
		if err != nil {
			return devicesMsg{err: err}
		}
		return devicesMsg{devices: devices}
	}
}

func pairCmd(hostPort, code string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		result, err := adb.Pair(ctx, hostPort, code)
		return pairResultMsg{result: result, hostPort: hostPort, err: err}
	}
}

func connectCmd(hostPort string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		out, err := adb.Connect(ctx, hostPort)
		return connectResultMsg{hostPort: hostPort, output: out, err: err}
	}
}

func disconnectCmd(hostPort string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = adb.Disconnect(ctx, hostPort)
		return disconnectResultMsg{hostPort: hostPort}
	}
}

// purgeStaleDevice removes a known device by host from config and returns
// disconnect commands for all its TCP endpoints so ADB's daemon state is cleared.
func (m *Model) purgeStaleDevice(host string) []tea.Cmd {
	var cmds []tea.Cmd
	kept := m.config.KnownDevices[:0]
	for _, kd := range m.config.KnownDevices {
		isMatch := kd.Alias == host || kd.LocalWiFiIP == host || kd.TailscaleIP == host
		for _, ep := range kd.Endpoints {
			if ep.Host == host {
				isMatch = true
				break
			}
		}
		if isMatch {
			// Disconnect every TCP endpoint so ADB drops its cached state.
			for _, ep := range kd.Endpoints {
				if ep.Transport == "tcp" {
					cmds = append(cmds, disconnectCmd(fmt.Sprintf("%s:%d", ep.Host, ep.Port)))
				}
			}
			m.appendLog("purged stale device: " + kd.Alias)
			continue // drop from config
		}
		kept = append(kept, kd)
	}
	m.config.KnownDevices = kept
	m.saveConfig()
	return cmds
}

// isKnownDeviceHost returns true when host matches any saved KnownDevice's endpoint or alias.
func (m *Model) isKnownDeviceHost(host string) bool {
	for _, kd := range m.config.KnownDevices {
		if kd.Alias == host || kd.LocalWiFiIP == host || kd.TailscaleIP == host {
			return true
		}
		for _, ep := range kd.Endpoints {
			if ep.Host == host {
				return true
			}
		}
	}
	return false
}

func startSessionCmd(plan *scrcpy.CommandPlan, ctx context.Context, sid sessionID, cancel context.CancelFunc, serial, profileID string) tea.Cmd {
	return func() tea.Msg {
		if ctx == nil {
			ctx = context.Background()
		}
		if !platform.IsAvailable(plan.Binary) {
			err := fmt.Errorf("exec: %q: executable file not found in $PATH", plan.Binary)
			return launchMsg{reason: adb.ClassifyFailure(err), err: err, cancel: cancel}
		}
		sess, err := scrcpy.ExecuteDetached(ctx, plan)
		if err != nil {
			return launchMsg{reason: adb.ClassifyFailure(err), err: err, cancel: cancel}
		}
		return launchMsg{reason: adb.FailureNone, sid: sid, session: sess, cancel: cancel, serial: serial, profileID: profileID}
	}
}

func monitorSessionCmd(sid sessionID, sess *scrcpy.Session) tea.Cmd {
	return func() tea.Msg {
		res, err := sess.Wait()
		return sessionExitedMsg{id: sid, res: res, err: err}
	}
}

func (m *Model) setLaunchState(next LaunchState) {
	prev := m.launchState
	m.launchState = next
	m.appendLog(fmt.Sprintf("launch %s → %s", prev, next))
}

func launchDetail(res scrcpy.ExecutionResult) string {
	var parts []string
	if s := strings.TrimSpace(res.Stderr); s != "" {
		const lim = 240
		if r := []rune(s); len(r) > lim {
			s = string(r[:lim]) + "…"
		}
		parts = append(parts, "stderr="+s)
	}
	if res.ExitCode > 0 {
		parts = append(parts, fmt.Sprintf("exit_code=%d", res.ExitCode))
	}
	if res.TimedOut {
		parts = append(parts, "timed_out=true")
	}
	if res.Canceled {
		parts = append(parts, "canceled=true")
	}
	return strings.Join(parts, " ")
}

const maxLogLines = 2000

func (m *Model) appendLog(msg string) {
	line := time.Now().Format(time.RFC3339) + " " + msg
	m.logs = append(m.logs, line)
	// Cap the in-memory buffer so long-running sessions don't grow unboundedly.
	if len(m.logs) > maxLogLines {
		m.logs = m.logs[len(m.logs)-maxLogLines:]
	}
	if m.logFile != nil {
		_, _ = fmt.Fprintln(m.logFile, line)
	}
}

// ── Misc ──────────────────────────────────────────────────────────────────────

func (m Model) isTerminalTooSmall() bool {
	return m.width < minFullLayoutWidth || m.height < minFullLayoutHeight
}


func capSource(serial string) string {
	if serial == "" || serial == "simulated" {
		return "simulated"
	}
	return "live"
}

func extractTCPHost(hostPort string) string {
	if idx := strings.LastIndex(hostPort, ":"); idx >= 0 {
		return hostPort[:idx]
	}
	return hostPort
}

func batteryBadge(level int) string {
	s := fmt.Sprintf(" %d%%", level)
	switch {
	case level >= 60:
		return ui.StyleGood(s)
	case level >= 20:
		return ui.StyleWarn(s)
	default:
		return ui.StyleErr(s)
	}
}

func fmtBytes(b int64) string {
	const gb = 1024 * 1024 * 1024
	const mb = 1024 * 1024
	switch {
	case b >= gb:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(gb))
	case b >= mb:
		return fmt.Sprintf("%.0f MB", float64(b)/float64(mb))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

func deviceIconStr(state string) string {
	switch state {
	case "device":
		return ui.StyleGood("●")
	case "unauthorized":
		return ui.StyleWarn("⚠")
	case "offline", "known-offline":
		return ui.StyleMuted("○")
	default:
		return ui.StyleMuted("·")
	}
}

func flagRow(p *core.ProfileDefinition) string {
	return flagBadge(p, "turn_screen_off", "screen-off") + "  " +
		flagBadge(p, "stay_awake", "stay-awake") + "  " +
		flagBadge(p, "prefer_h265", "H265") + "  " +
		flagBadge(p, "require_audio", "audio")
}

func flagBadge(p *core.ProfileDefinition, key, label string) string {
	on := false
	if p.DesiredFlags != nil {
		on = p.DesiredFlags[key]
	} else if p.FeatureFlags != nil {
		on = p.FeatureFlags[key]
	}
	if on {
		return ui.StyleGood(label + " ✓")
	}
	return ui.StyleMuted(label + " ○")
}

func profileModeLabel(p *core.ProfileDefinition) string {
	if p.Desired != nil {
		if v := p.Desired["new_display"]; v == "true" || v == "1" {
			return "Virtual Display"
		}
	}
	for _, a := range p.ExtraArgs {
		if a == "--new-display" || strings.HasPrefix(a, "--new-display=") {
			return "Virtual Display"
		}
	}
	return fmt.Sprintf("Main (id=%d)", p.DisplayID)
}

func wrapText(s string, maxW int) []string {
	if maxW <= 0 {
		return []string{s}
	}
	words := strings.Fields(s)
	if len(words) == 0 {
		return nil
	}
	var out []string
	cur := words[0]
	for _, w := range words[1:] {
		if len(cur)+1+len(w) <= maxW {
			cur += " " + w
		} else {
			out = append(out, cur)
			cur = "  " + w
		}
	}
	out = append(out, cur)
	return out
}

func padRight(s string, n int) string {
	r := []rune(s)
	if len(r) >= n {
		return string(r[:n])
	}
	return s + strings.Repeat(" ", n-len(r))
}

func itoa(n int) string { return fmt.Sprintf("%d", n) }

func newView(content string) tea.View {
	v := tea.NewView(content)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

// truncateName clips s to maxW visible runes, appending '…' if truncated.
// maxW <= 0 is treated as unlimited.
func truncateName(s string, maxW int) string {
	if maxW <= 0 {
		return s
	}
	r := []rune(s)
	if len(r) <= maxW {
		return s
	}
	if maxW <= 1 {
		return "…"
	}
	return string(r[:maxW-1]) + "…"
}

func isPrintableText(text string) bool {
	r := []rune(text)
	return len(r) == 1 && unicode.IsPrint(r[0])
}
