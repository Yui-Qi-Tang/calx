package mus_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/Yui-Qi-Tang/calx/digest"
	"github.com/Yui-Qi-Tang/calx/internal/acceptance"
	"github.com/Yui-Qi-Tang/calx/mus"
	"github.com/Yui-Qi-Tang/calx/sat"
)

var strict = sat.Requirements{SATModelChecked: true, UNSATProofChecked: true}

// checked returns the record of a fully checked answer. tag becomes the digests,
// so a test can tell which check produced a result.
func checked(status sat.Status, tag string) sat.Result {
	id := digest.Sum([]byte(tag))
	r := sat.Result{Status: status, InputDigest: id, FormulaDigest: id}
	if status == sat.SAT {
		r.Model = sat.Validation{Present: true, Decoded: true, Checked: true, TargetDigest: id}
	} else {
		r.Proof = sat.Validation{Present: true, Checked: true, TargetDigest: id}
	}
	return r
}

// bruteForce solves with the independent enumerator and no external tool.
func bruteForce(_ context.Context, _ int, c sat.CNF) (sat.Result, error) {
	return checked(acceptance.Oracle(c), "brute force"), nil
}

// fakeOracle answers from a predicate over the kept groups.
type fakeOracle struct {
	groups int
	answer func(step int, keep []bool) (sat.Result, error)
	seen   [][]bool
}

func (f *fakeOracle) Groups() int { return f.groups }

func (f *fakeOracle) Check(_ context.Context, step int, keep []bool) (sat.Result, error) {
	f.seen = append(f.seen, slices.Clone(keep))
	return f.answer(step, keep)
}

// conflictOf is UNSAT exactly while all of the listed groups are kept.
func conflictOf(groups ...int) func(int, []bool) (sat.Result, error) {
	return func(_ int, keep []bool) (sat.Result, error) {
		tag := fmt.Sprint(keep)
		for _, g := range groups {
			if !keep[g] {
				return checked(sat.SAT, tag), nil
			}
		}
		return checked(sat.UNSAT, tag), nil
	}
}

func TestShrinkMatchesEnumeratedCores(t *testing.T) {
	// Every formula of the acceptance matrix, with one group per clause. The core
	// must be one of the irreducible subsets found by enumerating all subsets
	// with the independent oracle, which never calls this package.
	unsat := 0
	for name, c := range acceptance.Cases() {
		c.Origins = make([][]string, len(c.Clauses))
		for i := range c.Clauses {
			c.Origins[i] = []string{fmt.Sprintf("c%02d", i)}
		}
		o, err := mus.ByOrigin(c, bruteForce)
		if err != nil {
			t.Fatal(name, err)
		}
		core, err := mus.Shrink(context.Background(), o, strict)
		if acceptance.Oracle(c) == sat.SAT {
			if !errors.Is(err, mus.ErrSatisfiable) || core.Witness.Status == sat.UNSAT || core.Groups != nil {
				t.Fatalf("%s: %+v %v", name, core, err)
			}
			continue
		}
		unsat++
		if err != nil || !core.Minimal || len(core.Steps) != o.Groups()+1 {
			t.Fatalf("%s: %+v %v", name, core, err)
		}
		if !slices.ContainsFunc(enumerateCores(c), func(want []int) bool { return slices.Equal(want, core.Groups) }) {
			t.Fatalf("%s: %v is not an irreducible subset", name, core.Groups)
		}
	}
	if unsat == 0 {
		t.Fatal("no unsatisfiable formula tested")
	}
}

// enumerateCores returns every irreducible unsatisfiable subset of c's clauses.
func enumerateCores(c sat.CNF) [][]int {
	unsatisfiable := func(mask int) bool {
		s := sat.CNF{Variables: c.Variables}
		for i, clause := range c.Clauses {
			if mask&(1<<i) != 0 {
				s.Clauses = append(s.Clauses, clause)
			}
		}
		return acceptance.Oracle(s) == sat.UNSAT
	}
	var cores [][]int
	for mask := range 1 << len(c.Clauses) {
		if !unsatisfiable(mask) {
			continue
		}
		var core []int
		irreducible := true
		for i := range c.Clauses {
			if mask&(1<<i) != 0 {
				core = append(core, i)
				irreducible = irreducible && !unsatisfiable(mask&^(1<<i))
			}
		}
		if irreducible {
			cores = append(cores, core)
		}
	}
	return cores
}

