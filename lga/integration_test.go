//go:build resolverintegration

package lga

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Yui-Qi-Tang/calx/automaton"
	"github.com/Yui-Qi-Tang/calx/digest"
	"github.com/Yui-Qi-Tang/calx/sat"
	"github.com/Yui-Qi-Tang/calx/sat/cadical"
)

func realRunner(t *testing.T, name string) *cadical.Runner {
	t.Helper()
	for _, name := range []string{"CALX_CADICAL", "CALX_CADICAL_DIGEST", "CALX_DRAT", "CALX_DRAT_DIGEST", "CALX_ARTIFACTS"} {
		if os.Getenv(name) == "" {
			t.Fatalf("%s is required for explicit acceptance", name)
		}
	}
	r, err := cadical.New(cadical.Config{
		SolverPath: os.Getenv("CALX_CADICAL"), SolverDigest: digest.Digest(os.Getenv("CALX_CADICAL_DIGEST")),
		CheckerPath: os.Getenv("CALX_DRAT"), CheckerDigest: digest.Digest(os.Getenv("CALX_DRAT_DIGEST")),
		ArtifactDir: filepath.Join(os.Getenv("CALX_ARTIFACTS"), name),
	})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// solve compiles, runs the checked solver and requires the guarantee for the
// returned answer. A SAT trajectory is decoded and replayed through sys.
func solve(t *testing.T, r *cadical.Runner, id string, sys automaton.System, q automaton.Query) (sat.Status, automaton.Trajectory) {
	t.Helper()
	e, err := automaton.Compile(context.Background(), sys, q)
	if err != nil {
		t.Fatal(err)
	}
	cnf, err := e.CNF()
	if err != nil {
		t.Fatal(err)
	}
	result, err := r.Solve(context.Background(), id, cnf)
	if err != nil {
		t.Fatal(err)
	}
	if err := result.Require(sat.Requirements{SATModelChecked: true, UNSATProofChecked: true}); err != nil {
		t.Fatal(err)
	}
	if result.Status != sat.SAT {
		return result.Status, automaton.Trajectory{}
	}
	run, err := e.Decode(result.Assignment)
	if err != nil {
		t.Fatal(err)
	}
	if err := automaton.Replay(sys, run); err != nil {
		t.Fatal(err)
	}
	return result.Status, run
}

func TestRealToolsHPP(t *testing.T) {
	r := realRunner(t, "lga-hpp")
	h := HPP{Width: 4, Height: 4}
	// Whole-lattice sums are proved on 2×2: CDCL proofs of counting arguments
	// stop finishing at about 32 terms (see docs/contracts.md), while
	// counterexample search and row or column sums stay fast on 4×4.
	small := HPP{Width: 2, Height: 2}
	const steps = 3

	// Conservation: no run of the declared HPP changes these quantities.
	for _, c := range []struct {
		name string
		sys  HPP
		q    automaton.Quantity
	}{
		{"mass-2x2", small, small.Mass()},
		{"momentum-x-2x2", small, small.MomentumX()},
		{"momentum-y-2x2", small, small.MomentumY()},
		{"row-x1-4x4", h, h.RowMomentumX(1)},
		{"column-y2-4x4", h, h.ColumnMomentumY(2)},
	} {
		t.Run("conserved-"+c.name, func(t *testing.T) {
			if status, _ := solve(t, r, "conserved-"+c.name, c.sys, automaton.Query{Steps: steps, Violates: c.q}); status != sat.UNSAT {
				t.Fatalf("status %s, want UNSAT", status)
			}
		})
	}

	// Mutations: each broken rule must yield a replayable counterexample that
	// really changes the quantity (on 4×4), while unaffected quantities stay
	// proven (on 2×2).
	mutations := []struct {
		name     string
		sys      mutatedHPP
		keeper   mutatedHPP
		breaks   map[string]automaton.Quantity
		conserve map[string]automaton.Quantity
	}{
		{
			name:     "head-on-to-north",
			sys:      mutatedHPP{HPP: h, from: headOnToNorth[0], to: headOnToNorth[1]},
			keeper:   mutatedHPP{HPP: small, from: headOnToNorth[0], to: headOnToNorth[1]},
			breaks:   map[string]automaton.Quantity{"mass": h.Mass(), "momentum-y": h.MomentumY()},
			conserve: map[string]automaton.Quantity{"momentum-x": small.MomentumX()},
		},
		{
			name:     "head-on-to-north-east",
			sys:      mutatedHPP{HPP: h, from: headOnToNorthEast[0], to: headOnToNorthEast[1]},
			keeper:   mutatedHPP{HPP: small, from: headOnToNorthEast[0], to: headOnToNorthEast[1]},
			breaks:   map[string]automaton.Quantity{"momentum-x": h.MomentumX(), "row-x1": h.RowMomentumX(1)},
			conserve: map[string]automaton.Quantity{"mass": small.Mass()},
		},
	}
	for _, m := range mutations {
		for name, q := range m.breaks {
			t.Run(m.name+"-breaks-"+name, func(t *testing.T) {
				status, run := solve(t, r, m.name+"-breaks-"+name, m.sys, automaton.Query{Steps: steps, Violates: q})
				if status != sat.SAT {
					t.Fatalf("status %s, want SAT", status)
				}
				if value(q, run.States[0], 4) == value(q, run.States[steps], 4) {
					t.Fatal("counterexample does not change the quantity")
				}
			})
		}
		for name, q := range m.conserve {
			t.Run(m.name+"-keeps-"+name, func(t *testing.T) {
				if status, _ := solve(t, r, m.name+"-keeps-"+name, m.keeper, automaton.Query{Steps: steps, Violates: q}); status != sat.UNSAT {
					t.Fatalf("status %s, want UNSAT", status)
				}
			})
		}
	}

	// Reachability: a head-on east-west pair becomes a north-south pair after
	// one step, and never stays east-west.
	start := automaton.Pattern{}
	for c := range h.Cells() {
		for d := range 4 {
			start[automaton.Bit{Cell: c, Index: d}] = false
		}
	}
	// Particles at (0,1) moving east and (2,1) moving west meet at (1,1).
	start[automaton.Bit{Cell: 4, Index: East}] = true
	start[automaton.Bit{Cell: 6, Index: West}] = true
	collided := automaton.Pattern{{Cell: 5, Index: North}: true, {Cell: 5, Index: South}: true}
	passed := automaton.Pattern{{Cell: 5, Index: East}: true, {Cell: 5, Index: West}: true}
	t.Run("head-on-collides", func(t *testing.T) {
		if status, _ := solve(t, r, "head-on-collides", h, automaton.Query{Steps: 1, Start: start, Goal: collided}); status != sat.SAT {
			t.Fatalf("status %s, want SAT", status)
		}
	})
	t.Run("head-on-cannot-pass", func(t *testing.T) {
		if status, _ := solve(t, r, "head-on-cannot-pass", h, automaton.Query{Steps: 1, Start: start, Goal: passed}); status != sat.UNSAT {
			t.Fatalf("status %s, want UNSAT", status)
		}
	})
}

// leakyFHP breaks FHP on purpose: a counterclockwise head-on pair loses one
// particle.
type leakyFHP struct{ FHP }

func (l leakyFHP) Next(cell int, in, out []bool) {
	l.FHP.Next(cell, in, out)
	if !l.FHP.geometry().wall(cell) && [6]bool(in[:6]) == pairFHP(0) && in[6] {
		out[HexSW] = false
	}
}

func TestRealToolsFHPAndWalls(t *testing.T) {
	r := realRunner(t, "lga-fhp")

	small := FHP{Width: 2, Height: 1}
	for name, q := range map[string]automaton.Quantity{"mass": small.Mass(), "momentum-q": small.MomentumQ(), "momentum-r": small.MomentumR()} {
		t.Run("fhp-conserved-"+name, func(t *testing.T) {
			if status, _ := solve(t, r, "fhp-conserved-"+name, small, automaton.Query{Steps: 2, Violates: q}); status != sat.UNSAT {
				t.Fatalf("status %s, want UNSAT", status)
			}
		})
	}

	f := FHP{Width: 3, Height: 3}
	t.Run("fhp-leaky-mass", func(t *testing.T) {
		status, run := solve(t, r, "fhp-leaky-mass", leakyFHP{f}, automaton.Query{Steps: 2, Violates: f.Mass()})
		if status != sat.SAT || value(f.Mass(), run.States[0], 6) == value(f.Mass(), run.States[2], 6) {
			t.Fatalf("status %s, want a counterexample that loses mass", status)
		}
	})

	// A head-on east-west pair meeting at the center rotates either way, chosen
	// by the free choice bit, and never passes straight through.
	start := automaton.Pattern{}
	for c := range f.Cells() {
		for d := range 6 {
			start[automaton.Bit{Cell: c, Index: d}] = false
		}
	}
	start[automaton.Bit{Cell: 3, Index: HexE}] = true // (0,1) moving east
	start[automaton.Bit{Cell: 5, Index: HexW}] = true // (2,1) moving west
	for _, c := range []struct {
		name string
		goal automaton.Pattern
		want sat.Status
	}{
		{"fhp-rotates-counterclockwise", automaton.Pattern{{Cell: 4, Index: HexNE}: true, {Cell: 4, Index: HexSW}: true}, sat.SAT},
		{"fhp-rotates-clockwise", automaton.Pattern{{Cell: 4, Index: HexNW}: true, {Cell: 4, Index: HexSE}: true}, sat.SAT},
		{"fhp-cannot-pass", automaton.Pattern{{Cell: 4, Index: HexE}: true, {Cell: 4, Index: HexW}: true}, sat.UNSAT},
	} {
		t.Run(c.name, func(t *testing.T) {
			if status, _ := solve(t, r, c.name, f, automaton.Query{Steps: 1, Start: start, Goal: c.goal}); status != c.want {
				t.Fatalf("status %s, want %s", status, c.want)
			}
		})
	}

	// Walls conserve mass but absorb momentum.
	walled := HPP{Width: 2, Height: 2, Walls: []bool{false, false, false, true}}
	t.Run("hpp-walls-conserve-mass", func(t *testing.T) {
		if status, _ := solve(t, r, "hpp-walls-conserve-mass", walled, automaton.Query{Steps: 3, Violates: walled.Mass()}); status != sat.UNSAT {
			t.Fatalf("status %s, want UNSAT", status)
		}
	})
	t.Run("hpp-walls-change-momentum", func(t *testing.T) {
		status, run := solve(t, r, "hpp-walls-change-momentum", walled, automaton.Query{Steps: 3, Violates: walled.MomentumX()})
		if status != sat.SAT || value(walled.MomentumX(), run.States[0], 4) == value(walled.MomentumX(), run.States[3], 4) {
			t.Fatalf("status %s, want a counterexample that changes momentum", status)
		}
	})
}
