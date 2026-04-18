package core

import (
	"strings"
	"testing"
)

func TestAllOptionsNonEmpty(t *testing.T) {
	if len(AllOptions()) == 0 {
		t.Fatal("AllOptions() must not return an empty slice")
	}
}

func TestAllOptionsUniqueIDs(t *testing.T) {
	seen := map[string]bool{}
	for _, opt := range AllOptions() {
		if opt.ID == "" {
			t.Fatalf("option with flag %q has empty ID", opt.Flag)
		}
		if seen[opt.ID] {
			t.Fatalf("duplicate option ID: %q", opt.ID)
		}
		seen[opt.ID] = true
	}
}

func TestAllOptionsValidGroups(t *testing.T) {
	valid := map[string]bool{
		GroupVideo: true, GroupAudio: true, GroupCamera: true,
		GroupInput: true, GroupScreen: true, GroupWindow: true,
		GroupRecording: true, GroupConnection: true, GroupAdvanced: true,
	}
	for _, opt := range AllOptions() {
		if !valid[opt.Group] {
			t.Fatalf("option %q has unrecognised group %q", opt.ID, opt.Group)
		}
	}
}

func TestAllOptionsFlagsNonEmpty(t *testing.T) {
	for _, opt := range AllOptions() {
		if opt.Flag == "" {
			t.Fatalf("option %q has empty Flag", opt.ID)
		}
		if !strings.HasPrefix(opt.Flag, "--") {
			t.Fatalf("option %q flag %q must start with '--'", opt.ID, opt.Flag)
		}
	}
}

func TestGroupOrderCoversAllOptions(t *testing.T) {
	inOrder := map[string]bool{}
	for _, g := range GroupOrder {
		inOrder[g] = true
	}
	for _, opt := range AllOptions() {
		if !inOrder[opt.Group] {
			t.Fatalf("option %q group %q is missing from GroupOrder", opt.ID, opt.Group)
		}
	}
}

func TestOptionsByGroupTotalMatchesAllOptions(t *testing.T) {
	byGroup := OptionsByGroup()
	total := 0
	for _, opts := range byGroup {
		total += len(opts)
	}
	if total != len(AllOptions()) {
		t.Fatalf("OptionsByGroup total %d != AllOptions total %d", total, len(AllOptions()))
	}
}

func TestFindOptionFound(t *testing.T) {
	opt, ok := FindOption("video_codec")
	if !ok {
		t.Fatal("FindOption(\"video_codec\") returned false")
	}
	if opt.Flag != "--video-codec" {
		t.Fatalf("expected --video-codec, got %q", opt.Flag)
	}
	if opt.Type != OptEnum {
		t.Fatalf("expected OptEnum, got %v", opt.Type)
	}
}

func TestFindOptionMissing(t *testing.T) {
	if _, ok := FindOption("definitely_not_a_real_option_xyz"); ok {
		t.Fatal("FindOption should return false for unknown ID")
	}
}

// ── ScrcpyOption.Check ────────────────────────────────────────────────────────

func TestCheckMinSDKIncompatibleBelow(t *testing.T) {
	opt := ScrcpyOption{ID: "t", MinSDK: 31}
	if cr := opt.Check(DeviceCapabilitySnapshot{SDKInt: 28}, "usb"); cr.Compatible {
		t.Fatal("expected incompatible for SDK 28 with MinSDK=31")
	}
}

func TestCheckMinSDKCompatibleAtOrAbove(t *testing.T) {
	opt := ScrcpyOption{ID: "t", MinSDK: 31}
	for _, sdk := range []int{31, 33, 34, 35} {
		if cr := opt.Check(DeviceCapabilitySnapshot{SDKInt: sdk}, "usb"); !cr.Compatible {
			t.Fatalf("expected compatible for SDK %d with MinSDK=31: %s", sdk, cr.Reason)
		}
	}
}

func TestCheckMinSDKZeroAlwaysCompatible(t *testing.T) {
	opt := ScrcpyOption{ID: "t", MinSDK: 0}
	// Even with unknown SDK (0) the option is compatible when MinSDK=0.
	if cr := opt.Check(DeviceCapabilitySnapshot{SDKInt: 0}, "usb"); !cr.Compatible {
		t.Fatalf("expected compatible when MinSDK=0: %s", cr.Reason)
	}
}

func TestCheckRequiresUSBIncompatibleOverTCP(t *testing.T) {
	opt := ScrcpyOption{ID: "otg", Flag: "--otg", RequiresUSB: true}
	if cr := opt.Check(DeviceCapabilitySnapshot{SDKInt: 34}, "tcp"); cr.Compatible {
		t.Fatal("expected incompatible for RequiresUSB option over TCP")
	}
}

