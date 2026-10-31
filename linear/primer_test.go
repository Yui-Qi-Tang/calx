package linear

import (
	"bytes"
	"errors"
	"slices"
	"strconv"
	"testing"

	"github.com/Yui-Qi-Tang/calx/sat"
)

// The tests in this file pin the claims of docs/linear-primer.md without a
// solver. The primer's answers come from Z3; these enumerate a range of integers
// wide enough for each small problem, so SAT solutions and UNSAT claims are
// checked by a route that does not use one.

func one(coefficient, relation, rhs string) Constraint {
	return Constraint{Terms: []Term{{Variable: 1, Coefficient: coefficient}}, Relation: Relation(relation), RHS: rhs}
}

// solutions lists the assignments in [-20, 20]^n that satisfy p.
func solutions(t *testing.T, p Problem) [][]int {
	t.Helper()
	var found [][]int
	values := make([]int, len(p.Integers))
	var walk func(i int)
	walk = func(i int) {
		if i < len(values) {
			for v := -20; v <= 20; v++ {
				values[i] = v
				walk(i + 1)
			}
			return
		}
		a := Assignment{Integers: map[int]string{}}
		for j, v := range values {
			a.Integers[j+1] = strconv.Itoa(v)
		}
		if p.CheckAssignment(a) == nil {
			found = append(found, slices.Clone(values))
		}
	}
	walk(0)
	return found
}

func TestLinearPrimerFourProblems(t *testing.T) {
	x := []sat.Variable{{ID: 1}}
	pair := []Constraint{
		{Terms: []Term{{Variable: 1, Coefficient: "1"}, {Variable: 2, Coefficient: "1"}}, Relation: Equal, RHS: "4"},
		{Terms: []Term{{Variable: 1, Coefficient: "1"}, {Variable: 2, Coefficient: "-1"}}, Relation: Equal, RHS: "2"},
	}
	for name, c := range map[string]struct {
		p    Problem
		want [][]int
	}{
		"range":      {Problem{Integers: x, Constraints: []Constraint{one("1", ">=", "3"), one("1", "<=", "5")}}, [][]int{{3}, {4}, {5}}},
		"no overlap": {Problem{Integers: x, Constraints: []Constraint{one("1", ">=", "6"), one("1", "<=", "5")}}, nil},
		"half":       {Problem{Integers: x, Constraints: []Constraint{one("2", "=", "1")}}, nil},
		"pair":       {Problem{Integers: []sat.Variable{{ID: 1}, {ID: 2}}, Constraints: pair}, [][]int{{3, 1}}},
	} {
		t.Run(name, func(t *testing.T) {
			got := solutions(t, c.p)
			if !slices.EqualFunc(got, c.want, slices.Equal[[]int]) {
				t.Fatalf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestLinearPrimerRewrites(t *testing.T) {
	// x < y as x - y <= -1, over every pair in the enumerated range.
	two := []sat.Variable{{ID: 1}, {ID: 2}}
	lt := Problem{Integers: two, Constraints: []Constraint{{Terms: []Term{{Variable: 1, Coefficient: "1"}, {Variable: 2, Coefficient: "-1"}}, Relation: AtMost, RHS: "-1"}}}
	got := solutions(t, lt)
	if len(got) != 41*40/2 {
		t.Fatalf("%d solutions", len(got))
	}
	for _, s := range got {
		if s[0] >= s[1] {
			t.Fatal(s)
		}
	}
	// x > 3 as x >= 4.
	gt := solutions(t, Problem{Integers: two[:1], Constraints: []Constraint{one("1", ">=", "4")}})
	if gt[0][0] != 4 || len(gt) != 17 {
		t.Fatal(gt)
	}
	// 2x + 1 = y as 2x - y = -1.
	line := Problem{Integers: two, Constraints: []Constraint{{Terms: []Term{{Variable: 1, Coefficient: "2"}, {Variable: 2, Coefficient: "-1"}}, Relation: Equal, RHS: "-1"}}}
	for _, s := range solutions(t, line) {
		if 2*s[0]+1 != s[1] {
			t.Fatal(s)
		}
	}
	if !errors.Is(Problem{Constraints: []Constraint{{Relation: "<", RHS: "0"}}}.CheckShape(), ErrUnsupported) {
		t.Fatal("the primer's strict relation must be unsupported")
	}
}

func TestLinearPrimerSMTLIB(t *testing.T) {
	p := Problem{Integers: []sat.Variable{{ID: 1}, {ID: 2}}, Constraints: []Constraint{
		{Terms: []Term{{Variable: 1, Coefficient: "1"}, {Variable: 2, Coefficient: "1"}}, Relation: Equal, RHS: "4"},
		{Terms: []Term{{Variable: 1, Coefficient: "1"}, {Variable: 2, Coefficient: "-1"}}, Relation: Equal, RHS: "2"},
	}}
	const want = `(set-logic QF_LIA)
(declare-const i1 Int)
(declare-const i2 Int)
(assert (= (+ 0 0 (* 1 i1) (* 1 i2)) 4))
(assert (= (+ 0 0 (* 1 i1) (* (- 1) i2)) 2))
`
	var got bytes.Buffer
	if err := p.WriteSMTLIB(&got); err != nil || got.String() != want {
		t.Fatalf("%q %v", got.String(), err)
	}
}

func TestLinearPrimerBeyondSixtyFourBits(t *testing.T) {
	p := Problem{Integers: []sat.Variable{{ID: 1}}, Constraints: []Constraint{one("2", ">=", "18446744073709551616")}}
	if err := p.CheckAssignment(Assignment{Integers: map[int]string{1: "9223372036854775808"}}); err != nil {
		t.Fatal(err)
	}
	if err := p.CheckAssignment(Assignment{Integers: map[int]string{1: "9223372036854775807"}}); err == nil {
		t.Fatal("accepted a value one too small")
	}
}
