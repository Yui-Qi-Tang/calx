package finite

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Yui-Qi-Tang/calx/sat"
)

func testCompiler(ctx context.Context) compiler {
	model := Model{Fields: map[string][]string{"x": {"a", "b"}}, Actions: []Action{{ID: "move", Guard: State{"x": "a"}, Effect: State{"x": "b"}}}}
	return compiler{ctx: ctx, limit: budget{variables: 1, clauses: 1}, scope: newVariableScope(model, 1)}
}

func TestCompilerReturnsErrorsWithoutSentinelIDs(t *testing.T) {
	b := testCompiler(context.Background())
	if id, err := b.variable(Variable{Kind: ActionVariable, Action: "unknown"}); id != 0 || !errors.Is(err, ErrInput) {
		t.Fatal(id, err)
	}
	if len(b.cnf.Variables) != 0 || len(b.mapping) != 0 {
		t.Fatal("failed declaration changed compiler state")
	}
	if id, err := b.variable(Variable{Kind: StateVariable, Field: "x", Value: "a"}); id != 1 || err != nil {
		t.Fatal(id, err)
	}
	if id, err := b.variable(Variable{Kind: StateVariable, Field: "x", Value: "b"}); id != 0 || !errors.Is(err, ErrEncodingLimit) {
		t.Fatal(id, err)
	}
	if err := b.add(0); !errors.Is(err, ErrInput) {
		t.Fatal(err)
	}
	if err := b.add(1); err != nil {
		t.Fatal(err)
	}
	if err := b.one([]int{1}); !errors.Is(err, ErrEncodingLimit) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	canceled := testCompiler(ctx)
	if _, err := canceled.variable(Variable{Kind: StateVariable, Field: "x", Value: "a"}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := canceled.add(); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestBudgetOptions(t *testing.T) {
	b, err := newBudget(nil)
	if err != nil {
		t.Fatal(err)
	}
	l, err := sat.Limits{}.WithDefaults()
	if err != nil {
		t.Fatal(err)
	}
	if b.variables != l.MaxVariables || b.clauses != l.MaxClauses {
		t.Fatalf("defaults %+v differ from sat.Limits defaults %+v", b, l)
	}
	if b, err = newBudget([]Option{WithMaxClauses(5), WithMaxClauses(7)}); err != nil || b.clauses != 7 {
		t.Fatalf("later option must win: %+v %v", b, err)
	}
	for name, opts := range map[string][]Option{
		"zero variables":     {WithMaxVariables(0)},
		"negative clauses":   {WithMaxClauses(-1)},
		"after a valid one":  {WithMaxVariables(10), WithMaxClauses(0)},
		"first invalid wins": {WithMaxClauses(0), WithMaxVariables(-1)},
	} {
		_, err := newBudget(opts)
		if !errors.Is(err, ErrInput) {
			t.Errorf("%s: got %v, want ErrInput", name, err)
		}
		if name == "first invalid wins" && !strings.Contains(err.Error(), "clauses") {
			t.Errorf("%s: got %v, want the clauses error", name, err)
		}
	}
}
