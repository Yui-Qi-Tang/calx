// Command solve demonstrates checked SAT/UNSAT and finite trace decoding.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"os"

	"github.com/Yui-Qi-Tang/calx/digest"
	"github.com/Yui-Qi-Tang/calx/finite"
	"github.com/Yui-Qi-Tang/calx/sat"
	"github.com/Yui-Qi-Tang/calx/sat/cadical"
)

func main() {
	horizon := flag.Int("horizon", 1, "exact action count (not an upper bound); 0 checks the start state")
	artifacts := flag.String("artifacts", "artifacts/go-solve", "new run directory; parent must exist and the run directory must not exist")
	flag.Parse()
	if err := run(context.Background(), *horizon, *artifacts); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, horizon int, artifacts string) error {
	// 1. Declare the model and the exact question.
	model := finite.Model{
		Fields: map[string][]string{"state": {"off", "on"}},
		Actions: []finite.Action{{
			ID:     "enable",
			Guard:  finite.State{"state": "off"},
			Effect: finite.State{"state": "on"},
		}},
	}
	query := finite.Query{
		Profile: finite.Profile,
		Start:   finite.State{"state": "off"},
		Goal:    finite.State{"state": "on"},
		Horizon: horizon,
	}

	// 2. Compile the model/query to a formula and its decoding map.
	encoding, err := finite.Compile(ctx, model, query, finite.Options{})
	if err != nil {
		return err
	}

	// 3. Configure pinned tools and solve the formula.
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
	formula, err := encoding.CNF()
	if err != nil {
		return err
	}
	result, err := runner.Solve(ctx, "example", formula)
	if err != nil {
		return err // An answer label can survive a failed validation.
	}

	// 4. Require the evidence appropriate to SAT or UNSAT.
	if err := result.Require(sat.Requirements{
		SATModelChecked:   true,
		UNSATProofChecked: true,
	}); err != nil {
		return err
	}

	// 5. Only a checked SAT assignment can be decoded into a trace.
	var trace *finite.Trace
	if result.Status == sat.SAT {
		decoded, err := encoding.Decode(result.Assignment)
		if err != nil {
			return err
		}
		trace = &decoded
	}
	return json.NewEncoder(os.Stdout).Encode(struct {
		Status  sat.Status    `json:"status"`
		Horizon int           `json:"horizon"`
		Trace   *finite.Trace `json:"trace,omitempty"`
	}{Status: result.Status, Horizon: horizon, Trace: trace})
}
