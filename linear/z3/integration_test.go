//go:build resolverintegration

package z3

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/Yui-Qi-Tang/calx/digest"
	"github.com/Yui-Qi-Tang/calx/internal/acceptance"
	"github.com/Yui-Qi-Tang/calx/linear"
	"github.com/Yui-Qi-Tang/calx/mus"
	"github.com/Yui-Qi-Tang/calx/sat"
)

func TestRealTools(t *testing.T) {
	for _, name := range []string{"CALX_Z3", "CALX_Z3_DIGEST", "CALX_ARTIFACTS"} {
		if os.Getenv(name) == "" {
			t.Fatalf("%s is required for explicit acceptance", name)
		}
	}
	r, err := New(Config{SolverPath: os.Getenv("CALX_Z3"), SolverDigest: digest.Digest(os.Getenv("CALX_Z3_DIGEST")), ArtifactDir: filepath.Join(os.Getenv("CALX_ARTIFACTS"), "z3")})
	if err != nil {
		t.Fatal(err)
	}
	check := func(t *testing.T, name string, p linear.Problem, want sat.Status) {
		t.Helper()
		got, err := r.Solve(context.Background(), name, p)
		if err != nil {
			t.Fatal(err)
		}
		if got.Status != want {
			t.Fatalf("want %s got %+v", want, got)
		}
		if got.Status == sat.SAT {
			if err = got.Require(sat.Requirements{SATModelChecked: true}); err != nil {
				t.Fatal(err)
			}
		} else if !errors.Is(got.Require(sat.Requirements{UNSATProofChecked: true}), sat.ErrGuarantee) {
			t.Fatal("UNSAT must remain unchecked")
		}
	}
	// Same fixed 512 Boolean inputs and independent enumerating oracle as SAT.
	formulas := acceptance.Cases()
	names := []string{}
	for name := range formulas {
		if strings.HasPrefix(name, "matrix-") {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			c := formulas[name]
			check(t, name, linear.Problem{Booleans: c}, acceptance.Oracle(c))
		})
	}
	constraint := func(coefficient, rhs string, op linear.Relation) linear.Constraint {
		return linear.Constraint{Terms: []linear.Term{{Variable: 1, Coefficient: coefficient}}, Relation: op, RHS: rhs}
	}
	integer := []sat.Variable{{ID: 1}}
	two := []sat.Variable{{ID: 1}, {ID: 2}}
	cases := []struct {
		p    linear.Problem
		want sat.Status
	}{
		{linear.Problem{}, sat.SAT},
		{linear.Problem{Booleans: acceptance.CNF(1, [][]int{{1}, {-1}})}, sat.UNSAT},
		{linear.Problem{Integers: integer, Constraints: []linear.Constraint{constraint("1", "-2", linear.Equal)}}, sat.SAT},
		{linear.Problem{Integers: integer, Constraints: []linear.Constraint{constraint("1", "3", linear.AtLeast), constraint("1", "5", linear.AtMost)}}, sat.SAT},
		{linear.Problem{Integers: integer, Constraints: []linear.Constraint{constraint("1", "6", linear.AtLeast), constraint("1", "5", linear.AtMost)}}, sat.UNSAT},
		{linear.Problem{Integers: integer, Constraints: []linear.Constraint{constraint("2", "1", linear.Equal)}}, sat.UNSAT},
		{linear.Problem{Integers: two, Constraints: []linear.Constraint{
			{Terms: []linear.Term{{Variable: 1, Coefficient: "1"}, {Variable: 2, Coefficient: "1"}}, Relation: linear.Equal, RHS: "4"},
			{Terms: []linear.Term{{Variable: 1, Coefficient: "1"}, {Variable: 2, Coefficient: "-1"}}, Relation: linear.Equal, RHS: "2"},
		}}, sat.SAT},
		{linear.Problem{Booleans: acceptance.CNF(1, [][]int{{1}}), Integers: integer, Constraints: []linear.Constraint{constraint("1", "0", linear.AtLeast), constraint("1", "0", linear.AtMost)}}, sat.SAT},
		// Values beyond 64 bits, an unconstrained variable and an empty sum.
		{linear.Problem{Integers: two, Constraints: []linear.Constraint{constraint("3", "340282366920938463463374607431768211456", linear.AtLeast), {Relation: linear.Equal, RHS: "0"}}}, sat.SAT},
		{linear.Problem{Integers: integer, Constraints: []linear.Constraint{{Relation: linear.AtLeast, RHS: "1"}}}, sat.UNSAT},
	}
	for i, c := range cases {
		name := fmt.Sprintf("linear-%d", i)
		t.Run(name, func(t *testing.T) { check(t, name, c.p, c.want) })
	}
	t.Run("primer-model", func(t *testing.T) {
		// docs/linear-primer.md: the only solution of x + y = 4, x - y = 2.
		got, err := r.Solve(context.Background(), "primer-pair", cases[6].p)
		if err != nil || got.Assignment.Integers[1] != "3" || got.Assignment.Integers[2] != "1" {
			t.Fatalf("%+v %v", got, err)
		}
		if got.Model.Method != "total-bool-and-big-integer-evaluation/v0" {
			t.Fatal(got.Model.Method)
		}
	})
	t.Run("mus", func(t *testing.T) {
		// mus.Oracle is not tied to CNF. Groups here are constraints.
		o := &constraintOracle{runner: r, problem: linear.Problem{Integers: two, Constraints: []linear.Constraint{
			constraint("1", "6", linear.AtLeast),
			{Terms: []linear.Term{{Variable: 2, Coefficient: "1"}}, Relation: linear.AtMost, RHS: "0"},
			constraint("1", "5", linear.AtMost),
		}}}
		// Z3 UNSAT has no checked proof, so accepting it has to be requested.
		if _, err := mus.Shrink(context.Background(), o, sat.Requirements{UNSATProofChecked: true}); !errors.Is(err, sat.ErrGuarantee) {
			t.Fatal(err)
		}
		o.prefix = "unchecked"
		core, err := mus.Shrink(context.Background(), o, sat.Requirements{SATModelChecked: true})
		if err != nil || !core.Minimal || !slices.Equal(core.Groups, []int{0, 2}) {
			t.Fatalf("%+v %v", core, err)
		}
	})
}

type constraintOracle struct {
	runner  *Runner
	problem linear.Problem
	prefix  string
}

func (o *constraintOracle) Groups() int { return len(o.problem.Constraints) }

func (o *constraintOracle) Check(ctx context.Context, step int, keep []bool) (sat.Result, error) {
	trial := linear.Problem{Booleans: o.problem.Booleans, Integers: o.problem.Integers}
	for i, c := range o.problem.Constraints {
		if keep[i] {
			trial.Constraints = append(trial.Constraints, c)
		}
	}
	solved, err := o.runner.Solve(ctx, fmt.Sprintf("core-%s-%d", o.prefix, step), trial)
	return solved.Result, err
}
