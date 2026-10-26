package lga

import (
	"context"
	"errors"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/Yui-Qi-Tang/calx/automaton"
	"github.com/Yui-Qi-Tang/calx/sat"
)

// mutatedHPP replaces one collision table entry, for mutation tests.
type mutatedHPP struct {
	HPP
	from, to [4]bool
}

func (m mutatedHPP) Next(cell int, in, out []bool) {
	if [4]bool(in) == m.from {
		copy(out, m.to[:])
		return
	}
	m.HPP.Next(cell, in, out)
}

// headOnToNorth loses a particle: an east-west pair becomes one north particle.
var headOnToNorth = [2][4]bool{{East: true, West: true}, {North: true}}

// headOnToNorthEast keeps two particles but gains momentum in x and y.
var headOnToNorthEast = [2][4]bool{{East: true, West: true}, {East: true, North: true}}

func value(q automaton.Quantity, s automaton.State) int {
	v := 0
	for _, t := range q {
		if s[t.Bit.Cell*4+t.Bit.Index] {
			v += t.Weight
		}
	}
	return v
}

func randomState(r *rand.Rand, cells int, density float64) automaton.State {
	s := make(automaton.State, cells*4)
	for i := range s {
		s[i] = r.Float64() < density
	}
	return s
}

func TestCollisionTable(t *testing.T) {
	changed := 0
	for mask := range 16 {
		var p [4]bool
		for d := range 4 {
			p[d] = mask>>d&1 == 1
		}
		q := collideHPP(p)
		count := func(a [4]bool) (m, x, y int) {
			for d, b := range a {
				if !b {
					continue
				}
				m++
				x += map[int]int{East: 1, West: -1}[d]
				y += map[int]int{North: 1, South: -1}[d]
			}
			return m, x, y
		}
		m1, x1, y1 := count(p)
		m2, x2, y2 := count(q)
		if m1 != m2 || x1 != x2 || y1 != y2 {
			t.Errorf("%v -> %v changes mass or momentum", p, q)
		}
		if collideHPP(q) != p {
			t.Errorf("%v: collision is not an involution", p)
		}
		if q != p {
			changed++
		}
	}
	if changed != 2 {
		t.Fatalf("%d entries change, want the two head-on pairs", changed)
	}
}

// directStep is an independent HPP implementation on explicit grids: stream
// every particle to its neighbor, then collide each cell.
func directStep(w, h int, s automaton.State) automaton.State {
	at := func(x, y, d int) bool {
		x, y = (x%w+w)%w, (y%h+h)%h
		return s[(y*w+x)*4+d]
	}
	next := make(automaton.State, len(s))
	for y := range h {
		for x := range w {
			arrived := [4]bool{
				East:  at(x-1, y, East),
				North: at(x, y-1, North),
				West:  at(x+1, y, West),
				South: at(x, y+1, South),
			}
			if arrived == [4]bool{East: true, West: true} {
				arrived = [4]bool{North: true, South: true}
			} else if arrived == [4]bool{North: true, South: true} {
				arrived = [4]bool{East: true, West: true}
			}
			copy(next[(y*w+x)*4:], arrived[:])
		}
	}
	return next
}

func TestSimulateMatchesDirectImplementation(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	for _, size := range [][2]int{{1, 1}, {2, 1}, {1, 3}, {2, 3}, {5, 4}, {8, 8}} {
		h := HPP{Width: size[0], Height: size[1]}
		start := randomState(r, h.Cells(), 0.4)
		states, err := automaton.Simulate(h, start, 25, nil)
		if err != nil {
			t.Fatal(err)
		}
		want := start
		for step := 1; step <= 25; step++ {
			want = directStep(h.Width, h.Height, want)
			if !slices.Equal(states[step], want) {
				t.Fatalf("%dx%d step %d differs", h.Width, h.Height, step)
			}
		}
	}
}

func collideAll(s automaton.State) automaton.State {
	out := make(automaton.State, len(s))
	for base := 0; base < len(s); base += 4 {
		p := collideHPP([4]bool(s[base : base+4]))
		copy(out[base:], p[:])
	}
	return out
}

// TestTimeReversal uses HPP's reversibility. One step is F = C∘S (stream, then
// collide). Reversing every direction R satisfies R∘S∘R = S⁻¹ and R∘C∘R = C,
// so the start is recovered as C(R(F^k(R(C(end))))).
func TestTimeReversal(t *testing.T) {
	r := rand.New(rand.NewPCG(3, 4))
	h := HPP{Width: 6, Height: 5}
	for k := 1; k <= 12; k++ {
		start := randomState(r, h.Cells(), 0.5)
		forward, err := automaton.Simulate(h, start, k, nil)
		if err != nil {
			t.Fatal(err)
		}
		back, err := automaton.Simulate(h, reverse(collideAll(forward[k])), k, nil)
		if err != nil {
			t.Fatal(err)
		}
		if got := collideAll(reverse(back[k])); !slices.Equal(got, start) {
			t.Fatalf("k=%d: reversal did not recover the start", k)
		}
	}
}

