package automaton

import (
	"context"
	"maps"
	"slices"
	"testing"
)

// The tests in this file pin the claims of docs/automata-primer.md without a
// SAT solver: forward runs use Simulate, and the formula's answers come from
// the exhaustive search in satisfiable.

// dots draws a one-bit state as the primer does: ● true, ○ false.
func dots(s State) string {
	out := ""
	for _, v := range s {
		if v {
			out += "●"
		} else {
			out += "○"
		}
	}
	return out
}

func undots(s string) State {
	var out State
	for _, r := range s {
		out = append(out, r == '●')
	}
	return out
}

func TestAutomataPrimerRun(t *testing.T) {
	states, err := Simulate(elementary{cells: 4, rule: 90}, undots("●○○○"), 3, nil)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, s := range states {
		got = append(got, dots(s))
	}
	if want := []string{"●○○○", "○●○●", "○○○○", "○○○○"}; !slices.Equal(got, want) {
		t.Fatalf("run %v, want %v", got, want)
	}
}

// TestAutomataPrimerCellClauses pins the eight clauses of cell 1 from time 0
// to 1, in the primer's row order.
func TestAutomataPrimerCellClauses(t *testing.T) {
	sys := elementary{cells: 4, rule: 90}
	e, err := Compile(context.Background(), sys, Query{
		Steps: 2,
		Start: Pattern{{Cell: 0}: true, {Cell: 1}: false, {Cell: 2}: false, {Cell: 3}: false},
		Goal:  Pattern{{Cell: 0}: false, {Cell: 1}: false, {Cell: 2}: false, {Cell: 3}: false},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := [][]int{
		{1, 2, 3, -6}, {-1, 2, 3, 6}, {1, -2, 3, -6}, {-1, -2, 3, 6},
		{1, 2, -3, 6}, {-1, 2, -3, -6}, {1, -2, -3, 6}, {-1, -2, -3, -6},
	}
	// Cell 0's eight rows come first.
	if got := e.formula.Clauses[8:16]; !slices.EqualFunc(got, want, slices.Equal) {
		t.Fatalf("cell 1 clauses %v, want %v", got, want)
	}
	if len(e.formula.Variables) != 12 || len(e.formula.Clauses) != 72 {
		t.Fatalf("%d variables, %d clauses, want 12 and 72", len(e.formula.Variables), len(e.formula.Clauses))
	}
	units := [][]int{{1}, {-2}, {-3}, {-4}, {-9}, {-10}, {-11}, {-12}}
	if got := e.formula.Clauses[64:]; !slices.EqualFunc(got, units, slices.Equal) {
		t.Fatalf("unit clauses %v, want %v", got, units)
	}
	for time := range 3 {
		for c := range 4 {
			if got := e.layout.stateVar(time, Bit{Cell: c}); got != 1+4*time+c {
				t.Fatalf("cell %d at time %d is x%d, want x%d", c, time, got, 1+4*time+c)
			}
		}
	}
	if !satisfiable(e.formula, map[int]bool{}) {
		t.Fatal("the primer's run ●○○○ → ○●○● → ○○○○ is not admitted")
	}
}

// TestAutomataPrimerGardenOfEden pins the primer's state graph and checks that
// the predecessor query is SAT exactly for the states that have a predecessor.
func TestAutomataPrimerGardenOfEden(t *testing.T) {
	sys := elementary{cells: 4, rule: 90}
	predecessors := map[string][]string{}
	for mask := range 16 {
		s := make(State, 4)
		for i := range s {
			s[i] = mask>>i&1 == 1
		}
		states, err := Simulate(sys, s, 1, nil)
		if err != nil {
			t.Fatal(err)
		}
		next := dots(states[1])
		predecessors[next] = append(predecessors[next], dots(s))
	}
	want := map[string][]string{
		"○○○○": {"○○○○", "○●○●", "●○●○", "●●●●"},
		"○●○●": {"○○●○", "○●●●", "●○○○", "●●○●"},
		"●○●○": {"○○○●", "○●○○", "●○●●", "●●●○"},
		"●●●●": {"○○●●", "○●●○", "●○○●", "●●○○"},
	}
	for next := range predecessors {
		slices.Sort(predecessors[next])
	}
	if !maps.EqualFunc(predecessors, want, slices.Equal) {
		t.Fatalf("predecessors %v, want %v", predecessors, want)
	}
	for mask := range 16 {
		s := make(State, 4)
		goal := Pattern{}
		for i := range s {
			s[i] = mask>>i&1 == 1
			goal[Bit{Cell: i}] = s[i]
		}
		e, err := Compile(context.Background(), sys, Query{Steps: 1, Goal: goal})
		if err != nil {
			t.Fatal(err)
		}
		_, hasPredecessor := want[dots(s)]
		if got := satisfiable(e.formula, map[int]bool{}); got != hasPredecessor {
			t.Errorf("%s: predecessor query SAT %v, want %v", dots(s), got, hasPredecessor)
		}
	}
}
