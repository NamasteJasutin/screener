package core

import (
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	LaunchModeMainDisplay = "main_display"
	LaunchModeNewDisplay  = "new_display"
	LaunchModeDexVirtual  = "dex_virtual"
	LaunchModeCamera      = "camera_source"

	GamepadModeOff  = "off"
	GamepadModeUHID = "uhid"
	GamepadModeAOA  = "aoa"

	DesiredKeyLaunchMode = "launch_mode"
	DesiredKeyGamepad    = "gamepad_mode"
	DesiredKeyMaxFPS     = "max_fps"
	DesiredKeyStartApp   = "start_app"

	defaultDexStartApp = "com.sec.android.app.desktoplauncher"
)

type ProfileDefinition struct {
	Name              string            `json:"name"`
	ProfileID         string            `json:"profile_id,omitempty"`
	DeviceFingerprint string            `json:"device_fingerprint,omitempty"`
	IsDefault         bool              `json:"is_default,omitempty"`
	DisplayID         int               `json:"display_id"`
	MaxSize           int               `json:"max_size"`
	VideoBitRateMB    int               `json:"video_bitrate_mb"`
	FeatureFlags      map[string]bool   `json:"feature_flags"`
	ExtraArgs         []string          `json:"extra_args"`
	Metadata          map[string]string `json:"metadata,omitempty"`
	Tags              []string          `json:"tags,omitempty"`
	Notes             string            `json:"notes,omitempty"`
	Desired           map[string]string `json:"desired,omitempty"`
	DesiredFlags      map[string]bool   `json:"desired_flags,omitempty"`
}

type DeviceCapabilitySnapshot struct {
	AndroidRelease         string `json:"android_release,omitempty"`
	Manufacturer           string `json:"manufacturer,omitempty"`
	Model                  string `json:"model"`
	DeviceName             string `json:"device_name,omitempty"` // user-set name from device settings
	SDKInt                 int    `json:"sdk_int"`
	Serial                 string `json:"serial"`
	AndroidID              string `json:"android_id,omitempty"`
	Hostname               string `json:"hostname,omitempty"`
	StorageTotal           int64  `json:"storage_total,omitempty"`
	StorageFree            int64  `json:"storage_free,omitempty"`
	StorageFreePct         int    `json:"storage_free_pct,omitempty"`
	ExtStorageTotal        int64  `json:"ext_storage_total,omitempty"`
	ExtStorageFree         int64  `json:"ext_storage_free,omitempty"`
	BatteryLevel           int    `json:"battery_level,omitempty"`
	LocalWiFiIP            string `json:"local_wifi_ip,omitempty"`
	TailscaleIP            string `json:"tailscale_ip,omitempty"`
	ScreenMetrics          string `json:"screen_metrics,omitempty"`
	SupportsAudio          bool   `json:"supports_audio,omitempty"`
	SupportsCamera         bool   `json:"supports_camera,omitempty"`
	SupportsVirtualDisplay bool   `json:"supports_virtual_display,omitempty"`
	SupportsGamepadUHID    bool   `json:"supports_gamepad_uhid,omitempty"`
	SupportsGamepadAOA     bool   `json:"supports_gamepad_aoa,omitempty"`
	State                  string `json:"state"`
	SupportsH265           bool   `json:"supports_h265"`
}

type EffectiveProfileResolution struct {
	SupportedFeatures   []string `json:"supported_features"`
	Warnings            []string `json:"warnings"`
	UnsupportedFeatures []string `json:"unsupported_features"`
	BlockedFeatures     []string `json:"blocked_features"`
	DroppedFlags        []string `json:"dropped_flags"`
	FinalArgs           []string `json:"final_args"`
	Launchable          bool     `json:"launchable"`
}

