package linear

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/Yui-Qi-Tang/calx/sat"
)

func TestExactIntegersAndUnsupportedInputs(t *testing.T) {
	p := Problem{Integers: []sat.Variable{{ID: 1}}, Constraints: []Constraint{{Terms: []Term{{Variable: 1, Coefficient: "2"}}, Relation: Equal, RHS: "18446744073709551616"}}}
	if err := p.CheckAssignment(Assignment{Integers: map[int]string{1: "9223372036854775808"}}); err != nil {
		t.Fatal(err)
	}
	if err := p.CheckAssignment(Assignment{Integers: map[int]string{1: "9223372036854775807"}}); err == nil {
		t.Fatal("accepted off-by-one model")
	}
	p.Constraints[0].Relation = "forall"
	if !errors.Is(p.CheckShape(), ErrUnsupported) {
		t.Fatal("accepted unsupported operator")
	}
	p.Constraints[0].Relation = Equal
	p.Constraints[0].Terms[0].Variable = 2
	if !errors.Is(p.CheckShape(), sat.ErrInput) {
		t.Fatal("accepted wrong variable sort/ID")
	}
	for _, s := range []string{"1.5", "(+ 1 2)", "-0", "01", "", "true", "+1", " 1"} {
		if _, err := Integer(s); !errors.Is(err, sat.ErrInput) {
			t.Fatalf("accepted %q: %v", s, err)
		}
	}
}

func TestTotalModelAndNegativeCoefficients(t *testing.T) {
	p := Problem{Integers: []sat.Variable{{ID: 1}, {ID: 2}}, Constraints: []Constraint{{Terms: []Term{{Variable: 1, Coefficient: "-3"}, {Variable: 2, Coefficient: "2"}}, Relation: AtLeast, RHS: "8"}}}
	for _, a := range []Assignment{{Integers: map[int]string{1: "-2"}}, {Integers: map[int]string{1: "-2", 3: "1"}}, {Integers: map[int]string{1: "-1", 2: "1"}}} {
		if p.CheckAssignment(a) == nil {
			t.Fatal("accepted invalid model")
		}
	}
	if err := p.CheckAssignment(Assignment{Integers: map[int]string{1: "-2", 2: "1"}}); err != nil {
		t.Fatal(err)
	}
}

func TestCheckAssignmentCoversEveryRelationAndBooleans(t *testing.T) {
	one := []Term{{Variable: 1, Coefficient: "1"}}
	for _, c := range []struct {
		relation Relation
		value    string
		ok       bool
	}{
		{Equal, "5", true}, {Equal, "4", false}, {Equal, "6", false},
		{AtMost, "5", true}, {AtMost, "4", true}, {AtMost, "6", false},
		{AtLeast, "5", true}, {AtLeast, "6", true}, {AtLeast, "4", false},
	} {
		p := Problem{Integers: []sat.Variable{{ID: 1}}, Constraints: []Constraint{{Terms: one, Relation: c.relation, RHS: "5"}}}
		if err := p.CheckAssignment(Assignment{Integers: map[int]string{1: c.value}}); (err == nil) != c.ok {
			t.Fatalf("%s %s: %v", c.relation, c.value, err)
		}
	}
	p := Problem{Booleans: sat.CNF{Variables: []sat.Variable{{ID: 1}}, Clauses: [][]int{{1}}}}
	if err := p.CheckAssignment(Assignment{Booleans: map[int]bool{1: false}}); err == nil {
		t.Fatal("accepted a Boolean model that violates a clause")
	}
	empty := Problem{Integers: []sat.Variable{{ID: 1}}, Constraints: []Constraint{{Relation: AtMost, RHS: "-1"}}}
	if err := empty.CheckAssignment(Assignment{Integers: map[int]string{1: "0"}}); err == nil {
		t.Fatal("an empty sum must be zero")
	}
}

