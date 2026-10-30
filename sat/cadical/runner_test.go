package cadical

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Yui-Qi-Tang/calx/digest"
	"github.com/Yui-Qi-Tang/calx/sat"
)

func pinnedScript(t *testing.T, body string) (string, digest.Digest) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell process fixture requires POSIX")
	}
	path := filepath.Join(t.TempDir(), "tool")
	raw := []byte("#!/bin/sh\n" + body + "\n")
	if err := os.WriteFile(path, raw, 0700); err != nil {
		t.Fatal(err)
	}
	return path, digest.Sum(raw)
}
func fixtureRunner(t *testing.T, solverBody, checkerBody string, timeout time.Duration) *Runner {
	t.Helper()
	solver, solverHash := pinnedScript(t, solverBody)
	checker, checkerHash := pinnedScript(t, checkerBody)
	r, err := New(Config{SolverPath: solver, SolverDigest: solverHash, CheckerPath: checker, CheckerDigest: checkerHash, ArtifactDir: filepath.Join(t.TempDir(), "evidence"), Limits: sat.Limits{Timeout: timeout}})
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func tinyCNF() sat.CNF {
	return sat.CNF{Variables: []sat.Variable{{ID: 1, Metadata: map[string]string{"kind": "state"}}}, Clauses: [][]int{{1}}}
}
func TestCheckedSATAndArtifactIsolation(t *testing.T) {
	r := fixtureRunner(t, "echo 's SATISFIABLE'; echo 'v 1 0'; exit 10", "exit 1", time.Second)
	result, err := r.Solve(context.Background(), "q0", tinyCNF())
	if err != nil || !(result.Model.Checked || result.Proof.Checked) || result.Status != "SAT" {
		t.Fatalf("%+v %v", result, err)
	}
	if len(result.Artifacts) < 5 {
		t.Fatal("missing evidence")
	}
	for _, a := range result.Artifacts {
		if filepath.IsAbs(a.ID) || strings.Contains(a.ID, "..") {
			t.Fatal(a.ID)
		}
	}
	if _, err = r.Solve(context.Background(), "q0", tinyCNF()); err == nil {
		t.Fatal("overwrote query")
	}
	if _, err = r.Solve(context.Background(), "../escape", tinyCNF()); err == nil {
		t.Fatal("accepted traversal")
	}
}
func TestRejectWrongAssignmentAndCheckerExit(t *testing.T) {
	r := fixtureRunner(t, "echo 's SATISFIABLE'; echo 'v -1 0'; exit 10", "exit 1", time.Second)
	result, err := r.Solve(context.Background(), "wrong", tinyCNF())
	if err == nil || (result.Model.Checked || result.Proof.Checked) {
		t.Fatal("accepted invalid assignment")
	}
	r = fixtureRunner(t, "touch proof.drat; echo 's UNSATISFIABLE'; exit 20", "echo 's VERIFIED'; exit 1", time.Second)
	result, err = r.Solve(context.Background(), "wrong", tinyCNF())
	if err == nil || (result.Model.Checked || result.Proof.Checked) {
		t.Fatal("accepted checker VERIFIED with exit 1")
	}
}
func TestTimeoutKillsToolGroupAndPreservesEvidence(t *testing.T) {
	r := fixtureRunner(t, "sleep 30; echo 's SATISFIABLE'; exit 10", "exit 1", 30*time.Millisecond)
	start := time.Now()
	result, err := r.Solve(context.Background(), "timeout", tinyCNF())
	if !errors.Is(err, context.DeadlineExceeded) || (result.Model.Checked || result.Proof.Checked) {
		t.Fatalf("%+v %v", result, err)
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal("process cancellation did not complete")
	}
	if len(result.Artifacts) < 5 {
		t.Fatal("missing timeout artifacts")
	}
}
func TestPinsAndPrivateCopy(t *testing.T) {
	solver, hash := pinnedScript(t, "echo 's SATISFIABLE'; echo 'v 1 0'; exit 10")
	checker, checkHash := pinnedScript(t, "exit 1")
	cfg := Config{SolverPath: solver, SolverDigest: digest.Digest(strings.Repeat("0", 64)), CheckerPath: checker, CheckerDigest: checkHash, ArtifactDir: filepath.Join(t.TempDir(), "evidence")}
	if runner, err := New(cfg); !errors.Is(err, sat.ErrValidation) || runner != nil {
		t.Fatalf("wrong digest returned runner %v, error %v", runner, err)
	}
	if _, err := os.Stat(cfg.ArtifactDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("invalid digest created artifacts: %v", err)
	}
	cfg.SolverDigest = hash
	r, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(solver, []byte("#!/bin/sh\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	result, err := r.Solve(context.Background(), "private", tinyCNF())
	if err != nil || !(result.Model.Checked || result.Proof.Checked) {
		t.Fatalf("caller edit changed pinned copy: %v", err)
	}
}

func TestNewRequiresBothExecutablePathsAndDigests(t *testing.T) {
	solver, solverDigest := pinnedScript(t, "exit 10")
	checker, checkerDigest := pinnedScript(t, "exit 0")
	for _, test := range []struct {
		name   string
		remove func(*Config)
	}{
		{name: "solver path", remove: func(c *Config) { c.SolverPath = "" }},
		{name: "solver digest", remove: func(c *Config) { c.SolverDigest = "" }},
		{name: "checker path", remove: func(c *Config) { c.CheckerPath = "" }},
		{name: "checker digest", remove: func(c *Config) { c.CheckerDigest = "" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			config := Config{
				SolverPath: solver, SolverDigest: solverDigest,
				CheckerPath: checker, CheckerDigest: checkerDigest,
				ArtifactDir: filepath.Join(t.TempDir(), "run"),
			}
			test.remove(&config)
			if runner, err := New(config); !errors.Is(err, sat.ErrInput) || runner != nil {
				t.Fatalf("runner = %v, error = %v, want invalid input", runner, err)
			}
			if _, err := os.Stat(config.ArtifactDir); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("missing tool input created artifacts: %v", err)
			}
		})
	}
}
func TestAssignmentSyntax(t *testing.T) {
	for _, s := range []string{"v 1 -1 0\n", "v 2 0\n", "v 1\n", "v 0 1\n", "v true 0\n"} {
		if _, err := assignment([]byte(s), 1); err == nil {
			t.Fatalf("accepted %q", s)
		}
	}
}

func TestExactStatusLineHandlesCRWithoutAcceptingWhitespace(t *testing.T) {
	if !exactLine([]byte("\rc progress\n\rs VERIFIED\n"), "s VERIFIED") {
		t.Fatal("CR line separators rejected")
	}
	for _, text := range []string{" s VERIFIED\n", "s VERIFIED extra\n", "c s VERIFIED\n"} {
		if exactLine([]byte(text), "s VERIFIED") {
			t.Fatalf("accepted %q", text)
		}
	}
}

func TestResultIsZeroIncludesAssignment(t *testing.T) {
	if !(Result{}).IsZero() {
		t.Fatal("zero Result must report IsZero")
	}
	if (Result{Assignment: map[int]bool{1: true}}).IsZero() {
		t.Fatal("assignment ignored")
	}
	if (Result{Status: sat.SAT}).IsZero() {
		t.Fatal("embedded result ignored")
	}
	r := fixtureRunner(t, "echo 's UNKNOWN'; exit 0", "exit 1", 10*time.Second)
	got, err := r.Solve(context.Background(), "empty", tinyCNF())
	if !errors.Is(err, sat.ErrInconclusive) {
		t.Fatalf("got %v, want ErrInconclusive", err)
	}
	if got.IsZero() {
		t.Fatal("Solve returned a zero Result")
	}
}

func TestTrivialUNSATCheckerExitOne(t *testing.T) {
	const solver = "touch proof.drat; echo 's UNSATISFIABLE'; exit 20"
	// Lines as upstream DRAT-trim prints them for a formula with an empty clause.
	const trivial = `printf 'c parsing input formula with 1 variables and 1 clauses\nc finished parsing\n\rc trivial UNSAT\ns VERIFIED\n\rc verification time: 0.001 seconds\n'`
	empty := sat.CNF{Variables: []sat.Variable{{ID: 1}}, Clauses: [][]int{{1}, {}}}
	for name, c := range map[string]struct {
		checker string
		cnf     sat.CNF
		method  string // empty when the proof must be rejected
	}{
		"accepted":             {trivial + "; exit 1", empty, "drat-trim/trivial-unsat-empty-clause"},
		"exit zero":            {trivial + "; exit 0", empty, "drat-trim/exit-zero-exact-verified"},
		"no empty clause":      {trivial + "; exit 1", tinyCNF(), ""},
		"not trivial":          {"echo 's VERIFIED'; exit 1", empty, ""},
		"other exit code":      {trivial + "; exit 2", empty, ""},
		"also not verified":    {trivial + "; echo 's NOT VERIFIED'; exit 1", empty, ""},
		"extra status line":    {trivial + "; echo 's DERIVATION'; exit 1", empty, ""},
		"trivial not verified": {"echo 'c trivial UNSAT'; exit 1", empty, ""},
	} {
		t.Run(name, func(t *testing.T) {
			r := fixtureRunner(t, solver, c.checker, 10*time.Second)
			got, err := r.Solve(context.Background(), "q", c.cnf)
			if c.method == "" {
				if !errors.Is(err, sat.ErrValidation) || got.Proof.Checked {
					t.Fatalf("accepted: %+v %v", got.Proof, err)
				}
				return
			}
			if err != nil || got.Proof.Method != c.method {
				t.Fatalf("%+v %v", got.Proof, err)
			}
			if err := got.Require(sat.Requirements{UNSATProofChecked: true}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
