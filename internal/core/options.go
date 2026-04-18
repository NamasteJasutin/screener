package core

import "fmt"

// ScrcpyOptionType defines how a flag value is presented and edited.
type ScrcpyOptionType int

const (
	OptBool   ScrcpyOptionType = iota // --flag present or absent
	OptEnum                           // --flag=choice from a fixed list
	OptInt                            // --flag=integer
	OptString                         // --flag=freeform text
)

// OptionGroup labels for UI section navigation.
const (
	GroupVideo      = "Video"
	GroupAudio      = "Audio"
	GroupCamera     = "Camera"
	GroupInput      = "Input"
	GroupScreen     = "Screen"
	GroupWindow     = "Window"
	GroupRecording  = "Recording"
	GroupConnection = "Connection"
	GroupAdvanced   = "Advanced"
)

// ScrcpyOption describes one scrcpy command-line flag.
// All SDK requirements are based on the Android version that introduced the
// underlying API. The binary is treated as a black box — we never emit flags
// that are gated behind requirements the device cannot satisfy.
type ScrcpyOption struct {
	ID      string           // internal key used in Desired/DesiredFlags maps
	Flag    string           // e.g. "--video-codec"
	Label   string           // human-readable name in the editor
	Desc    string           // one-line description shown in the editor
	Type    ScrcpyOptionType
	Choices []string         // for OptEnum: ordered list of valid values
	Default string           // default value (empty string = not set by default)
	Group   string           // one of the Group* constants

	// Compatibility gates — any unmet condition marks the option Incompatible.
	MinSDK      int    // minimum Android SDK; 0 = any
	RequiresUSB bool   // must be USB transport (AOA)
	RequiresOpt string // another option that must be set, e.g. "video_source=camera"
	LinuxOnly   bool   // Linux-specific (v4l2)
}

// CompatResult explains why an option is or is not compatible.
type CompatResult struct {
	Compatible bool
	Reason     string // non-empty when !Compatible
}

// Check returns whether this option can be used with the given device caps and transport.
// transport: "usb" | "tcp" | ""
func (opt ScrcpyOption) Check(caps DeviceCapabilitySnapshot, transport string) CompatResult {
	if opt.MinSDK > 0 && caps.SDKInt > 0 && caps.SDKInt < opt.MinSDK {
		androidVer := sdkToAndroid(opt.MinSDK)
		return CompatResult{
			Compatible: false,
			Reason: fmt.Sprintf("requires Android %s (SDK %d) — device has SDK %d",
				androidVer, opt.MinSDK, caps.SDKInt),
		}
	}
	if opt.RequiresUSB && transport != "usb" {
		return CompatResult{
			Compatible: false,
			Reason:     "requires USB connection (AOA protocol)",
		}
	}
	// H265 codec — check device capability snapshot.
	if opt.ID == "video_codec" {
		// H265/AV1 are checked per-value in the resolver; the option itself is always visible.
		// Incompatibility is shown per-choice in the enum editor.
	}
	return CompatResult{Compatible: true}
}

func sdkToAndroid(sdk int) string {
	versions := map[int]string{
		29: "10", 30: "11", 31: "12", 32: "12L", 33: "13", 34: "14", 35: "15",
	}
	if v, ok := versions[sdk]; ok {
		return v
	}
	return fmt.Sprintf("SDK %d", sdk)
}

// GroupOrder defines the canonical display order of option groups.
var GroupOrder = []string{
	GroupVideo, GroupAudio, GroupCamera, GroupInput,
	GroupScreen, GroupWindow, GroupRecording, GroupConnection, GroupAdvanced,
}