func TestShrinkWitnessIsTheCheckOfGroups(t *testing.T) {
	f := &fakeOracle{groups: 5, answer: conflictOf(1, 3)}
	core, err := mus.Shrink(context.Background(), f, strict)
	if err != nil || !core.Minimal || !slices.Equal(core.Groups, []int{1, 3}) {
		t.Fatalf("%+v %v", core, err)
	}
	want := []bool{false, true, false, true, false}
	if core.Witness.FormulaDigest != digest.Sum([]byte(fmt.Sprint(want))) {
		t.Fatal("witness is not the check of the remaining groups")
	}
	if got := len(f.seen); got != 6 {
		t.Fatalf("%d checks, want 6", got)
	}
	for i, step := range core.Steps {
		if step.Removed != i-1 {
			t.Fatalf("step %d removed %d", i, step.Removed)
		}
	}
}

func TestShrinkPassesConsecutiveSteps(t *testing.T) {
	var steps []int
	conflict := conflictOf(0)
	f := &fakeOracle{groups: 2, answer: func(step int, keep []bool) (sat.Result, error) {
		steps = append(steps, step)
		return conflict(step, keep)
	}}
	if _, err := mus.Shrink(context.Background(), f, strict); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(steps, []int{0, 1, 2}) {
		t.Fatal(steps)
	}
}

func TestShrinkHardConstraintsAloneConflict(t *testing.T) {
	f := &fakeOracle{groups: 2, answer: conflictOf()}
	core, err := mus.Shrink(context.Background(), f, strict)
	if err != nil || !core.Minimal || len(core.Groups) != 0 || core.Witness.Status != sat.UNSAT {
		t.Fatalf("%+v %v", core, err)
	}
	f = &fakeOracle{groups: 0, answer: conflictOf()}
	core, err = mus.Shrink(context.Background(), f, strict)
	if err != nil || !core.Minimal || len(core.Steps) != 1 {
		t.Fatalf("%+v %v", core, err)
	}
}

func TestShrinkBudgetKeepsAValidCore(t *testing.T) {
	f := &fakeOracle{groups: 4, answer: conflictOf(2, 3)}
	core, err := mus.Shrink(context.Background(), f, strict, mus.WithMaxChecks(3))
	if !errors.Is(err, mus.ErrBudget) || core.Minimal || !slices.Equal(core.Groups, []int{2, 3}) {
		t.Fatalf("%+v %v", core, err)
	}
	if len(f.seen) != 3 || core.Witness.Status != sat.UNSAT {
		t.Fatalf("%d checks, %+v", len(f.seen), core)
	}
	// One check more covers the last group, whose deletion is what shows it stays.
	f = &fakeOracle{groups: 4, answer: conflictOf(2, 3)}
	core, err = mus.Shrink(context.Background(), f, strict, mus.WithMaxChecks(4))
	if !errors.Is(err, mus.ErrBudget) || core.Minimal {
		t.Fatalf("%+v %v", core, err)
	}
	f = &fakeOracle{groups: 4, answer: conflictOf(2, 3)}
	core, err = mus.Shrink(context.Background(), f, strict, mus.WithMaxChecks(5))
	if err != nil || !core.Minimal {
		t.Fatalf("%+v %v", core, err)
	}
}

