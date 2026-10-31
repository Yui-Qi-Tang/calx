// Command linear demonstrates solving linear integer problems with the pinned
// Z3: a checked SAT model beyond 64 bits, and an UNSAT answer with no proof.
package main

import (
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"flag"
	"log"
	"os"

	"github.com/Yui-Qi-Tang/calx/digest"
	"github.com/Yui-Qi-Tang/calx/linear"
	"github.com/Yui-Qi-Tang/calx/linear/z3"
	"github.com/Yui-Qi-Tang/calx/sat"
)

func main() {
	artifacts := flag.String("artifacts", "artifacts/go-linear", "new run directory; parent must exist and the run directory must not exist")
	flag.Parse()
	if err := run(context.Background(), *artifacts); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, artifacts string) error {
	// 1. Pin the solver.
	runner, err := z3.New(z3.Config{
		SolverPath:   os.Getenv("CALX_Z3"),
		SolverDigest: digest.Digest(os.Getenv("CALX_Z3_DIGEST")),
		ArtifactDir:  artifacts,
	})
	if err != nil {
		return err
	}

	// 2. x + y = 2^64 and x - y = 2. x does not fit in an int64.
	pair := linear.Problem{
		Integers: []sat.Variable{{ID: 1}, {ID: 2}},
		Constraints: []linear.Constraint{{
			Terms:    []linear.Term{{Variable: 1, Coefficient: "1"}, {Variable: 2, Coefficient: "1"}},
			Relation: linear.Equal,
			RHS:      "18446744073709551616",
		}, {
			Terms:    []linear.Term{{Variable: 1, Coefficient: "1"}, {Variable: 2, Coefficient: "-1"}},
			Relation: linear.Equal,
			RHS:      "2",
		}},
	}
	solved, err := runner.Solve(ctx, "pair", pair)
	if err != nil {
		return err
	}
	// The model was evaluated against pair with exact arithmetic.
	if err := solved.Require(sat.Requirements{SATModelChecked: true}); err != nil {
		return err
	}

	// 3. 2x = 1 has a real solution but no integer one.
	half := linear.Problem{
		Integers: []sat.Variable{{ID: 1}},
		Constraints: []linear.Constraint{{
			Terms:    []linear.Term{{Variable: 1, Coefficient: "2"}},
			Relation: linear.Equal,
			RHS:      "1",
		}},
	}
	refuted, err := runner.Solve(ctx, "half", half)
	if err != nil {
		return err
	}

	// 4. The answer is Z3's report alone, so requiring a proof must fail.
	err = refuted.Require(sat.Requirements{UNSATProofChecked: true})
	if !errors.Is(err, sat.ErrGuarantee) {
		return errors.New("an unproved UNSAT met the proof requirement")
	}

	return json.MarshalEncode(jsontext.NewEncoder(os.Stdout), struct {
		Pair        sat.Status `json:"pair"`
		X           string     `json:"x"`
		Y           string     `json:"y"`
		ModelMethod string     `json:"model_method"`
		Half        sat.Status `json:"half"`
		HalfProof   bool       `json:"half_proof_present"`
		ProofError  string     `json:"proof_requirement"`
	}{
		Pair: solved.Status, X: solved.Assignment.Integers[1], Y: solved.Assignment.Integers[2],
		ModelMethod: solved.Model.Method,
		Half:        refuted.Status, HalfProof: refuted.Proof.Present, ProofError: err.Error(),
	})
}
