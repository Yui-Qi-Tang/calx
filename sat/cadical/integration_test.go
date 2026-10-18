//go:build resolverintegration

package cadical

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/Yui-Qi-Tang/calx/internal/acceptance"
	"github.com/Yui-Qi-Tang/calx/sat"
)

func TestRealTools(t *testing.T) {
	for _, name := range []string{"CALX_CADICAL", "CALX_CADICAL_SHA256", "CALX_DRAT", "CALX_DRAT_SHA256", "CALX_ARTIFACTS"} {
		if os.Getenv(name) == "" {
			t.Fatalf("%s is required for explicit acceptance", name)
		}
	}
	r, err := New(Config{SolverPath: os.Getenv("CALX_CADICAL"), SolverSHA256: os.Getenv("CALX_CADICAL_SHA256"), CheckerPath: os.Getenv("CALX_DRAT"), CheckerSHA256: os.Getenv("CALX_DRAT_SHA256"), ArtifactDir: filepath.Join(os.Getenv("CALX_ARTIFACTS"), "cadical")})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"sat", "unsat"} {
		t.Run("pouch-"+name, func(t *testing.T) {
			adapter := pouchAdapter{runner: r}
			got, err := adapter.Solve(context.Background(), "pouch-"+name, pouchFixture(t, name))
			if err != nil || !got.Verified || got.Status != strings.ToUpper(name) {
				t.Fatalf("%+v %v", got, err)
			}
		})
	}
	cases := acceptance.Cases()
	names := make([]string, 0, len(cases))
	for name := range cases {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			c := cases[name]
			got, err := r.Solve(context.Background(), name, c)
			if err != nil {
				t.Fatal(err)
			}
			if got.Status != acceptance.Oracle(c) {
				t.Fatalf("status %s", got.Status)
			}
			if err = got.Require(sat.Requirements{SATModelChecked: true, UNSATProofChecked: true}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
