// Package consumercheck runs bounded compatibility checks in isolated outputs.
// It never rewrites a consumer checkout or executes Python code.
package consumercheck

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Yui-Qi-Tang/calx/digest"
	"github.com/Yui-Qi-Tang/calx/sat/cadical"
)

//go:embed testdata
var fixtures embed.FS

// Config selects explicit checkouts, trusted tools and a new output directory.
type Config struct {
	CalxDir, CoreDir, MCPDir, PouchDir, LabDir, OutputDir string
	Tools                                                 cadical.Config
}

// Outcome identifies the state of a compatibility check.
type Outcome string

const (
	// Passed means all checks in the stated bounded scope passed.
	Passed Outcome = "PASS"
	// Failed means a required check failed or could not complete.
	Failed Outcome = "FAIL"
)

// Check reports one explicit scope, not adoption by a deployed consumer.
type Check struct {
	Outcome Outcome `json:"outcome"`
	Scope   string  `json:"scope"`
	Cases   int     `json:"cases,omitempty"`
}

// Summary keeps partial results on failure.
type Summary struct {
	Outcome Outcome          `json:"outcome"`
	Checks  map[string]Check `json:"checks"`
	Error   string           `json:"error,omitempty"`
}

type sourceIdentity struct {
	Path   string        `json:"path"`
	Digest digest.Digest `json:"digest"`
}

type session struct {
	config  Config
	sources map[string]sourceIdentity
	checks  map[string]Check
}

// Run snapshots selected Go source and checks frozen Lab CNFs. Its output and
// logs are retained on failure; source identities are rechecked before return.
func Run(ctx context.Context, config Config) (summary Summary, err error) {
	if err := config.validate(); err != nil {
		return Summary{Outcome: Failed, Error: err.Error()}, err
	}
	if err := os.MkdirAll(filepath.Dir(config.OutputDir), 0755); err != nil {
		return Summary{Outcome: Failed, Error: err.Error()}, err
	}
	if err := os.Mkdir(config.OutputDir, 0700); err != nil {
		return Summary{Outcome: Failed, Error: err.Error()}, fmt.Errorf("output directory must be new: %w", err)
	}
	s := session{config: config, sources: map[string]sourceIdentity{}, checks: map[string]Check{}}
	summary = Summary{Outcome: Failed, Checks: s.checks}
	defer func() {
		err = errors.Join(err, s.checkSources(), writeJSON(filepath.Join(config.OutputDir, "source-manifest.json"), s.sources))
		if err == nil {
			summary.Outcome = Passed
		} else {
			summary.Outcome, summary.Error = Failed, err.Error()
		}
		if saveErr := writeJSON(filepath.Join(config.OutputDir, "results.json"), summary); saveErr != nil {
			err = errors.Join(err, saveErr)
			summary.Outcome, summary.Error = Failed, err.Error()
		}
	}()
	binary := filepath.Join(config.OutputDir, "calx")
	if _, err := s.command(ctx, config.CalxDir, "build-cli", nil, nil, 0, "go", "build", "-o", binary, "./cmd/calx"); err != nil {
		return summary, err
	}
	for _, consumer := range []struct{ name, root string }{{"core", config.CoreDir}, {"mcp", config.MCPDir}} {
		if err := s.checkAHE(ctx, consumer.name, consumer.root); err != nil {
			return summary, err
		}
	}
	if err := s.checkPouch(ctx); err != nil {
		return summary, err
	}
	if err := s.checkLab(ctx, binary); err != nil {
		return summary, err
	}
	return summary, nil
}

func (c *Config) validate() error {
	for _, path := range []*string{&c.CalxDir, &c.CoreDir, &c.MCPDir, &c.PouchDir, &c.LabDir, &c.OutputDir} {
		if *path == "" {
			return errors.New("all checkout and output paths are required")
		}
		absolute, err := filepath.Abs(*path)
		if err != nil {
			return err
		}
		*path = absolute
	}
	for _, root := range []string{c.CoreDir, c.MCPDir, c.PouchDir, c.LabDir} {
		relative, err := filepath.Rel(root, c.OutputDir)
		if err != nil {
			return err
		}
		if relative == "." || filepath.IsLocal(relative) {
			return errors.New("output must be outside consumer checkouts")
		}
	}
	if c.Tools.SolverPath == "" || c.Tools.SolverDigest == "" || c.Tools.CheckerPath == "" || c.Tools.CheckerDigest == "" {
		return errors.New("both solver/checker executable paths and digests are required")
	}
	for _, path := range []*string{&c.Tools.SolverPath, &c.Tools.CheckerPath} {
		absolute, err := filepath.Abs(*path)
		if err != nil {
			return err
		}
		*path = absolute
	}
	module, err := os.ReadFile(filepath.Join(c.CalxDir, "go.mod"))
	if err != nil {
		return err
	}
	if !strings.Contains(string(module), "module github.com/Yui-Qi-Tang/calx\n") {
		return errors.New("calx root does not identify the Calx module")
	}
	return nil
}

func (s *session) source(kind, root, relative string) ([]byte, error) {
	if !filepath.IsLocal(relative) {
		return nil, errors.New("invalid source-relative path")
	}
	path := filepath.Join(root, relative)
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	s.sources[kind+"/"+filepath.ToSlash(relative)] = sourceIdentity{Path: path, Digest: digest.Sum(raw)}
	return raw, nil
}

func (s *session) checkSources() error {
	for name, source := range s.sources {
		raw, err := os.ReadFile(source.Path)
		if err != nil {
			return fmt.Errorf("rechecking %s: %w", name, err)
		}
		if digest.Sum(raw) != source.Digest {
			return fmt.Errorf("source changed during check: %s", name)
		}
	}
	return nil
}

func writeJSON(path string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0600)
}

func copyFixture(name, target string) error {
	raw, err := fixtures.ReadFile("testdata/" + name)
	if err != nil {
		return err
	}
	return os.WriteFile(target, raw, 0600)
}
