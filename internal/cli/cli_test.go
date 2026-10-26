package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Yui-Qi-Tang/calx/digest"
	"github.com/Yui-Qi-Tang/calx/sat"
)

func call(t *testing.T, args []string, raw string) (int, map[string]json.RawMessage) {
	t.Helper()
	var out, errout bytes.Buffer
	status := Run(context.Background(), args, strings.NewReader(raw), &out, &errout)
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatal(out.String(), errout.String(), err)
	}
	if string(doc["schema_version"]) != `"calx-cli/v2"` {
		t.Fatalf("unexpected schema: %s", doc["schema_version"])
	}
	return status, doc
}
func TestProjectAndReconstructCLI(t *testing.T) {
	for _, raw := range []string{`{"node_ids":[],"edges":[]}`, `{"node_ids":["a"],"edges":[{"id":"loop","from":"a","to":"a","relation":"r"}]}`} {
		status, doc := call(t, []string{"project"}, raw)
		if status != 0 {
			t.Fatal(doc)
		}
		status, _ = call(t, []string{"reconstruct"}, string(doc["data"]))
		if status != 0 {
			t.Fatal("cannot reconstruct own output")
		}
	}
	for _, raw := range []string{`{}`, `{"node_ids":[],"edges":null}`, `{"NodeIDs":[],"edges":[]}`, `{"node_ids":[],"node_ids":[],"edges":[]}`} {
		status, _ := call(t, []string{"project"}, raw)
		if status == 0 {
			t.Fatal(raw)
		}
	}
}
func TestEncodeRequiresExplicitHorizonAndProfile(t *testing.T) {
	raw := `{"model":{"fields":{"x":["a","b"]},"actions":[{"id":"ab","guard":{"x":"a"},"effect":{"x":"b"},"frame":[]}]},"query":{"profile":"categorical-simple-first-hit/v1","start":{"x":"a"},"goal":{"x":"b"},"horizon":1}}`
	if status, _ := call(t, []string{"encode"}, raw); status != 0 {
		t.Fatal("encode failed")
	}
	for _, bad := range []string{strings.Replace(raw, `,"horizon":1`, "", 1), strings.Replace(raw, `"horizon":1`, `"horizon":null`, 1), strings.Replace(raw, `"horizon":1`, `"horizon":-1`, 1), strings.Replace(raw, `"x":"b"`, `"x":true`, 1), strings.Replace(raw, "categorical-simple-first-hit/v1", "all-walks", 1)} {
		if status, _ := call(t, []string{"encode"}, bad); status == 0 {
			t.Fatal(bad)
		}
	}
}
func script(t *testing.T, body string) (string, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX fixture")
	}
	raw := []byte("#!/bin/sh\n" + body + "\n")
	p := filepath.Join(t.TempDir(), "tool")
	if e := os.WriteFile(p, raw, 0700); e != nil {
		t.Fatal(e)
	}
	return p, digest.Sum(raw).String()
}
func TestSolveResultErrorAndExactInput(t *testing.T) {
	solver, sh := script(t, "echo 's SATISFIABLE'; echo 'v 1 0'; exit 10")
	checker, ch := script(t, "exit 1")
	raw := `{"variables":[{"id":1}],"clauses":[[1]]}`
	artifacts := filepath.Join(t.TempDir(), "run")
	args := []string{"solve", "--solver", solver, "--solver-digest", sh, "--checker", checker, "--checker-digest", ch, "--artifacts", artifacts}
	status, doc := call(t, args, raw)
	if status != 0 {
		t.Fatal(string(doc["error"]))
	}
	var data struct {
		Result struct {
			Solver  string `json:"solver_digest"`
			Checker string `json:"checker_digest"`
		} `json:"result"`
	}
	if err := json.Unmarshal(doc["data"], &data); err != nil {
		t.Fatal(err)
	}
	if data.Result.Solver != sh || data.Result.Checker != ch {
		t.Fatal("result digests do not identify the configured tools")
	}
	saved, e := os.ReadFile(filepath.Join(artifacts, "request.input"))
	if e != nil || string(saved) != raw {
		t.Fatal("lost original input")
	}
	args[len(args)-1] = filepath.Join(t.TempDir(), "bad")
	status, doc = call(t, args, strings.Replace(raw, "[[1]]", "[[-1]]", 1))
	if status == 0 || !bytes.Contains(doc["data"], []byte(`"SAT"`)) || doc["error"] == nil {
		t.Fatal("failure did not preserve separate answer/error", doc)
	}
}

func TestInconclusiveCommandsRetainDiagnosticResults(t *testing.T) {
	solver, solverDigest := script(t, "echo 's UNKNOWN'; exit 0")
	checker, checkerDigest := script(t, "exit 1")
	for _, test := range []struct{ command, input string }{
		{"solve", `{"variables":[{"id":1}],"clauses":[[1]]}`},
		{"solve-finite", `{"model":{"fields":{"x":["a","b"]},"actions":[{"id":"ab","guard":{"x":"a"},"effect":{"x":"b"},"frame":[]}]},"query":{"profile":"categorical-simple-first-hit/v1","start":{"x":"a"},"goal":{"x":"b"},"horizon":1}}`},
	} {
		t.Run(test.command, func(t *testing.T) {
			artifacts := filepath.Join(t.TempDir(), "run")
			args := []string{test.command, "--solver", solver, "--solver-digest", solverDigest, "--checker", checker, "--checker-digest", checkerDigest, "--artifacts", artifacts}
			exit, doc := call(t, args, test.input)
			if exit != 1 || string(doc["outcome"]) != `"ERROR"` {
				t.Fatalf("exit = %d, outcome = %s", exit, doc["outcome"])
			}
			var gotError failure
			if err := json.Unmarshal(doc["error"], &gotError); err != nil {
				t.Fatal(err)
			}
			if gotError.Code != "INCONCLUSIVE" || !strings.Contains(gotError.Message, sat.ErrInconclusive.Error()) {
				t.Fatalf("unexpected error: %+v", gotError)
			}
			var data struct {
				Result sat.Result `json:"result"`
			}
			if err := json.Unmarshal(doc["data"], &data); err != nil {
				t.Fatal(err)
			}
			if data.Result.Status != sat.Unknown || data.Result.Model.Checked || data.Result.Proof.Checked || len(data.Result.Artifacts) == 0 {
				t.Fatalf("diagnostic result was lost or accepted: %+v", data.Result)
			}
			raw, err := os.ReadFile(filepath.Join(artifacts, "query", "outcome.json"))
			if err != nil || !bytes.Contains(raw, []byte(sat.ErrInconclusive.Error())) {
				t.Fatalf("saved outcome = %s, error = %v", raw, err)
			}
		})
	}
}

func TestContextErrorCodesTakePrecedenceOverInconclusive(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
		want string
	}{
		{name: "inconclusive", err: sat.ErrInconclusive, want: "INCONCLUSIVE"},
		{name: "canceled", err: errors.Join(sat.ErrInconclusive, context.Canceled), want: "CANCELED"},
		{name: "timeout", err: errors.Join(sat.ErrInconclusive, context.DeadlineExceeded), want: "TIMEOUT"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := code(test.err); got != test.want {
				t.Fatalf("code = %s, want %s", got, test.want)
			}
		})
	}
}
