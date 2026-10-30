// Command lga demonstrates an HPP lattice gas: forward simulation with flow
// fields, and with -check a proved conservation law and a counterexample for a
// deliberately broken collision rule.
package main

import (
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/Yui-Qi-Tang/calx/automaton"
	"github.com/Yui-Qi-Tang/calx/digest"
	"github.com/Yui-Qi-Tang/calx/lga"
	"github.com/Yui-Qi-Tang/calx/sat"
	"github.com/Yui-Qi-Tang/calx/sat/cadical"
)

func main() {
	check := flag.Bool("check", false, "run checked SAT queries; needs the CALX_* tool variables")
	artifacts := flag.String("artifacts", "artifacts/go-lga", "new run directory for -check; parent must exist and the run directory must not exist")
	flag.Parse()
	var err error
	if *check {
		err = runCheck(context.Background(), *artifacts)
	} else {
		err = simulate()
	}
	if err != nil {
		log.Fatal(err)
	}
}

func value(q automaton.Quantity, s automaton.State) int {
	v := 0
	for _, t := range q {
		if s[t.Bit.Cell*4+t.Bit.Index] {
			v += t.Weight
		}
	}
	return v
}

// simulate runs 8 steps of a 16×8 lattice: an east-moving band in columns 0–3
// and a west-moving band in column 8, then reports conserved quantities and
// 4×4 block averages of density and x velocity.
func simulate() error {
	h := lga.HPP{Width: 16, Height: 8}
	start := make(automaton.State, h.Cells()*4)
	for y := range h.Height {
		for x := range 4 {
			start[(y*h.Width+x)*4+lga.East] = true
		}
		start[(y*h.Width+8)*4+lga.West] = true
	}
	states, err := automaton.Simulate(h, start, 8, nil)
	if err != nil {
		return err
	}
	last := states[len(states)-1]
	density, err := h.Density(states[len(states)-1:], 4)
	if err != nil {
		return err
	}
	ux, _, err := h.Velocity(states[len(states)-1:], 4)
	if err != nil {
		return err
	}
	return json.MarshalEncode(jsontext.NewEncoder(os.Stdout), struct {
		Mass      [2]int      `json:"mass"`
		MomentumX [2]int      `json:"momentum_x"`
		Density   [][]float64 `json:"density"`
		VelocityX [][]float64 `json:"velocity_x"`
	}{
		Mass:      [2]int{value(h.Mass(), start), value(h.Mass(), last)},
		MomentumX: [2]int{value(h.MomentumX(), start), value(h.MomentumX(), last)},
		Density:   density,
		VelocityX: ux,
	})
}

// leaky breaks HPP on purpose: an east-west head-on pair becomes a single
// north-moving particle, so mass is no longer conserved.
type leaky struct{ lga.HPP }

func (l leaky) Next(cell int, in, out []bool) {
	if in[lga.East] && in[lga.West] && !in[lga.North] && !in[lga.South] {
		out[lga.North] = true
		return
	}
	l.HPP.Next(cell, in, out)
}

func runCheck(ctx context.Context, artifacts string) error {
	runner, err := cadical.New(cadical.Config{
		SolverPath:    os.Getenv("CALX_CADICAL"),
		SolverDigest:  digest.Digest(os.Getenv("CALX_CADICAL_DIGEST")),
		CheckerPath:   os.Getenv("CALX_DRAT"),
		CheckerDigest: digest.Digest(os.Getenv("CALX_DRAT_DIGEST")),
		ArtifactDir:   artifacts,
	})
	if err != nil {
		return err
	}

	// 1. HPP conserves mass: no 3-step run of a 2×2 lattice changes it.
	small := lga.HPP{Width: 2, Height: 2}
	conserved, _, err := solve(ctx, runner, "conserved-mass", small, automaton.Query{Steps: 3, Violates: small.Mass()})
	if err != nil {
		return err
	}

	// 2. The broken rule loses mass, and the solver shows how.
	h := lga.HPP{Width: 4, Height: 4}
	broken, run, err := solve(ctx, runner, "leaky-mass", leaky{h}, automaton.Query{Steps: 3, Violates: h.Mass()})
	if err != nil {
		return err
	}
	out := struct {
		ConservedMass  sat.Status `json:"conserved_mass_2x2"`
		LeakyMass      sat.Status `json:"leaky_mass_4x4"`
		Counterexample []int      `json:"counterexample_mass,omitempty"`
	}{ConservedMass: conserved, LeakyMass: broken}
	if broken == sat.SAT {
		out.Counterexample = []int{value(h.Mass(), run.States[0]), value(h.Mass(), run.States[len(run.States)-1])}
	}
	return json.MarshalEncode(jsontext.NewEncoder(os.Stdout), out)
}

// solve compiles a query, runs the checked solver, requires the evidence for
// the answer, and decodes and replays a SAT trajectory.
func solve(ctx context.Context, r *cadical.Runner, id string, sys automaton.System, q automaton.Query) (sat.Status, automaton.Trajectory, error) {
	encoding, err := automaton.Compile(ctx, sys, q)
	if err != nil {
		return "", automaton.Trajectory{}, err
	}
	formula, err := encoding.CNF()
	if err != nil {
		return "", automaton.Trajectory{}, err
	}
	result, err := r.Solve(ctx, id, formula)
	if err != nil {
		return "", automaton.Trajectory{}, err
	}
	if err := result.Require(sat.Requirements{SATModelChecked: true, UNSATProofChecked: true}); err != nil {
		return "", automaton.Trajectory{}, err
	}
	if result.Status != sat.SAT {
		return result.Status, automaton.Trajectory{}, nil
	}
	run, err := encoding.Decode(result.Assignment)
	if err != nil {
		return "", automaton.Trajectory{}, err
	}
	if err := automaton.Replay(sys, run); err != nil {
		return "", automaton.Trajectory{}, fmt.Errorf("replaying %s: %w", id, err)
	}
	return result.Status, run, nil
}
