package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Yui-Qi-Tang/calx/digest"
)

func pinned(t *testing.T, solver, checker string) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "tools.json")
	var stdout, stderr bytes.Buffer
	input := &trackingReader{}
	if got := Run(context.Background(), []string{"pin-tools", "--solver", solver, "--checker", checker, "--out", out}, input, &stdout, &stderr); got != 0 {
		t.Fatalf("pin-tools exit %d: %s %s", got, &stdout, &stderr)
	}
	if input.read {
		t.Fatal("pin-tools read stdin")
	}
	return out
}

func TestPinToolsThenSolveWithManifest(t *testing.T) {
	solver, solverDigest := script(t, "echo 's SATISFIABLE'; echo 'v 1 0'; exit 10")
	checker, checkerDigest := script(t, "exit 1")
	manifest := pinned(t, solver, checker)

	var m toolManifest
	raw, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	want := toolManifest{Schema: toolsSchema, Solver: toolPin{Path: solver, Digest: digest.Digest(solverDigest)}, Checker: toolPin{Path: checker, Digest: digest.Digest(checkerDigest)}}
	if m != want {
		t.Fatalf("manifest %+v, want %+v", m, want)
	}

	args := []string{"solve", "--tools", manifest, "--artifacts", filepath.Join(t.TempDir(), "run")}
	status, doc := call(t, args, `{"variables":[{"id":1}],"clauses":[[1]]}`)
	if status != 0 || string(doc["outcome"]) != `"OK"` {
		t.Fatalf("status %d: %s", status, doc["error"])
	}
}

func TestPinToolsRelativePathsBecomeAbsolute(t *testing.T) {
	solver, _ := script(t, "exit 0")
	checker, _ := script(t, "exit 0")
	t.Chdir(filepath.Dir(solver))
	manifest := pinned(t, filepath.Base(solver), checker)
	m, err := loadManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if m.Solver.Path != solver {
		t.Fatalf("solver path %q, want %q", m.Solver.Path, solver)
	}
}

func TestPinToolsFailures(t *testing.T) {
	tool, _ := script(t, "exit 0")
	existing := filepath.Join(t.TempDir(), "tools.json")
	if err := os.WriteFile(existing, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	plain := filepath.Join(t.TempDir(), "plain")
	if err := os.WriteFile(plain, []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}
	fresh := func() string { return filepath.Join(t.TempDir(), "tools.json") }
	for name, test := range map[string]struct {
		args       []string
		exit       int
		diagnostic string
	}{
		"missing out":     {[]string{"--solver", tool, "--checker", tool}, 2, "--out is required"},
		"missing checker": {[]string{"--solver", tool, "--out", fresh()}, 2, "--checker is required"},
		"digest flag":     {[]string{"--solver", tool, "--checker", tool, "--out", fresh(), "--solver-digest", strings.Repeat("a", 64)}, 2, "flag provided but not defined"},
		"existing out":    {[]string{"--solver", tool, "--checker", tool, "--out", existing}, 1, "manifest must be a new file"},
		"not executable":  {[]string{"--solver", plain, "--checker", tool, "--out", fresh()}, 1, "not an executable regular file"},
		"missing tool":    {[]string{"--solver", tool, "--checker", filepath.Join(t.TempDir(), "absent"), "--out", fresh()}, 1, "checker:"},
	} {
		t.Run(name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			input := &trackingReader{}
			got := Run(context.Background(), append([]string{"pin-tools"}, test.args...), input, &stdout, &stderr)
			if got != test.exit || input.read {
				t.Fatalf("exit %d, read %v: %s %s", got, input.read, &stdout, &stderr)
			}
			if !strings.Contains(stdout.String()+stderr.String(), test.diagnostic) {
				t.Fatalf("output %q %q, want %q", &stdout, &stderr, test.diagnostic)
			}
		})
	}
	if raw, err := os.ReadFile(existing); err != nil || string(raw) != "keep" {
		t.Fatalf("existing manifest changed: %q %v", raw, err)
	}
}

