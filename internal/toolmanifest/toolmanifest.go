// Package toolmanifest reads and writes the calx-tools/v1 manifest that pins
// the solver and checker executables by absolute path and content digest.
package toolmanifest

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Yui-Qi-Tang/calx/digest"
	"github.com/Yui-Qi-Tang/calx/internal/jsoncompat"
	"github.com/Yui-Qi-Tang/calx/internal/jsoninput"
)

// Schema versions the manifest format.
const Schema = "calx-tools/v1"

// maxManifest bounds a manifest file; real manifests are a few hundred bytes.
const maxManifest = 64 << 10

// Manifest pins the solver and checker by absolute path and content digest.
// It is a reviewed record of a trust decision, not a trust root: the runner
// still verifies each executable against its digest before every run.
type Manifest struct {
	Schema  string `json:"schema_version"`
	Solver  Pin    `json:"solver"`
	Checker Pin    `json:"checker"`
}

// Pin identifies one executable.
type Pin struct {
	Path   string        `json:"path"`
	Digest digest.Digest `json:"digest"`
}

// Write hashes both executables and writes a new manifest file. It refuses to
// overwrite an existing file, and does not apply the runner's executable size
// limit.
func Write(solver, checker, out string) (Manifest, error) {
	m := Manifest{Schema: Schema}
	var err error
	if m.Solver, err = pin(solver); err != nil {
		return Manifest{}, fmt.Errorf("solver: %w", err)
	}
	if m.Checker, err = pin(checker); err != nil {
		return Manifest{}, fmt.Errorf("checker: %w", err)
	}
	raw, err := jsoncompat.MarshalIndent(m)
	if err != nil {
		return Manifest{}, err
	}
	f, err := os.OpenFile(out, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return Manifest{}, fmt.Errorf("manifest must be a new file: %w", err)
	}
	_, err = f.Write(append(raw, '\n'))
	if err := errors.Join(err, f.Close()); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

func pin(path string) (p Pin, err error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return Pin{}, err
	}
	f, err := os.Open(abs)
	if err != nil {
		return Pin{}, err
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	info, err := f.Stat()
	if err != nil {
		return Pin{}, err
	}
	if !info.Mode().IsRegular() || info.Mode()&0111 == 0 {
		return Pin{}, fmt.Errorf("%s is not an executable regular file", abs)
	}
	d, _, err := digest.Read(f)
	if err != nil {
		return Pin{}, err
	}
	return Pin{Path: abs, Digest: d}, nil
}

// Load reads and validates a manifest. Paths must be absolute so the
// pinned file does not depend on the working directory of a later run.
func Load(path string) (m Manifest, err error) {
	f, err := os.Open(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("tool manifest: %w", err)
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	raw, err := io.ReadAll(io.LimitReader(f, maxManifest+1))
	if err != nil {
		return Manifest{}, fmt.Errorf("tool manifest: %w", err)
	}
	if len(raw) > maxManifest {
		return Manifest{}, errors.New("tool manifest: file too large")
	}
	if err := jsoninput.Decode(raw, &m); err != nil {
		return Manifest{}, fmt.Errorf("tool manifest: %w", err)
	}
	if err := jsoninput.Require(raw, "schema_version", "solver", "checker"); err != nil {
		return Manifest{}, fmt.Errorf("tool manifest: %w", err)
	}
	if m.Schema != Schema {
		return Manifest{}, fmt.Errorf("tool manifest: schema_version %q, want %q", m.Schema, Schema)
	}
	for _, p := range []struct {
		name string
		pin  Pin
	}{{"solver", m.Solver}, {"checker", m.Checker}} {
		if !filepath.IsAbs(p.pin.Path) {
			return Manifest{}, fmt.Errorf("tool manifest: %s path must be absolute", p.name)
		}
		if err := p.pin.Digest.Validate(); err != nil {
			return Manifest{}, fmt.Errorf("tool manifest: %s digest: %w", p.name, err)
		}
	}
	return m, nil
}
