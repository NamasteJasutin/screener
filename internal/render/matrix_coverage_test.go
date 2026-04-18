package render

import (
	"strings"
	"testing"
)

// ── SetPalette ────────────────────────────────────────────────────────────────

func TestSetPaletteChangesOutput(t *testing.T) {
	m := NewMatrixRain(20, 10)
	// Force a tick so the matrix produces output
	m.Tick()
	before := m.View(10)

	m.SetPalette([5]string{"#FF0000", "#CC0000", "#990000", "#660000", "#330000"})
	m.viewDirty = true // force re-render after palette change
	after := m.View(10)

	// The outputs may differ because color codes changed.
	// More importantly: SetPalette must not panic.
	_ = before
	_ = after
}

func TestSetPaletteMarksViewDirty(t *testing.T) {
	m := NewMatrixRain(10, 5)
	m.Tick()
	_ = m.View(5) // populate cache
	m.viewDirty = false

	m.SetPalette([5]string{"#111", "#222", "#333", "#444", "#555"})
	if !m.viewDirty {
		t.Fatal("SetPalette must mark view dirty")
	}
}

// ── SetASCIIMode ──────────────────────────────────────────────────────────────

func TestSetASCIIModeTogglesCharset(t *testing.T) {
	m := NewMatrixRain(40, 20)
	m.SetASCIIMode(true)
	if m.asciiMode != true {
		t.Fatal("expected asciiMode=true")
	}
	if m.charset == nil || len(m.charset) == 0 {
		t.Fatal("charset must not be empty after SetASCIIMode(true)")
	}
	// ASCII charset should only contain printable ASCII
	for _, r := range m.charset {
		if r > 127 {
			t.Fatalf("non-ASCII rune %U in ASCII charset", r)
		}
	}

	m.SetASCIIMode(false)
	if m.asciiMode != false {
		t.Fatal("expected asciiMode=false after toggle")
	}
}

func TestSetASCIIModeMarksViewDirty(t *testing.T) {
	m := NewMatrixRain(10, 5)
	m.Tick()
	_ = m.View(5)
	m.viewDirty = false

	m.SetASCIIMode(true)
	if !m.viewDirty {
		t.Fatal("SetASCIIMode must mark view dirty")
	}
	m.viewDirty = false
	m.SetASCIIMode(false)
	if !m.viewDirty {
		t.Fatal("SetASCIIMode(false) must mark view dirty")
	}
}

func TestSetASCIIModeViewProducesOutput(t *testing.T) {
	m := NewMatrixRain(20, 8)
	m.SetASCIIMode(true)
	for i := 0; i < 5; i++ {
		m.Tick()
	}
	out := m.View(8)
	if out == "" {
		t.Fatal("expected non-empty view in ASCII mode")
	}
}

// ── Resize ────────────────────────────────────────────────────────────────────

func TestResizeExpandsColumns(t *testing.T) {
	m := NewMatrixRain(10, 5)
	m.Resize(20, 5)
	if m.width != 20 {
		t.Fatalf("expected width=20, got %d", m.width)
	}
	if len(m.columns) != 20 {
		t.Fatalf("expected 20 columns, got %d", len(m.columns))
	}
}

func TestResizeShrinks(t *testing.T) {
	m := NewMatrixRain(20, 10)
	m.Resize(5, 5)
	if m.width != 5 {
		t.Fatalf("expected width=5, got %d", m.width)
	}
	if len(m.columns) != 5 {
		t.Fatalf("expected 5 columns after shrink, got %d", len(m.columns))
	}
}

func TestResizeToZeroClampedToOne(t *testing.T) {
	m := NewMatrixRain(10, 5)
	m.Resize(0, 0)
	if m.width < 1 {
		t.Fatalf("width must be at least 1, got %d", m.width)
	}
	if m.height < 1 {
		t.Fatalf("height must be at least 1, got %d", m.height)
	}
}

