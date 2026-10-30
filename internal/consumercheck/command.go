package consumercheck

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func (s *session) command(ctx context.Context, dir, name string, input []byte, extra map[string]string, expectedExit int, program string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, program, args...)
	command.Dir = dir
	command.Stdin = bytes.NewReader(input)
	command.Env = replaceEnv(os.Environ(), map[string]string{
		"GOWORK": "off", "CALX_CONSUMER_OUTPUT": s.config.OutputDir,
		"CALX_CADICAL": s.config.Tools.SolverPath, "CALX_CADICAL_DIGEST": s.config.Tools.SolverDigest.String(),
		"CALX_DRAT": s.config.Tools.CheckerPath, "CALX_DRAT_DIGEST": s.config.Tools.CheckerDigest.String(),
	})
	command.Env = replaceEnv(command.Env, extra)
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	runErr := command.Run()
	exit := -1
	if command.ProcessState != nil {
		exit = command.ProcessState.ExitCode()
	}
	s.checks[name] = Check{Outcome: Failed, Scope: "command execution"}
	logErr := errors.Join(
		os.WriteFile(filepath.Join(s.config.OutputDir, name+".stdout"), stdout.Bytes(), 0600),
		os.WriteFile(filepath.Join(s.config.OutputDir, name+".stderr"), stderr.Bytes(), 0600),
		writeJSON(filepath.Join(s.config.OutputDir, name+".command.json"), struct {
			Arguments        []string `json:"arguments"`
			ExitCode         int      `json:"exit_code"`
			ExpectedExitCode int      `json:"expected_exit_code"`
		}{Arguments: append([]string{program}, args...), ExitCode: exit, ExpectedExitCode: expectedExit}),
	)
	if runErr != nil {
		var exitErr *exec.ExitError
		if !errors.As(runErr, &exitErr) || exitErr.ExitCode() != expectedExit {
			return stdout.Bytes(), errors.Join(fmt.Errorf("%s: %w", name, runErr), logErr)
		}
	}
	if exit != expectedExit {
		return stdout.Bytes(), errors.Join(fmt.Errorf("%s: exit %d, want %d", name, exit, expectedExit), logErr)
	}
	if logErr != nil {
		return stdout.Bytes(), logErr
	}
	s.checks[name] = Check{Outcome: Passed, Scope: "command execution"}
	return stdout.Bytes(), nil
}

func replaceEnv(env []string, changes map[string]string) []string {
	out := make([]string, 0, len(env)+len(changes))
	for _, pair := range env {
		key, _, found := strings.Cut(pair, "=")
		if _, replaced := changes[key]; !found || !replaced {
			out = append(out, pair)
		}
	}
	for key, value := range changes {
		out = append(out, key+"="+value)
	}
	return out
}

func (s *session) testGo(ctx context.Context, dir, kind string, expectedTests int, packages ...string) error {
	arguments := append([]string{"test", "-json", "-count=1", "-run", "TestCalx"}, packages...)
	raw, err := s.command(ctx, dir, kind, nil, map[string]string{"CALX_CONSUMER_KIND": kind}, 0, "go", arguments...)
	if err != nil {
		return err
	}
	s.checks[kind] = Check{Outcome: Failed, Scope: "selected native Go source boundary"}
	decoder := jsontext.NewDecoder(bytes.NewReader(raw))
	passed := 0
	for {
		var event struct{ Action, Test string }
		if err := json.UnmarshalDecode(decoder, &event); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return err
		}
		if event.Test != "" && event.Action == "pass" {
			passed++
		}
		if event.Action == "fail" || event.Action == "skip" {
			return fmt.Errorf("%s: unexpected test %s: %s", kind, event.Action, event.Test)
		}
	}
	if passed != expectedTests {
		return fmt.Errorf("%s: passed %d tests, want %d", kind, passed, expectedTests)
	}
	s.checks[kind] = Check{Outcome: Passed, Scope: "selected native Go source boundary", Cases: passed}
	return nil
}
