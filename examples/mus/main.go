// Command mus demonstrates shrinking a checked UNSAT answer to one irreducible
// conflict with mus.Shrink and the pinned solver and proof checker.
package main

import (
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/Yui-Qi-Tang/calx/digest"
	"github.com/Yui-Qi-Tang/calx/mus"
	"github.com/Yui-Qi-Tang/calx/sat"
	"github.com/Yui-Qi-Tang/calx/sat/cadical"
)

func main() {
	artifacts := flag.String("artifacts", "artifacts/go-mus", "new run directory; parent must exist and the run directory must not exist")
	flag.Parse()
	if err := run(context.Background(), *artifacts); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, artifacts string) error {
	// 1. Declare the formula. Origins name the group of each clause.
	formula := sat.CNF{
		Variables: []sat.Variable{{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}},
		Clauses:   [][]int{{3}, {1}, {-1, 2}, {-2}, {4}},
		Origins: [][]string{
			{"clock-set"}, {"power-on"}, {"power-implies-light"}, {"light-off"}, {"tv-on"},
		},
	}

	// 2. Pin the solver and the proof checker.
	runner, err := cadical.New(cadical.Config{
		SolverPath:    os.Getenv("CALX_CADICAL"),
		SolverDigest:  digest.Digest(os.Getenv("CALX_CADICAL_DIGEST")),
		CheckerPath:   os.Getenv("CALX_DRAT"),
		CheckerDigest: digest.Digest(os.Getenv("CALX_DRAT_DIGEST")),
		ArtifactDir:   artifacts,
	})
	if err != nil {
		return err
	}

	// 3. Build the Oracle. Each check gets its own query ID.
	oracle, err := mus.ByOrigin(formula, func(ctx context.Context, step int, trial sat.CNF) (sat.Result, error) {
		solved, err := runner.Solve(ctx, fmt.Sprintf("core-%d", step), trial)
		return solved.Result, err
	})
	if err != nil {
		return err
	}

	// 4. Shrink, requiring a checked model or proof for every answer.
	core, err := mus.Shrink(ctx, oracle, sat.Requirements{SATModelChecked: true, UNSATProofChecked: true})
	if err != nil {
		return err
	}

	// 5. Translate group indices back to the caller's names.
	names := oracle.Names()
	out := struct {
		Conflict []string   `json:"conflict"`
		Minimal  bool       `json:"minimal"`
		Witness  sat.Status `json:"witness"`
		Checks   []string   `json:"checks"`
	}{Minimal: core.Minimal, Witness: core.Witness.Status}
	for _, g := range core.Groups {
		out.Conflict = append(out.Conflict, names[g])
	}
	for _, step := range core.Steps {
		checked := "all groups"
		if step.Removed >= 0 {
			checked = "without " + names[step.Removed]
		}
		out.Checks = append(out.Checks, fmt.Sprintf("%s: %s", checked, step.Result.Status))
	}
	return json.MarshalEncode(jsontext.NewEncoder(os.Stdout), out)
}