func hppQuantities(h HPP) map[string]automaton.Quantity {
	q := map[string]automaton.Quantity{"mass": h.Mass(), "momentum-x": h.MomentumX(), "momentum-y": h.MomentumY()}
	for y := range h.Height {
		q["row-x"+string(rune('0'+y))] = h.RowMomentumX(y)
	}
	for x := range h.Width {
		q["column-y"+string(rune('0'+x))] = h.ColumnMomentumY(x)
	}
	return q
}

// TestQuantitiesConservedExhaustively checks one step from every state of a
// 2×2 lattice, and that a mutated rule is caught by the same check.
func TestQuantitiesConservedExhaustively(t *testing.T) {
	h := HPP{Width: 2, Height: 2}
	quantities := hppQuantities(h)
	broken := map[string]bool{}
	for mask := range 1 << 16 {
		start := make(automaton.State, 16)
		for i := range start {
			start[i] = mask>>i&1 == 1
		}
		for _, sys := range []automaton.System{h, mutatedHPP{HPP: h, from: headOnToNorthEast[0], to: headOnToNorthEast[1]}} {
			states, err := automaton.Simulate(sys, start, 1, nil)
			if err != nil {
				t.Fatal(err)
			}
			for name, q := range quantities {
				if value(q, states[0]) == value(q, states[1]) {
					continue
				}
				if _, mutated := sys.(mutatedHPP); !mutated {
					t.Fatalf("HPP changes %s from state %016b", name, mask)
				}
				broken[name] = true
			}
		}
	}
	for _, name := range []string{"momentum-x", "momentum-y"} {
		if !broken[name] {
			t.Errorf("mutation did not change %s", name)
		}
	}
	if broken["mass"] {
		t.Error("mass-preserving mutation changed mass")
	}
}

func TestQuantityShapes(t *testing.T) {
	h := HPP{Width: 3, Height: 2}
	for name, want := range map[string]int{"mass": 24, "momentum-x": 12, "momentum-y": 12} {
		if got := len(hppQuantities(h)[name]); got != want {
			t.Errorf("%s has %d terms, want %d", name, got, want)
		}
	}
	if len(h.RowMomentumX(1)) != 6 || len(h.ColumnMomentumY(2)) != 4 {
		t.Error("row or column quantity has the wrong size")
	}
	for name, q := range map[string]automaton.Quantity{
		"row -1": h.RowMomentumX(-1), "row 2": h.RowMomentumX(2),
		"column -1": h.ColumnMomentumY(-1), "column 3": h.ColumnMomentumY(3),
	} {
		if _, err := automaton.Compile(context.Background(), h, automaton.Query{Steps: 1, Violates: q}, automaton.Options{}); !errors.Is(err, automaton.ErrInput) {
			t.Errorf("%s: got %v, want automaton.ErrInput", name, err)
		}
	}
}

func TestDensityAndVelocity(t *testing.T) {
	h := HPP{Width: 4, Height: 2}
	s := make(automaton.State, h.Cells()*4)
	s[0*4+East] = true  // (0,0) east
	s[1*4+East] = true  // (1,0) east
	s[1*4+North] = true // (1,0) north
	s[6*4+West] = true  // (2,1) west
	density, err := h.Density([]automaton.State{s, s}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(density[0], []float64{3.0 / 4, 1.0 / 4}) {
		t.Fatalf("density %v", density)
	}
	ux, uy, err := h.Velocity([]automaton.State{s}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ux[0], []float64{2.0 / 3, -1}) || !slices.Equal(uy[0], []float64{1.0 / 3, 0}) {
		t.Fatalf("velocity %v %v", ux, uy)
	}
	empty, _, err := h.Velocity([]automaton.State{make(automaton.State, 32)}, 1)
	if err != nil || empty[1][3] != 0 {
		t.Fatalf("empty block: %v %v", empty, err)
	}
	for name, call := range map[string]func() error{
		"zero block":    func() error { _, err := h.Density([]automaton.State{s}, 0); return err },
		"not a divisor": func() error { _, err := h.Density([]automaton.State{s}, 3); return err },
		"no states":     func() error { _, err := h.Density(nil, 1); return err },
		"short state":   func() error { _, _, err := h.Velocity([]automaton.State{s[:8]}, 1); return err },
	} {
		if err := call(); !errors.Is(err, ErrInput) {
			t.Errorf("%s: got %v, want ErrInput", name, err)
		}
	}
}

// TestDefaultBudgetSteps measures the design estimate: a 32×32 HPP run fits 12
// steps in the default literal budget and not 13.
func TestDefaultBudgetSteps(t *testing.T) {
	if testing.Short() {
		t.Skip("allocates a full-size encoding")
	}
	h := HPP{Width: 32, Height: 32}
	if _, err := automaton.Compile(context.Background(), h, automaton.Query{Steps: 12}, automaton.Options{}); err != nil {
		t.Fatal(err)
	}
	if _, err := automaton.Compile(context.Background(), h, automaton.Query{Steps: 13}, automaton.Options{}); !errors.Is(err, sat.ErrLimit) {
		t.Fatalf("got %v, want limit", err)
	}
}
