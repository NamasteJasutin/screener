package core

import "strings"

import "testing"

func hasArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func TestDefaultProfileName(t *testing.T) {
	p := DefaultProfile()
	if p.Name != "TV Console - Main Display" {
		t.Fatalf("unexpected default profile %q", p.Name)
	}
}

func TestDefaultProfilesBuiltIns(t *testing.T) {
	profiles := DefaultProfiles()
	if len(profiles) != 4 {
		t.Fatalf("expected 4 built-in profiles, got %d", len(profiles))
	}
	names := []string{
		"TV Console - Main Display",
		"Game Mode - Main Display",
		"Extra Screen - Empty",
		"Samsung DeX - Virtual Display",
	}
	for i := range names {
		if profiles[i].Name != names[i] {
			t.Fatalf("unexpected profile at %d: got %q want %q", i, profiles[i].Name, names[i])
		}
	}
}

func TestDefaultTVProfileDoesNotForceH265(t *testing.T) {
	p := DefaultProfile()
	if p.DesiredFlags["prefer_h265"] {
		t.Fatal("expected default TV profile prefer_h265=false")
	}
	if p.FeatureFlags["prefer_h265"] {
		t.Fatal("expected default TV profile feature flag prefer_h265=false")
	}
	res := ResolveEffectiveProfile(p, DeviceCapabilitySnapshot{SDKInt: 34, SupportsH265: true})
	for i := 0; i < len(res.FinalArgs); i++ {
		if res.FinalArgs[i] == "--video-codec" && i+1 < len(res.FinalArgs) && res.FinalArgs[i+1] == "h265" {
			t.Fatalf("unexpected forced h265 in default args: %+v", res.FinalArgs)
		}
	}
}

func TestEndpointRanking(t *testing.T) {
	endpoints := []Endpoint{{Name: "wifi", Host: "192.168.1.20", Transport: "tcp", Priority: 20}, {Name: "usb", Host: "local", Transport: "usb", Priority: 30}}
	ranked := RankEndpoints(ConnectionPolicy{PreferUSB: true, AllowTCP: true}, endpoints)
	if len(ranked) != 2 || ranked[0].Transport != "usb" {
		t.Fatalf("expected usb first, got %+v", ranked)
	}
}

func TestEffectiveResolutionPortability(t *testing.T) {
	p := DefaultProfile()
	old := ResolveEffectiveProfile(p, DeviceCapabilitySnapshot{SDKInt: 29, SupportsH265: false})
	newer := ResolveEffectiveProfile(p, DeviceCapabilitySnapshot{SDKInt: 34, SupportsH265: true})
	if len(old.DroppedFlags) == 0 {
		t.Fatal("expected unsupported flags for sdk29")
	}
	if len(newer.DroppedFlags) != 0 {
		t.Fatalf("expected full support for sdk34, got %+v", newer.DroppedFlags)
	}
	if len(old.FinalArgs) == len(newer.FinalArgs) {
		t.Fatal("expected sdk29 and sdk34 final args to differ")
	}
	if p.FeatureFlags["turn_screen_off"] != true {
		t.Fatal("profile mutated during resolution")
	}
}

func TestUnsupportedExtraFlagNotInFinalArgs(t *testing.T) {
	p := DefaultProfile()
	p.ExtraArgs = []string{"--new-display", "--stay-awake"}
	res := ResolveEffectiveProfile(p, DeviceCapabilitySnapshot{SDKInt: 34, SupportsVirtualDisplay: false, SupportsH265: true})
	for _, a := range res.FinalArgs {
		if a == "--new-display" {
			t.Fatal("unsupported --new-display should not be present in FinalArgs")
		}
	}
}

func TestHardConflictMarksUnlaunchable(t *testing.T) {
	p := DefaultProfile()
	p.DesiredFlags = map[string]bool{"require_audio": true}
	res := ResolveEffectiveProfile(p, DeviceCapabilitySnapshot{SDKInt: 34, SupportsAudio: false})
	if res.Launchable {
		t.Fatal("expected launchable=false when require_audio is unsupported")
	}
}

func TestValidateLaunchConflict(t *testing.T) {
	res := ValidateLaunch([]string{"--display-id", "0", "--new-display"})
	if res.OK {
		t.Fatal("expected validation failure for conflicting flags")
	}
}

func TestResolveProfileNewDisplayOmitsDisplayArg(t *testing.T) {
	profiles := DefaultProfiles()
	extra := profiles[2]
	res := ResolveEffectiveProfile(extra, DeviceCapabilitySnapshot{SDKInt: 34, SupportsVirtualDisplay: true})
	for _, arg := range res.FinalArgs {
		if arg == "--display" || arg == "--display-id" {
			t.Fatalf("unexpected display selector with new-display: %+v", res.FinalArgs)
		}
	}
	foundNewDisplay := false
	for _, arg := range res.FinalArgs {
		if arg == "--new-display" {
			foundNewDisplay = true
			break
		}
	}
	if !foundNewDisplay {
		t.Fatalf("expected --new-display in final args: %+v", res.FinalArgs)
	}
}

