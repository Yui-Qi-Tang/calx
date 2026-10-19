package cadical

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Yui-Qi-Tang/calx/sat"
)

func TestArtifactArchivalBoundary(t *testing.T) {
	r := fixtureRunner(t, "echo 's SATISFIABLE'; echo 'v 1 0'; exit 10", "exit 1", time.Second)
	result, err := r.Solve(context.Background(), "archive", tinyCNF())
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range result.Artifacts {
		if _, err := r.ReadArtifact(a); err != nil {
			t.Fatal(err)
		}
	}
	a := result.Artifacts[0]
	raw, err := os.ReadFile(filepath.Join(r.root, a.ID))
	if err != nil || len(raw) == 0 {
		t.Fatal(err)
	}
	raw[0] ^= 1
	if err := os.WriteFile(filepath.Join(r.root, a.ID), raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := r.ReadArtifact(a); err == nil {
		t.Fatal("accepted tampered artifact")
	}
	for _, id := range []string{"../escape", "/etc/hosts", "absent"} {
		if _, err := r.ReadArtifact(sat.Artifact{ID: id, Bytes: 0}); err == nil {
			t.Fatal("accepted invalid path", id)
		}
	}
	if _, err := r.ReadArtifact(sat.Artifact{ID: a.ID, Bytes: 1 << 60}); err == nil {
		t.Fatal("accepted unbounded read")
	}
	// Identity follows semantics and resource limits, independent of storage.
	cfg := r.config
	cfg.ArtifactDir = filepath.Join(t.TempDir(), "other")
	second, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	firstID, err := r.Identity()
	if err != nil {
		t.Fatal(err)
	}
	secondID, err := second.Identity()
	if err != nil {
		t.Fatal(err)
	}
	if secondID != firstID {
		t.Fatal("storage path changes execution identity")
	}
	cfg.Limits.Timeout *= 2
	cfg.ArtifactDir = filepath.Join(t.TempDir(), "third")
	third, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	thirdID, err := third.Identity()
	if err != nil {
		t.Fatal(err)
	}
	if thirdID == firstID {
		t.Fatal("limits omitted from identity")
	}
}

func TestUninitializedRunnerReturnsErrors(t *testing.T) {
	for _, runner := range []*Runner{nil, new(Runner)} {
		if _, err := runner.Identity(); err == nil {
			t.Fatal("uninitialized identity accepted")
		}
		if _, err := runner.ReadArtifact(sat.Artifact{}); err == nil {
			t.Fatal("uninitialized artifact reader accepted")
		}
		if _, err := runner.Solve(context.Background(), "q", sat.CNF{}); err == nil {
			t.Fatal("uninitialized solver accepted")
		}
	}
}
