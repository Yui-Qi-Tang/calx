package finite

import (
	"context"
	"errors"
	"testing"
)

func testCompiler(ctx context.Context) compiler {
	model := Model{Fields: map[string][]string{"x": {"a", "b"}}, Actions: []Action{{ID: "move", Guard: State{"x": "a"}, Effect: State{"x": "b"}}}}
	return compiler{ctx: ctx, limit: Options{MaxVariables: 1, MaxClauses: 1}, scope: newVariableScope(model, 1)}
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