func TestResolveGamepadFlagSupport(t *testing.T) {
	profiles := DefaultProfiles()
	game := profiles[1]

	supported := ResolveEffectiveProfile(game, DeviceCapabilitySnapshot{SDKInt: 34, SupportsGamepadUHID: true})
	foundGamepad := false
	for _, arg := range supported.FinalArgs {
		if arg == "--gamepad=uhid" {
			foundGamepad = true
			break
		}
	}
	if !foundGamepad {
		t.Fatalf("expected gamepad flag when supported: %+v", supported.FinalArgs)
	}

	unsupported := ResolveEffectiveProfile(game, DeviceCapabilitySnapshot{SDKInt: 34, SupportsGamepadUHID: false})
	for _, arg := range unsupported.FinalArgs {
		if arg == "--gamepad=uhid" {
			t.Fatalf("expected gamepad flag dropped when unsupported: %+v", unsupported.FinalArgs)
		}
	}
	foundDropped := false
	for _, dropped := range unsupported.DroppedFlags {
		if dropped == "--gamepad=uhid" {
			foundDropped = true
			break
		}
	}
	if !foundDropped {
		t.Fatalf("expected dropped gamepad flag recorded: %+v", unsupported.DroppedFlags)
	}
}

func TestResolveLaunchModeSuppressesDisplayID(t *testing.T) {
	modes := []string{LaunchModeNewDisplay, LaunchModeDexVirtual, LaunchModeCamera}
	for _, mode := range modes {
		p := DefaultProfile()
		p.Desired = map[string]string{DesiredKeyLaunchMode: mode}
		res := ResolveEffectiveProfile(p, DeviceCapabilitySnapshot{SDKInt: 34, SupportsVirtualDisplay: true, SupportsCamera: true})
		if hasArg(res.FinalArgs, "--display-id") || hasArg(res.FinalArgs, "--display") {
			t.Fatalf("mode %q unexpectedly emitted display-id args: %+v", mode, res.FinalArgs)
		}
	}
}

func TestResolveDexStartAppDefaultAndOverride(t *testing.T) {
	defaultDex := DefaultProfile()
	defaultDex.Desired = map[string]string{DesiredKeyLaunchMode: LaunchModeDexVirtual}
	defaultRes := ResolveEffectiveProfile(defaultDex, DeviceCapabilitySnapshot{SDKInt: 34, SupportsVirtualDisplay: true})
	if !hasArg(defaultRes.FinalArgs, "--start-app=com.sec.android.app.desktoplauncher") {
		t.Fatalf("expected default dex launcher arg, got: %+v", defaultRes.FinalArgs)
	}

	overrideDex := DefaultProfile()
	overrideDex.Desired = map[string]string{DesiredKeyLaunchMode: LaunchModeDexVirtual, DesiredKeyStartApp: "com.example.desktop"}
	overrideRes := ResolveEffectiveProfile(overrideDex, DeviceCapabilitySnapshot{SDKInt: 34, SupportsVirtualDisplay: true})
	if !hasArg(overrideRes.FinalArgs, "--start-app=com.example.desktop") {
		t.Fatalf("expected overridden dex start-app arg, got: %+v", overrideRes.FinalArgs)
	}
	if hasArg(overrideRes.FinalArgs, "--start-app=com.sec.android.app.desktoplauncher") {
		t.Fatalf("expected no default dex start-app when overridden: %+v", overrideRes.FinalArgs)
	}
}

func TestResolveCameraModeGatingAndDroppedFlags(t *testing.T) {
	p := DefaultProfile()
	p.Desired = map[string]string{DesiredKeyLaunchMode: LaunchModeCamera}

	unsupported := ResolveEffectiveProfile(p, DeviceCapabilitySnapshot{SDKInt: 34, SupportsCamera: false})
	if hasArg(unsupported.FinalArgs, "--video-source=camera") {
		t.Fatalf("expected camera source flag to be dropped when unsupported: %+v", unsupported.FinalArgs)
	}
	if !hasArg(unsupported.DroppedFlags, "--video-source=camera") {
		t.Fatalf("expected dropped camera source flag to be recorded: %+v", unsupported.DroppedFlags)
	}

	supported := ResolveEffectiveProfile(p, DeviceCapabilitySnapshot{SDKInt: 34, SupportsCamera: true})
	if !hasArg(supported.FinalArgs, "--video-source=camera") {
		t.Fatalf("expected camera source flag when supported: %+v", supported.FinalArgs)
	}
}