func TestShrinkStopsWithoutGuessing(t *testing.T) {
	conflict := conflictOf(0, 1)
	failAt := func(at int, r sat.Result, err error) func(int, []bool) (sat.Result, error) {
		return func(step int, keep []bool) (sat.Result, error) {
			if step == at {
				return r, err
			}
			return conflict(step, keep)
		}
	}
	unknown := sat.Result{Status: sat.Unknown}
	unchecked := sat.Result{Status: sat.UNSAT}
	broken := errors.New("backend failed")
	for name, c := range map[string]struct {
		at   int
		r    sat.Result
		err  error
		want error
	}{
		"first check fails":      {0, sat.Result{}, broken, broken},
		"first check unknown":    {0, unknown, nil, sat.ErrInconclusive},
		"first check unchecked":  {0, unchecked, nil, sat.ErrGuarantee},
		"deletion fails":         {1, sat.Result{}, broken, broken},
		"deletion unknown":       {1, unknown, nil, sat.ErrInconclusive},
		"deletion unchecked":     {1, unchecked, nil, sat.ErrGuarantee},
		"deletion solve unknown": {1, unknown, sat.ErrInconclusive, sat.ErrInconclusive},
		"unset status":           {1, sat.Result{}, nil, sat.ErrGuarantee},
	} {
		t.Run(name, func(t *testing.T) {
			f := &fakeOracle{groups: 3, answer: failAt(c.at, c.r, c.err)}
			core, err := mus.Shrink(context.Background(), f, strict)
			if !errors.Is(err, c.want) || core.Minimal {
				t.Fatalf("%+v %v", core, err)
			}
			if len(core.Steps) != c.at+1 {
				t.Fatalf("%d steps", len(core.Steps))
			}
			if c.at == 0 {
				if core.Groups != nil || core.Witness.Status == sat.UNSAT {
					t.Fatalf("claimed a conflict: %+v", core)
				}
				return
			}
			if !slices.Equal(core.Groups, []int{0, 1, 2}) || core.Witness.Status != sat.UNSAT {
				t.Fatalf("lost the established conflict: %+v", core)
			}
		})
	}
}

func TestShrinkAcceptsUncheckedOnlyWhenAsked(t *testing.T) {
	answer := func(_ int, keep []bool) (sat.Result, error) {
		if keep[0] {
			return sat.Result{Status: sat.UNSAT}, nil
		}
		return sat.Result{Status: sat.SAT}, nil
	}
	f := &fakeOracle{groups: 2, answer: answer}
	if _, err := mus.Shrink(context.Background(), f, strict); !errors.Is(err, sat.ErrGuarantee) {
		t.Fatal(err)
	}
	f = &fakeOracle{groups: 2, answer: answer}
	core, err := mus.Shrink(context.Background(), f, sat.Requirements{})
	if err != nil || !core.Minimal || !slices.Equal(core.Groups, []int{0}) {
		t.Fatalf("%+v %v", core, err)
	}
}

func TestShrinkCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	f := &fakeOracle{groups: 2, answer: conflictOf(0)}
	if _, err := mus.Shrink(ctx, f, strict); !errors.Is(err, context.Canceled) || len(f.seen) != 0 {
		t.Fatalf("%d checks, %v", len(f.seen), err)
	}
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	conflict := conflictOf(0)
	f = &fakeOracle{groups: 3, answer: func(step int, keep []bool) (sat.Result, error) {
		if step == 1 {
			cancel()
		}
		return conflict(step, keep)
	}}
	core, err := mus.Shrink(ctx, f, strict)
	if !errors.Is(err, context.Canceled) || core.Minimal || len(f.seen) != 2 || core.Witness.Status != sat.UNSAT {
		t.Fatalf("%+v %v", core, err)
	}
}

func TestShrinkRejectsInvalidInput(t *testing.T) {
	ok := &fakeOracle{groups: 1, answer: conflictOf(0)}
	for name, c := range map[string]struct {
		o    mus.Oracle
		opts []mus.Option
	}{
		"nil oracle":     {nil, nil},
		"negative count": {&fakeOracle{groups: -1}, nil},
		"nil option":     {ok, []mus.Option{nil}},
		"zero budget":    {ok, []mus.Option{mus.WithMaxChecks(0)}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := mus.Shrink(context.Background(), c.o, strict, c.opts...); !errors.Is(err, mus.ErrInput) {
				t.Fatal(err)
			}
		})
	}
	if len(ok.seen) != 0 {
		t.Fatal("checked before validating options")
	}
}

