package lga

import (
	"context"
	"errors"
	"math"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/Yui-Qi-Tang/calx/automaton"
)

// hashChoice is a deterministic per-step, per-cell choice for tests.
func hashChoice(step, cell int) bool { return (step*7919+cell*104729)%3 == 0 }

func simulateChoice(step, cell int) []bool { return []bool{hashChoice(step, cell)} }

func TestFHPCollisionTable(t *testing.T) {
	for _, ccw := range []bool{false, true} {
		changed := 0
		for mask := range 64 {
			var p [6]bool
			for d := range 6 {
				p[d] = mask>>d&1 == 1
			}
			q := collideFHP(p, ccw)
			var m1, m2 int
			var q1, r1, q2, r2 int
			for d := range 6 {
				if p[d] {
					m1++
					q1, r1 = q1+fhpSteps[d][0], r1+fhpSteps[d][1]
				}
				if q[d] {
					m2++
					q2, r2 = q2+fhpSteps[d][0], r2+fhpSteps[d][1]
				}
			}
			if m1 != m2 || q1 != q2 || r1 != r2 {
				t.Errorf("ccw=%v %v -> %v changes mass or momentum", ccw, p, q)
			}
			if collideFHP(q, !ccw) != p {
				t.Errorf("ccw=%v %v: opposite choice does not restore", ccw, p)
			}
			if q != p {
				changed++
			}
		}
		if changed != 5 {
			t.Errorf("ccw=%v: %d entries change, want 3 pairs and 2 triples", ccw, changed)
		}
	}
}

func TestFHPAxialVelocitiesMatchCartesian(t *testing.T) {
	for d, s := range fhpSteps {
		x := float64(s[0]) + float64(s[1])/2
		y := float64(s[1]) * math.Sqrt(3) / 2
		if math.Abs(x-fhpVelocity[d][0]) > 1e-12 || math.Abs(y-fhpVelocity[d][1]) > 1e-12 {
			t.Errorf("direction %d: axial %v is (%g, %g), velocity %v", d, s, x, y, fhpVelocity[d])
		}
	}
}

// TestFHPSingleParticleMoves places one particle per direction in an empty
// lattice and checks where it is after one step.
func TestFHPSingleParticleMoves(t *testing.T) {
	f := FHP{Width: 5, Height: 4}
	for d, s := range fhpSteps {
		start := make(automaton.State, f.Cells()*6)
		start[(1*f.Width+2)*6+d] = true // cell (q=2, r=1)
		states, err := automaton.Simulate(f, start, 1, simulateChoice)
		if err != nil {
			t.Fatal(err)
		}
		q, r := (2+s[0]+f.Width)%f.Width, (1+s[1]+f.Height)%f.Height
		want := make(automaton.State, len(start))
		want[(r*f.Width+q)*6+d] = true
		if !slices.Equal(states[1], want) {
			t.Errorf("direction %d did not move to (%d, %d)", d, q, r)
		}
	}
}

// TestFHPConservedExhaustively checks one step from every state and every
// choice of small lattices, with and without a wall.
func TestFHPConservedExhaustively(t *testing.T) {
	for _, f := range []FHP{{Width: 2, Height: 1}, {Width: 1, Height: 2}, {Width: 2, Height: 1, Walls: []bool{true, false}}} {
		bits := f.Cells() * 6
		walls := f.Walls != nil
		for mask := range 1 << bits {
			start := make(automaton.State, bits)
			for i := range start {
				start[i] = mask>>i&1 == 1
			}
			for choices := range 1 << f.Cells() {
				states, err := automaton.Simulate(f, start, 1, func(_, cell int) []bool { return []bool{choices>>cell&1 == 1} })
				if err != nil {
					t.Fatal(err)
				}
				if value(f.Mass(), states[0], 6) != value(f.Mass(), states[1], 6) {
					t.Fatalf("%+v changes mass from %b", f, mask)
				}
				if walls {
					continue
				}
				if value(f.MomentumQ(), states[0], 6) != value(f.MomentumQ(), states[1], 6) ||
					value(f.MomentumR(), states[0], 6) != value(f.MomentumR(), states[1], 6) {
					t.Fatalf("%+v changes momentum from %b", f, mask)
				}
			}
		}
	}
}

func TestFHPWallsReflectAndHaveNoChoice(t *testing.T) {
	f := FHP{Width: 3, Height: 1, Walls: []bool{false, true, false}}
	if f.Choices(1) != 0 || f.Choices(0) != 1 {
		t.Fatal("wall cells must have no choice bit")
	}
	start := make(automaton.State, f.Cells()*6)
	start[0*6+HexE] = true // moving east from cell 0 into the wall at cell 1
	states, err := automaton.Simulate(f, start, 2, simulateChoice)
	if err != nil {
		t.Fatal(err)
	}
	if !states[1][1*6+HexW] || value(f.Mass(), states[1], 6) != 1 {
		t.Fatal("wall did not reflect the particle")
	}
	if !states[2][0*6+HexW] {
		t.Fatal("reflected particle did not travel back west")
	}
}

func TestFHPValidateAndQuantities(t *testing.T) {
	for _, bad := range []FHP{{Width: 0, Height: 2}, {Width: 2, Height: 2, Walls: []bool{true}}} {
		if err := bad.Validate(); !errors.Is(err, ErrInput) {
			t.Errorf("%+v: got %v", bad, err)
		}
	}
	f := FHP{Width: 3, Height: 2}
	if len(f.Mass()) != 36 || len(f.MomentumQ()) != 24 || len(f.MomentumR()) != 24 {
		t.Fatal("quantity sizes")
	}
	if _, err := automaton.Compile(context.Background(), f, automaton.Query{Steps: 1, Violates: f.MomentumQ()}); err != nil {
		t.Fatal(err)
	}
}

