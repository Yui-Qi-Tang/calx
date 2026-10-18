package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func call(t *testing.T, args []string, raw string) (int, map[string]json.RawMessage) {
	t.Helper()
	var out, errout bytes.Buffer
	status := Run(context.Background(), args, strings.NewReader(raw), &out, &errout)
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatal(out.String(), errout.String(), err)
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
	s := sha256.Sum256(raw)
	return p, hex.EncodeToString(s[:])
}
func TestSolveResultErrorAndExactInput(t *testing.T) {
	solver, sh := script(t, "echo 's SATISFIABLE'; echo 'v 1 0'; exit 10")
	checker, ch := script(t, "exit 1")
	raw := `{"variables":[{"id":1}],"clauses":[[1]]}`
	artifacts := filepath.Join(t.TempDir(), "run")
	args := []string{"solve", "--solver", solver, "--solver-sha256", sh, "--checker", checker, "--checker-sha256", ch, "--artifacts", artifacts}
	status, doc := call(t, args, raw)
	if status != 0 {
		t.Fatal(string(doc["error"]))
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
