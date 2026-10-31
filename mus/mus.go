// Package mus shrinks an unsatisfiable set of constraint groups to an
// irreducible subset by deleting one group at a time. The caller implements
// Oracle, as it would sort.Interface, so Calx sees group indices and each
// check's verification record, never what a group means or which backend
// decided it. A result is one conflicting subset: not every conflict, not a
// smallest one, and not a claim about which group is wrong.
package mus

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/Yui-Qi-Tang/calx/sat"
)

var (
	// ErrInput marks a malformed oracle, formula or option, not a satisfiability
	// answer.
	ErrInput = errors.New("invalid mus input")
	// ErrSatisfiable means the complete set of groups is satisfiable, so it has
	// no conflicting subset.
	ErrSatisfiable = errors.New("constraint groups are satisfiable")
	// ErrBudget means the check budget ended before every group was tried.
	ErrBudget = errors.New("mus check budget exhausted")
)

// Oracle decides subsets of a fixed list of constraint groups. Removing a group
// must never turn a satisfiable subset unsatisfiable: Minimal relies on this.
// Methods are never called concurrently, and panics in them are not recovered.
type Oracle interface {
	// Groups returns the number of removable groups, at least 0. Constraints
	// that belong to no group are always part of every check.
	Groups() int
	// Check decides the groups with keep[i] true. step numbers the checks of one
	// Shrink call from 0, which suits building a unique query ID. keep is valid
	// only during the call and must not be modified. Check must return the
	// backend's answer and verification record, also with an error when it has
	// them.
	Check(ctx context.Context, step int, keep []bool) (sat.Result, error)
}

// Step records one check. Removed is the group left out of it, or -1 for the
// first check, which keeps every group.
type Step struct {
	Removed int
	Result  sat.Result
}

// Core is one conflicting subset. When Witness.Status is sat.UNSAT, Witness is
// the check of exactly Groups, including on an error return. Otherwise Shrink
// failed before any conflict was established, and Groups is nil.
type Core struct {
	Groups  []int // Remaining group indices in ascending order.
	Minimal bool  // Every group in Groups had a satisfiable deletion.
	Witness sat.Result
	Steps   []Step // Every check made, in order.
}

// Option changes the behavior of Shrink and reports an invalid value.
type Option func(*settings) error

type settings struct {
	maxChecks int
}

// WithMaxChecks limits the number of checks, including the first. n must be at
// least 1. Without it Shrink makes at most Groups()+1 checks.
func WithMaxChecks(n int) Option {
	return func(s *settings) error {
		if n < 1 {
			return fmt.Errorf("%w: max checks %d, want at least 1", ErrInput, n)
		}
		s.maxChecks = n
		return nil
	}
}

// Shrink checks every group together, then tries deleting each group in index
// order. A group stays only if deleting it made the rest satisfiable, so the
// index order decides which conflict is found. It stops at the first check that
// fails or does not meet want: UNKNOWN stops it, and is never read as SAT. Pass
// the zero sat.Requirements to accept a backend's unchecked answers.
//
// On error it returns the checks made so far; see Core for what remains valid.
// A complete set that is satisfiable returns ErrSatisfiable.
func Shrink(ctx context.Context, o Oracle, want sat.Requirements, opts ...Option) (Core, error) {
	if err := ctx.Err(); err != nil {
		return Core{}, err
	}
	if o == nil {
		return Core{}, fmt.Errorf("%w: nil oracle", ErrInput)
	}
	groups := o.Groups()
	if groups < 0 {
		return Core{}, fmt.Errorf("%w: group count %d", ErrInput, groups)
	}
	s := settings{maxChecks: groups + 1}
	for _, opt := range opts {
		if opt == nil {
			return Core{}, fmt.Errorf("%w: nil option", ErrInput)
		}
		if err := opt(&s); err != nil {
			return Core{}, err
		}
	}

	var core Core
	keep := make([]bool, groups)
	remaining := make([]int, groups)
	for i := range groups {
		keep[i] = true
		remaining[i] = i
	}
	result, err := check(ctx, o, &core, -1, keep, want)
	if err != nil {
		return core, err
	}
	if result.Status != sat.UNSAT {
		return core, ErrSatisfiable
	}
	core.Groups, core.Witness = remaining, result

	for i := range groups {
		if err := ctx.Err(); err != nil {
			return core, err
		}
		if len(core.Steps) >= s.maxChecks {
			return core, fmt.Errorf("%w: %d checks", ErrBudget, len(core.Steps))
		}
		keep[i] = false
		result, err := check(ctx, o, &core, i, keep, want)
		if err != nil {
			return core, err
		}
		if result.Status == sat.UNSAT {
			core.Groups = slices.DeleteFunc(core.Groups, func(g int) bool { return g == i })
			core.Witness = result
			continue
		}
		keep[i] = true
	}
	core.Minimal = true
	return core, nil
}

// check records one Oracle answer. A nil error leaves only SAT or UNSAT.
func check(ctx context.Context, o Oracle, core *Core, removed int, keep []bool, want sat.Requirements) (sat.Result, error) {
	result, err := o.Check(ctx, len(core.Steps), keep)
	core.Steps = append(core.Steps, Step{Removed: removed, Result: result})
	if err != nil {
		return result, err
	}
	return result, result.Require(want)
}
