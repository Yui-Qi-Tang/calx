package consumercheck

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"

	"github.com/Yui-Qi-Tang/calx/finite"
	"github.com/Yui-Qi-Tang/calx/sat"
	"github.com/Yui-Qi-Tang/calx/sat/cadical"
)

type labManifest struct {
	Provenance string            `json:"provenance"`
	Scope      string            `json:"scope"`
	Sources    map[string]string `json:"sources"`
	Fixtures   []struct {
		Name   string     `json:"name"`
		File   string     `json:"file"`
		SHA256 string     `json:"sha256"`
		Status sat.Status `json:"status"`
	} `json:"fixtures"`
}

type solveEnvelope struct {
	Outcome string `json:"outcome"`
	Data    struct {
		Result cadical.Result `json:"result"`
		Trace  *finite.Trace  `json:"trace"`
	} `json:"data"`
	Error *struct{ Code, Message string } `json:"error"`
}

func (s *session) solveCLI(ctx context.Context, binary, name, command, inputFormat string, input []byte) (solveEnvelope, error) {
	raw, err := s.command(ctx, s.config.CalxDir, name, input, nil, 0, binary,
		command, "--format", inputFormat,
		"--solver", s.config.Tools.SolverPath, "--solver-sha256", s.config.Tools.SolverSHA256,
		"--checker", s.config.Tools.CheckerPath, "--checker-sha256", s.config.Tools.CheckerSHA256,
		"--artifacts", filepath.Join(s.config.OutputDir, name+"-artifacts"))
	if err != nil {
		return solveEnvelope{}, err
	}
	var response solveEnvelope
	if err := json.Unmarshal(raw, &response); err != nil {
		return response, err
	}
	if response.Outcome != "OK" || response.Error != nil {
		return response, fmt.Errorf("%s: CLI did not accept the input", name)
	}
	if err := response.Data.Result.Require(sat.Requirements{SATModelChecked: true, UNSATProofChecked: true}); err != nil {
		return response, err
	}
	return response, nil
}

func (s *session) checkLab(ctx context.Context, binary string) error {
	raw, err := fixtures.ReadFile("testdata/lab/manifest.json")
	if err != nil {
		return err
	}
	var manifest labManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return err
	}
	for relative, expected := range manifest.Sources {
		source, err := s.source("lab", s.config.LabDir, relative)
		if err != nil {
			return err
		}
		if digest(source) != expected {
			return fmt.Errorf("Lab source differs from frozen fixture provenance: %s", relative)
		}
	}
	if len(manifest.Fixtures) != 13 {
		return fmt.Errorf("expected 13 frozen Lab fixtures")
	}
	sort.Slice(manifest.Fixtures, func(i, j int) bool { return manifest.Fixtures[i].Name < manifest.Fixtures[j].Name })
	for _, fixture := range manifest.Fixtures {
		input, err := fixtures.ReadFile("testdata/lab/" + fixture.File)
		if err != nil {
			return err
		}
		if digest(input) != fixture.SHA256 {
			return fmt.Errorf("frozen fixture digest mismatch: %s", fixture.Name)
		}
		result, err := s.solveCLI(ctx, binary, "lab-"+fixture.Name, "solve", "dimacs", input)
		if err != nil {
			return err
		}
		if result.Data.Result.Status != fixture.Status {
			return fmt.Errorf("%s: status %s, want %s", fixture.Name, result.Data.Result.Status, fixture.Status)
		}
	}
	s.checks["lab"] = Check{Outcome: Passed, Scope: "frozen H2/H6 CNF fixtures; source hashes matched; no live encoder or semantic replay", Cases: len(manifest.Fixtures)}
	request := []byte(`{"model":{"fields":{"x":["a","b"]},"actions":[{"id":"ab","guard":{"x":"a"},"effect":{"x":"b"},"frame":[]}]},"query":{"profile":"categorical-simple-first-hit/v1","start":{"x":"a"},"goal":{"x":"b"},"horizon":1}}`)
	result, err := s.solveCLI(ctx, binary, "finite-cli", "solve-finite", "json", request)
	if err != nil {
		return err
	}
	if result.Data.Result.Status != sat.SAT || result.Data.Trace == nil || len(result.Data.Trace.Actions) != 1 || result.Data.Trace.Actions[0] != "ab" {
		return fmt.Errorf("finite CLI trace mismatch")
	}
	invalid := []byte(`{"model":{"fields":{"x":[false,true]},"actions":[]},"query":{"profile":"categorical-simple-first-hit/v1","start":{"x":false},"goal":{"x":true},"horizon":1}}`)
	raw, err = s.command(ctx, s.config.CalxDir, "reject-typed-input", invalid, nil, 1, binary, "encode")
	if err != nil {
		return err
	}
	var rejected solveEnvelope
	if err := json.Unmarshal(raw, &rejected); err != nil {
		return err
	}
	if rejected.Outcome != "ERROR" || rejected.Error == nil {
		return fmt.Errorf("typed input rejection was not reported")
	}
	return nil
}