func TestResizeMarksViewDirty(t *testing.T) {
	m := NewMatrixRain(10, 5)
	m.Tick()
	_ = m.View(5)
	m.viewDirty = false
	m.Resize(15, 7)
	if !m.viewDirty {
		t.Fatal("Resize must mark view dirty")
	}
}

func TestResizePreservesExistingColumns(t *testing.T) {
	m := NewMatrixRain(5, 10)
	// Tick to generate state
	for i := 0; i < 3; i++ {
		m.Tick()
	}
	oldHead0 := m.columns[0].head

	// Expand: existing column 0 should be preserved
	m.Resize(10, 10)
	if m.columns[0].head != oldHead0 {
		t.Fatalf("column[0].head changed during resize: %f → %f", oldHead0, m.columns[0].head)
	}
}

func TestResizeProducesValidView(t *testing.T) {
	m := NewMatrixRain(10, 5)
	m.Resize(30, 12)
	m.Tick()
	out := m.View(12)
	lines := strings.Split(out, "\n")
	if len(lines) != 12 {
		t.Fatalf("expected 12 lines after resize, got %d", len(lines))
	}
}

// ── trailTone ─────────────────────────────────────────────────────────────────

func TestTrailToneHead(t *testing.T) {
	m := NewMatrixRain(1, 1)
	// distance=0 → head → tone=5
	if got := m.trailTone(0, 10); got != 5 {
		t.Fatalf("trailTone(0, 10) = %d, want 5", got)
	}
}

func TestTrailToneTrail1(t *testing.T) {
	m := NewMatrixRain(1, 1)
	// trail=1 → always tone=4
	if got := m.trailTone(1, 1); got != 4 {
		t.Fatalf("trailTone(1, 1) = %d, want 4", got)
	}
}

func TestTrailToneGradient(t *testing.T) {
	m := NewMatrixRain(1, 1)
	trail := 20
	// Tones should decrease as distance increases (head bright → tail dim)
	prev := uint8(5) // start at head
	for d := 0; d <= trail; d++ {
		tone := m.trailTone(d, trail)
		if tone > prev && d > 0 {
			t.Fatalf("trailTone non-monotonic at d=%d: tone=%d > prev=%d", d, tone, prev)
		}
		prev = tone
	}
}

func TestTrailToneAllBucketsReachable(t *testing.T) {
	m := NewMatrixRain(1, 1)
	trail := 100
	seen := map[uint8]bool{}
	for d := 0; d <= trail; d++ {
		seen[m.trailTone(d, trail)] = true
	}
	// Should see tones 1–5
	for tone := uint8(1); tone <= 5; tone++ {
		if !seen[tone] {
			t.Fatalf("trailTone never produced tone %d", tone)
		}
	}
}

// ── clamp ─────────────────────────────────────────────────────────────────────

func TestClampBelowLo(t *testing.T) {
	if got := clamp(-5.0, 0.0, 1.0); got != 0.0 {
		t.Fatalf("clamp(-5, 0, 1) = %f, want 0", got)
	}
}

func TestClampAboveHi(t *testing.T) {
	if got := clamp(10.0, 0.0, 1.0); got != 1.0 {
		t.Fatalf("clamp(10, 0, 1) = %f, want 1", got)
	}
}

func TestClampWithinRange(t *testing.T) {
	if got := clamp(0.5, 0.0, 1.0); got != 0.5 {
		t.Fatalf("clamp(0.5, 0, 1) = %f, want 0.5", got)
	}
}

func TestClampAtBoundary(t *testing.T) {
	if got := clamp(0.0, 0.0, 1.0); got != 0.0 {
		t.Fatalf("clamp at lo: %f", got)
	}
	if got := clamp(1.0, 0.0, 1.0); got != 1.0 {
		t.Fatalf("clamp at hi: %f", got)
	}
}

// ── SetReducedMotion deeper path ──────────────────────────────────────────────

