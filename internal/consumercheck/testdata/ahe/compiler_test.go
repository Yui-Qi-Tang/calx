package consumer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Yui-Qi-Tang/calx/digest"
	"github.com/Yui-Qi-Tang/calx/sat"
	"github.com/Yui-Qi-Tang/calx/sat/cadical"
)

func TestCalxDeclaredConditions(t *testing.T) {
	runner, err := cadical.New(cadical.Config{
		SolverPath: os.Getenv("CALX_CADICAL"), SolverDigest: digest.Digest(os.Getenv("CALX_CADICAL_DIGEST")),
		CheckerPath: os.Getenv("CALX_DRAT"), CheckerDigest: digest.Digest(os.Getenv("CALX_DRAT_DIGEST")),
		ArtifactDir: filepath.Join(os.Getenv("CALX_CONSUMER_OUTPUT"), "ahe-"+os.Getenv("CALX_CONSUMER_KIND")),
	})
	if err != nil {
		t.Fatal(err)
	}
	view := ConsistencyView{Members: []ConsistencyMember{{NodeID: "a"}, {NodeID: "b"}}}
	atom := ConsistencyAtom{Subject: "service", Property: "ready", Context: "snapshot"}
	for i, negated := range []bool{false, true} {
		input := ConsistencyRequest{Evidence: []ConsistencyCondition{
			{ID: "a", Clauses: [][]ConsistencyLiteral{{{Atom: atom}}}},
			{ID: "b", Clauses: [][]ConsistencyLiteral{{{Atom: atom, Negated: negated}}}},
		}}
		rules, all, err := compileConsistency(view, input)
		if err != nil {
			t.Fatal(err)
		}
		if len(all.Origins) != 2 || all.Origins[0][0] != "node:a" {
			t.Fatal("lost origin mapping")
		}
		for j, formula := range []sat.CNF{rules, all} {
			result, err := runner.Solve(context.Background(), fmt.Sprintf("case-%d-%d", i, j), formula)
			if err != nil {
				t.Fatal(err)
			}
			if err := result.Require(sat.Requirements{SATModelChecked: true, UNSATProofChecked: true}); err != nil {
				t.Fatal(err)
			}
			want := sat.SAT
			if negated && j == 1 {
				want = sat.UNSAT
			}
			if result.Status != want {
				t.Fatalf("status %s, want %s", result.Status, want)
			}
		}
		input.Evidence = input.Evidence[:1]
		if _, _, err := compileConsistency(view, input); err == nil {
			t.Fatal("incomplete evidence accepted")
		}
	}
}
