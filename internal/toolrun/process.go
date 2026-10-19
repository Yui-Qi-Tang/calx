// Package toolrun contains process and artifact operations for the concrete
// backends. Adapted from Pouch; see ../../NOTICE.
package toolrun

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"time"

	"github.com/Yui-Qi-Tang/calx/sat"
)

func readTool(path, expected string) (raw []byte, err error) {
	hash, err := hex.DecodeString(expected)
	if err != nil {
		return nil, fmt.Errorf("decoding executable sha256: %w", err)
	}
	if len(hash) != 32 || expected != hex.EncodeToString(hash) {
		return nil, fmt.Errorf("lowercase sha256 is required")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening executable: %w", err)
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("inspecting executable: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&0111 == 0 {
		return nil, fmt.Errorf("not an executable regular file")
	}
	if info.Size() > 64<<20 {
		return nil, fmt.Errorf("%w: executable size", sat.ErrLimit)
	}
	raw, err = io.ReadAll(io.LimitReader(f, (64<<20)+1))
	if err != nil {
		return nil, fmt.Errorf("reading executable: %w", err)
	}
	if len(raw) > 64<<20 {
		return nil, fmt.Errorf("%w: executable size", sat.ErrLimit)
	}
	digest := sha256.Sum256(raw)
	if hex.EncodeToString(digest[:]) != expected {
		return nil, fmt.Errorf("executable hash mismatch")
	}
	return raw, nil
}

// Execution records the process contract and termination cause.
type Execution struct {
	ExitCode   int      `json:"exit_code"`
	TimedOut   bool     `json:"timed_out"`
	Canceled   bool     `json:"canceled"`
	DurationMS int64    `json:"duration_ms"`
	Arguments  []string `json:"arguments"`
}

// Execute runs one process with bounded output. Each file is closed once, and
// deferred close errors are joined into the returned error before the caller
// reads any output. Nonzero process exit codes are data for the backend parser.
func Execute(ctx context.Context, dir, stem, executable string, maxOutput int64, args ...string) (record Execution, err error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	record = Execution{ExitCode: -1, Arguments: append([]string{filepath.Base(executable)}, args...)}
	stdout, err := os.OpenFile(filepath.Join(dir, stem+".stdout"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return record, fmt.Errorf("creating tool output: %w", err)
	}
	defer func() { err = errors.Join(err, stdout.Close()) }()
	stderr, err := os.OpenFile(filepath.Join(dir, stem+".stderr"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return record, fmt.Errorf("creating tool error output: %w", err)
	}
	defer func() { err = errors.Join(err, stderr.Close()) }()
	out := &limitedWriter{w: stdout, remaining: maxOutput, cancel: cancel}
	errOut := &limitedWriter{w: stderr, remaining: maxOutput, cancel: cancel}
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Dir, cmd.Stdout, cmd.Stderr = dir, out, errOut
	controlProcess(cmd)
	start := time.Now()
	runErr := cmd.Run()
	record.DurationMS = time.Since(start).Milliseconds()
	if cmd.ProcessState != nil {
		record.ExitCode = cmd.ProcessState.ExitCode()
	}
	record.TimedOut = errors.Is(ctx.Err(), context.DeadlineExceeded)
	record.Canceled = errors.Is(ctx.Err(), context.Canceled)
	var processErr error
	if runErr != nil {
		var exit *exec.ExitError
		if !errors.As(runErr, &exit) {
			processErr = fmt.Errorf("executing tool: %w", runErr)
		}
	}
	// os/exec prioritizes a nonzero exit over pipe-copy failures. Preserve the
	// writer errors separately, including when 10/20 is a valid solver exit.
	processErr = errors.Join(processErr, out.err, errOut.err, WriteJSON(filepath.Join(dir, stem+".json"), record))
	if out.exceeded || errOut.exceeded {
		processErr = errors.Join(processErr, fmt.Errorf("%w: tool output", sat.ErrLimit))
	}
	if err := ctx.Err(); err != nil {
		processErr = errors.Join(processErr, err)
	}
	return record, processErr
}

type limitedWriter struct {
	w         io.Writer
	remaining int64
	exceeded  bool
	cancel    context.CancelFunc
	err       error
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	if int64(len(p)) > w.remaining {
		w.exceeded = true
		w.cancel()
		return 0, fmt.Errorf("tool output limit")
	}
	n, err := w.w.Write(p)
	if n != len(p) && err == nil {
		err = io.ErrShortWrite
	}
	if err != nil {
		w.err = errors.Join(w.err, err)
		w.cancel()
	}
	w.remaining -= int64(n)
	return n, err
}

// WriteJSON creates a new JSON artifact and preserves write and close errors.
func WriteJSON(path string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding evidence: %w", err)
	}
	raw = append(raw, '\n')
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("creating evidence file: %w", err)
	}
	_, writeErr := f.Write(raw)
	if err := errors.Join(writeErr, f.Close()); err != nil {
		return fmt.Errorf("writing evidence file: %w", err)
	}
	return nil
}

// Inventory hashes each regular file within a query directory.
func Inventory(dir, id string) ([]sat.Artifact, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("listing evidence files: %w", err)
	}
	artifacts := make([]sat.Artifact, 0, len(entries))
	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			return artifacts, fmt.Errorf("unexpected nonregular evidence")
		}
		f, err := os.Open(filepath.Join(dir, entry.Name()))
		if err != nil {
			return artifacts, fmt.Errorf("opening evidence: %w", err)
		}
		hash := sha256.New()
		n, copyErr := io.Copy(hash, f)
		if err := errors.Join(copyErr, f.Close()); err != nil {
			return artifacts, fmt.Errorf("hashing evidence: %w", err)
		}
		artifacts = append(artifacts, sat.Artifact{ID: id + "/" + entry.Name(), SHA256: hex.EncodeToString(hash.Sum(nil)), Bytes: n})
	}
	sort.Slice(artifacts, func(i, j int) bool { return artifacts[i].ID < artifacts[j].ID })
	return artifacts, nil
}