func TestByOriginGroups(t *testing.T) {
	c := acceptance.CNF(2, [][]int{{1}, {-1}, {2}, {-2}})
	c.Origins = [][]string{{"b"}, {"a", "b"}, {"c"}, nil}
	o, err := mus.ByOrigin(c, bruteForce)
	if err != nil {
		t.Fatal(err)
	}
	names := o.Names()
	if !slices.Equal(names, []string{"a", "b", "c"}) {
		t.Fatal(names)
	}
	names[0] = "changed"
	if o.Names()[0] != "a" {
		t.Fatal("Names exposed internal state")
	}
	var trials [][][]int
	solve := func(_ context.Context, _ int, trial sat.CNF) (sat.Result, error) {
		if err := trial.CheckShape(); err != nil {
			return sat.Result{}, err
		}
		trials = append(trials, trial.Clauses)
		return checked(sat.SAT, "trial"), nil
	}
	o, err = mus.ByOrigin(c, solve)
	if err != nil {
		t.Fatal(err)
	}
	for _, keep := range [][]bool{{false, false, false}, {true, false, false}, {false, true, false}, {false, false, true}} {
		if _, err := o.Check(context.Background(), 0, keep); err != nil {
			t.Fatal(err)
		}
	}
	for i, want := range [][][]int{
		{{-2}},            // A clause with no origin always stays.
		{{-1}, {-2}},      // One kept origin of two retains a clause.
		{{1}, {-1}, {-2}}, // So does the other.
		{{2}, {-2}},       // Removing every origin of a clause drops it.
	} {
		if fmt.Sprint(trials[i]) != fmt.Sprint(want) {
			t.Fatalf("trial %d: %v, want %v", i, trials[i], want)
		}
	}
	if _, err := o.Check(context.Background(), 0, []bool{true}); !errors.Is(err, mus.ErrInput) {
		t.Fatal(err)
	}
}

func TestByOriginHardClausesAreNeverRemoved(t *testing.T) {
	c := acceptance.CNF(1, [][]int{{1}, {-1}, {1}})
	c.Origins = [][]string{nil, {"soft"}, {"other"}}
	o, err := mus.ByOrigin(c, bruteForce)
	if err != nil {
		t.Fatal(err)
	}
	core, err := mus.Shrink(context.Background(), o, strict)
	if err != nil || !core.Minimal {
		t.Fatalf("%+v %v", core, err)
	}
	// Index 1 is "soft": the unit clause {1} without an origin stays, so {1}
	// with origin "other" is redundant.
	if !slices.Equal(core.Groups, []int{1}) {
		t.Fatal(core.Groups, o.Names())
	}
}

func TestByOriginRejectsInvalidInput(t *testing.T) {
	bad := acceptance.CNF(1, [][]int{{2}})
	bad.Origins = [][]string{{"a"}}
	for name, c := range map[string]struct {
		cnf   sat.CNF
		solve mus.Solve
		sat   bool // the error must also be a sat.ErrInput
	}{
		"nil solve":   {acceptance.CNF(1, nil), nil, false},
		"no origins":  {acceptance.CNF(1, [][]int{{1}}), bruteForce, false},
		"bad formula": {bad, bruteForce, true},
	} {
		t.Run(name, func(t *testing.T) {
			o, err := mus.ByOrigin(c.cnf, c.solve)
			if !errors.Is(err, mus.ErrInput) || o != nil {
				t.Fatalf("%v %v", o, err)
			}
			if c.sat && !errors.Is(err, sat.ErrInput) {
				t.Fatal(err)
			}
		})
	}
	if _, err := mus.ByOrigin(acceptance.CNF(1, nil), bruteForce); err == nil || !strings.Contains(err.Error(), "origins") {
		t.Fatal(err)
	}
}