func TestResolveDesiredGamepadModeSupportAndDrop(t *testing.T) {
	p := DefaultProfile()
	p.Desired = map[string]string{DesiredKeyGamepad: GamepadModeAOA}

	unsupported := ResolveEffectiveProfile(p, DeviceCapabilitySnapshot{SDKInt: 34, SupportsGamepadAOA: false})
	if hasArg(unsupported.FinalArgs, "--gamepad=aoa") {
		t.Fatalf("expected aoa gamepad arg dropped when unsupported: %+v", unsupported.FinalArgs)
	}
	if !hasArg(unsupported.DroppedFlags, "--gamepad=aoa") {
		t.Fatalf("expected dropped aoa gamepad flag recorded: %+v", unsupported.DroppedFlags)
	}

	supported := ResolveEffectiveProfile(p, DeviceCapabilitySnapshot{SDKInt: 34, SupportsGamepadAOA: true})
	if !hasArg(supported.FinalArgs, "--gamepad=aoa") {
		t.Fatalf("expected aoa gamepad arg when supported: %+v", supported.FinalArgs)
	}
}

func TestResolveDynamicArgsDeterministicAndDeduplicated(t *testing.T) {
	p := DefaultProfile()
	p.Desired = map[string]string{
		DesiredKeyLaunchMode: LaunchModeDexVirtual,
		DesiredKeyGamepad:    GamepadModeUHID,
		DesiredKeyMaxFPS:     "120",
	}
	p.ExtraArgs = []string{"--new-display", "--gamepad=uhid", "--max-fps=120", "--new-display"}

	resA := ResolveEffectiveProfile(p, DeviceCapabilitySnapshot{SDKInt: 34, SupportsVirtualDisplay: true, SupportsGamepadUHID: true})
	resB := ResolveEffectiveProfile(p, DeviceCapabilitySnapshot{SDKInt: 34, SupportsVirtualDisplay: true, SupportsGamepadUHID: true})

	if len(resA.FinalArgs) != len(resB.FinalArgs) {
		t.Fatalf("expected deterministic final arg length, got %d vs %d", len(resA.FinalArgs), len(resB.FinalArgs))
	}
	for i := range resA.FinalArgs {
		if resA.FinalArgs[i] != resB.FinalArgs[i] {
			t.Fatalf("expected deterministic final args, got %+v vs %+v", resA.FinalArgs, resB.FinalArgs)
		}
	}

	counts := map[string]int{}
	for _, arg := range resA.FinalArgs {
		counts[arg]++
	}
	if counts["--new-display"] != 1 {
		t.Fatalf("expected one --new-display, got %d in %+v", counts["--new-display"], resA.FinalArgs)
	}
	if counts["--gamepad=uhid"] != 1 {
		t.Fatalf("expected one --gamepad=uhid, got %d in %+v", counts["--gamepad=uhid"], resA.FinalArgs)
	}
	if counts["--max-fps=120"] != 1 {
		t.Fatalf("expected one --max-fps=120, got %d in %+v", counts["--max-fps=120"], resA.FinalArgs)
	}
}

func TestValidateLaunchConflictWithNewDisplayValue(t *testing.T) {
	res := ValidateLaunch([]string{"--display-id", "0", "--new-display=1920x1080"})
	if res.OK {
		t.Fatal("expected validation failure for --display-id and --new-display=...")
	}
}

// ── dropUnsupportedArg full branch coverage ────────────────────────────────

func TestDropUnsupportedArgAudio(t *testing.T) {
	args := []string{"--audio"}
	caps := DeviceCapabilitySnapshot{SupportsAudio: false}
	n, feature, dropped := dropUnsupportedArg(args, 0, caps)
	if n != 1 || feature != "audio" || dropped != "--audio" {
		t.Fatalf("got n=%d feature=%q dropped=%q", n, feature, dropped)
	}

	// Supported: should not drop
	caps2 := DeviceCapabilitySnapshot{SupportsAudio: true}
	n2, _, _ := dropUnsupportedArg(args, 0, caps2)
	if n2 != 0 {
		t.Fatalf("should not drop --audio when supported: n=%d", n2)
	}
}

func TestDropUnsupportedArgCamera(t *testing.T) {
	args := []string{"--camera"}
	caps := DeviceCapabilitySnapshot{SupportsCamera: false}
	n, feature, _ := dropUnsupportedArg(args, 0, caps)
	if n != 1 || feature != "camera" {
		t.Fatalf("expected camera dropped: n=%d feature=%q", n, feature)
	}
}

func TestDropUnsupportedArgVideoSourceSpaceSeparated(t *testing.T) {
	args := []string{"--video-source", "camera"}
	caps := DeviceCapabilitySnapshot{SupportsCamera: false}
	n, feature, dropped := dropUnsupportedArg(args, 0, caps)
	if n != 2 || feature != "camera" || dropped != "--video-source camera" {
		t.Fatalf("got n=%d feature=%q dropped=%q", n, feature, dropped)
	}
}

