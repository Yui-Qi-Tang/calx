package lga

import (
	"slices"
	"strings"
	"testing"

	"github.com/Yui-Qi-Tang/calx/automaton"
)

// The tests in this file pin the claims of docs/lga-primer.md without a SAT
// solver: every answer comes from Simulate.

// arrows draws an HPP state row by row from the top, as the primer does.
func arrows(s automaton.State, width, height int) []string {
	var rows []string
	for y := height - 1; y >= 0; y-- {
		var cells []string
		for x := range width {
			cell := ""
			for d, r := range []string{"→", "↑", "←", "↓"} {
				if s[(y*width+x)*4+d] {
					cell += r
				}
			}
			if cell == "" {
				cell = "·"
			}
			cells = append(cells, cell)
		}
		rows = append(rows, strings.Join(cells, " "))
	}
	return rows
}

func primerRun(t *testing.T, h HPP, start automaton.State, steps int) [][]string {
	t.Helper()
	states, err := automaton.Simulate(h, start, steps, nil)
	if err != nil {
		t.Fatal(err)
	}
	var frames [][]string
	for _, s := range states {
		frames = append(frames, arrows(s, h.Width, h.Height))
	}
	return frames
}

func TestLGAPrimerCollision(t *testing.T) {
	h := HPP{Width: 5, Height: 3}
	start := make(automaton.State, h.Cells()*4)
	start[(1*5+0)*4+East] = true
	start[(1*5+4)*4+West] = true
	want := [][]string{
		{"· · · · ·", "→ · · · ←", "· · · · ·"},
		{"· · · · ·", "· → · ← ·", "· · · · ·"},
		{"· · · · ·", "· · ↑↓ · ·", "· · · · ·"},
		{"· · ↑ · ·", "· · · · ·", "· · ↓ · ·"},
		{"· · ↓ · ·", "· · · · ·", "· · ↑ · ·"},
		{"· · · · ·", "· · →← · ·", "· · · · ·"},
	}
	if got := primerRun(t, h, start, 5); !slices.EqualFunc(got, want, slices.Equal) {
		t.Fatalf("frames %q, want %q", got, want)
	}
}

func TestLGAPrimerWall(t *testing.T) {
	h := HPP{Width: 5, Height: 1, Walls: []bool{false, false, false, false, true}}
	start := make(automaton.State, h.Cells()*4)
	start[East] = true
	want := []string{"→ · · · ·", "· → · · ·", "· · → · ·", "· · · → ·", "· · · · ←", "· · · ← ·", "· · ← · ·"}
	var got []string
	for _, f := range primerRun(t, h, start, 6) {
		got = append(got, f[0])
	}
	if !slices.Equal(got, want) {
		t.Fatalf("frames %q, want %q", got, want)
	}
}

// TestLGAPrimerFlow repeats examples/lga without -check.
func TestLGAPrimerFlow(t *testing.T) {
	h := HPP{Width: 16, Height: 8}
	start := make(automaton.State, h.Cells()*4)
	for y := range h.Height {
		for x := range 4 {
			start[(y*h.Width+x)*4+East] = true
		}
		start[(y*h.Width+8)*4+West] = true
	}
	states, err := automaton.Simulate(h, start, 8, nil)
	if err != nil {
		t.Fatal(err)
	}
	last := states[8:]
	if value(h.Mass(), start, 4) != 40 || value(h.Mass(), last[0], 4) != 40 ||
		value(h.MomentumX(), start, 4) != 24 || value(h.MomentumX(), last[0], 4) != 24 {
		t.Fatal("mass or x momentum differs from the primer")
	}
	density, err := h.Density(last, 4)
	if err != nil {
		t.Fatal(err)
	}
	ux, _, err := h.Velocity(last, 4)
	if err != nil {
		t.Fatal(err)
	}
	for row := range 2 {
		if !slices.Equal(density[row], []float64{0.25, 0.25, 0.75, 0}) || !slices.Equal(ux[row], []float64{-1, 1, 1, 0}) {
			t.Fatalf("row %d: density %v, velocity %v", row, density[row], ux[row])
		}
	}
}

// TestLGAPrimerQuestions answers the primer's two solver questions by
// simulation: every 2×2 start keeps its mass at every time up to 3, and the
// broken rule has a start whose mass changes.
func TestLGAPrimerQuestions(t *testing.T) {
	small := HPP{Width: 2, Height: 2}
	for mask := range 1 << 16 {
		start := make(automaton.State, 16)
		for i := range start {
			start[i] = mask>>i&1 == 1
		}
		states, err := automaton.Simulate(small, start, 3, nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range states {
			if value(small.Mass(), s, 4) != value(small.Mass(), start, 4) {
				t.Fatalf("start %016b changes mass", mask)
			}
		}
	}
	h := HPP{Width: 4, Height: 4}
	leaky := mutatedHPP{HPP: h, from: headOnToNorth[0], to: headOnToNorth[1]}
	start := make(automaton.State, h.Cells()*4)
	start[(1*4+0)*4+East] = true // meets the west mover at (1,1) after one step
	start[(1*4+2)*4+West] = true
	states, err := automaton.Simulate(leaky, start, 3, nil)
	if err != nil {
		t.Fatal(err)
	}
	var masses []int
	for _, s := range states {
		masses = append(masses, value(h.Mass(), s, 4))
	}
	if !slices.Equal(masses, []int{2, 1, 1, 1}) {
		t.Fatalf("broken rule masses %v, want 2 1 1 1", masses)
	}
}

// TestLGAPrimerFHP pins section 5: head-on pairs rotate by the choice bit, the
// symmetric triples swap, and wall cells have no choice bit.
func TestLGAPrimerFHP(t *testing.T) {
	if got := collideFHP([6]bool{HexE: true, HexW: true}, true); got != [6]bool{HexNE: true, HexSW: true} {
		t.Fatalf("counterclockwise E+W gives %v", got)
	}
	if got := collideFHP([6]bool{HexE: true, HexW: true}, false); got != [6]bool{HexSE: true, HexNW: true} {
		t.Fatalf("clockwise E+W gives %v", got)
	}
	if got := collideFHP([6]bool{HexE: true, HexNW: true, HexSW: true}, true); got != [6]bool{HexNE: true, HexW: true, HexSE: true} {
		t.Fatalf("triple gives %v", got)
	}
	f := FHP{Width: 2, Height: 1, Walls: []bool{false, true}}
	if f.Choices(0) != 1 || f.Choices(1) != 0 {
		t.Fatalf("choices %d %d, want 1 0", f.Choices(0), f.Choices(1))
	}
}
