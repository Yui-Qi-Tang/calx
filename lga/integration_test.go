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

func realRunner(t *testing.T) *cadical.Runner {
	t.Helper()
	for _, name := range []string{"CALX_CADICAL", "CALX_CADICAL_DIGEST", "CALX_DRAT", "CALX_DRAT_DIGEST", "CALX_ARTIFACTS"} {
		if os.Getenv(name) == "" {
			t.Fatalf("%s is required for explicit acceptance", name)
		}
	}
	r, err := cadical.New(cadical.Config{
		SolverPath: os.Getenv("CALX_CADICAL"), SolverDigest: digest.Digest(os.Getenv("CALX_CADICAL_DIGEST")),
		CheckerPath: os.Getenv("CALX_DRAT"), CheckerDigest: digest.Digest(os.Getenv("CALX_DRAT_DIGEST")),
		ArtifactDir: filepath.Join(os.Getenv("CALX_ARTIFACTS"), "lga"),
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
	e, err := automaton.Compile(context.Background(), sys, q, automaton.Options{})
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
	r := realRunner(t)
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
				if value(q, run.States[0]) == value(q, run.States[steps]) {
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