// AllOptions returns the complete catalog of scrcpy 3.x options, ordered by
// group and then by importance within the group.
// This catalog is the single source of truth for what Screener can configure.
// Do not emit flags not in this list — the binary is treated as a black box.
func AllOptions() []ScrcpyOption {
	return []ScrcpyOption{

		// ── Video ─────────────────────────────────────────────────────────────

		{
			ID: "video_source", Flag: "--video-source", Label: "Video Source",
			Desc:    "display = mirror screen;  camera = camera passthrough (Android 12+)",
			Type:    OptEnum,
			Choices: []string{"display", "camera"},
			Default: "display", Group: GroupVideo,
			MinSDK: 31, // camera requires Android 12+; display has no minimum
			// Special: MinSDK only applies to the "camera" choice; shown but gated.
		},
		{
			ID: "display_id", Flag: "--display-id", Label: "Display ID",
			Desc:    "Physical display to mirror (0 = primary).  List with: scrcpy --list-displays",
			Type:    OptInt, Default: "0", Group: GroupVideo,
		},
		{
			ID: "new_display", Flag: "--new-display", Label: "New Virtual Display",
			Desc:    "Create a virtual display instead of mirroring an existing one (Android 10+)",
			Type:    OptBool, Default: "", Group: GroupVideo,
			MinSDK: 29,
		},
		{
			ID: "new_display_size", Flag: "--new-display", Label: "Virtual Display Size",
			Desc:    "Resolution for virtual display e.g. 1920x1080 or 1920x1080/420",
			Type:    OptString, Default: "", Group: GroupVideo,
			MinSDK: 29, RequiresOpt: "new_display",
		},
		{
			ID: "video_codec", Flag: "--video-codec", Label: "Video Codec",
			Desc:    "h264 = universal;  h265 = device-dependent;  av1 = Android 12+ device-dependent",
			Type:    OptEnum,
			Choices: []string{"h264", "h265", "av1"},
			Default: "h264", Group: GroupVideo,
		},
		{
			ID: "video_bit_rate", Flag: "--video-bit-rate", Label: "Video Bitrate",
			Desc:    "Encoding bitrate. Use suffix K or M, e.g. 8M, 4000K",
			Type:    OptString, Default: "8M", Group: GroupVideo,
		},
		{
			ID: "max_size", Flag: "--max-size", Label: "Max Size (px)",
			Desc:    "Limit width and height. 0 = unlimited. Preserves aspect ratio.",
			Type:    OptInt, Default: "0", Group: GroupVideo,
		},
		{
			ID: "max_fps", Flag: "--max-fps", Label: "Max FPS",
			Desc:    "Cap frame rate. 0 = unlimited. Officially Android 10+.",
			Type:    OptInt, Default: "0", Group: GroupVideo,
			MinSDK: 29,
		},
		{
			ID: "capture_orientation", Flag: "--capture-orientation", Label: "Capture Orientation",
			Desc:    "Rotate/flip before encoding. @ prefix locks to device orientation.",
			Type:    OptEnum,
			Choices: []string{"0", "90", "180", "270", "flip0", "flip90", "flip180", "flip270"},
			Default: "0", Group: GroupVideo,
		},
		{
			ID: "crop", Flag: "--crop", Label: "Crop",
			Desc:    "Crop before encoding: width:height:x:y (device natural orientation)",
			Type:    OptString, Default: "", Group: GroupVideo,
		},
		{
			ID: "angle", Flag: "--angle", Label: "Rotation Angle",
			Desc:    "Rotate video content clockwise by N degrees (arbitrary, not just 90°)",
			Type:    OptInt, Default: "0", Group: GroupVideo,
		},
		{
			ID: "video_buffer", Flag: "--video-buffer", Label: "Video Buffer (ms)",
			Desc:    "Buffering delay before display (ms). 0 = none. Reduces jitter at cost of latency.",
			Type:    OptInt, Default: "0", Group: GroupVideo,
		},
		{
			ID: "no_video", Flag: "--no-video", Label: "No Video",
			Desc:    "Disable video forwarding entirely (audio only session)",
			Type:    OptBool, Default: "", Group: GroupVideo,
		},
		{
			ID: "no_video_playback", Flag: "--no-video-playback", Label: "No Video Playback",
			Desc:    "Forward video to recorder/V4L2 only — do not display it",
			Type:    OptBool, Default: "", Group: GroupVideo,
		},

		// ── Audio ─────────────────────────────────────────────────────────────

		{
			ID: "no_audio", Flag: "--no-audio", Label: "Disable Audio",
			Desc:    "Do not forward audio. Use when audio capture fails or is unwanted.",
			Type:    OptBool, Default: "", Group: GroupAudio,
		},
		{
			ID: "audio_source", Flag: "--audio-source", Label: "Audio Source",
			Desc:    "output=full device audio;  playback=app audio;  mic=microphone",
			Type:    OptEnum,
			Choices: []string{
				"output", "playback",
				"mic", "mic-unprocessed", "mic-camcorder",
				"mic-voice-recognition", "mic-voice-communication",
				"voice-call", "voice-call-uplink", "voice-call-downlink", "voice-performance",
			},
			Default: "output", Group: GroupAudio,
			MinSDK: 29,
		},
		{
			ID: "audio_codec", Flag: "--audio-codec", Label: "Audio Codec",
			Desc:    "opus (default/low-latency)  aac  flac  raw (no compression)",
			Type:    OptEnum,
			Choices: []string{"opus", "aac", "flac", "raw"},
			Default: "opus", Group: GroupAudio,
			MinSDK: 29,
		},
		{
			ID: "audio_bit_rate", Flag: "--audio-bit-rate", Label: "Audio Bitrate",
			Desc:    "e.g. 128K (default), 256K, 64K",
			Type:    OptString, Default: "128K", Group: GroupAudio,
			MinSDK: 29,
		},
		{
			ID: "audio_buffer", Flag: "--audio-buffer", Label: "Audio Buffer (ms)",
			Desc:    "Buffering delay for audio. Lower = less latency, more glitches. Default 50.",
			Type:    OptInt, Default: "50", Group: GroupAudio,
			MinSDK: 29,
		},
		{
			ID: "audio_dup", Flag: "--audio-dup", Label: "Audio Dup",
			Desc:    "Keep playing audio on device while forwarding (requires audio-source=playback)",
			Type:    OptBool, Default: "", Group: GroupAudio,
			MinSDK: 29, RequiresOpt: "audio_source=playback",
		},
		{
			ID: "require_audio", Flag: "--require-audio", Label: "Require Audio",
			Desc:    "Abort if audio cannot be captured (default: fall back to video-only)",
			Type:    OptBool, Default: "", Group: GroupAudio,
		},
		{
			ID: "no_audio_playback", Flag: "--no-audio-playback", Label: "No Audio Playback",
			Desc:    "Forward audio to recorder only — do not play it on the computer",
			Type:    OptBool, Default: "", Group: GroupAudio,
		},

		// ── Camera ────────────────────────────────────────────────────────────
		// All camera options require --video-source=camera which requires Android 12+.

		{
			ID: "camera_facing", Flag: "--camera-facing", Label: "Camera Facing",
			Desc:    "Which camera to use",
			Type:    OptEnum,
			Choices: []string{"back", "front", "external"},
			Default: "back", Group: GroupCamera,
			MinSDK: 31, RequiresOpt: "video_source=camera",
		},
		{
			ID: "camera_id", Flag: "--camera-id", Label: "Camera ID",
			Desc:    "Specific camera ID. List available: scrcpy --list-cameras",
			Type:    OptString, Default: "", Group: GroupCamera,
			MinSDK: 31, RequiresOpt: "video_source=camera",
		},
		{
			ID: "camera_size", Flag: "--camera-size", Label: "Camera Size",
			Desc:    "Explicit capture resolution e.g. 1920x1080. List valid: scrcpy --list-camera-sizes",
			Type:    OptString, Default: "", Group: GroupCamera,
			MinSDK: 31, RequiresOpt: "video_source=camera",
		},
		{
			ID: "camera_fps", Flag: "--camera-fps", Label: "Camera FPS",
			Desc:    "Camera capture frame rate (default: Android default ~30fps)",
			Type:    OptInt, Default: "30", Group: GroupCamera,
			MinSDK: 31, RequiresOpt: "video_source=camera",
		},
		{
			ID: "camera_ar", Flag: "--camera-ar", Label: "Camera Aspect Ratio",
			Desc:    `Match by aspect ratio ± 10%. E.g. "sensor", "4:3", "1.6"`,
			Type:    OptString, Default: "", Group: GroupCamera,
			MinSDK: 31, RequiresOpt: "video_source=camera",
		},
		{
			ID: "camera_high_speed", Flag: "--camera-high-speed", Label: "High-Speed Mode",
			Desc:    "High frame rate capture (restricted to specific sizes — see --list-camera-sizes)",
			Type:    OptBool, Default: "", Group: GroupCamera,
			MinSDK: 31, RequiresOpt: "video_source=camera",
		},

		// ── Input ─────────────────────────────────────────────────────────────

		{
			ID: "keyboard", Flag: "--keyboard", Label: "Keyboard Mode",
			Desc:    "sdk=Android API;  uhid=HID kernel module;  aoa=USB HID;  disabled=no keyboard",
			Type:    OptEnum,
			Choices: []string{"sdk", "uhid", "aoa", "disabled"},
			Default: "sdk", Group: GroupInput,
		},
		{
			ID: "mouse", Flag: "--mouse", Label: "Mouse Mode",
			Desc:    "sdk=Android API;  uhid=HID relative;  aoa=USB HID;  disabled=no mouse",
			Type:    OptEnum,
			Choices: []string{"sdk", "uhid", "aoa", "disabled"},
			Default: "sdk", Group: GroupInput,
		},
		{
			ID: "gamepad", Flag: "--gamepad", Label: "Gamepad Mode",
			Desc:    "uhid=UHID gamepad (Android 10+);  aoa=USB AOA;  disabled=no gamepad",
			Type:    OptEnum,
			Choices: []string{"disabled", "uhid", "aoa"},
			Default: "disabled", Group: GroupInput,
			MinSDK: 29,
		},
		{
			ID: "no_control", Flag: "--no-control", Label: "Read-Only (No Control)",
			Desc:    "Mirror only — no keyboard/mouse/touch input forwarded",
			Type:    OptBool, Default: "", Group: GroupInput,
		},
		{
			ID: "prefer_text", Flag: "--prefer-text", Label: "Prefer Text Events",
			Desc:    "Inject letters/space as text events (avoids key-combo issues; breaks WASD in games)",
			Type:    OptBool, Default: "", Group: GroupInput,
		},
		{
			ID: "legacy_paste", Flag: "--legacy-paste", Label: "Legacy Paste",
			Desc:    "Inject clipboard as key events instead of setting device clipboard directly",
			Type:    OptBool, Default: "", Group: GroupInput,
		},
		{
			ID: "no_key_repeat", Flag: "--no-key-repeat", Label: "No Key Repeat",
			Desc:    "Don't forward held-key repeat events to device",
			Type:    OptBool, Default: "", Group: GroupInput,
		},
		{
			ID: "raw_key_events", Flag: "--raw-key-events", Label: "Raw Key Events",
			Desc:    "Inject key events for all input, ignore text events",
			Type:    OptBool, Default: "", Group: GroupInput,
		},
		{
			ID: "no_clipboard_autosync", Flag: "--no-clipboard-autosync", Label: "No Clipboard Sync",
			Desc:    "Disable automatic computer ↔ device clipboard synchronisation",
			Type:    OptBool, Default: "", Group: GroupInput,
		},
		{
			ID: "no_mouse_hover", Flag: "--no-mouse-hover", Label: "No Mouse Hover",
			Desc:    "Don't forward mouse motion events without a button press",
			Type:    OptBool, Default: "", Group: GroupInput,
		},
		{
			ID: "otg", Flag: "--otg", Label: "OTG Mode",
			Desc:    "Simulate physical keyboard+mouse over USB — no ADB or mirroring needed",
			Type:    OptBool, Default: "", Group: GroupInput,
			RequiresUSB: true,
		},

		// ── Screen ────────────────────────────────────────────────────────────

		{
			ID: "turn_screen_off", Flag: "--turn-screen-off", Label: "Turn Screen Off",
			Desc:    "Turn device screen off immediately (still mirrors while off)",
			Type:    OptBool, Default: "", Group: GroupScreen,
		},
		{
			ID: "stay_awake", Flag: "--stay-awake", Label: "Stay Awake",
			Desc:    "Prevent device screen from sleeping while scrcpy is running (requires USB power)",
			Type:    OptBool, Default: "", Group: GroupScreen,
		},
		{
			ID: "power_off_on_close", Flag: "--power-off-on-close", Label: "Power Off on Close",
			Desc:    "Turn device screen off when scrcpy window is closed",
			Type:    OptBool, Default: "", Group: GroupScreen,
		},
		{
			ID: "no_power_on", Flag: "--no-power-on", Label: "No Power On",
			Desc:    "Don't power on device screen at start (keep current state)",
			Type:    OptBool, Default: "", Group: GroupScreen,
		},
		{
			ID: "show_touches", Flag: "--show-touches", Label: "Show Touches",
			Desc:    "Enable Android 'show touches' debug overlay (restored on exit)",
			Type:    OptBool, Default: "", Group: GroupScreen,
		},
		{
			ID: "disable_screensaver", Flag: "--disable-screensaver", Label: "Disable Screensaver",
			Desc:    "Prevent computer screensaver from activating while scrcpy is running",
			Type:    OptBool, Default: "", Group: GroupScreen,
		},
		{
			ID: "screen_off_timeout", Flag: "--screen-off-timeout", Label: "Screen Off Timeout (s)",
			Desc:    "Override device screen timeout in seconds while running (restored on exit)",
			Type:    OptInt, Default: "", Group: GroupScreen,
		},

		// ── Window ────────────────────────────────────────────────────────────

		{
			ID: "fullscreen", Flag: "--fullscreen", Label: "Start Fullscreen",
			Desc:    "Open scrcpy window in fullscreen mode",
			Type:    OptBool, Default: "", Group: GroupWindow,
		},
		{
			ID: "always_on_top", Flag: "--always-on-top", Label: "Always on Top",
			Desc:    "Keep scrcpy window above all other windows",
			Type:    OptBool, Default: "", Group: GroupWindow,
		},
		{
			ID: "window_borderless", Flag: "--window-borderless", Label: "Borderless Window",
			Desc:    "Remove window title bar and decorations",
			Type:    OptBool, Default: "", Group: GroupWindow,
		},
		{
			ID: "window_title", Flag: "--window-title", Label: "Window Title",
			Desc:    "Custom title for the scrcpy window",
			Type:    OptString, Default: "", Group: GroupWindow,
		},
		{
			ID: "window_width", Flag: "--window-width", Label: "Window Width (px)",
			Desc:    "Initial window width. 0 = automatic.",
			Type:    OptInt, Default: "0", Group: GroupWindow,
		},
		{
			ID: "window_height", Flag: "--window-height", Label: "Window Height (px)",
			Desc:    "Initial window height. 0 = automatic.",
			Type:    OptInt, Default: "0", Group: GroupWindow,
		},
		{
			ID: "window_x", Flag: "--window-x", Label: "Window X Position",
			Desc:    "Initial horizontal position on screen",
			Type:    OptInt, Default: "", Group: GroupWindow,
		},
		{
			ID: "window_y", Flag: "--window-y", Label: "Window Y Position",
			Desc:    "Initial vertical position on screen",
			Type:    OptInt, Default: "", Group: GroupWindow,
		},
		{
			ID: "display_orientation", Flag: "--display-orientation", Label: "Display Orientation",
			Desc:    "Initial rotation of the scrcpy window content",
			Type:    OptEnum,
			Choices: []string{"0", "90", "180", "270", "flip0", "flip90", "flip180", "flip270"},
			Default: "0", Group: GroupWindow,
		},
		{
			ID: "render_driver", Flag: "--render-driver", Label: "Render Driver",
			Desc:    "SDL render driver hint (usually auto). opengl, opengles2, metal, software",
			Type:    OptEnum,
			Choices: []string{"auto", "opengl", "opengles2", "opengles", "metal", "software"},
			Default: "auto", Group: GroupWindow,
		},
		{
			ID: "no_mipmaps", Flag: "--no-mipmaps", Label: "No Mipmaps",
			Desc:    "Disable OpenGL mipmap generation (only matters for downscaled display)",
			Type:    OptBool, Default: "", Group: GroupWindow,
		},
		{
			ID: "display_ime_policy", Flag: "--display-ime-policy", Label: "IME Policy",
			Desc:    "Where to show the on-screen keyboard: local, fallback or hide",
			Type:    OptEnum,
			Choices: []string{"local", "fallback", "hide"},
			Default: "fallback", Group: GroupWindow,
		},

		// ── Recording ─────────────────────────────────────────────────────────

		{
			ID: "record", Flag: "--record", Label: "Record to File",
			Desc:    "Output file path. Format inferred from extension. e.g. screen.mp4",
			Type:    OptString, Default: "", Group: GroupRecording,
		},
		{
			ID: "record_format", Flag: "--record-format", Label: "Record Format",
			Desc:    "Override format: mp4 mkv m4a mka opus aac flac wav",
			Type:    OptEnum,
			Choices: []string{"mp4", "mkv", "m4a", "mka", "opus", "aac", "flac", "wav"},
			Default: "mp4", Group: GroupRecording,
		},
		{
			ID: "record_orientation", Flag: "--record-orientation", Label: "Record Orientation",
			Desc:    "Rotation applied to the recorded file content",
			Type:    OptEnum,
			Choices: []string{"0", "90", "180", "270"},
			Default: "0", Group: GroupRecording,
		},
		{
			ID: "v4l2_sink", Flag: "--v4l2-sink", Label: "V4L2 Sink",
			Desc:    "Output to V4L2 loopback device e.g. /dev/video0 (Linux only)",
			Type:    OptString, Default: "", Group: GroupRecording,
			LinuxOnly: true,
		},
		{
			ID: "v4l2_buffer", Flag: "--v4l2-buffer", Label: "V4L2 Buffer (ms)",
			Desc:    "Buffering delay for V4L2 output in milliseconds (Linux only)",
			Type:    OptInt, Default: "0", Group: GroupRecording,
			LinuxOnly: true, RequiresOpt: "v4l2_sink",
		},

		// ── Connection ────────────────────────────────────────────────────────

		{
			ID: "select_usb", Flag: "--select-usb", Label: "Force USB Device",
			Desc:    "Select USB device when multiple are connected (like adb -d)",
			Type:    OptBool, Default: "", Group: GroupConnection,
		},
		{
			ID: "select_tcpip", Flag: "--select-tcpip", Label: "Force TCP/IP Device",
			Desc:    "Select TCP/IP device when multiple are connected (like adb -e)",
			Type:    OptBool, Default: "", Group: GroupConnection,
		},
		{
			ID: "tcpip", Flag: "--tcpip", Label: "TCP/IP Auto-Connect",
			Desc:    "Switch device to TCP/IP and connect. Optionally: host[:port]",
			Type:    OptString, Default: "", Group: GroupConnection,
		},
		{
			ID: "port", Flag: "--port", Label: "Client TCP Port",
			Desc:    "Port or port range for client listener. Default 27183:27199",
			Type:    OptString, Default: "27183:27199", Group: GroupConnection,
		},
		{
			ID: "force_adb_forward", Flag: "--force-adb-forward", Label: "Force ADB Forward",
			Desc:    "Disable adb reverse; always use adb forward (needed behind some NATs)",
			Type:    OptBool, Default: "", Group: GroupConnection,
		},
		{
			ID: "tunnel_host", Flag: "--tunnel-host", Label: "Tunnel Host",
			Desc:    "IP address of the adb tunnel host (implies --force-adb-forward)",
			Type:    OptString, Default: "", Group: GroupConnection,
		},
		{
			ID: "tunnel_port", Flag: "--tunnel-port", Label: "Tunnel Port",
			Desc:    "TCP port of the adb tunnel (0 = use local forward port)",
			Type:    OptInt, Default: "0", Group: GroupConnection,
		},
		{
			ID: "kill_adb_on_close", Flag: "--kill-adb-on-close", Label: "Kill ADB on Close",
			Desc:    "Kill the adb server when scrcpy exits",
			Type:    OptBool, Default: "", Group: GroupConnection,
		},

		// ── Advanced ──────────────────────────────────────────────────────────

		{
			ID: "start_app", Flag: "--start-app", Label: "Start App",
			Desc:    "Launch Android app by package name. Prefix ? for fuzzy, + to force-stop first.",
			Type:    OptString, Default: "", Group: GroupAdvanced,
		},
		{
			ID: "no_cleanup", Flag: "--no-cleanup", Label: "No Cleanup",
			Desc:    "Don't remove server binary or restore device state on exit",
			Type:    OptBool, Default: "", Group: GroupAdvanced,
		},
		{
			ID: "no_downsize_on_error", Flag: "--no-downsize-on-error", Label: "No Auto-Downsize",
			Desc:    "Don't retry with lower resolution on MediaCodec error",
			Type:    OptBool, Default: "", Group: GroupAdvanced,
		},
		{
			ID: "time_limit", Flag: "--time-limit", Label: "Time Limit (s)",
			Desc:    "Automatically stop mirroring after N seconds",
			Type:    OptInt, Default: "", Group: GroupAdvanced,
		},
		{
			ID: "verbosity", Flag: "--verbosity", Label: "Log Verbosity",
			Desc:    "scrcpy process log level",
			Type:    OptEnum,
			Choices: []string{"verbose", "debug", "info", "warn", "error"},
			Default: "info", Group: GroupAdvanced,
		},
		{
			ID: "shortcut_mod", Flag: "--shortcut-mod", Label: "Shortcut Modifier",
			Desc:    "Key(s) for scrcpy shortcuts. e.g. lalt,lsuper",
			Type:    OptString, Default: "lalt,lsuper", Group: GroupAdvanced,
		},
		{
			ID: "print_fps", Flag: "--print-fps", Label: "Print FPS",
			Desc:    "Log FPS to console (toggle with MOD+i during session)",
			Type:    OptBool, Default: "", Group: GroupAdvanced,
		},
		{
			ID: "pause_on_exit", Flag: "--pause-on-exit", Label: "Pause on Exit",
			Desc:    "Keep terminal window open on exit: true, false or if-error",
			Type:    OptEnum,
			Choices: []string{"false", "if-error", "true"},
			Default: "false", Group: GroupAdvanced,
		},
		{
			ID: "no_vd_destroy_content", Flag: "--no-vd-destroy-content", Label: "No VD Destroy Content",
			Desc:    "Move apps to main display when virtual display closes (instead of destroying them)",
			Type:    OptBool, Default: "", Group: GroupAdvanced,
			MinSDK: 29, RequiresOpt: "new_display",
		},
		{
			ID: "no_vd_system_decorations", Flag: "--no-vd-system-decorations", Label: "No VD Decorations",
			Desc:    "Disable virtual display system decorations flag",
			Type:    OptBool, Default: "", Group: GroupAdvanced,
			MinSDK: 29, RequiresOpt: "new_display",
		},
		{
			ID: "video_codec_options", Flag: "--video-codec-options", Label: "Video Codec Options",
			Desc:    "Advanced MediaCodec options: key[:type]=value,... (see Android MediaFormat docs)",
			Type:    OptString, Default: "", Group: GroupAdvanced,
		},
		{
			ID: "audio_codec_options", Flag: "--audio-codec-options", Label: "Audio Codec Options",
			Desc:    "Advanced MediaCodec audio options: key[:type]=value,...",
			Type:    OptString, Default: "", Group: GroupAdvanced,
		},
		{
			ID: "video_encoder", Flag: "--video-encoder", Label: "Video Encoder",
			Desc:    "Specific MediaCodec video encoder name. List available: scrcpy --list-encoders",
			Type:    OptString, Default: "", Group: GroupAdvanced,
		},
		{
			ID: "audio_encoder", Flag: "--audio-encoder", Label: "Audio Encoder",
			Desc:    "Specific MediaCodec audio encoder name. List available: scrcpy --list-encoders",
			Type:    OptString, Default: "", Group: GroupAdvanced,
		},
		{
			ID: "push_target", Flag: "--push-target", Label: "Push Target Path",
			Desc:    "Device path for drag-and-drop file push. Default /sdcard/Download/",
			Type:    OptString, Default: "/sdcard/Download/", Group: GroupAdvanced,
		},
	}
}

