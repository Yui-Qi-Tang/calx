package mus_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/Yui-Qi-Tang/calx/internal/acceptance"
	"github.com/Yui-Qi-Tang/calx/mus"
	"github.com/Yui-Qi-Tang/calx/sat"
)

// The tests in this file pin the claims of docs/mus-primer.md without a SAT
// solver. bruteForce answers from the independent enumerator.

// houseFormula is the formula of the primer.
func houseFormula() sat.CNF {
	c := acceptance.CNF(4, [][]int{{3}, {1}, {-1, 2}, {-2}, {4}})
	c.Origins = [][]string{{"clock-set"}, {"power-on"}, {"power-implies-light"}, {"light-off"}, {"tv-on"}}
	return c
}

func TestMUSPrimerHouse(t *testing.T) {
	o, err := mus.ByOrigin(houseFormula(), bruteForce)
	if err != nil {
		t.Fatal(err)
	}
	if got := o.Names(); !slices.Equal(got, []string{"clock-set", "light-off", "power-implies-light", "power-on", "tv-on"}) {
		t.Fatal(got)
	}
	core, err := mus.Shrink(context.Background(), o, strict)
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		removed int
		status  sat.Status
	}{{-1, sat.UNSAT}, {0, sat.UNSAT}, {1, sat.SAT}, {2, sat.SAT}, {3, sat.SAT}, {4, sat.UNSAT}}
	if len(core.Steps) != len(want) {
		t.Fatalf("%d steps", len(core.Steps))
	}
	for i, w := range want {
		if s := core.Steps[i]; s.Removed != w.removed || s.Result.Status != w.status {
			t.Fatalf("check %d: %+v", i, s)
		}
	}
	if !core.Minimal || !slices.Equal(core.Groups, []int{1, 2, 3}) {
		t.Fatalf("%+v", core)
	}
}

func TestMUSPrimerBudget(t *testing.T) {
	o, err := mus.ByOrigin(houseFormula(), bruteForce)
	if err != nil {
		t.Fatal(err)
	}
	core, err := mus.Shrink(context.Background(), o, strict, mus.WithMaxChecks(3))
	if !errors.Is(err, mus.ErrBudget) || core.Minimal || !slices.Equal(core.Groups, []int{1, 2, 3, 4}) {
		t.Fatalf("%+v %v", core, err)
	}
}

func TestMUSPrimerGroupOrderChoosesTheConflict(t *testing.T) {
	clauses := [][]int{{1}, {-1}, {2}, {-2}}
	find := func(origins [][]string) sat.CNF {
		c := acceptance.CNF(2, clauses)
		c.Origins = origins
		var last sat.CNF
		o, err := mus.ByOrigin(c, func(ctx context.Context, step int, trial sat.CNF) (sat.Result, error) {
			r, err := bruteForce(ctx, step, trial)
			if r.Status == sat.UNSAT {
				last = trial
			}
			return r, err
		})
		if err != nil {
			t.Fatal(err)
		}
		core, err := mus.Shrink(context.Background(), o, strict)
		if err != nil || !core.Minimal || !slices.Equal(core.Groups, []int{2, 3}) {
			t.Fatalf("%+v %v", core, err)
		}
		return last
	}
	first := find([][]string{{"a"}, {"b"}, {"c"}, {"d"}})
	if !slices.EqualFunc(first.Clauses, [][]int{{2}, {-2}}, slices.Equal[[]int]) {
		t.Fatal(first.Clauses)
	}
	second := find([][]string{{"c"}, {"d"}, {"a"}, {"b"}})
	if !slices.EqualFunc(second.Clauses, [][]int{{1}, {-1}}, slices.Equal[[]int]) {
		t.Fatal(second.Clauses)
	}
}

// TestMUSPrimerStopTable pins the early-stop rows that a budget does not cover.
func TestMUSPrimerStopTable(t *testing.T) {
	satisfiable := acceptance.CNF(1, [][]int{{1}})
	satisfiable.Origins = [][]string{{"only"}}
	o, err := mus.ByOrigin(satisfiable, bruteForce)
	if err != nil {
		t.Fatal(err)
	}
	core, err := mus.Shrink(context.Background(), o, strict)
	if !errors.Is(err, mus.ErrSatisfiable) || core.Groups != nil || core.Witness.Status == sat.UNSAT {
		t.Fatalf("%+v %v", core, err)
	}
	f := &fakeOracle{groups: 3, answer: func(step int, keep []bool) (sat.Result, error) {
		if step == 2 {
			return sat.Result{Status: sat.Unknown}, nil
		}
		return checked(sat.UNSAT, "conflict"), nil
	}}
	core, err = mus.Shrink(context.Background(), f, strict)
	if !errors.Is(err, sat.ErrInconclusive) || core.Minimal || core.Witness.Status != sat.UNSAT || !slices.Equal(core.Groups, []int{1, 2}) {
		t.Fatalf("%+v %v", core, err)
	}
}
