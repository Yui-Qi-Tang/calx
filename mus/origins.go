package mus

import (
	"context"
	"fmt"
	"slices"

	"github.com/Yui-Qi-Tang/calx/sat"
)

// Solve decides one trial formula. step is the Oracle check number.
type Solve func(ctx context.Context, step int, cnf sat.CNF) (sat.Result, error)

// Origins is an Oracle that groups a CNF's clauses by their Origins strings.
// A clause with no origins is in every check. A clause with several origins
// stays while any of them is kept, because each is taken to assert the clause
// on its own. Clauses that depend on several conditions jointly need their own
// Oracle.
type Origins struct {
	cnf   sat.CNF
	names []string
	index map[string]int
	solve Solve
}

// ByOrigin returns an Oracle over cnf. Groups are the distinct origin strings in
// sorted order. cnf must have Origins, and must not be mutated while in use.
func ByOrigin(cnf sat.CNF, solve Solve) (*Origins, error) {
	if solve == nil {
		return nil, fmt.Errorf("%w: nil solve", ErrInput)
	}
	if cnf.Origins == nil {
		return nil, fmt.Errorf("%w: origins required", ErrInput)
	}
	if err := cnf.CheckShape(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInput, err)
	}
	var names []string
	for _, origins := range cnf.Origins {
		names = append(names, origins...)
	}
	slices.Sort(names)
	names = slices.Compact(names)
	index := make(map[string]int, len(names))
	for i, name := range names {
		index[name] = i
	}
	return &Origins{cnf: cnf, names: names, index: index, solve: solve}, nil
}

// Groups returns the number of distinct origins.
func (o *Origins) Groups() int { return len(o.names) }

// Names returns a copy of the origin of each group index.
func (o *Origins) Names() []string { return slices.Clone(o.names) }

// Check solves the clauses that keep retains. Variables are unchanged, so a
// trial's assignment is total over the original formula's variables.
func (o *Origins) Check(ctx context.Context, step int, keep []bool) (sat.Result, error) {
	if len(keep) != len(o.names) {
		return sat.Result{}, fmt.Errorf("%w: keep has %d groups, want %d", ErrInput, len(keep), len(o.names))
	}
	trial := sat.CNF{Variables: o.cnf.Variables}
	for i, clause := range o.cnf.Clauses {
		origins := o.cnf.Origins[i]
		kept := func(name string) bool { return keep[o.index[name]] }
		if len(origins) == 0 || slices.ContainsFunc(origins, kept) {
			trial.Clauses = append(trial.Clauses, clause)
			trial.Origins = append(trial.Origins, origins)
		}
	}
	return o.solve(ctx, step, trial)
}
