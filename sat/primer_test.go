package sat_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/Yui-Qi-Tang/calx/internal/acceptance"
	"github.com/Yui-Qi-Tang/calx/sat"
)

// The tests in this file pin the claims of docs/sat-primer.md without a SAT
// solver.

func TestSATPrimerFormulas(t *testing.T) {
	for _, c := range []struct {
		clauses [][]int
		dimacs  string
		// violated clause for (x1, x2) = ff, ft, tf, tt; -1 means satisfied
		violated [4]int
	}{
		{[][]int{{1}, {-1, 2}}, "p cnf 2 2\n1 0\n-1 2 0\n", [4]int{0, 0, 1, -1}},
		{[][]int{{1, 2}, {-1, 2}, {1, -2}, {-1, -2}}, "p cnf 2 4\n1 2 0\n-1 2 0\n1 -2 0\n-1 -2 0\n", [4]int{0, 2, 1, 3}},
	} {
		f := acceptance.CNF(2, c.clauses)
		var d bytes.Buffer
		if err := f.WriteDIMACS(&d); err != nil || d.String() != c.dimacs {
			t.Fatalf("DIMACS %q, %v", d.String(), err)
		}
		for i, a := range []map[int]bool{{1: false, 2: false}, {1: false, 2: true}, {1: true, 2: false}, {1: true, 2: true}} {
			err := f.CheckAssignment(a)
			if c.violated[i] < 0 {
				if err != nil {
					t.Errorf("%v %v: %v, want satisfied", c.clauses, a, err)
				}
			} else if err == nil || !strings.HasSuffix(err.Error(), fmt.Sprintf("clause %d", c.violated[i])) {
				t.Errorf("%v %v: %v, want clause %d", c.clauses, a, err, c.violated[i])
			}
		}
	}
	if acceptance.Oracle(acceptance.CNF(2, [][]int{{1, 2}, {-1, 2}, {1, -2}, {-1, -2}})) != sat.UNSAT {
		t.Fatal("the second formula is not UNSAT")
	}
	// No clauses is true; a clause with no literals is false.
	if acceptance.CNF(1, nil).CheckAssignment(map[int]bool{1: false}) != nil ||
		acceptance.CNF(1, [][]int{{}}).CheckAssignment(map[int]bool{1: true}) == nil {
		t.Fatal("empty formula or empty clause misjudged")
	}
}

// TestSATPrimerProof checks that each line of the primer's DRAT proof follows
// from the formula and the earlier lines by unit propagation, the reverse unit
// propagation (RUP) check that DRAT-trim applies to these lines.
func TestSATPrimerProof(t *testing.T) {
	clauses := [][]int{{1, 2}, {-1, 2}, {1, -2}, {-1, -2}}
	if refutesNegation(clauses, []int{}) {
		t.Fatal("the empty clause follows without the lemmas; the check is vacuous")
	}
	for _, lemma := range [][]int{{1}, {2}, {}} {
		if !refutesNegation(clauses, lemma) {
			t.Fatalf("lemma %v does not follow by unit propagation", lemma)
		}
		clauses = append(clauses, lemma)
	}
}

// refutesNegation assumes every literal of lemma false and propagates units
// until some clause is falsified (true) or nothing changes (false).
func refutesNegation(clauses [][]int, lemma []int) bool {
	value := map[int]bool{}
	for _, lit := range lemma {
		value[abs(lit)] = lit < 0
	}
	for {
		changed := false
		for _, clause := range clauses {
			var open []int
			satisfied := false
			for _, lit := range clause {
				v, ok := value[abs(lit)]
				switch {
				case !ok:
					open = append(open, lit)
				case v == (lit > 0):
					satisfied = true
				}
			}
			if satisfied {
				continue
			}
			if len(open) == 0 {
				return true
			}
			if len(open) == 1 {
				value[abs(open[0])] = open[0] > 0
				changed = true
			}
		}
		if !changed {
			return false
		}
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