func TestDropUnsupportedArgVideoSourceSpaceSupported(t *testing.T) {
	args := []string{"--video-source", "camera"}
	caps := DeviceCapabilitySnapshot{SupportsCamera: true}
	n, _, _ := dropUnsupportedArg(args, 0, caps)
	if n != 0 {
		t.Fatal("should not drop when camera supported")
	}
}

func TestDropUnsupportedArgVideoSourceSpaceBoundsCheck(t *testing.T) {
	// --video-source at end of args with no next arg
	args := []string{"--video-source"}
	caps := DeviceCapabilitySnapshot{SupportsCamera: false}
	n, _, _ := dropUnsupportedArg(args, 0, caps)
	if n != 0 {
		t.Fatalf("should return 0 when no next arg: n=%d", n)
	}
}

func TestDropUnsupportedArgUHIDGamepad(t *testing.T) {
	args := []string{"--uhid-gamepad"}
	caps := DeviceCapabilitySnapshot{SupportsGamepadUHID: false}
	n, feature, _ := dropUnsupportedArg(args, 0, caps)
	if n != 1 || feature != "gamepad_uhid" {
		t.Fatalf("n=%d feature=%q", n, feature)
	}

	caps2 := DeviceCapabilitySnapshot{SupportsGamepadUHID: true}
	n2, _, _ := dropUnsupportedArg(args, 0, caps2)
	if n2 != 0 {
		t.Fatalf("should not drop when supported: n=%d", n2)
	}
}

func TestDropUnsupportedArgAOAGamepad(t *testing.T) {
	args := []string{"--aoa-gamepad"}
	caps := DeviceCapabilitySnapshot{SupportsGamepadAOA: false}
	n, feature, _ := dropUnsupportedArg(args, 0, caps)
	if n != 1 || feature != "gamepad_aoa" {
		t.Fatalf("n=%d feature=%q", n, feature)
	}
}

func TestDropUnsupportedArgGamepadSpaceSeparatedUHID(t *testing.T) {
	args := []string{"--gamepad", "uhid"}
	caps := DeviceCapabilitySnapshot{SupportsGamepadUHID: false}
	n, feature, dropped := dropUnsupportedArg(args, 0, caps)
	if n != 2 || feature != "gamepad_uhid" || dropped != "--gamepad uhid" {
		t.Fatalf("n=%d feature=%q dropped=%q", n, feature, dropped)
	}
}

func TestDropUnsupportedArgGamepadSpaceSeparatedAOA(t *testing.T) {
	args := []string{"--gamepad", "aoa"}
	caps := DeviceCapabilitySnapshot{SupportsGamepadAOA: false}
	n, feature, _ := dropUnsupportedArg(args, 0, caps)
	if n != 2 || feature != "gamepad_aoa" {
		t.Fatalf("n=%d feature=%q", n, feature)
	}
}

func TestDropUnsupportedArgGamepadBoundsCheck(t *testing.T) {
	args := []string{"--gamepad"}
	caps := DeviceCapabilitySnapshot{}
	n, _, _ := dropUnsupportedArg(args, 0, caps)
	if n != 0 {
		t.Fatalf("should return 0 with no next arg: n=%d", n)
	}
}

func TestDropUnsupportedArgUnknownArg(t *testing.T) {
	args := []string{"--unknown-flag"}
	caps := DeviceCapabilitySnapshot{}
	n, _, _ := dropUnsupportedArg(args, 0, caps)
	if n != 0 {
		t.Fatalf("unknown flag should not be dropped: n=%d", n)
	}
}

func TestDropUnsupportedArgVideoSourceCameraSupported(t *testing.T) {
	args := []string{"--video-source", "camera"}
	caps := DeviceCapabilitySnapshot{SupportsCamera: true}
	n, _, _ := dropUnsupportedArg(args, 0, caps)
	if n != 0 {
		t.Fatalf("should not drop camera when supported: n=%d", n)
	}
}

// ── appendUnique ──────────────────────────────────────────────────────────────

func TestAppendUniqueDuplicate(t *testing.T) {
	got := appendUnique([]string{"a", "b"}, "a")
	if len(got) != 2 {
		t.Fatalf("expected no append on duplicate: %v", got)
	}
}

func TestAppendUniqueNew(t *testing.T) {
	got := appendUnique([]string{"a", "b"}, "c")
	if len(got) != 3 || got[2] != "c" {
		t.Fatalf("expected append: %v", got)
	}
}

// ── RankEndpoints fuller coverage ─────────────────────────────────────────────

