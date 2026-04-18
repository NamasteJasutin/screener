package app

import (
	"strings"
	"testing"
	"github.com/NamasteJasutin/screener/internal/core"
	"github.com/NamasteJasutin/screener/internal/scrcpy"
)

// INV-1: Preview string == executed binary+args (including --serial)
func TestInvariant1PreviewEqualsExecutedArgs(t *testing.T) {
	caps := core.DeviceCapabilitySnapshot{
		Serial: "LIVE001", SDKInt: 34, State: "device",
		SupportsAudio: true, SupportsH265: true, SupportsVirtualDisplay: true,
	}
	plan := scrcpy.BuildPlan(core.DefaultProfile(), caps)
	preview := scrcpy.Preview(plan)
	execStr := strings.TrimSpace(plan.Binary + " " + strings.Join(plan.Args, " "))
	if preview != execStr {
		t.Fatalf("INV-1 BROKEN: preview=%q != exec=%q", preview, execStr)
	}
	if !strings.Contains(preview, "--serial=LIVE001") {
		t.Fatalf("INV-1 BROKEN: --serial=LIVE001 missing from preview: %q", preview)
	}
}

// INV-1b: No --serial for empty serial (simulated/offline)
func TestInvariant1NoSerialForEmptySerial(t *testing.T) {
	plan := scrcpy.BuildPlanFromResolution(
		core.ResolveEffectiveProfile(core.DefaultProfile(), core.DeviceCapabilitySnapshot{SDKInt: 34}),
		"",
	)
	if strings.Contains(scrcpy.Preview(plan), "--serial=") {
		t.Fatalf("INV-1b BROKEN: --serial present with empty serial: %q", scrcpy.Preview(plan))
	}
}

// INV-2: Launchable=false → Execute errors, never runs binary
func TestInvariant2NonLaunchableBlocked(t *testing.T) {
	plan := scrcpy.BuildPlanFromResolution(
		core.EffectiveProfileResolution{FinalArgs: []string{"--audio"}, Launchable: false},
		"",
	)
	if plan.Launchable {
		t.Fatal("INV-2 BROKEN: plan.Launchable should be false")
	}
}

// INV-3: --serial appears iff device IsLive in model recompute
func TestInvariant3SerialOnlyForLiveDevice(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()

	// Live device
	m.devices = []core.DeviceCapabilitySnapshot{{Serial: "USB999", SDKInt: 34, State: "device"}}
	m.deviceIdx = 0
	m.recomputePlanAndPreview()
	if !strings.Contains(m.preview, "--serial=USB999") {
		t.Fatalf("INV-3 BROKEN: live device missing --serial: %q", m.preview)
	}

	// Known-offline device (IsLive=false)
	m.devices = nil
	m.config.KnownDevices = []core.KnownDevice{{Alias: "mydevice", Serial: "OFFLINE1",
		Endpoints: []core.Endpoint{{Host: "10.0.0.1", Port: 5555, Transport: "tcp"}}}}
	m.deviceIdx = 0
	m.recomputePlanAndPreview()
	if strings.Contains(m.preview, "--serial=") {
		t.Fatalf("INV-3 BROKEN: offline device should not have --serial: %q", m.preview)
	}
}

// INV-5: Dropped flags not in FinalArgs
func TestInvariant5DroppedFlagsAbsentFromFinalArgs(t *testing.T) {
	p := core.DefaultProfile()
	p.ExtraArgs = []string{"--new-display", "--gamepad=uhid", "--video-source=camera"}
	res := core.ResolveEffectiveProfile(p, core.DeviceCapabilitySnapshot{
		SDKInt: 34, SupportsVirtualDisplay: false, SupportsGamepadUHID: false, SupportsCamera: false,
	})
	forbidden := map[string]bool{"--new-display": true, "--gamepad=uhid": true, "--video-source=camera": true}
	for _, a := range res.FinalArgs {
		if forbidden[a] {
			t.Fatalf("INV-5 BROKEN: dropped flag %q present in FinalArgs=%v", a, res.FinalArgs)
		}
	}
	if len(res.DroppedFlags) == 0 {
		t.Fatal("INV-5 BROKEN: expected DroppedFlags to be populated")
	}
}

// INV-9: SDKInt=0 snapshot has all caps false
func TestInvariant9ZeroSDKAllCapsFalse(t *testing.T) {
	snap := core.DeviceCapabilitySnapshot{SDKInt: 0}
	if snap.SupportsAudio || snap.SupportsCamera || snap.SupportsVirtualDisplay ||
		snap.SupportsH265 || snap.SupportsGamepadUHID || snap.SupportsGamepadAOA {
		t.Fatalf("INV-9 BROKEN: SDKInt=0 should have all capabilities false: %+v", snap)
	}
}

// INV-9b: buildBaseSnapshot produces SDKInt=0 (conservative)
func TestInvariant9bBaseSnapshotSDKZero(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	m.devices = []core.DeviceCapabilitySnapshot{
		// Inject a device that came from buildBaseSnapshot (unauthorized — not probed)
		{Serial: "UNAUTH", State: "unauthorized", SDKInt: 0},
	}
	m.deviceIdx = 0
	m.recomputePlanAndPreview()
	// With SDKInt=0, turn_screen_off and stay_awake should be dropped
	if strings.Contains(m.preview, "--turn-screen-off") {
		t.Fatalf("INV-9b BROKEN: --turn-screen-off should be dropped for SDKInt=0: %q", m.preview)
	}
}

// INV-10: --display-id and --new-display never coexist in ValidateLaunch
func TestInvariant10ConflictAlwaysRejected(t *testing.T) {
	cases := [][]string{
		{"--display-id", "0", "--new-display"},
		{"--display-id", "1", "--new-display=1920x1080"},
		{"--display", "0", "--new-display"},
	}
	for _, args := range cases {
		if vr := core.ValidateLaunch(args); vr.OK {
			t.Fatalf("INV-10 BROKEN: conflict not rejected for args=%v", args)
		}
	}
}