func TestToolsManifestIsValidatedBeforeInput(t *testing.T) {
	tool, toolDigest := script(t, "exit 0")
	good := `{"schema_version":"calx-tools/v1","solver":{"path":"` + tool + `","digest":"` + toolDigest + `"},"checker":{"path":"` + tool + `","digest":"` + toolDigest + `"}}`
	write := func(content string) string {
		p := filepath.Join(t.TempDir(), "tools.json")
		if err := os.WriteFile(p, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	for name, test := range map[string]struct {
		manifest   string
		extra      []string
		diagnostic string
	}{
		"absent file":      {filepath.Join(t.TempDir(), "absent.json"), nil, "tool manifest"},
		"combined flag":    {write(good), []string{"--solver", tool}, "--tools cannot be combined with --solver"},
		"combined digest":  {write(good), []string{"--checker-digest", toolDigest}, "--tools cannot be combined with --checker-digest"},
		"wrong schema":     {write(strings.Replace(good, "calx-tools/v1", "calx-tools/v0", 1)), nil, "schema_version"},
		"unknown field":    {write(strings.Replace(good, `"schema_version"`, `"extra":1,"schema_version"`, 1)), nil, "unknown JSON field"},
		"missing checker":  {write(`{"schema_version":"calx-tools/v1","solver":{"path":"` + tool + `","digest":"` + toolDigest + `"}}`), nil, `missing field "checker"`},
		"relative path":    {write(strings.Replace(good, `"path":"`+tool, `"path":"tool`, 1)), nil, "solver path must be absolute"},
		"malformed digest": {write(strings.Replace(good, toolDigest+`"},"checker"`, `abc"},"checker"`, 1)), nil, "solver digest"},
		"null":             {write(`null`), nil, "null is not supported"},
	} {
		t.Run(name, func(t *testing.T) {
			args := append([]string{"solve-finite", "--tools", test.manifest, "--artifacts", filepath.Join(t.TempDir(), "run")}, test.extra...)
			var stdout, stderr bytes.Buffer
			input := &trackingReader{}
			if got := Run(context.Background(), args, input, &stdout, &stderr); got != 2 || input.read || stdout.Len() != 0 {
				t.Fatalf("exit %d, read %v: %s %s", got, input.read, &stdout, &stderr)
			}
			if !strings.Contains(stderr.String(), test.diagnostic) {
				t.Fatalf("stderr %q, want %q", &stderr, test.diagnostic)
			}
		})
	}
}

// TestManifestIsNotATrustRoot changes a tool after pinning: the runner still
// checks the executable bytes and refuses the stale digest.
func TestManifestIsNotATrustRoot(t *testing.T) {
	solver, _ := script(t, "echo 's SATISFIABLE'; echo 'v 1 0'; exit 10")
	checker, _ := script(t, "exit 1")
	manifest := pinned(t, solver, checker)
	if err := os.WriteFile(solver, []byte("#!/bin/sh\nexit 10\n"), 0700); err != nil {
		t.Fatal(err)
	}
	status, doc := call(t, []string{"solve", "--tools", manifest, "--artifacts", filepath.Join(t.TempDir(), "run")}, `{"variables":[{"id":1}],"clauses":[[1]]}`)
	if status != 1 || !strings.Contains(string(doc["error"]), "digest mismatch") {
		t.Fatalf("status %d: %s", status, doc["error"])
	}
}

func TestPinToolsHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if got := Run(context.Background(), []string{"pin-tools", "--help"}, &trackingReader{}, &stdout, &stderr); got != 0 {
		t.Fatalf("exit %d", got)
	}
	for _, want := range []string{"-solver", "-checker", "-out", "never overwritten"} {
		if !strings.Contains(stderr.String(), want) {
			t.Errorf("help lacks %q: %s", want, &stderr)
		}
	}
	if strings.Contains(stderr.String(), "-solver-digest") || strings.Contains(stderr.String(), "-artifacts") {
		t.Errorf("help exposes solve-only flags: %s", &stderr)
	}
	stderr.Reset()
	if got := Run(context.Background(), []string{"solve", "--help"}, &trackingReader{}, &stdout, &stderr); got != 0 || !strings.Contains(stderr.String(), "-tools") {
		t.Fatalf("solve help lacks -tools: %s", &stderr)
	}
}