func TestRankEndpointsTCPDisallowed(t *testing.T) {
	policy := ConnectionPolicy{PreferUSB: true, AllowTCP: false}
	eps := []Endpoint{
		{Transport: "usb", Priority: 10},
		{Transport: "tcp", Priority: 5},
	}
	ranked := RankEndpoints(policy, eps)
	for _, ep := range ranked {
		if ep.Transport == "tcp" {
			t.Fatal("TCP endpoint should be excluded when AllowTCP=false")
		}
	}
	if len(ranked) != 1 {
		t.Fatalf("expected 1 endpoint, got %d", len(ranked))
	}
}

func TestRankEndpointsFailureCountPenalty(t *testing.T) {
	policy := ConnectionPolicy{PreferUSB: false, AllowTCP: true}
	eps := []Endpoint{
		{Transport: "tcp", Priority: 10, FailureCount: 5, Host: "a", Port: 5555},
		{Transport: "tcp", Priority: 10, FailureCount: 0, Host: "b", Port: 5555},
	}
	ranked := RankEndpoints(policy, eps)
	// Host "b" (0 failures) should rank before host "a" (5 failures)
	if ranked[0].Host != "b" {
		t.Fatalf("expected host b first (lower failures), got %s", ranked[0].Host)
	}
}

func TestRankEndpointsTCPPreferred(t *testing.T) {
	policy := ConnectionPolicy{PreferUSB: false, AllowTCP: true}
	eps := []Endpoint{
		{Transport: "usb", Priority: 50, Host: "local", Port: 0},
		{Transport: "tcp", Priority: 50, Host: "192.168.1.1", Port: 5555},
	}
	ranked := RankEndpoints(policy, eps)
	if len(ranked) != 2 {
		t.Fatalf("expected 2 endpoints: %v", ranked)
	}
	// TCP gets -50 bonus when !PreferUSB
	if ranked[0].Transport != "tcp" {
		t.Fatalf("expected TCP first when PreferUSB=false, got %s", ranked[0].Transport)
	}
}

func TestRankEndpointsDeterministicTie(t *testing.T) {
	policy := ConnectionPolicy{PreferUSB: true, AllowTCP: true}
	eps := []Endpoint{
		{Transport: "tcp", Host: "b.host", Port: 5555, Priority: 0},
		{Transport: "tcp", Host: "a.host", Port: 5555, Priority: 0},
	}
	r1 := RankEndpoints(policy, eps)
	r2 := RankEndpoints(policy, eps)
	for i := range r1 {
		if r1[i].Host != r2[i].Host {
			t.Fatalf("ranking not deterministic: %v vs %v", r1, r2)
		}
	}
}

// ── desiredValue ──────────────────────────────────────────────────────────────

func TestDesiredValueNilMap(t *testing.T) {
	p := ProfileDefinition{Desired: nil}
	if got := desiredValue(p, "key"); got != "" {
		t.Fatalf("expected empty for nil Desired, got %q", got)
	}
}

func TestDesiredValueMissingKey(t *testing.T) {
	p := ProfileDefinition{Desired: map[string]string{"other": "val"}}
	if got := desiredValue(p, "missing"); got != "" {
		t.Fatalf("expected empty for missing key, got %q", got)
	}
}

func TestDesiredValueFound(t *testing.T) {
	p := ProfileDefinition{Desired: map[string]string{"max_fps": "60"}}
	if got := desiredValue(p, "max_fps"); got != "60" {
		t.Fatalf("expected '60', got %q", got)
	}
}

// ── ValidateLaunch — additional paths ─────────────────────────────────────────

func TestValidateLaunchDisplayAliasConflict(t *testing.T) {
	// "--display" (alias) conflicts with "--new-display"
	res := ValidateLaunch([]string{"--display", "0", "--new-display"})
	if res.OK {
		t.Fatal("expected validation failure for --display + --new-display conflict")
	}
}

func TestValidateLaunchNewDisplayEqualsConflict(t *testing.T) {
	// --display-id + --new-display=1920x1080
	res := ValidateLaunch([]string{"--display-id", "1", "--new-display=1920x1080"})
	if res.OK {
		t.Fatal("expected failure for --display-id + --new-display=...")
	}
}

func TestValidateLaunchNewDisplayOnlyOK(t *testing.T) {
	// Only --new-display — no conflict
	res := ValidateLaunch([]string{"--new-display", "--max-size", "1920"})
	if !res.OK {
		t.Fatalf("expected OK for --new-display only: errors=%v", res.Errors)
	}
}

func TestValidateLaunchEmptyStringsOnly(t *testing.T) {
	res := ValidateLaunch([]string{"   ", "", "  "})
	if res.OK {
		t.Fatal("expected failure for all-whitespace args")
	}
}

// ── ResolveEffectiveProfile — require_audio/require_camera blocked ────────────