// KnownDevice represents a remembered/paired device that persists across sessions.
type KnownDevice struct {
	Alias          string     `json:"alias"`
	Nickname       string     `json:"nickname,omitempty"`     // user-set display name
	DeviceName     string     `json:"device_name,omitempty"`  // fetched from device settings
	Serial         string     `json:"serial,omitempty"`
	Model          string     `json:"model,omitempty"`
	Manufacturer   string     `json:"manufacturer,omitempty"`
	AndroidID      string     `json:"android_id,omitempty"`
	AndroidRelease string     `json:"android_release,omitempty"`
	LocalWiFiIP    string     `json:"local_wifi_ip,omitempty"`
	TailscaleIP    string     `json:"tailscale_ip,omitempty"`
	PairedAt       time.Time  `json:"paired_at,omitempty"`
	LastSeenAt     time.Time  `json:"last_seen_at,omitempty"`
	Endpoints      []Endpoint `json:"endpoints,omitempty"`
	DefaultProfile string     `json:"default_profile,omitempty"`
	Tags           []string   `json:"tags,omitempty"`
}

// DisplayName returns the best human-readable name for the device.
// Priority: Nickname → DeviceName (from device settings) → Model → Alias (IP).
func (kd *KnownDevice) DisplayName() string {
	if kd.Nickname != "" {
		return kd.Nickname
	}
	if kd.DeviceName != "" {
		return kd.DeviceName
	}
	if kd.Model != "" {
		return kd.Model
	}
	return kd.Alias
}

type Endpoint struct {
	Name          string    `json:"name,omitempty"`
	Host          string    `json:"host"`
	Port          int       `json:"port"`
	Transport     string    `json:"transport"`
	Priority      int       `json:"priority"`
	LastSuccessAt time.Time `json:"last_success_at,omitempty"`
	FailureCount  int       `json:"failure_count"`
	Notes         string    `json:"notes,omitempty"`
	Rank          int       `json:"rank,omitempty"`
}

type ConnectionPolicy struct {
	PreferUSB bool `json:"prefer_usb"`
	AllowTCP  bool `json:"allow_tcp"`
}

type LaunchValidationResult struct {
	OK       bool     `json:"ok"`
	Errors   []string `json:"errors"`
	Warnings []string `json:"warnings"`
}

func DefaultProfile() ProfileDefinition {
	profiles := DefaultProfiles()
	return profiles[0]
}

func DefaultProfiles() []ProfileDefinition {
	return []ProfileDefinition{
		{
			Name:           "TV Console - Main Display",
			ProfileID:      "tv-console-main-display",
			IsDefault:      true,
			DisplayID:      0,
			MaxSize:        1920,
			VideoBitRateMB: 8,
			Desired:        map[string]string{"display_id": "0", "max_size": "1920", "video_bitrate_mb": "8"},
			DesiredFlags:   map[string]bool{"turn_screen_off": true, "stay_awake": true, "prefer_h265": false},
			FeatureFlags: map[string]bool{
				"turn_screen_off": true,
				"stay_awake":      true,
				"prefer_h265":     false,
			},
			Tags:  []string{"builtin", "main-display", "tv"},
			Notes: "Default mirrored main-display profile for general use.",
		},
		{
			Name:           "Game Mode - Main Display",
			ProfileID:      "game-mode-main-display",
			DisplayID:      0,
			MaxSize:        1440,
			VideoBitRateMB: 24,
			Desired: map[string]string{
				"display_id":       "0",
				"max_size":         "1440",
				"video_bitrate_mb": "24",
				"max_fps":          "120",
				"gamepad":          "uhid",
			},
			DesiredFlags: map[string]bool{"turn_screen_off": true, "stay_awake": true},
			FeatureFlags: map[string]bool{
				"turn_screen_off": true,
				"stay_awake":      true,
			},
			ExtraArgs: []string{"--gamepad=uhid", "--max-fps=120", "--video-bit-rate=24M", "--max-size=1440"},
			Tags:      []string{"builtin", "main-display", "game"},
			Notes:     "Validated high-performance main-display profile with UHID gamepad.",
		},
		{
			Name:           "Extra Screen - Empty",
			ProfileID:      "extra-screen-empty",
			DisplayID:      0,
			MaxSize:        1920,
			VideoBitRateMB: 8,
			Desired: map[string]string{
				"new_display": "true",
			},
			ExtraArgs: []string{"--new-display"},
			Tags:      []string{"builtin", "virtual-display", "extra-screen"},
			Notes:     "Creates an empty virtual display session when supported.",
		},
		{
			Name:           "Samsung DeX - Virtual Display",
			ProfileID:      "samsung-dex-virtual-display",
			DisplayID:      0,
			MaxSize:        1920,
			VideoBitRateMB: 16,
			Desired: map[string]string{
				"new_display": "true",
				"start_app":   "com.sec.android.app.desktoplauncher",
			},
			ExtraArgs: []string{"--new-display", "--start-app=com.sec.android.app.desktoplauncher"},
			Tags:      []string{"builtin", "virtual-display", "dex", "samsung"},
			Notes:     "Starts a DeX-targeted virtual display session; launcher intent may warn on some firmware.",
		},
	}
}