// OptionsByGroup returns the catalog split into groups in canonical order.
func OptionsByGroup() map[string][]ScrcpyOption {
	all := AllOptions()
	result := make(map[string][]ScrcpyOption, len(GroupOrder))
	for _, g := range GroupOrder {
		result[g] = nil
	}
	for _, opt := range all {
		result[opt.Group] = append(result[opt.Group], opt)
	}
	return result
}

// FindOption returns the option with the given ID, or a zero value + false.
func FindOption(id string) (ScrcpyOption, bool) {
	for _, opt := range AllOptions() {
		if opt.ID == id {
			return opt, true
		}
	}
	return ScrcpyOption{}, false
}

// BuildArgsFromValues converts a map of option-ID → value into scrcpy argv.
// It respects the option type and emits nothing for default/empty values.
// caps is used to skip codec choices unsupported by the device.
func BuildArgsFromValues(values map[string]string, caps DeviceCapabilitySnapshot, transport string) (args []string, dropped []string) {
	all := AllOptions()
	for _, opt := range all {
		val, ok := values[opt.ID]
		if !ok || val == "" {
			continue
		}
		// Skip incompatible options.
		if cr := opt.Check(caps, transport); !cr.Compatible {
			dropped = append(dropped, opt.Flag+"  ("+cr.Reason+")")
			continue
		}
		// Skip codec values the device doesn't support.
		if opt.ID == "video_codec" {
			if val == "h265" && !caps.SupportsH265 {
				dropped = append(dropped, "--video-codec=h265  (device does not support H265 encoding)")
				continue
			}
		}
		// Emit arg.
		switch opt.Type {
		case OptBool:
			if val == "true" || val == "on" || val == "1" {
				args = append(args, opt.Flag)
			}
		case OptEnum, OptString:
			// Don't emit if it equals the default.
			if val == opt.Default && opt.Default != "" {
				continue
			}
			if opt.ID == "new_display_size" {
				// Merged into --new-display=<size>.
				continue
			}
			args = append(args, opt.Flag+"="+val)
		case OptInt:
			if val == "0" && opt.Default == "0" {
				continue // skip "no change from default"
			}
			if val != "" && val != opt.Default {
				args = append(args, opt.Flag+"="+val)
			}
		}
	}
	return args, dropped
}

// CompatSplit partitions options from a group into compatible and incompatible
// slices based on the device capability snapshot and transport.
func CompatSplit(opts []ScrcpyOption, caps DeviceCapabilitySnapshot, transport string) (compatible, incompatible []ScrcpyOption) {
	for _, opt := range opts {
		if cr := opt.Check(caps, transport); cr.Compatible {
			compatible = append(compatible, opt)
		} else {
			incompatible = append(incompatible, opt)
		}
	}
	return compatible, incompatible
}