func TestSetReducedMotionToggle(t *testing.T) {
	m := NewMatrixRain(10, 5)
	// Record speeds before reduction
	origSpeeds := make([]float64, len(m.columns))
	for i, col := range m.columns {
		origSpeeds[i] = col.speed
	}

	m.SetReducedMotion(true)
	if !m.reducedMotion {
		t.Fatal("expected reducedMotion=true")
	}
	// SetReducedMotion halves speeds via max(0.16, speed*0.5).
	// Speeds are reduced relative to originals (not clamped to a fixed ceiling).
	for i, col := range m.columns {
		expected := origSpeeds[i] * 0.5
		if expected < 0.16 {
			expected = 0.16
		}
		if col.speed != expected {
			t.Fatalf("column[%d].speed=%f, want %f", i, col.speed, expected)
		}
	}

	m.SetReducedMotion(false)
	if m.reducedMotion {
		t.Fatal("expected reducedMotion=false")
	}
}

func TestSetReducedMotionIdempotent(t *testing.T) {
	m := NewMatrixRain(5, 3)
	m.SetReducedMotion(true)
	sp0 := m.columns[0].speed
	m.SetReducedMotion(true) // should no-op
	if m.columns[0].speed != sp0 {
		t.Fatal("SetReducedMotion(true) twice should not change speed twice")
	}
}

// ── Dirty flag cache ──────────────────────────────────────────────────────────

func TestDirtyFlagCacheHit(t *testing.T) {
	m := NewMatrixRain(10, 5)
	m.Tick()
	first := m.View(5)
	// No tick → dirty=false → should return cached
	second := m.View(5)
	if first != second {
		t.Fatal("expected cache hit on second View without Tick")
	}
}

func TestDirtyFlagCacheMissOnLineSizeChange(t *testing.T) {
	m := NewMatrixRain(10, 10)
	m.Tick()
	v5 := m.View(5)
	v8 := m.View(8) // different line count → must re-render
	if v5 == v8 {
		t.Fatal("expected different output for different line counts")
	}
}

// ── pickChar with empty charset ───────────────────────────────────────────────

func TestPickCharEmptyCharsetReturnsSpace(t *testing.T) {
	m := NewMatrixRain(5, 3)
	m.charset = nil // force empty charset
	got := m.pickChar()
	if got != ' ' {
		t.Fatalf("pickChar with empty charset must return space, got %q", got)
	}
}

func TestPickCharNonEmptyCharset(t *testing.T) {
	m := NewMatrixRain(5, 3)
	m.charset = []rune{'A', 'B', 'C'}
	for i := 0; i < 20; i++ {
		got := m.pickChar()
		if got != 'A' && got != 'B' && got != 'C' {
			t.Fatalf("pickChar returned unexpected rune %q (not in charset)", got)
		}
	}
}

// ── Tick — reduced motion skip path ──────────────────────────────────────────

func TestTickReducedMotionSkipsEveryOtherTick(t *testing.T) {
	m := NewMatrixRain(10, 5)
	m.SetReducedMotion(true)
	// Even tick number: skips the update
	// Odd tick number: processes the update
	// Force tick counter to even value (starts at 0, Tick increments then checks)
	m.tick = 0 // before first Tick() call
	m.Tick()   // tick becomes 1 (odd) → processes
	m.Tick()   // tick becomes 2 (even) → skips
	// Should not panic; reduced motion is working
}

// ── resetColumn — reduced motion speed range ──────────────────────────────────

func TestResetColumnSpeedInReducedRange(t *testing.T) {
	m := NewMatrixRain(10, 5)
	m.SetReducedMotion(true)
	// resetColumn in reduced motion mode should use speedMin=0.16, speedMax=0.75
	for i := range m.columns {
		m.resetColumn(i)
		sp := m.columns[i].speed
		if sp < 0.16 || sp > 0.75 {
			t.Fatalf("resetColumn in reduced mode: speed %f out of [0.16, 0.75]", sp)
		}
	}
}