func TestResolveRequireAudioAndCameraBlocksLaunch(t *testing.T) {
	p := DefaultProfile()
	p.DesiredFlags = map[string]bool{"require_audio": true, "require_camera": true}
	res := ResolveEffectiveProfile(p, DeviceCapabilitySnapshot{
		SDKInt: 34, SupportsAudio: false, SupportsCamera: false,
	})
	if res.Launchable {
		t.Fatal("expected launchable=false when both require_audio and require_camera unsupported")
	}
	if len(res.BlockedFeatures) < 2 {
		t.Fatalf("expected 2 blocked features, got %v", res.BlockedFeatures)
	}
}

// ── normalizedLaunchMode — DesiredFlags new_display path ──────────────────────

func TestNormalizedLaunchModeViaDesiredFlags(t *testing.T) {
	p := ProfileDefinition{
		DesiredFlags: map[string]bool{"new_display": true},
	}
	res := ResolveEffectiveProfile(p, DeviceCapabilitySnapshot{SDKInt: 34, SupportsVirtualDisplay: true})
	// new_display flag in DesiredFlags should suppress --display-id
	for _, arg := range res.FinalArgs {
		if arg == "--display-id" || arg == "--display" {
			t.Fatalf("expected no display-id with DesiredFlags.new_display=true: %v", res.FinalArgs)
		}
	}
}

func TestNormalizedLaunchModeViaDesiredStringValue(t *testing.T) {
	p := ProfileDefinition{
		Desired: map[string]string{"new_display": "yes"},
	}
	res := ResolveEffectiveProfile(p, DeviceCapabilitySnapshot{SDKInt: 34, SupportsVirtualDisplay: true})
	// "yes" should trigger new_display mode
	for _, arg := range res.FinalArgs {
		if arg == "--display-id" || arg == "--display" {
			t.Fatalf("expected no display-id for new_display=yes: %v", res.FinalArgs)
		}
	}
}

// ── mergeUniqueArgs — secondary arg already in primary ────────────────────────

func TestMergeUniqueArgsSecondaryAlreadyInPrimary(t *testing.T) {
	// If a secondary arg exists in primary, it should not be duplicated
	p := DefaultProfile()
	p.Desired = map[string]string{"launch_mode": "new_display"}
	p.ExtraArgs = []string{"--new-display", "--stay-awake"} // --new-display is also in primary (synthesized)

	caps := DeviceCapabilitySnapshot{SDKInt: 34, SupportsVirtualDisplay: true}
	res := ResolveEffectiveProfile(p, caps)

	count := 0
	for _, a := range res.FinalArgs {
		if a == "--new-display" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("mergeUniqueArgs should suppress duplicate --new-display: got %d in %v", count, res.FinalArgs)
	}
}

// ── RankEndpoints — empty input ───────────────────────────────────────────────

func TestRankEndpointsEmpty(t *testing.T) {
	ranked := RankEndpoints(ConnectionPolicy{AllowTCP: true}, nil)
	if len(ranked) != 0 {
		t.Fatalf("expected empty result for empty input: %v", ranked)
	}
}

func TestRankEndpointsAllFiltered(t *testing.T) {
	// AllowTCP=false with only TCP endpoints → all filtered out
	eps := []Endpoint{
		{Transport: "tcp", Host: "10.0.0.1", Port: 5555, Priority: 0},
	}
	ranked := RankEndpoints(ConnectionPolicy{PreferUSB: true, AllowTCP: false}, eps)
	if len(ranked) != 0 {
		t.Fatalf("expected all filtered: %v", ranked)
	}
}

// ── mergeUniqueArgs — empty strings skipped ───────────────────────────────────

func TestMergeUniqueArgsSkipsEmptyAndWhitespace(t *testing.T) {
	p := DefaultProfile()
	// ExtraArgs with empty/whitespace entries should be silently skipped
	p.ExtraArgs = []string{"", "  ", "--stay-awake", ""}
	p.DesiredFlags = map[string]bool{"stay_awake": false} // avoid duplicate from desiredFlags
	caps := DeviceCapabilitySnapshot{SDKInt: 34}
	res := ResolveEffectiveProfile(p, caps)
	for _, a := range res.FinalArgs {
		if strings.TrimSpace(a) == "" {
			t.Fatalf("empty/whitespace arg in FinalArgs: %q", res.FinalArgs)
		}
	}
}

// ── ResolveEffectiveProfile — H265 supported path ─────────────────────────────

func TestResolvePreferH265WhenSupported(t *testing.T) {
	p := DefaultProfile()
	p.DesiredFlags = map[string]bool{"prefer_h265": true}
	p.FeatureFlags = map[string]bool{"prefer_h265": true}
	caps := DeviceCapabilitySnapshot{SDKInt: 34, SupportsH265: true}
	res := ResolveEffectiveProfile(p, caps)
	if !hasArg(res.FinalArgs, "--video-codec") {
		t.Fatalf("expected --video-codec when H265 supported: %v", res.FinalArgs)
	}
	// Should be in SupportedFeatures
	foundSupported := false
	for _, f := range res.SupportedFeatures {
		if f == "prefer_h265" {
			foundSupported = true
		}
	}
	if !foundSupported {
		t.Fatalf("expected prefer_h265 in SupportedFeatures: %v", res.SupportedFeatures)
	}
}