func TestFHPFlowFields(t *testing.T) {
	f := FHP{Width: 2, Height: 2}
	s := make(automaton.State, f.Cells()*6)
	s[0*6+HexE] = true
	s[0*6+HexNE] = true
	ux, uy, err := f.Velocity([]automaton.State{s}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(ux[0][0]-0.75) > 1e-12 || math.Abs(uy[0][0]-math.Sqrt(3)/4) > 1e-12 {
		t.Fatalf("velocity (%g, %g)", ux[0][0], uy[0][0])
	}
	density, err := f.Density([]automaton.State{s}, 1)
	if err != nil || density[0][0] != 2 || density[1][1] != 0 {
		t.Fatalf("density %v %v", density, err)
	}
}

func TestRunMatchesSimulate(t *testing.T) {
	r := rand.New(rand.NewPCG(5, 6))
	walls := func(cells int) []bool {
		w := make([]bool, cells)
		for c := range w {
			w[c] = r.Float64() < 0.15
		}
		return w
	}
	sizes := [][2]int{{1, 1}, {2, 1}, {3, 2}, {63, 2}, {64, 3}, {65, 2}, {130, 3}}
	for _, size := range sizes {
		for _, withWalls := range []bool{false, true} {
			cells := size[0] * size[1]
			var w []bool
			if withWalls {
				w = walls(cells)
			}
			h := HPP{Width: size[0], Height: size[1], Walls: w}
			start := randomState(r, cells, 0.35)
			want, err := automaton.Simulate(h, start, 17, nil)
			if err != nil {
				t.Fatal(err)
			}
			got, err := h.Run(start, 17)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got, want[17]) {
				t.Errorf("HPP %dx%d walls=%v: packed run differs", size[0], size[1], withWalls)
			}

			fh := FHP{Width: size[0], Height: size[1], Walls: w}
			fstart := make(automaton.State, cells*6)
			for i := range fstart {
				fstart[i] = r.Float64() < 0.35
			}
			fwant, err := automaton.Simulate(fh, fstart, 17, simulateChoice)
			if err != nil {
				t.Fatal(err)
			}
			fgot, err := fh.Run(fstart, 17, hashChoice)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(fgot, fwant[17]) {
				t.Errorf("FHP %dx%d walls=%v: packed run differs", size[0], size[1], withWalls)
			}
		}
	}
}

// TestRunLongMultiWordRows runs long enough for a stray bit past the row width
// to drift into the next word, which short runs cannot reveal.
func TestRunLongMultiWordRows(t *testing.T) {
	r := rand.New(rand.NewPCG(11, 12))
	h := HPP{Width: 130, Height: 2}
	start := randomState(r, h.Cells(), 0.4)
	want, err := automaton.Simulate(h, start, 150, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := h.Run(start, 150)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, want[150]) {
		t.Fatal("HPP packed run differs after 150 steps")
	}
	f := FHP{Width: 130, Height: 2}
	fstart := make(automaton.State, f.Cells()*6)
	for i := range fstart {
		fstart[i] = r.Float64() < 0.4
	}
	fwant, err := automaton.Simulate(f, fstart, 150, simulateChoice)
	if err != nil {
		t.Fatal(err)
	}
	fgot, err := f.Run(fstart, 150, hashChoice)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(fgot, fwant[150]) {
		t.Fatal("FHP packed run differs after 150 steps")
	}
}

func TestRunValidates(t *testing.T) {
	h := HPP{Width: 2, Height: 2}
	f := FHP{Width: 2, Height: 2}
	for name, err := range map[string]error{
		"hpp short state": func() error { _, err := h.Run(make(automaton.State, 3), 1); return err }(),
		"hpp negative":    func() error { _, err := h.Run(make(automaton.State, 16), -1); return err }(),
		"hpp walls length": func() error {
			_, err := HPP{Width: 2, Height: 2, Walls: []bool{true}}.Run(make(automaton.State, 16), 1)
			return err
		}(),
		"fhp nil choose": func() error { _, err := f.Run(make(automaton.State, 24), 1, nil); return err }(),
		"fhp zero width": func() error { _, err := FHP{Height: 2}.Run(nil, 1, hashChoice); return err }(),
	} {
		if !errors.Is(err, ErrInput) {
			t.Errorf("%s: got %v, want ErrInput", name, err)
		}
	}
}

func BenchmarkHPPSimulate(b *testing.B) {
	h := HPP{Width: 128, Height: 128}
	start := randomState(rand.New(rand.NewPCG(7, 8)), h.Cells(), 0.3)
	for b.Loop() {
		if _, err := automaton.Simulate(h, start, 20, nil); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkHPPRun(b *testing.B) {
	h := HPP{Width: 128, Height: 128}
	start := randomState(rand.New(rand.NewPCG(7, 8)), h.Cells(), 0.3)
	for b.Loop() {
		if _, err := h.Run(start, 20); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFHPRun(b *testing.B) {
	f := FHP{Width: 128, Height: 128}
	start := make(automaton.State, f.Cells()*6)
	r := rand.New(rand.NewPCG(9, 10))
	for i := range start {
		start[i] = r.Float64() < 0.3
	}
	for b.Loop() {
		if _, err := f.Run(start, 20, hashChoice); err != nil {
			b.Fatal(err)
		}
	}
}
