package mus_test

import (
	"context"
	"fmt"

	"github.com/Yui-Qi-Tang/calx/internal/acceptance"
	"github.com/Yui-Qi-Tang/calx/mus"
	"github.com/Yui-Qi-Tang/calx/sat"
)

// The formula is unsatisfiable, but "unrelated" plays no part in the conflict.
// A real caller solves with a pinned runner and sets UNSATProofChecked.
func ExampleShrink() {
	formula := acceptance.CNF(3, [][]int{{1}, {-1, 2}, {-2}, {3}})
	formula.Origins = [][]string{{"power-on"}, {"power-implies-light"}, {"light-off"}, {"unrelated"}}
	oracle, err := mus.ByOrigin(formula, func(_ context.Context, _ int, c sat.CNF) (sat.Result, error) {
		return sat.Result{Status: acceptance.Oracle(c)}, nil // Unchecked brute force.
	})
	if err != nil {
		panic(err)
	}
	core, err := mus.Shrink(context.Background(), oracle, sat.Requirements{})
	if err != nil {
		panic(err)
	}
	for _, g := range core.Groups {
		if _, err := fmt.Println(oracle.Names()[g]); err != nil {
			panic(err)
		}
	}
	if _, err := fmt.Println("minimal:", core.Minimal, "checks:", len(core.Steps)); err != nil {
		panic(err)
	}
	// Output:
	// light-off
	// power-implies-light
	// power-on
	// minimal: true checks: 5
}
