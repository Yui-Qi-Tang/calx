//go:build resolverintegration

package mus_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Yui-Qi-Tang/calx/digest"
	"github.com/Yui-Qi-Tang/calx/internal/acceptance"
	"github.com/Yui-Qi-Tang/calx/mus"
	"github.com/Yui-Qi-Tang/calx/sat"
	"github.com/Yui-Qi-Tang/calx/sat/cadical"
)

func TestRealTools(t *testing.T) {
	for _, name := range []string{"CALX_CADICAL", "CALX_CADICAL_DIGEST", "CALX_DRAT", "CALX_DRAT_DIGEST", "CALX_ARTIFACTS"} {
		if os.Getenv(name) == "" {
			t.Fatalf("%s is required for explicit acceptance", name)
		}
	}
	r, err := cadical.New(cadical.Config{SolverPath: os.Getenv("CALX_CADICAL"), SolverDigest: digest.Digest(os.Getenv("CALX_CADICAL_DIGEST")), CheckerPath: os.Getenv("CALX_DRAT"), CheckerDigest: digest.Digest(os.Getenv("CALX_DRAT_DIGEST")), ArtifactDir: filepath.Join(os.Getenv("CALX_ARTIFACTS"), "mus")})
	if err != nil {
		t.Fatal(err)
	}
	// Two conflicts share no clause, so the group order decides which is found.
	c := acceptance.CNF(4, [][]int{{1}, {-1, 2}, {-2}, {3}, {-3, 4}, {-4}, {1, 2, 3}})
	c.Origins = [][]string{{"a1"}, {"a2"}, {"a3"}, {"b1"}, {"b2"}, {"b3"}, {"free"}}
	o, err := mus.ByOrigin(c, func(ctx context.Context, step int, trial sat.CNF) (sat.Result, error) {
		solved, err := r.Solve(ctx, fmt.Sprintf("core-%d", step), trial)
		return solved.Result, err
	})
	if err != nil {
		t.Fatal(err)
	}
	core, err := mus.Shrink(context.Background(), o, sat.Requirements{SATModelChecked: true, UNSATProofChecked: true})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, g := range core.Groups {
		names = append(names, o.Names()[g])
	}
	if !core.Minimal || !slices.Equal(names, []string{"b1", "b2", "b3"}) {
		t.Fatalf("%+v %v", core, names)
	}
	if !core.Witness.Proof.Checked || len(core.Steps) != o.Groups()+1 {
		t.Fatalf("%+v", core)
	}
}
