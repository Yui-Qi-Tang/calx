// Package cadical runs pinned CaDiCaL and DRAT-trim executables. SAT assignments
// are checked against every input clause; UNSAT requires a checked DRAT proof.
package cadical

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/Yui-Qi-Tang/calx/digest"
	"github.com/Yui-Qi-Tang/calx/internal/jsoncompat"
	"github.com/Yui-Qi-Tang/calx/internal/toolrun"
	"github.com/Yui-Qi-Tang/calx/sat"
)

// Config pins both tools and a new artifact directory. MaxProofBytes limits
// proofs accepted after solver exit, not disk consumption during execution.
type Config struct {
	SolverPath  string // Path to the CaDiCaL executable.
	CheckerPath string // Path to the DRAT-trim executable; required even for SAT.
	// Digests identify the exact executable bytes. The current format is
	// SHA-256 encoded as 64 lowercase hexadecimal characters, without a prefix.
	SolverDigest  digest.Digest
	CheckerDigest digest.Digest
	// ArtifactDir must not exist; its parent directory must already exist.
	ArtifactDir   string
	Limits        sat.Limits
	MaxProofBytes int64
}

// Runner keeps private tool copies. Concurrent Solve calls use distinct query
// directories; duplicate IDs fail atomically. Callers own artifact retention.
type Runner struct {
	root   string
	config Config
}

// Result carries the total Boolean assignment when decoding succeeds.
// Assignment presence alone does not imply a successful model check.
type Result struct {
	sat.Result
	Assignment map[int]bool
}

// IsZero reports whether r is the zero Result, including no assignment. It
// shadows sat.Result.IsZero so that Assignment is taken into account.
func (r Result) IsZero() bool { return r.Result.IsZero() && len(r.Assignment) == 0 }

// New verifies executable hashes before creating private copies.
func New(c Config) (*Runner, error) {
	limits, err := c.Limits.WithDefaults()
	if err != nil {
		return nil, err
	}
	c.Limits = limits
	if c.MaxProofBytes < 0 {
		return nil, fmt.Errorf("%w: negative proof limit", sat.ErrInput)
	}
	if c.MaxProofBytes == 0 {
		c.MaxProofBytes = 64 << 20
	}
	root, err := toolrun.Prepare(c.ArtifactDir,
		toolrun.Tool{Name: "cadical", Path: c.SolverPath, Digest: c.SolverDigest},
		toolrun.Tool{Name: "drat-trim", Path: c.CheckerPath, Digest: c.CheckerDigest})
	if err != nil {
		return nil, err
	}
	if err := toolrun.WriteJSON(filepath.Join(root, "limits.json"), struct {
		Limits        sat.Limits
		MaxProofBytes int64
	}{c.Limits, c.MaxProofBytes}); err != nil {
		return nil, err
	}
	return &Runner{root: root, config: c}, nil
}

// Solve retains exact inputs, metadata, output and check records. All processes
// share one deadline. It never silently fills an incomplete assignment.
// An explicit UNKNOWN response returns sat.ErrInconclusive with its diagnostics;
// observed cancellation or deadline errors remain discoverable with errors.Is.
func (r *Runner) Solve(ctx context.Context, id string, cnf sat.CNF) (result Result, err error) {
	if r == nil || r.root == "" {
		return Result{Status: sat.Unknown}, fmt.Errorf("%w: uninitialized runner", sat.ErrInput)
	}
	result.Result = sat.Result{Status: sat.Unknown, SolverDigest: r.config.SolverDigest, CheckerDigest: r.config.CheckerDigest}
	ctx, cancel := context.WithTimeout(ctx, r.config.Limits.Timeout)
	defer cancel()
	if err = ctx.Err(); err != nil {
		return result, err
	}
	if err = r.config.Limits.CheckCNF(cnf); err != nil {
		return result, err
	}
	dir, err := toolrun.Query(r.root, id)
	if err != nil {
		return result, err
	}
	defer toolrun.Finish(dir, id, &result.Result, &err)
	input, err := jsoncompat.Marshal(inputJSON(cnf))
	if err != nil {
		return result, err
	}
	if len(input) > r.config.Limits.MaxInputBytes {
		return result, sat.ErrLimit
	}
	result.InputDigest, err = toolrun.Save(dir, "input.json", input)
	if err != nil {
		return result, err
	}
	var formula bytes.Buffer
	if err = cnf.WriteDIMACS(&formula); err != nil {
		return result, err
	}
	if formula.Len() > r.config.Limits.MaxInputBytes {
		return result, sat.ErrLimit
	}
	result.FormulaDigest, err = toolrun.Save(dir, "input.cnf", formula.Bytes())
	if err != nil {
		return result, err
	}
	process, err := toolrun.Execute(ctx, dir, "solver", filepath.Join(r.root, "tools", "cadical"), r.config.Limits.MaxOutputBytes, "--no-binary", "--seed=0", "input.cnf", "proof.drat")
	if err != nil {
		return result, err
	}
	stdout, err := os.ReadFile(filepath.Join(dir, "solver.stdout"))
	if err != nil {
		return result, err
	}
	switch {
	case process.ExitCode == 10 && soleStatus(stdout, "s SATISFIABLE"):
		result.Status = sat.SAT
		err = checkModel(dir, cnf, stdout, &result)
	case process.ExitCode == 20 && soleStatus(stdout, "s UNSATISFIABLE"):
		result.Status = sat.UNSAT
		err = r.checkProof(ctx, dir, hasEmptyClause(cnf), &result.Result)
	case process.ExitCode == 0 && soleStatus(stdout, "s UNKNOWN"):
		return result, errors.Join(sat.ErrInconclusive, ctx.Err())
	default:
		return result, sat.ErrProtocol
	}
	if err != nil {
		return result, err
	}
	return result, ctx.Err()
}

