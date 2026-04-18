package platform

import (
	"runtime"
	"testing"
)

func TestCurrentOSNonEmpty(t *testing.T) {
	if CurrentOS() == "" {
		t.Fatal("CurrentOS() returned empty string")
	}
}

func TestCurrentOSMatchesRuntimeGOOS(t *testing.T) {
	if CurrentOS() != runtime.GOOS {
		t.Fatalf("CurrentOS() = %q, want %q", CurrentOS(), runtime.GOOS)
	}
}

func TestOSFunctionsConsistentWithCurrentOS(t *testing.T) {
	os := CurrentOS()
	if IsLinux() && os != OSLinux {
		t.Fatalf("IsLinux()=true but CurrentOS()=%q", os)
	}
	if IsMacOS() && os != OSDarwin {
		t.Fatalf("IsMacOS()=true but CurrentOS()=%q", os)
	}
	if IsWindows() && os != OSWindows {
		t.Fatalf("IsWindows()=true but CurrentOS()=%q", os)
	}
}

func TestExactlyOneOSFunctionTrue(t *testing.T) {
	count := 0
	if IsLinux() {
		count++
	}
	if IsMacOS() {
		count++
	}
	if IsWindows() {
		count++
	}
	// On known platforms exactly one function must be true.
	switch runtime.GOOS {
	case OSLinux, OSDarwin, OSWindows:
		if count != 1 {
			t.Fatalf("expected exactly one OS function to return true on %s, got %d", runtime.GOOS, count)
		}
	}
}

func TestUHIDAvailableEqualsIsLinux(t *testing.T) {
	if UHIDAvailable() != IsLinux() {
		t.Fatalf("UHIDAvailable()=%v but IsLinux()=%v — must be equal", UHIDAvailable(), IsLinux())
	}
}

func TestV4L2AvailableEqualsIsLinux(t *testing.T) {
	if V4L2Available() != IsLinux() {
		t.Fatalf("V4L2Available()=%v but IsLinux()=%v", V4L2Available(), IsLinux())
	}
}

func TestMetalAvailableEqualsIsMacOS(t *testing.T) {
	if MetalAvailable() != IsMacOS() {
		t.Fatalf("MetalAvailable()=%v but IsMacOS()=%v", MetalAvailable(), IsMacOS())
	}
}

func TestDirect3DAvailableEqualsIsWindows(t *testing.T) {
	if Direct3DAvailable() != IsWindows() {
		t.Fatalf("Direct3DAvailable()=%v but IsWindows()=%v", Direct3DAvailable(), IsWindows())
	}
}

func TestUnsupportedOptionIDsV4L2NonLinux(t *testing.T) {
	unsupported := UnsupportedOptionIDs()
	if !IsLinux() {
		for _, key := range []string{"v4l2_sink", "v4l2_buffer"} {
			if _, ok := unsupported[key]; !ok {
				t.Fatalf("expected %q in UnsupportedOptionIDs on non-Linux", key)
			}
		}
	} else {
		if _, ok := unsupported["v4l2_sink"]; ok {
			t.Fatal("unexpected v4l2_sink in UnsupportedOptionIDs on Linux")
		}
	}
}

func TestUnsupportedOptionIDsUHIDNonLinux(t *testing.T) {
	unsupported := UnsupportedOptionIDs()
	if !IsLinux() {
		for _, key := range []string{"keyboard_uhid", "mouse_uhid", "gamepad_uhid"} {
			if _, ok := unsupported[key]; !ok {
				t.Fatalf("expected %q in UnsupportedOptionIDs on non-Linux", key)
			}
		}
	}
}

func TestUnsupportedOptionIDsNonNilAlways(t *testing.T) {
	if UnsupportedOptionIDs() == nil {
		t.Fatal("UnsupportedOptionIDs() must never return nil")
	}
}

// ── UnsupportedOptionIDs deeper Linux path ────────────────────────────────────

func TestUnsupportedOptionIDsOnLinuxHasNoV4L2Entries(t *testing.T) {
	if !IsLinux() {
		t.Skip("Linux-only test")
	}
	unsupported := UnsupportedOptionIDs()
	if _, ok := unsupported["v4l2_sink"]; ok {
		t.Fatal("v4l2_sink should NOT be in UnsupportedOptionIDs on Linux")
	}
	if _, ok := unsupported["v4l2_buffer"]; ok {
		t.Fatal("v4l2_buffer should NOT be in UnsupportedOptionIDs on Linux")
	}
}

func TestUnsupportedOptionIDsOnLinuxHasNoUHIDEntries(t *testing.T) {
	if !IsLinux() {
		t.Skip("Linux-only test")
	}
	unsupported := UnsupportedOptionIDs()
	// On Linux, UHID IS available, so these should not be in the unsupported map
	if _, ok := unsupported["keyboard_uhid"]; ok {
		t.Fatal("keyboard_uhid should NOT be in UnsupportedOptionIDs on Linux")
	}
	if _, ok := unsupported["mouse_uhid"]; ok {
		t.Fatal("mouse_uhid should NOT be in UnsupportedOptionIDs on Linux")
	}
	if _, ok := unsupported["gamepad_uhid"]; ok {
		t.Fatal("gamepad_uhid should NOT be in UnsupportedOptionIDs on Linux")
	}
}

func TestUnsupportedOptionIDsMapIsNotNilOnAnyOS(t *testing.T) {
	// Verify the map is always non-nil and safe to iterate
	m := UnsupportedOptionIDs()
	if m == nil {
		t.Fatal("UnsupportedOptionIDs must never return nil")
	}
	for k, v := range m {
		if k == "" {
			t.Fatal("UnsupportedOptionIDs must not have empty keys")
		}
		if v == "" {
			t.Fatalf("UnsupportedOptionIDs[%q] has empty reason", k)
		}
	}
}