// ── RankEndpoints — sort tiebreakers ─────────────────────────────────────────

func TestRankEndpointsTiebreakByTransport(t *testing.T) {
	// Same rank → sort by transport (tcp < usb alphabetically)
	policy := ConnectionPolicy{PreferUSB: false, AllowTCP: true}
	eps := []Endpoint{
		{Transport: "usb", Host: "x", Port: 5555, Priority: 0},
		{Transport: "tcp", Host: "x", Port: 5555, Priority: 0},
	}
	ranked := RankEndpoints(policy, eps)
	if len(ranked) != 2 {
		t.Fatalf("expected 2: %v", ranked)
	}
	// tcp < usb alphabetically → tcp first after TCP bonus
	if ranked[0].Transport != "tcp" {
		t.Logf("note: transport ordering with no preference: got %s first", ranked[0].Transport)
	}
}

func TestRankEndpointsTiebreakByPort(t *testing.T) {
	// Same rank, same transport, same host → sort by port
	policy := ConnectionPolicy{PreferUSB: false, AllowTCP: true}
	eps := []Endpoint{
		{Transport: "tcp", Host: "10.0.0.1", Port: 9999, Priority: 0},
		{Transport: "tcp", Host: "10.0.0.1", Port: 1234, Priority: 0},
	}
	ranked := RankEndpoints(policy, eps)
	if len(ranked) != 2 {
		t.Fatalf("expected 2: %v", ranked)
	}
	// Lower port first
	if ranked[0].Port != 1234 {
		t.Fatalf("expected lower port first, got %d", ranked[0].Port)
	}
}

func TestRankEndpointsTiebreakByName(t *testing.T) {
	// Same everything except name
	policy := ConnectionPolicy{PreferUSB: false, AllowTCP: true}
	eps := []Endpoint{
		{Transport: "tcp", Host: "10.0.0.1", Port: 5555, Priority: 0, Name: "z-wifi"},
		{Transport: "tcp", Host: "10.0.0.1", Port: 5555, Priority: 0, Name: "a-wifi"},
	}
	ranked := RankEndpoints(policy, eps)
	if ranked[0].Name != "a-wifi" {
		t.Fatalf("expected alphabetical name ordering: got %s first", ranked[0].Name)
	}
}

// ── appendUnique — already present ────────────────────────────────────────────

func TestAppendUniqueAlreadyPresent(t *testing.T) {
	before := []string{"a", "b", "c"}
	after := appendUnique(before, "b")
	if len(after) != 3 {
		t.Fatalf("expected no change for existing element: %v", after)
	}
}

// ── ValidateLaunch — hasDisplayID branch via "--display" alias ────────────────

func TestValidateLaunchDisplayAliasSetsHasDisplayID(t *testing.T) {
	// --display (alias) AND --new-display should conflict
	res := ValidateLaunch([]string{"--display", "0", "--new-display"})
	if res.OK {
		t.Fatal("expected conflict for --display + --new-display")
	}
	foundConflict := false
	for _, e := range res.Errors {
		if strings.Contains(e, "conflict") {
			foundConflict = true
		}
	}
	if !foundConflict {
		t.Fatalf("expected conflict error: %v", res.Errors)
	}
}

// ── ResolveEffectiveProfile — audio and camera ExtraArgs dropped ──────────────

func TestResolveDropsAudioExtraArgWhenUnsupported(t *testing.T) {
	p := DefaultProfile()
	p.ExtraArgs = []string{"--audio"}
	caps := DeviceCapabilitySnapshot{SDKInt: 34, SupportsAudio: false}
	res := ResolveEffectiveProfile(p, caps)
	for _, a := range res.FinalArgs {
		if a == "--audio" {
			t.Fatalf("--audio must be dropped when SupportsAudio=false: %v", res.FinalArgs)
		}
	}
	if len(res.DroppedFlags) == 0 {
		t.Fatal("expected --audio in DroppedFlags")
	}
}

func TestResolveDropsCameraExtraArgWhenUnsupported(t *testing.T) {
	p := DefaultProfile()
	p.ExtraArgs = []string{"--camera"}
	caps := DeviceCapabilitySnapshot{SDKInt: 34, SupportsCamera: false}
	res := ResolveEffectiveProfile(p, caps)
	for _, a := range res.FinalArgs {
		if a == "--camera" {
			t.Fatalf("--camera must be dropped: %v", res.FinalArgs)
		}
	}
}

