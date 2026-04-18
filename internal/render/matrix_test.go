package render

import (
	"strings"
	"testing"
)

func TestTickKeepsColumnLayoutStable(t *testing.T) {
	m := NewMatrixRain(16, 10)
	m.SetReducedMotion(true)

	beforeLen := len(m.columns)
	beforeTrail := make([]int, len(m.columns))
	beforeSpeed := make([]float64, len(m.columns))
	for i := range m.columns {
		beforeTrail[i] = m.columns[i].trail
		beforeSpeed[i] = m.columns[i].speed
	}

	m.Tick()

	if len(m.columns) != beforeLen {
		t.Fatalf("column count drifted: got %d want %d", len(m.columns), beforeLen)
	}
	for i := range m.columns {
		if m.columns[i].trail != beforeTrail[i] {
			t.Fatalf("trail mutated at x=%d: got %d want %d", i, m.columns[i].trail, beforeTrail[i])
		}
		if m.columns[i].speed != beforeSpeed[i] {
			t.Fatalf("speed mutated at x=%d: got %f want %f", i, m.columns[i].speed, beforeSpeed[i])
		}
	}
}

func TestViewRespectsRequestedLines(t *testing.T) {
	m := NewMatrixRain(12, 9)
	out := m.View(3)
	lines := strings.Split(out, "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
}
