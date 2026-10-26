package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Yui-Qi-Tang/calx/digest"
	"github.com/Yui-Qi-Tang/calx/internal/jsoninput"
)

// toolsSchema versions the tool manifest written by pin-tools.
const toolsSchema = "calx-tools/v1"

// maxManifest bounds a manifest file; real manifests are a few hundred bytes.
const maxManifest = 64 << 10

// toolManifest pins the solver and checker by absolute path and content digest.
// It is a reviewed record of a trust decision, not a trust root: the runner
// still verifies each executable against its digest before every run.
type toolManifest struct {
	Schema  string  `json:"schema_version"`
	Solver  toolPin `json:"solver"`
	Checker toolPin `json:"checker"`
}

type toolPin struct {
	Path   string        `json:"path"`
	Digest digest.Digest `json:"digest"`
}

// pinTools hashes both executables and writes a new manifest file. It refuses
// to overwrite an existing file.
func pinTools(solver, checker, out string) (toolManifest, error) {
	m := toolManifest{Schema: toolsSchema}
	var err error
	if m.Solver, err = pin(solver); err != nil {
		return toolManifest{}, fmt.Errorf("solver: %w", err)
	}
	if m.Checker, err = pin(checker); err != nil {
		return toolManifest{}, fmt.Errorf("checker: %w", err)
	}
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return toolManifest{}, err
	}
	f, err := os.OpenFile(out, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return toolManifest{}, fmt.Errorf("manifest must be a new file: %w", err)
	}
	_, err = f.Write(append(raw, '\n'))
	if err := errors.Join(err, f.Close()); err != nil {
		return toolManifest{}, err
	}
	return m, nil
}

func pin(path string) (p toolPin, err error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return toolPin{}, err
	}
	f, err := os.Open(abs)
	if err != nil {
		return toolPin{}, err
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	info, err := f.Stat()
	if err != nil {
		return toolPin{}, err
	}
	if !info.Mode().IsRegular() || info.Mode()&0111 == 0 {
		return toolPin{}, fmt.Errorf("%s is not an executable regular file", abs)
	}
	d, _, err := digest.Read(f)
	if err != nil {
		return toolPin{}, err
	}
	return toolPin{Path: abs, Digest: d}, nil
}

// loadManifest reads and validates a manifest. Paths must be absolute so the
// pinned file does not depend on the working directory of a later run.
func loadManifest(path string) (m toolManifest, err error) {
	f, err := os.Open(path)
	if err != nil {
		return toolManifest{}, fmt.Errorf("tool manifest: %w", err)
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	raw, err := io.ReadAll(io.LimitReader(f, maxManifest+1))
	if err != nil {
		return toolManifest{}, fmt.Errorf("tool manifest: %w", err)
	}
	if len(raw) > maxManifest {
		return toolManifest{}, errors.New("tool manifest: file too large")
	}
	if err := jsoninput.Decode(raw, &m); err != nil {
		return toolManifest{}, fmt.Errorf("tool manifest: %w", err)
	}
	if err := jsoninput.Require(raw, "schema_version", "solver", "checker"); err != nil {
		return toolManifest{}, fmt.Errorf("tool manifest: %w", err)
	}
	if m.Schema != toolsSchema {
		return toolManifest{}, fmt.Errorf("tool manifest: schema_version %q, want %q", m.Schema, toolsSchema)
	}
	for _, p := range []struct {
		name string
		pin  toolPin
	}{{"solver", m.Solver}, {"checker", m.Checker}} {
		if !filepath.IsAbs(p.pin.Path) {
			return toolManifest{}, fmt.Errorf("tool manifest: %s path must be absolute", p.name)
		}
		if err := p.pin.Digest.Validate(); err != nil {
			return toolManifest{}, fmt.Errorf("tool manifest: %s digest: %w", p.name, err)
		}
	}
	return m, nil
}
