package consumercheck

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractDeclarationsWithGoParser(t *testing.T) {
	source := []byte("package original\ntype Wanted struct{Text string}\ntype Hidden int\nfunc check(s string) bool { return s != \"func }\" }\nfunc other(){}\n")
	raw, err := declarations(source, []string{"Wanted"}, []string{"check"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "Hidden") || strings.Contains(string(raw), "func other") || !strings.Contains(string(raw), "func check") {
		t.Fatal(string(raw))
	}
	if _, err := declarations(source, nil, []string{"missing"}); err == nil {
		t.Fatal("missing declaration accepted")
	}
	if _, err := declarations([]byte("invalid Go"), nil, nil); err == nil {
		t.Fatal("invalid source accepted")
	}
}

func TestSourceDriftIsRejected(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "source.go")
	if err := os.WriteFile(path, []byte("package original"), 0600); err != nil {
		t.Fatal(err)
	}
	s := session{sources: map[string]sourceIdentity{}}
	if _, err := s.source("sample", root, "source.go"); err != nil {
		t.Fatal(err)
	}
	if err := s.checkSources(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("package changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.checkSources(); err == nil {
		t.Fatal("source drift accepted")
	}
	if _, err := s.source("sample", root, "../outside"); err == nil {
		t.Fatal("path outside source accepted")
	}
}

func TestCommandFailureIsNotPassed(t *testing.T) {
	root := t.TempDir()
	s := session{config: Config{OutputDir: root}, checks: map[string]Check{}}
	if _, err := s.command(context.Background(), root, "missing", nil, nil, 0, filepath.Join(root, "no-program")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	if s.checks["missing"].Outcome != Failed {
		t.Fatal("failed execution was passed")
	}
}