func ResolveEffectiveProfile(profile ProfileDefinition, caps DeviceCapabilitySnapshot) EffectiveProfileResolution {
	dynamicArgs := synthesizeDynamicExtraArgs(profile)
	extraArgs := mergeUniqueArgs(dynamicArgs, profile.ExtraArgs)
	launchMode := normalizedLaunchMode(profile)

	args := []string{"--max-size", strconv.Itoa(profile.MaxSize), "--video-bit-rate", strconv.Itoa(profile.VideoBitRateMB) + "M"}
	if !launchModeSuppressesDisplayID(launchMode) && !profileRequestsLegacyNewDisplay(profile.ExtraArgs) {
		args = append([]string{"--display-id", strconv.Itoa(profile.DisplayID)}, args...)
	}
	res := EffectiveProfileResolution{FinalArgs: args, Launchable: true}

	if desiredFlag(profile, "turn_screen_off") {
		if caps.SDKInt >= 30 {
			res.FinalArgs = append(res.FinalArgs, "--turn-screen-off")
			res.SupportedFeatures = appendUnique(res.SupportedFeatures, "turn_screen_off")
		} else {
			res.Warnings = append(res.Warnings, "turn_screen_off ignored on sdk<30")
			res.UnsupportedFeatures = appendUnique(res.UnsupportedFeatures, "turn_screen_off")
			res.DroppedFlags = appendUnique(res.DroppedFlags, "--turn-screen-off")
		}
	}

	if desiredFlag(profile, "stay_awake") {
		if caps.SDKInt >= 29 {
			res.FinalArgs = append(res.FinalArgs, "--stay-awake")
			res.SupportedFeatures = appendUnique(res.SupportedFeatures, "stay_awake")
		} else {
			res.Warnings = append(res.Warnings, "stay_awake ignored on sdk<29")
			res.UnsupportedFeatures = appendUnique(res.UnsupportedFeatures, "stay_awake")
			res.DroppedFlags = appendUnique(res.DroppedFlags, "--stay-awake")
		}
	}

	if desiredFlag(profile, "prefer_h265") {
		if caps.SupportsH265 {
			res.FinalArgs = append(res.FinalArgs, "--video-codec", "h265")
			res.SupportedFeatures = appendUnique(res.SupportedFeatures, "prefer_h265")
		} else {
			res.Warnings = append(res.Warnings, "h265 unavailable on this device")
			res.UnsupportedFeatures = appendUnique(res.UnsupportedFeatures, "prefer_h265")
			res.DroppedFlags = appendUnique(res.DroppedFlags, "--video-codec h265")
		}
	}

	if desiredFlag(profile, "require_audio") && !caps.SupportsAudio {
		res.Launchable = false
		res.BlockedFeatures = appendUnique(res.BlockedFeatures, "require_audio")
		res.UnsupportedFeatures = appendUnique(res.UnsupportedFeatures, "require_audio")
		res.Warnings = append(res.Warnings, "audio is required by profile but unsupported by device")
	}
	if desiredFlag(profile, "require_camera") && !caps.SupportsCamera {
		res.Launchable = false
		res.BlockedFeatures = appendUnique(res.BlockedFeatures, "require_camera")
		res.UnsupportedFeatures = appendUnique(res.UnsupportedFeatures, "require_camera")
		res.Warnings = append(res.Warnings, "camera is required by profile but unsupported by device")
	}

	for i := 0; i < len(extraArgs); i++ {
		a := strings.TrimSpace(extraArgs[i])
		if a == "" {
			continue
		}
		dropN, feature, dropped := dropUnsupportedArg(extraArgs, i, caps)
		if dropN > 0 {
			res.UnsupportedFeatures = appendUnique(res.UnsupportedFeatures, feature)
			res.DroppedFlags = appendUnique(res.DroppedFlags, dropped)
			if feature == "audio" || feature == "camera" {
				res.Launchable = false
				res.BlockedFeatures = appendUnique(res.BlockedFeatures, feature)
			}
			i += dropN - 1
			continue
		}
		res.FinalArgs = append(res.FinalArgs, a)
	}

	return res
}