func TestCheckRequiresUSBCompatibleOverUSB(t *testing.T) {
	opt := ScrcpyOption{ID: "otg", Flag: "--otg", RequiresUSB: true}
	if cr := opt.Check(DeviceCapabilitySnapshot{SDKInt: 34}, "usb"); !cr.Compatible {
		t.Fatalf("expected compatible over USB: %s", cr.Reason)
	}
}

func TestCheckIncompatibleReasonNonEmpty(t *testing.T) {
	opt := ScrcpyOption{ID: "t", MinSDK: 31}
	cr := opt.Check(DeviceCapabilitySnapshot{SDKInt: 21}, "usb")
	if cr.Compatible {
		t.Fatal("expected incompatible")
	}
	if cr.Reason == "" {
		t.Fatal("Reason must be non-empty when !Compatible")
	}
}

// ── sdkToAndroid ─────────────────────────────────────────────────────────────

func TestSdkToAndroidKnownVersions(t *testing.T) {
	table := []struct {
		sdk  int
		want string
	}{
		{29, "10"}, {30, "11"}, {31, "12"}, {32, "12L"},
		{33, "13"}, {34, "14"}, {35, "15"},
	}
	for _, tc := range table {
		if got := sdkToAndroid(tc.sdk); got != tc.want {
			t.Fatalf("sdkToAndroid(%d) = %q, want %q", tc.sdk, got, tc.want)
		}
	}
}

func TestSdkToAndroidUnknownFallback(t *testing.T) {
	got := sdkToAndroid(99)
	if !strings.Contains(got, "99") {
		t.Fatalf("expected SDK number in fallback string, got %q", got)
	}
}

// ── BuildArgsFromValues ───────────────────────────────────────────────────────

func TestBuildArgsFromValuesBoolTrue(t *testing.T) {
	values := map[string]string{"turn_screen_off": "true"}
	args, dropped := BuildArgsFromValues(values, DeviceCapabilitySnapshot{SDKInt: 34}, "usb")
	if len(dropped) != 0 {
		t.Fatalf("unexpected dropped: %v", dropped)
	}
	if !hasArg(args, "--turn-screen-off") {
		t.Fatalf("expected --turn-screen-off in args: %v", args)
	}
}

func TestBuildArgsFromValuesBoolFalseNotEmitted(t *testing.T) {
	values := map[string]string{"turn_screen_off": "false"}
	args, _ := BuildArgsFromValues(values, DeviceCapabilitySnapshot{SDKInt: 34}, "usb")
	if hasArg(args, "--turn-screen-off") {
		t.Fatalf("unexpected --turn-screen-off for false value: %v", args)
	}
}

func TestBuildArgsFromValuesDefaultValueSkipped(t *testing.T) {
	// h264 is the default for video_codec; emitting it would be noise.
	values := map[string]string{"video_codec": "h264"}
	args, _ := BuildArgsFromValues(values, DeviceCapabilitySnapshot{SDKInt: 34}, "usb")
	for _, a := range args {
		if strings.Contains(a, "video-codec") {
			t.Fatalf("default value should not be emitted: %v", args)
		}
	}
}

func TestBuildArgsFromValuesIncompatibleDropped(t *testing.T) {
	// --otg requires USB; should be dropped and listed when transport is TCP.
	values := map[string]string{"otg": "true"}
	args, dropped := BuildArgsFromValues(values, DeviceCapabilitySnapshot{SDKInt: 34}, "tcp")
	if hasArg(args, "--otg") {
		t.Fatalf("--otg must not be emitted over TCP: %v", args)
	}
	foundDrop := false
	for _, d := range dropped {
		if strings.Contains(d, "--otg") {
			foundDrop = true
		}
	}
	if !foundDrop {
		t.Fatalf("expected --otg in dropped list: %v", dropped)
	}
}

func TestBuildArgsFromValuesH265DroppedOnUnsupportedDevice(t *testing.T) {
	values := map[string]string{"video_codec": "h265"}
	args, dropped := BuildArgsFromValues(values, DeviceCapabilitySnapshot{SDKInt: 34, SupportsH265: false}, "usb")
	for _, a := range args {
		if strings.Contains(strings.ToLower(a), "h265") {
			t.Fatalf("h265 arg emitted for device that doesn't support it: %v", args)
		}
	}
	if len(dropped) == 0 {
		t.Fatal("expected h265 in dropped list for unsupported device")
	}
}

func TestBuildArgsFromValuesH265EmittedWhenSupported(t *testing.T) {
	values := map[string]string{"video_codec": "h265"}
	args, dropped := BuildArgsFromValues(values, DeviceCapabilitySnapshot{SDKInt: 34, SupportsH265: true}, "usb")
	if len(dropped) != 0 {
		t.Fatalf("unexpected dropped when H265 supported: %v", dropped)
	}
	found := false
	for _, a := range args {
		if strings.Contains(a, "h265") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected --video-codec=h265 in args: %v", args)
	}
}

