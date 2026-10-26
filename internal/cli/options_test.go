package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

type trackingReader struct{ read bool }

func (r *trackingReader) Read([]byte) (int, error) {
	r.read = true
	return 0, io.EOF
}

func TestHelpShowsApplicableFlagsWithoutReadingInput(t *testing.T) {
	for _, test := range []struct {
		name    string
		args    []string
		present []string
		absent  []string
	}{
		{name: "root", args: []string{"--help"}, present: []string{"calx <command>", "not CALX_* environment variables"}},
		{name: "project", args: []string{"project", "--help"}, present: []string{"-in"}, absent: []string{"-solver", "-artifacts", "-format"}},
		{name: "reconstruct", args: []string{"reconstruct", "-h"}, present: []string{"-in"}, absent: []string{"-checker", "-timeout"}},
		{name: "encode", args: []string{"encode", "-h"}, present: []string{"-in"}, absent: []string{"-solver", "-format"}},
		{name: "decode", args: []string{"decode", "-h"}, present: []string{"-in"}, absent: []string{"-solver", "-artifacts"}},
		{name: "solve", args: []string{"solve", "--help"}, present: []string{"-solver-digest", "-checker-digest", "-format", "64 lowercase hex", "parent must exist", "excludes compilation"}, absent: []string{"-solver-sha256", "-checker-sha256"}},
		{name: "finite", args: []string{"solve-finite", "--help"}, present: []string{"-solver-digest", "-checker-digest", "-timeout"}, absent: []string{"-format"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			input := &trackingReader{}
			if got := Run(context.Background(), test.args, input, &stdout, &stderr); got != 0 {
				t.Fatalf("exit = %d, stderr = %s", got, &stderr)
			}
			if input.read || stdout.Len() != 0 {
				t.Fatal("help read input or emitted a result")
			}
			for _, want := range test.present {
				if !strings.Contains(stderr.String(), want) {
					t.Errorf("help does not describe %q: %s", want, &stderr)
				}
			}
			for _, unwanted := range test.absent {
				if strings.Contains(stderr.String(), unwanted) {
					t.Errorf("help exposes inapplicable flag %q", unwanted)
				}
			}
		})
	}
}

func TestInvalidOptionsDoNotReadInput(t *testing.T) {
	type testCase struct {
		name, diagnostic string
		args             []string
	}
	tests := []testCase{
		{name: "no command", diagnostic: "usage:"},
		{name: "unknown command", args: []string{"unknown"}, diagnostic: "unknown command"},
		{name: "irrelevant solver", args: []string{"project", "--solver", "tool"}, diagnostic: "flag provided but not defined"},
		{name: "obsolete digest name", args: []string{"solve", "--solver-sha256", "digest"}, diagnostic: "flag provided but not defined"},
		{name: "malformed digest", args: []string{"solve", "--solver-digest", "invalid"}, diagnostic: "invalid digest"},
		{name: "finite format", args: []string{"solve-finite", "--format", "dimacs"}, diagnostic: "flag provided but not defined"},
		{name: "unknown format", args: []string{"solve", "--format", "yaml"}, diagnostic: "unsupported input format"},
		{name: "positional argument", args: []string{"project", "input.json"}, diagnostic: "unexpected positional arguments"},
	}
	required := []string{"--solver", "solver", "--solver-digest", strings.Repeat("a", 64), "--checker", "checker", "--checker-digest", strings.Repeat("b", 64), "--artifacts", "new-run"}
	for _, command := range []string{"solve", "solve-finite"} {
		for i := 0; i < len(required); i += 2 {
			args := append([]string{command}, required[:i]...)
			args = append(args, required[i+2:]...)
			tests = append(tests, testCase{name: command + " missing " + required[i], diagnostic: required[i] + " is required", args: args})
		}
	}
	negativeTimeout := append([]string{"solve"}, required...)
	negativeTimeout = append(negativeTimeout, "--timeout", "-1s")
	tests = append(tests, testCase{name: "negative timeout", diagnostic: "--timeout must not be negative", args: negativeTimeout})
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			input := &trackingReader{}
			if got := Run(context.Background(), test.args, input, &stdout, &stderr); got != 2 {
				t.Fatalf("exit = %d, stderr = %s", got, &stderr)
			}
			if input.read || stdout.Len() != 0 {
				t.Fatal("invalid options read input or emitted a result")
			}
			if !strings.Contains(stderr.String(), test.diagnostic) {
				t.Errorf("stderr = %q, want %q", stderr.String(), test.diagnostic)
			}
		})
	}
}

func TestSolveRequiresFlagsEvenWhenToolEnvironmentIsSet(t *testing.T) {
	for _, name := range []string{"CALX_CADICAL", "CALX_CADICAL_DIGEST", "CALX_DRAT", "CALX_DRAT_DIGEST"} {
		t.Setenv(name, "set-by-caller")
	}
	var stdout, stderr bytes.Buffer
	input := &trackingReader{}
	if got := Run(context.Background(), []string{"solve"}, input, &stdout, &stderr); got != 2 || input.read {
		t.Fatalf("exit = %d, read = %v, stderr = %s", got, input.read, &stderr)
	}
	if !strings.Contains(stderr.String(), "--solver is required") {
		t.Fatalf("missing explicit flag diagnostic: %s", &stderr)
	}
}

type brokenWriter struct{}

func (brokenWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

func TestHelpWriteFailureIsNotSuccess(t *testing.T) {
	if got := Run(context.Background(), []string{"--help"}, strings.NewReader(""), io.Discard, brokenWriter{}); got != 1 {
		t.Fatalf("exit = %d, want 1", got)
	}
}