func RankEndpoints(policy ConnectionPolicy, endpoints []Endpoint) []Endpoint {
	out := make([]Endpoint, 0, len(endpoints))
	for _, ep := range endpoints {
		if ep.Transport == "tcp" && !policy.AllowTCP {
			continue
		}
		rank := ep.Priority
		if rank == 0 {
			rank = ep.Rank
		}
		rank += ep.FailureCount * 20
		if policy.PreferUSB && ep.Transport == "usb" {
			rank -= 100
		}
		if !policy.PreferUSB && ep.Transport == "tcp" {
			rank -= 50
		}
		ep.Rank = rank
		out = append(out, ep)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Rank != out[j].Rank {
			return out[i].Rank < out[j].Rank
		}
		if out[i].Transport != out[j].Transport {
			return out[i].Transport < out[j].Transport
		}
		if out[i].Host != out[j].Host {
			return out[i].Host < out[j].Host
		}
		if out[i].Port != out[j].Port {
			return out[i].Port < out[j].Port
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func ValidateLaunch(planArgs []string) LaunchValidationResult {
	res := LaunchValidationResult{OK: true}
	if len(planArgs) == 0 {
		res.OK = false
		res.Errors = append(res.Errors, "empty command")
		return res
	}
	hasNonEmpty := false
	hasDisplayID := false
	hasNewDisplay := false
	hasNoVideo := false
	hasNoAudio := false
	isCameraSource := false
	for _, arg := range planArgs {
		trimmed := strings.TrimSpace(arg)
		if trimmed == "" {
			continue
		}
		hasNonEmpty = true
		switch {
		case trimmed == "--display-id" || trimmed == "--display":
			hasDisplayID = true
		case trimmed == "--new-display" || strings.HasPrefix(trimmed, "--new-display="):
			hasNewDisplay = true
		case trimmed == "--no-video":
			hasNoVideo = true
		case trimmed == "--no-audio":
			hasNoAudio = true
		case trimmed == "--video-source=camera":
			isCameraSource = true
		}
	}
	if !hasNonEmpty {
		res.OK = false
		res.Errors = append(res.Errors, "empty command")
		return res
	}
	if hasDisplayID && hasNewDisplay {
		res.OK = false
		res.Errors = append(res.Errors, "--display-id and --new-display conflict")
	}
	if hasNoVideo {
		for _, arg := range planArgs {
			trimmed := strings.TrimSpace(arg)
			switch {
			case strings.HasPrefix(trimmed, "--video-bit-rate") || strings.HasPrefix(trimmed, "--video-bitrate"):
				res.Warnings = append(res.Warnings, "--no-video with "+trimmed+" has no effect")
			case strings.HasPrefix(trimmed, "--video-codec"):
				res.Warnings = append(res.Warnings, "--no-video with "+trimmed+" has no effect")
			case trimmed == "--max-size" || strings.HasPrefix(trimmed, "--max-size="):
				res.Warnings = append(res.Warnings, "--no-video with --max-size has no effect")
			}
		}
	}
	if hasNoAudio {
		for _, arg := range planArgs {
			trimmed := strings.TrimSpace(arg)
			switch {
			case strings.HasPrefix(trimmed, "--audio-codec"):
				res.Warnings = append(res.Warnings, "--no-audio with "+trimmed+" has no effect")
			case strings.HasPrefix(trimmed, "--audio-bit-rate"):
				res.Warnings = append(res.Warnings, "--no-audio with "+trimmed+" has no effect")
			}
		}
	}
	if isCameraSource {
		if hasDisplayID {
			res.Warnings = append(res.Warnings, "--video-source=camera ignores --display-id")
		}
		if hasNewDisplay {
			res.Warnings = append(res.Warnings, "--video-source=camera ignores --new-display")
		}
		for _, arg := range planArgs {
			if strings.TrimSpace(arg) == "--turn-screen-off" {
				res.Warnings = append(res.Warnings, "--video-source=camera incompatible with --turn-screen-off")
			}
		}
	}
	return res
}

func desiredFlag(profile ProfileDefinition, key string) bool {
	if profile.DesiredFlags != nil {
		if enabled, ok := profile.DesiredFlags[key]; ok {
			return enabled
		}
	}
	if profile.FeatureFlags != nil {
		return profile.FeatureFlags[key]
	}
	return false
}

func appendUnique(values []string, value string) []string {
	for _, v := range values {
		if v == value {
			return values
		}
	}
	return append(values, value)
}

func dropUnsupportedArg(args []string, i int, caps DeviceCapabilitySnapshot) (int, string, string) {
	arg := strings.TrimSpace(args[i])
	if strings.HasPrefix(arg, "--new-display=") {
		if !caps.SupportsVirtualDisplay {
			return 1, "virtual_display", arg
		}
	}
	if strings.HasPrefix(arg, "--video-source=") {
		source := strings.TrimSpace(strings.TrimPrefix(arg, "--video-source="))
		if source == "camera" && !caps.SupportsCamera {
			return 1, "camera", arg
		}
	}
	if strings.HasPrefix(arg, "--gamepad=") {
		mode := strings.TrimSpace(strings.TrimPrefix(arg, "--gamepad="))
		switch mode {
		case "uhid":
			if !caps.SupportsGamepadUHID {
				return 1, "gamepad_uhid", arg
			}
		case "aoa":
			if !caps.SupportsGamepadAOA {
				return 1, "gamepad_aoa", arg
			}
		}
	}
	switch arg {
	case "--audio":
		if !caps.SupportsAudio {
			return 1, "audio", "--audio"
		}
	case "--camera":
		if !caps.SupportsCamera {
			return 1, "camera", "--camera"
		}
	case "--video-source":
		if i+1 >= len(args) {
			return 0, "", ""
		}
		source := strings.TrimSpace(args[i+1])
		if source == "camera" && !caps.SupportsCamera {
			return 2, "camera", "--video-source camera"
		}
	case "--new-display":
		if !caps.SupportsVirtualDisplay {
			return 1, "virtual_display", "--new-display"
		}
	case "--uhid-gamepad":
		if !caps.SupportsGamepadUHID {
			return 1, "gamepad_uhid", "--uhid-gamepad"
		}
	case "--aoa-gamepad":
		if !caps.SupportsGamepadAOA {
			return 1, "gamepad_aoa", "--aoa-gamepad"
		}
	case "--gamepad":
		if i+1 >= len(args) {
			return 0, "", ""
		}
		mode := strings.TrimSpace(args[i+1])
		switch mode {
		case "uhid":
			if !caps.SupportsGamepadUHID {
				return 2, "gamepad_uhid", "--gamepad uhid"
			}
		case "aoa":
			if !caps.SupportsGamepadAOA {
				return 2, "gamepad_aoa", "--gamepad aoa"
			}
		}
	}
	return 0, "", ""
}

func synthesizeDynamicExtraArgs(profile ProfileDefinition) []string {
	out := make([]string, 0, 5)
	launchMode := normalizedLaunchMode(profile)

	switch launchMode {
	case LaunchModeNewDisplay, LaunchModeDexVirtual:
		out = appendUniqueArg(out, "--new-display")
	case LaunchModeCamera:
		out = appendUniqueArg(out, "--video-source=camera")
	}

	if launchMode == LaunchModeDexVirtual {
		startApp := desiredValue(profile, DesiredKeyStartApp)
		if startApp == "" {
			startApp = defaultDexStartApp
		}
		out = appendUniqueArg(out, "--start-app="+startApp)
	} else {
		if startApp := desiredValue(profile, DesiredKeyStartApp); startApp != "" {
			out = appendUniqueArg(out, "--start-app="+startApp)
		}
	}

	if maxFPS := desiredValue(profile, DesiredKeyMaxFPS); maxFPS != "" {
		if _, err := strconv.Atoi(maxFPS); err == nil {
			out = appendUniqueArg(out, "--max-fps="+maxFPS)
		}
	}

	switch normalizedGamepadMode(profile) {
	case GamepadModeUHID:
		out = appendUniqueArg(out, "--gamepad=uhid")
	case GamepadModeAOA:
		out = appendUniqueArg(out, "--gamepad=aoa")
	}

	return out
}

func mergeUniqueArgs(primary []string, secondary []string) []string {
	out := make([]string, 0, len(primary)+len(secondary))
	for _, raw := range primary {
		arg := strings.TrimSpace(raw)
		if arg == "" {
			continue
		}
		out = appendUniqueArg(out, arg)
	}
	for _, raw := range secondary {
		arg := strings.TrimSpace(raw)
		if arg == "" {
			continue
		}
		out = appendUniqueArg(out, arg)
	}
	return out
}

func appendUniqueArg(args []string, value string) []string {
	for _, existing := range args {
		if existing == value {
			return args
		}
	}
	return append(args, value)
}

func launchModeSuppressesDisplayID(mode string) bool {
	switch mode {
	case LaunchModeNewDisplay, LaunchModeDexVirtual, LaunchModeCamera:
		return true
	default:
		return false
	}
}

func normalizedLaunchMode(profile ProfileDefinition) string {
	mode := strings.ToLower(desiredValue(profile, DesiredKeyLaunchMode))
	switch mode {
	case LaunchModeMainDisplay, LaunchModeNewDisplay, LaunchModeDexVirtual, LaunchModeCamera:
		return mode
	}

	if profile.DesiredFlags != nil && profile.DesiredFlags["new_display"] {
		return LaunchModeNewDisplay
	}
	if profile.Desired != nil {
		if v, ok := profile.Desired["new_display"]; ok {
			normalized := strings.TrimSpace(strings.ToLower(v))
			if normalized == "" || normalized == "true" || normalized == "1" || normalized == "yes" {
				return LaunchModeNewDisplay
			}
		}
	}
	return LaunchModeMainDisplay
}

func normalizedGamepadMode(profile ProfileDefinition) string {
	mode := strings.ToLower(desiredValue(profile, DesiredKeyGamepad))
	if mode == "" {
		mode = strings.ToLower(desiredValue(profile, "gamepad"))
	}
	switch mode {
	case GamepadModeUHID, GamepadModeAOA:
		return mode
	default:
		return GamepadModeOff
	}
}

func desiredValue(profile ProfileDefinition, key string) string {
	if profile.Desired == nil {
		return ""
	}
	return strings.TrimSpace(profile.Desired[key])
}

func profileRequestsLegacyNewDisplay(args []string) bool {
	for _, raw := range args {
		arg := strings.TrimSpace(raw)
		if arg == "--new-display" || strings.HasPrefix(arg, "--new-display=") {
			return true
		}
	}
	return false
}