func TestBuildArgsFromValuesSDKGatedDropped(t *testing.T) {
	// max_fps requires SDK 29+; on SDK 21 it should be dropped.
	values := map[string]string{"max_fps": "60"}
	args, dropped := BuildArgsFromValues(values, DeviceCapabilitySnapshot{SDKInt: 21}, "usb")
	if hasArg(args, "--max-fps=60") {
		t.Fatalf("--max-fps should be dropped on SDK 21: %v", args)
	}
	if len(dropped) == 0 {
		t.Fatalf("expected --max-fps in dropped list: %v", dropped)
	}
}

// ── CompatSplit ───────────────────────────────────────────────────────────────

func TestCompatSplitSumEqualsTotal(t *testing.T) {
	opts := AllOptions()
	compat, incompat := CompatSplit(opts, DeviceCapabilitySnapshot{SDKInt: 28}, "tcp")
	if len(compat)+len(incompat) != len(opts) {
		t.Fatalf("CompatSplit total %d+%d != AllOptions %d", len(compat), len(incompat), len(opts))
	}
}

func TestCompatSplitOldSDKHasIncompatible(t *testing.T) {
	_, incompat := CompatSplit(AllOptions(), DeviceCapabilitySnapshot{SDKInt: 28}, "usb")
	if len(incompat) == 0 {
		t.Fatal("expected incompatible options for SDK 28 device")
	}
}

func TestCompatSplitModernSDKAllCompatibleUSBTransport(t *testing.T) {
	opts := AllOptions()
	// Filter to only non-USB-required options for a clean check.
	noUSBRequired := make([]ScrcpyOption, 0)
	for _, o := range opts {
		if !o.RequiresUSB {
			noUSBRequired = append(noUSBRequired, o)
		}
	}
	compat, incompat := CompatSplit(noUSBRequired, DeviceCapabilitySnapshot{SDKInt: 35}, "usb")
	if len(incompat) != 0 {
		t.Fatalf("expected all non-USB-required options compatible on SDK 35: %v", incompat)
	}
	if len(compat) != len(noUSBRequired) {
		t.Fatalf("compat count mismatch: %d vs %d", len(compat), len(noUSBRequired))
	}
}

// ── BuildArgsFromValues — string, int, new_display_size paths ─────────────────

func TestBuildArgsFromValuesStringType(t *testing.T) {
	// record= is a string option
	values := map[string]string{"record": "/tmp/screen.mp4"}
	args, dropped := BuildArgsFromValues(values, DeviceCapabilitySnapshot{SDKInt: 34}, "usb")
	if len(dropped) != 0 {
		t.Fatalf("unexpected dropped: %v", dropped)
	}
	found := false
	for _, a := range args {
		if strings.Contains(a, "record=") && strings.Contains(a, "screen.mp4") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected --record=/tmp/screen.mp4 in args: %v", args)
	}
}

func TestBuildArgsFromValuesIntType(t *testing.T) {
	// max_fps is an OptInt; non-default non-zero value
	values := map[string]string{"max_fps": "120"}
	args, dropped := BuildArgsFromValues(values, DeviceCapabilitySnapshot{SDKInt: 34}, "usb")
	if len(dropped) != 0 {
		t.Fatalf("unexpected dropped: %v", dropped)
	}
	found := false
	for _, a := range args {
		if strings.Contains(a, "max-fps=120") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected --max-fps=120 in args: %v", args)
	}
}

func TestBuildArgsFromValuesIntZeroDefaultSkipped(t *testing.T) {
	// max_size default is "0"; submitting "0" should be skipped
	values := map[string]string{"max_size": "0"}
	args, _ := BuildArgsFromValues(values, DeviceCapabilitySnapshot{SDKInt: 34}, "usb")
	for _, a := range args {
		if strings.Contains(a, "max-size=0") {
			t.Fatalf("zero default int should not be emitted: %v", args)
		}
	}
}

func TestBuildArgsFromValuesNewDisplaySizeSkipped(t *testing.T) {
	// new_display_size is special-cased to skip
	values := map[string]string{"new_display_size": "1920x1080"}
	args, _ := BuildArgsFromValues(values, DeviceCapabilitySnapshot{SDKInt: 34}, "usb")
	for _, a := range args {
		if strings.Contains(a, "new_display_size") || strings.Contains(a, "1920x1080") {
			t.Fatalf("new_display_size should be skipped: %v", args)
		}
	}
}

func TestBuildArgsFromValuesEmptyValueSkipped(t *testing.T) {
	// Empty value means option not set → skip
	values := map[string]string{"record": ""}
	args, _ := BuildArgsFromValues(values, DeviceCapabilitySnapshot{SDKInt: 34}, "usb")
	for _, a := range args {
		if strings.Contains(a, "record") {
			t.Fatalf("empty value should be skipped: %v", args)
		}
	}
}
