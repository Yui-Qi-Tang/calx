package z3

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
	"github.com/Yui-Qi-Tang/calx/linear"
	"github.com/Yui-Qi-Tang/calx/sat"
)

func pinnedScript(t *testing.T, body string) (string, digest.Digest) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX process fixture")
	}
	file := filepath.Join(t.TempDir(), "z3")
	raw := []byte("#!/bin/sh\n" + body + "\n")
	if err := os.WriteFile(file, raw, 0700); err != nil {
		t.Fatal(err)
	}
	return file, digest.Sum(raw)
}

func fixture(t *testing.T, body string, timeout time.Duration) *Runner {
	t.Helper()
	file, pin := pinnedScript(t, body)
	r, err := New(Config{SolverPath: file, SolverDigest: pin, ArtifactDir: filepath.Join(t.TempDir(), "runs"), Limits: sat.Limits{Timeout: timeout}})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func oneInteger() linear.Problem {
	return linear.Problem{Integers: []sat.Variable{{ID: 1}}, Constraints: []linear.Constraint{{Terms: []linear.Term{{Variable: 1, Coefficient: "1"}}, Relation: linear.Equal, RHS: "1"}}}
}

func TestUNSATCarriesNoProof(t *testing.T) {
	got, err := fixture(t, "echo unsat", 5*time.Second).Solve(context.Background(), "unsat", linear.Problem{})
	if err != nil || got.Status != sat.UNSAT {
		t.Fatalf("%+v %v", got, err)
	}
	if got.Proof.Present || got.Proof.Checked || got.CheckerDigest != "" {
		t.Fatal("fabricated proof")
	}
	if !errors.Is(got.Require(sat.Requirements{UNSATProofChecked: true}), sat.ErrGuarantee) {
		t.Fatal("accepted missing guarantee")
	}
	if err := got.Require(sat.Requirements{SATModelChecked: true}); err != nil {
		t.Fatal(err)
	}
	if len(got.Artifacts) < 5 {
		t.Fatal("missing evidence")
	}
}

func TestUnknownIsInconclusive(t *testing.T) {
	got, err := fixture(t, "echo unknown", 5*time.Second).Solve(context.Background(), "unknown", linear.Problem{})
	if !errors.Is(err, sat.ErrInconclusive) || got.Status != sat.Unknown || got.IsZero() {
		t.Fatalf("%+v %v", got, err)
	}
	if !errors.Is(got.Require(sat.Requirements{}), sat.ErrInconclusive) {
		t.Fatal("accepted UNKNOWN")
	}
}

func TestCheckedSATBindsInputAndModel(t *testing.T) {
	r := fixture(t, `echo sat; if [ "$2" = model.smt2 ]; then echo '((i1 1))'; fi`, 5*time.Second)
	got, err := r.Solve(context.Background(), "sat", oneInteger())
	if err != nil || got.Status != sat.SAT || got.Assignment.Integers[1] != "1" {
		t.Fatalf("%+v %v", got, err)
	}
	if err := got.Require(sat.Requirements{SATModelChecked: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Solve(context.Background(), "sat", oneInteger()); err == nil {
		t.Fatal("overwrote query")
	}
	if _, err := r.Solve(context.Background(), "../escape", oneInteger()); !errors.Is(err, sat.ErrInput) {
		t.Fatal(err)
	}
}

func TestEmptyProblemNeedsNoValues(t *testing.T) {
	r := fixture(t, "echo sat", 5*time.Second)
	got, err := r.Solve(context.Background(), "empty", linear.Problem{})
	if err != nil || !got.Model.Checked {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestBadModelAndProcessResponses(t *testing.T) {
	for _, c := range []struct {
		name, body string
		want       error
	}{
		{"no-model", "echo sat", sat.ErrProtocol},
		{"wrong-model", "echo sat; if [ \"$2\" = model.smt2 ]; then echo '((i1 2))'; fi", sat.ErrValidation},
		{"type-mismatch", "echo sat; if [ \"$2\" = model.smt2 ]; then echo '((i1 true))'; fi", sat.ErrProtocol},
		{"bad-exit", "echo unsat; exit 1", sat.ErrProtocol},
		{"garbage", "echo maybe", sat.ErrProtocol},
		{"output-limit", "yes sat | head -c 2000000", sat.ErrLimit},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := fixture(t, c.body, 5*time.Second)
			r.config.Limits.MaxOutputBytes = 1 << 16
			got, err := r.Solve(context.Background(), c.name, oneInteger())
			if !errors.Is(err, c.want) || got.Model.Checked {
				t.Fatalf("%+v %v", got, err)
			}
		})
	}
}

func TestTimeoutPreservesEvidence(t *testing.T) {
	r := fixture(t, "sleep 30", 50*time.Millisecond)
	start := time.Now()
	got, err := r.Solve(context.Background(), "timeout", oneInteger())
	if !errors.Is(err, context.DeadlineExceeded) || got.Status != sat.Unknown {
		t.Fatalf("%+v %v", got, err)
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal("process cancellation did not complete")
	}
	if len(got.Artifacts) < 4 {
		t.Fatal("missing timeout artifacts")
	}
}

func TestInputLimitsPrecedeTheTool(t *testing.T) {
	r := fixture(t, "exit 1", 5*time.Second)
	r.config.Limits.MaxVariables = 1
	p := linear.Problem{Integers: []sat.Variable{{ID: 1}, {ID: 2}}}
	if _, err := r.Solve(context.Background(), "limit", p); !errors.Is(err, sat.ErrLimit) {
		t.Fatal(err)
	}
	r.config.Limits.MaxVariables = 10
	r.config.Limits.MaxInputBytes = 8
	if _, err := r.Solve(context.Background(), "bytes", p); !errors.Is(err, sat.ErrLimit) {
		t.Fatal(err)
	}
}

func TestPinsAndUninitializedRunner(t *testing.T) {
	file, pin := pinnedScript(t, "echo unsat")
	dir := filepath.Join(t.TempDir(), "runs")
	if r, err := New(Config{SolverPath: file, SolverDigest: digest.Digest(strings.Repeat("0", 64)), ArtifactDir: dir}); !errors.Is(err, sat.ErrValidation) || r != nil {
		t.Fatalf("%v %v", r, err)
	}
	if r, err := New(Config{SolverPath: "", SolverDigest: pin, ArtifactDir: dir}); !errors.Is(err, sat.ErrInput) || r != nil {
		t.Fatalf("%v %v", r, err)
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed New created artifacts: %v", err)
	}
	r, err := New(Config{SolverPath: file, SolverDigest: pin, ArtifactDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("#!/bin/sh\necho sat\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if got, err := r.Solve(context.Background(), "private", linear.Problem{}); err != nil || got.Status != sat.UNSAT {
		t.Fatalf("caller edit changed pinned copy: %+v %v", got, err)
	}
	var missing *Runner
	if got, err := missing.Solve(context.Background(), "nil", linear.Problem{}); !errors.Is(err, sat.ErrInput) || got.IsZero() {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestResultIsZeroIncludesAssignment(t *testing.T) {
	if !(Result{}).IsZero() {
		t.Fatal("zero Result must report IsZero")
	}
	if (Result{Assignment: linear.Assignment{Integers: map[int]string{1: "0"}}}).IsZero() {
		t.Fatal("assignment ignored")
	}
	if (Result{Result: sat.Result{Status: sat.SAT}}).IsZero() {
		t.Fatal("embedded result ignored")
	}
}

func TestInputJSONIsTheModelCheckTarget(t *testing.T) {
	r := fixture(t, "echo unsat", 5*time.Second)
	p := linear.Problem{
		Booleans:    sat.CNF{Variables: []sat.Variable{{ID: 1, Metadata: map[string]string{"k": "v"}}}, Clauses: [][]int{{1}}, Origins: [][]string{{"o"}}},
		Integers:    []sat.Variable{{ID: 1}},
		Constraints: []linear.Constraint{{Terms: []linear.Term{{Variable: 1, Coefficient: "-1"}}, Relation: linear.AtMost, RHS: "0", Origins: []string{"p"}}},
	}
	got, err := r.Solve(context.Background(), "input", p)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(r.root, "input", "input.json"))
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"booleans":{"variables":[{"id":1,"metadata":{"k":"v"}}],"clauses":[[1]],"origins":[["o"]]},"integers":[{"id":1}],"constraints":[{"terms":[{"variable":1,"coefficient":"-1"}],"relation":"\u003c=","rhs":"0","origins":["p"]}]}`
	if string(raw) != want {
		t.Fatalf("got %s", raw)
	}
	if digest.Sum(raw) != got.InputDigest {
		t.Fatal("InputDigest does not identify input.json")
	}
}

func TestModelParserRejectsPartialAndExtraData(t *testing.T) {
	for _, s := range []string{"", "unsat", "sat", "sat ((i1 2))", "sat ((b1 true)(b1 false))", "sat ((b1 1)(i1 2))", "sat ((b1 true)(i1 (/ 1 2)))", "sat ((b1 true)(i1 2)) extra", "sat ((b2 true)(i1 2))", "sat ((b1 true)(i2 2))", "sat ((b1 true)(i1 02))", "sat ((b01 true)(i1 2))", "sat ((x1 true)(i1 2))"} {
		if _, err := parseAssignment([]byte(s), 1, 1); !errors.Is(err, errModel) {
			t.Fatalf("accepted %q: %v", s, err)
		}
	}
	a, err := parseAssignment([]byte("sat\n((b1 false) (i1 (- 22)))"), 1, 1)
	if err != nil || a.Integers[1] != "-22" || a.Booleans[1] {
		t.Fatalf("%+v %v", a, err)
	}
	if _, err := parseAssignment([]byte("sat extra"), 0, 0); !errors.Is(err, errModel) {
		t.Fatal(err)
	}
}
