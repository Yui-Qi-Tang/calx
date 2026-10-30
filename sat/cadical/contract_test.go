package cadical

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Yui-Qi-Tang/calx/sat"
)

func TestRejectedToolResponses(t *testing.T) {
	cases := []struct {
		name, solver, checker string
		want                  error
		status                sat.Status
	}{
		{"wrong-exit", "echo 's SATISFIABLE'; echo 'v 1 0'; exit 0", "exit 0", sat.ErrProtocol, sat.Unknown},
		{"sat-and-unknown", "echo 's SATISFIABLE'; echo 's UNKNOWN'; echo 'v 1 0'; exit 10", "exit 0", sat.ErrProtocol, sat.Unknown},
		{"dual-status", "echo 's SATISFIABLE'; echo 's UNSATISFIABLE'; exit 10", "exit 0", sat.ErrProtocol, sat.Unknown},
		{"no-model", "echo 's SATISFIABLE'; exit 10", "exit 0", sat.ErrProtocol, sat.SAT},
		{"partial-model", "echo 's SATISFIABLE'; echo 'v 0'; exit 10", "exit 0", sat.ErrProtocol, sat.SAT},
		{"missing-proof", "echo 's UNSATISFIABLE'; exit 20", "exit 0", sat.ErrProtocol, sat.UNSAT},
		{"false-proof", "touch proof.drat; echo 's UNSATISFIABLE'; exit 20", "echo 's NOT VERIFIED'; exit 0", sat.ErrValidation, sat.UNSAT},
		{"incorrect-model", "echo 's SATISFIABLE'; echo 'v -1 0'; exit 10", "exit 0", sat.ErrValidation, sat.SAT},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := fixtureRunner(t, c.solver, c.checker, 5*time.Second)
			got, err := r.Solve(context.Background(), c.name, tinyCNF())
			if !errors.Is(err, c.want) || got.Status != c.status || got.Model.Checked || got.Proof.Checked {
				t.Fatalf("got %+v, %v", got, err)
			}
			if _, err := os.Stat(filepath.Join(r.root, c.name, "outcome.json")); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestInconclusivePreservesStatusAndArtifacts(t *testing.T) {
	r := fixtureRunner(t, "echo 's UNKNOWN'; exit 0", "exit 1", time.Second)
	got, err := r.Solve(context.Background(), "unknown", tinyCNF())
	if !errors.Is(err, sat.ErrInconclusive) || got.Status != sat.Unknown || got.Model.Checked || got.Proof.Checked {
		t.Fatalf("%+v %v", got, err)
	}
	if !errors.Is(got.Require(sat.Requirements{}), sat.ErrInconclusive) {
		t.Fatal("Require lost the inconclusive classification")
	}
	files := map[string][]byte{}
	for _, artifact := range got.Artifacts {
		raw, readErr := r.ReadArtifact(artifact)
		if readErr != nil {
			t.Fatal(readErr)
		}
		files[filepath.Base(artifact.ID)] = raw
	}
	for _, name := range []string{"input.json", "input.cnf", "solver.stdout", "solver.stderr", "solver.json", "outcome.json"} {
		if _, ok := files[name]; !ok {
			t.Errorf("missing diagnostic artifact %s", name)
		}
	}
	if string(files["solver.stdout"]) != "s UNKNOWN\n" {
		t.Fatalf("solver output = %q", files["solver.stdout"])
	}
	var outcome struct {
		Result struct {
			Status sat.Status `json:"status"`
		} `json:"result"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(files["outcome.json"], &outcome); err != nil {
		t.Fatal(err)
	}
	if outcome.Result.Status != sat.Unknown || !strings.Contains(outcome.Error, sat.ErrInconclusive.Error()) {
		t.Fatalf("saved outcome lost status or error: %+v", outcome)
	}
	if _, err := os.Stat(filepath.Join(r.root, "unknown", "checker.stdout")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("UNKNOWN unexpectedly ran the proof checker: %v", err)
	}
}

func TestLimitsAndCancellation(t *testing.T) {
	r := fixtureRunner(t, "echo 's UNKNOWN'; exit 0", "exit 1", time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := r.Solve(ctx, "canceled", tinyCNF()); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	r.config.Limits.MaxVariables = 1
	c := tinyCNF()
	c.Variables = append(c.Variables, sat.Variable{ID: 2})
	if _, err := r.Solve(context.Background(), "limit", c); !errors.Is(err, sat.ErrLimit) {
		t.Fatal(err)
	}
	r = fixtureRunner(t, "while :; do echo lots-of-output; done", "exit 1", time.Second)
	r.config.Limits.MaxOutputBytes = 64
	if _, err := r.Solve(context.Background(), "output", tinyCNF()); !errors.Is(err, sat.ErrLimit) {
		t.Fatal(err)
	}
	r = fixtureRunner(t, "printf 'proof-too-large' > proof.drat; echo 's UNSATISFIABLE'; exit 20", "exit 1", time.Second)
	r.config.MaxProofBytes = 2
	if _, err := r.Solve(context.Background(), "proof-limit", tinyCNF()); !errors.Is(err, sat.ErrLimit) {
		t.Fatal(err)
	}
}

func TestConcurrentIsolationAndMapping(t *testing.T) {
	r := fixtureRunner(t, "echo 's SATISFIABLE'; echo 'v 1 0'; exit 10", "exit 1", time.Second)
	c := tinyCNF()
	c.Variables[0].Metadata = map[string]string{"kind": "action", "time": "0", "action": "rotate", "other_time": "2"}
	c.Origins = [][]string{{"rule-power-to-light", "fact-power"}}
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("parallel-%d", i)
			got, err := r.Solve(context.Background(), id, c)
			if err != nil || !got.Model.Checked {
				t.Errorf("%s: %v", id, err)
				return
			}
			raw, err := os.ReadFile(filepath.Join(r.root, id, "input.json"))
			if err != nil {
				t.Error(err)
				return
			}
			var saved cnfJSON
			if err = json.Unmarshal(raw, &saved); err != nil || !reflect.DeepEqual(saved, inputJSON(c)) {
				t.Errorf("mapping changed: %v", err)
			}
		}(i)
	}
	wg.Wait()
	// Simultaneous identical IDs must have exactly one owner.
	results := make(chan error, 2)
	for range 2 {
		go func() { _, err := r.Solve(context.Background(), "same", c); results <- err }()
	}
	e1, e2 := <-results, <-results
	if (e1 == nil) == (e2 == nil) {
		t.Fatalf("expected one winner: %v / %v", e1, e2)
	}
}
