// Command automaton implements automaton.System for elementary rule 90 on a ring
// of four cells: forward simulation, and with -check a checked question about
// which states have a predecessor.
package main

import (
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"flag"
	"log"
	"os"

	"github.com/Yui-Qi-Tang/calx/automaton"
	"github.com/Yui-Qi-Tang/calx/digest"
	"github.com/Yui-Qi-Tang/calx/sat"
	"github.com/Yui-Qi-Tang/calx/sat/cadical"
)

// rule90 is the caller's system: each cell becomes its left neighbor XOR its
// right neighbor. Calx sees only cell and bit indices and this local rule.
type rule90 struct{ cells int }

func (r rule90) Cells() int { return r.cells }

func (r rule90) Bits() int { return 1 }

func (r rule90) Inputs(cell int) []automaton.Bit {
	return []automaton.Bit{{Cell: (cell + r.cells - 1) % r.cells}, {Cell: (cell + 1) % r.cells}}
}

func (r rule90) Next(_ int, in, out []bool) { out[0] = in[0] != in[1] }

func main() {
	check := flag.Bool("check", false, "run checked SAT queries; needs the CALX_* tool variables")
	artifacts := flag.String("artifacts", "artifacts/go-automaton", "new run directory for -check; parent must exist and the run directory must not exist")
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

// picture draws a state as a row of cells: ● is true, ○ is false.
func picture(s automaton.State) string {
	out := ""
	for _, alive := range s {
		if alive {
			out += "●"
		} else {
			out += "○"
		}
	}
	return out
}

// simulate runs the rule forward with no solver.
func simulate() error {
	states, err := automaton.Simulate(rule90{cells: 4}, automaton.State{true, false, false, false}, 2, nil)
	if err != nil {
		return err
	}
	pictures := make([]string, len(states))
	for i, s := range states {
		pictures[i] = picture(s)
	}
	return json.MarshalEncode(jsontext.NewEncoder(os.Stdout), struct {
		States []string `json:"states"`
	}{States: pictures})
}

// runCheck asks whether two states have a predecessor. Simulation cannot answer
// this without trying every state; a solver answers it directly, and UNSAT
// comes with a checked proof.
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
	sys := rule90{cells: 4}

	// ●○○○ has no predecessor: it is a Garden of Eden state.
	eden, _, err := predecessor(ctx, runner, "eden", sys, automaton.State{true, false, false, false})
	if err != nil {
		return err
	}

	// ○●○● has one, and the solver names one that Replay confirms.
	reachable, before, err := predecessor(ctx, runner, "reachable", sys, automaton.State{false, true, false, true})
	if err != nil {
		return err
	}
	return json.MarshalEncode(jsontext.NewEncoder(os.Stdout), struct {
		Eden      sat.Status `json:"predecessor_of_eden"`
		Reachable sat.Status `json:"predecessor_of_reachable"`
		Before    string     `json:"one_predecessor"`
	}{Eden: eden, Reachable: reachable, Before: picture(before)})
}

// predecessor compiles "some state steps to goal", runs the checked solver, and
// decodes and replays a SAT trajectory.
func predecessor(ctx context.Context, r *cadical.Runner, id string, sys automaton.System, goal automaton.State) (sat.Status, automaton.State, error) {
	pattern := automaton.Pattern{}
	for cell, alive := range goal {
		pattern[automaton.Bit{Cell: cell}] = alive
	}
	encoding, err := automaton.Compile(ctx, sys, automaton.Query{Steps: 1, Goal: pattern})
	if err != nil {
		return "", nil, err
	}
	formula, err := encoding.CNF()
	if err != nil {
		return "", nil, err
	}
	result, err := r.Solve(ctx, id, formula)
	if err != nil {
		return "", nil, err
	}
	if err := result.Require(sat.Requirements{SATModelChecked: true, UNSATProofChecked: true}); err != nil {
		return "", nil, err
	}
	if result.Status != sat.SAT {
		return result.Status, nil, nil
	}
	run, err := encoding.Decode(result.Assignment)
	if err != nil {
		return "", nil, err
	}
	if err := automaton.Replay(sys, run); err != nil {
		return "", nil, err
	}
	return result.Status, run.States[0], nil
}