// checkModel decodes the solver assignment and checks it against every clause.
func checkModel(dir string, cnf sat.CNF, stdout []byte, result *Result) error {
	result.Model.Present = hasAssignment(stdout)
	var err error
	result.Assignment, err = assignment(stdout, len(cnf.Variables))
	if err != nil {
		return fmt.Errorf("%w: %v", sat.ErrProtocol, err)
	}
	result.Model.Decoded = true
	if err := toolrun.WriteJSON(filepath.Join(dir, "assignment.json"), result.Assignment); err != nil {
		return err
	}
	if err := cnf.CheckAssignment(result.Assignment); err != nil {
		return fmt.Errorf("%w: %v", sat.ErrValidation, err)
	}
	result.Model.Checked = true
	result.Model.Method = "total-assignment/every-input-clause/v0"
	result.Model.TargetDigest = result.InputDigest
	return nil
}

// checkProof runs the pinned DRAT checker on the retained formula and proof.
// emptyClause reports whether the formula itself contains an empty clause.
func (r *Runner) checkProof(ctx context.Context, dir string, emptyClause bool, result *sat.Result) error {
	proof, err := os.Lstat(filepath.Join(dir, "proof.drat"))
	if err != nil {
		return fmt.Errorf("%w: inspecting proof: %w", sat.ErrProtocol, err)
	}
	if !proof.Mode().IsRegular() {
		return fmt.Errorf("%w: missing regular proof", sat.ErrProtocol)
	}
	result.Proof.Present = true
	if proof.Size() > r.config.MaxProofBytes {
		return fmt.Errorf("%w: proof", sat.ErrLimit)
	}
	checked, err := toolrun.Execute(ctx, dir, "checker", filepath.Join(r.root, "tools", "drat-trim"), r.config.Limits.MaxOutputBytes, "input.cnf", "proof.drat")
	if err != nil {
		return err
	}
	output, err := os.ReadFile(filepath.Join(dir, "checker.stdout"))
	if err != nil {
		return err
	}
	switch {
	case !exactLine(output, "s VERIFIED") || exactLine(output, "s NOT VERIFIED"):
		return fmt.Errorf("%w: proof checker", sat.ErrValidation)
	case checked.ExitCode == 0:
		result.Proof.Method = "drat-trim/exit-zero-exact-verified"
	case checked.ExitCode == 1 && emptyClause && trivialVerified(output):
		// Upstream DRAT-trim reports a formula that already contains an empty
		// clause as "c trivial UNSAT" and "s VERIFIED", then exits 1.
		result.Proof.Method = "drat-trim/trivial-unsat-empty-clause"
	default:
		return fmt.Errorf("%w: proof checker", sat.ErrValidation)
	}
	result.Proof.Checked = true
	result.Proof.TargetDigest = result.FormulaDigest
	return nil
}

func hasEmptyClause(cnf sat.CNF) bool {
	return slices.ContainsFunc(cnf.Clauses, func(clause []int) bool { return len(clause) == 0 })
}