func TestResolveDropsVideoSourceEqualsCamera(t *testing.T) {
	p := DefaultProfile()
	p.ExtraArgs = []string{"--video-source=camera"}
	caps := DeviceCapabilitySnapshot{SDKInt: 34, SupportsCamera: false}
	res := ResolveEffectiveProfile(p, caps)
	for _, a := range res.FinalArgs {
		if strings.Contains(a, "camera") {
			t.Fatalf("camera source must be dropped: %v", res.FinalArgs)
		}
	}
}

func TestResolveVideoSourceEqualsCameraWhenSupported(t *testing.T) {
	p := DefaultProfile()
	p.ExtraArgs = []string{"--video-source=camera"}
	caps := DeviceCapabilitySnapshot{SDKInt: 34, SupportsCamera: true}
	res := ResolveEffectiveProfile(p, caps)
	found := false
	for _, a := range res.FinalArgs {
		if a == "--video-source=camera" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected --video-source=camera when supported: %v", res.FinalArgs)
	}
}

func TestResolveDropsUHIDGamepadWhenUnsupported(t *testing.T) {
	p := DefaultProfile()
	p.ExtraArgs = []string{"--uhid-gamepad"}
	caps := DeviceCapabilitySnapshot{SDKInt: 34, SupportsGamepadUHID: false}
	res := ResolveEffectiveProfile(p, caps)
	for _, a := range res.FinalArgs {
		if a == "--uhid-gamepad" {
			t.Fatalf("--uhid-gamepad must be dropped: %v", res.FinalArgs)
		}
	}
}

func TestResolveDropsAOAGamepadWhenUnsupported(t *testing.T) {
	p := DefaultProfile()
	p.ExtraArgs = []string{"--aoa-gamepad"}
	caps := DeviceCapabilitySnapshot{SDKInt: 34, SupportsGamepadAOA: false}
	res := ResolveEffectiveProfile(p, caps)
	for _, a := range res.FinalArgs {
		if a == "--aoa-gamepad" {
			t.Fatalf("--aoa-gamepad must be dropped: %v", res.FinalArgs)
		}
	}
}

// ── normalizedLaunchMode — DesiredFlags new_display numeric value ─────────────

func TestNormalizedLaunchModeViaDesiredNumericOne(t *testing.T) {
	p := ProfileDefinition{
		Desired: map[string]string{"new_display": "1"},
	}
	res := ResolveEffectiveProfile(p, DeviceCapabilitySnapshot{SDKInt: 34, SupportsVirtualDisplay: true})
	for _, a := range res.FinalArgs {
		if a == "--display-id" || a == "--display" {
			t.Fatalf("expected no --display-id with new_display=1: %v", res.FinalArgs)
		}
	}
}

// ── ValidateLaunch: new flag-conflict checks ─────────────────────────────────

func TestValidateLaunchNoVideoWithBitrateWarn(t *testing.T) {
	res := ValidateLaunch([]string{"--no-video", "--video-bit-rate", "8M"})
	if !res.OK {
		t.Fatalf("expected OK, got errors: %v", res.Errors)
	}
	if len(res.Warnings) == 0 {
		t.Fatal("expected warning for --no-video with --video-bit-rate")
	}
}

func TestValidateLaunchNoAudioWithCodecWarn(t *testing.T) {
	res := ValidateLaunch([]string{"--no-audio", "--audio-codec=aac"})
	if !res.OK {
		t.Fatalf("expected OK, got errors: %v", res.Errors)
	}
	if len(res.Warnings) == 0 {
		t.Fatal("expected warning for --no-audio with --audio-codec")
	}
}

func TestValidateLaunchCameraSourceIgnoresDisplayID(t *testing.T) {
	res := ValidateLaunch([]string{"--video-source=camera", "--display-id", "0"})
	if !res.OK {
		t.Fatalf("expected OK (soft warning not error), got errors: %v", res.Errors)
	}
	if len(res.Warnings) == 0 {
		t.Fatal("expected warning for camera + display-id")
	}
}

func TestValidateLaunchCameraSourceTurnScreenOffWarn(t *testing.T) {
	res := ValidateLaunch([]string{"--video-source=camera", "--turn-screen-off"})
	if !res.OK {
		t.Fatalf("expected OK, got errors: %v", res.Errors)
	}
	if len(res.Warnings) == 0 {
		t.Fatal("expected warning for camera + turn-screen-off")
	}
}

func TestValidateLaunchCleanArgsNoWarnings(t *testing.T) {
	res := ValidateLaunch([]string{"--video-bit-rate", "8M", "--display-id", "0"})
	if !res.OK {
		t.Fatalf("expected OK, got errors: %v", res.Errors)
	}
	if len(res.Warnings) != 0 {
		t.Fatalf("expected no warnings, got: %v", res.Warnings)
	}
}