func TestWriteSMTLIBExactBytes(t *testing.T) {
	p := Problem{
		Booleans: sat.CNF{Variables: []sat.Variable{{ID: 1}, {ID: 2}}, Clauses: [][]int{{1, -2}, {}}},
		Integers: []sat.Variable{{ID: 1}},
		Constraints: []Constraint{
			{Terms: []Term{{Variable: 1, Coefficient: "-2"}}, Relation: AtMost, RHS: "-7"},
			{Relation: Equal, RHS: "0"},
		},
	}
	want := strings.Join([]string{
		"(set-logic QF_LIA)",
		"(declare-const b1 Bool)",
		"(declare-const b2 Bool)",
		"(declare-const i1 Int)",
		"(assert (or b1 (not b2) false))",
		"(assert false)",
		"(assert (<= (+ 0 0 (* (- 2) i1)) (- 7)))",
		"(assert (= (+ 0 0) 0))",
		"",
	}, "\n")
	var got bytes.Buffer
	if err := p.WriteSMTLIB(&got); err != nil {
		t.Fatal(err)
	}
	if got.String() != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got.String(), want)
	}
	p.Constraints[0].Relation = "<"
	if err := p.WriteSMTLIB(&got); !errors.Is(err, ErrUnsupported) {
		t.Fatal(err)
	}
}

func TestWriteSMTLIBReportsWriteError(t *testing.T) {
	broken := errors.New("broken writer")
	if err := (Problem{}).WriteSMTLIB(failingWriter{broken}); !errors.Is(err, broken) {
		t.Fatal(err)
	}
}

type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }

func TestCheckShapeRejectsInvalidText(t *testing.T) {
	for name, p := range map[string]Problem{
		"noncontiguous integers": {Integers: []sat.Variable{{ID: 2}}},
		"integer metadata":       {Integers: []sat.Variable{{ID: 1, Metadata: map[string]string{"k": "\xff"}}}},
		"empty origin":           {Constraints: []Constraint{{Relation: Equal, RHS: "0", Origins: []string{""}}}},
		"invalid origin":         {Constraints: []Constraint{{Relation: Equal, RHS: "0", Origins: []string{"\xff"}}}},
		"bad right-hand side":    {Constraints: []Constraint{{Relation: Equal, RHS: "x"}}},
		"bad coefficient":        {Integers: []sat.Variable{{ID: 1}}, Constraints: []Constraint{{Terms: []Term{{Variable: 1, Coefficient: "x"}}, Relation: Equal, RHS: "0"}}},
		"bad boolean literal":    {Booleans: sat.CNF{Clauses: [][]int{{1}}}},
	} {
		t.Run(name, func(t *testing.T) {
			if err := p.CheckShape(); !errors.Is(err, sat.ErrInput) {
				t.Fatal(err)
			}
		})
	}
}

func TestCheckLimitsSharesBudgets(t *testing.T) {
	limits, err := sat.Limits{MaxVariables: 2, MaxClauses: 2, MaxLiterals: 3}.WithDefaults()
	if err != nil {
		t.Fatal(err)
	}
	term := []Term{{Variable: 1, Coefficient: "1"}}
	constraint := Constraint{Terms: term, Relation: Equal, RHS: "0"}
	base := sat.CNF{Variables: []sat.Variable{{ID: 1}}, Clauses: [][]int{{1, -1}}}
	if err := (Problem{Booleans: base, Integers: []sat.Variable{{ID: 1}}, Constraints: []Constraint{constraint}}).CheckLimits(limits); err != nil {
		t.Fatal(err)
	}
	for name, p := range map[string]Problem{
		"variables": {Booleans: base, Integers: []sat.Variable{{ID: 1}, {ID: 2}}},
		"clauses":   {Booleans: base, Integers: []sat.Variable{{ID: 1}}, Constraints: []Constraint{constraint, constraint}},
		"terms":     {Booleans: sat.CNF{Variables: base.Variables, Clauses: [][]int{{1, -1, 1}}}, Integers: []sat.Variable{{ID: 1}}, Constraints: []Constraint{constraint}},
	} {
		t.Run(name, func(t *testing.T) {
			if err := p.CheckLimits(limits); !errors.Is(err, sat.ErrLimit) {
				t.Fatal(err)
			}
		})
	}
	if err := (Problem{Integers: []sat.Variable{{ID: 2}}}).CheckLimits(limits); !errors.Is(err, sat.ErrInput) {
		t.Fatal(err)
	}
}
