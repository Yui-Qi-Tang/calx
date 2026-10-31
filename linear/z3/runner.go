// Package z3 runs a pinned Z3 executable for linear.Problem. It checks SAT
// models with exact arithmetic but provides no independently checked UNSAT
// proof. Consumers requiring that guarantee must reject such results, which
// Require does when asked for UNSATProofChecked.
package z3

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Yui-Qi-Tang/calx/digest"
	"github.com/Yui-Qi-Tang/calx/internal/jsoncompat"
	"github.com/Yui-Qi-Tang/calx/internal/toolrun"
	"github.com/Yui-Qi-Tang/calx/linear"
	"github.com/Yui-Qi-Tang/calx/sat"
)

// Config pins Z3 and a new artifact directory with per-Solve limits.
type Config struct {
	SolverPath string // Path to the Z3 executable.
	// SolverDigest identifies the exact executable bytes, in the format of
	// digest.Digest.
	SolverDigest digest.Digest
	// ArtifactDir must not exist; its parent directory must already exist.
	ArtifactDir string
	Limits      sat.Limits
}

// Runner keeps a private tool copy. Concurrent Solve calls use distinct query
// directories; duplicate IDs fail atomically. Callers own artifact retention.
type Runner struct {
	root   string
	config Config
}

// Result carries the total assignment when decoding succeeds. Assignment
// presence alone does not imply a successful model check.
type Result struct {
	sat.Result
	Assignment linear.Assignment
}

// IsZero reports whether r is the zero Result, including no assignment. It
// shadows sat.Result.IsZero so that Assignment is taken into account.
func (r Result) IsZero() bool {
	return r.Result.IsZero() && len(r.Assignment.Booleans) == 0 && len(r.Assignment.Integers) == 0
}

// New verifies the executable hash before creating a private copy.
func New(c Config) (*Runner, error) {
	limits, err := c.Limits.WithDefaults()
	if err != nil {
		return nil, err
	}
	c.Limits = limits
	root, err := toolrun.Prepare(c.ArtifactDir, toolrun.Tool{Name: "z3", Path: c.SolverPath, Digest: c.SolverDigest})
	if err != nil {
		return nil, err
	}
	if err := toolrun.WriteJSON(filepath.Join(root, "limits.json"), limits); err != nil {
		return nil, err
	}
	return &Runner{root: root, config: c}, nil
}

// Solve checks satisfiability, then requests values only for SAT. Both processes
// share one deadline. The second response must still be SAT, with a total model
// that satisfies the original typed problem; it need not be the first model.
// An explicit unknown response returns sat.ErrInconclusive with its diagnostics;
// observed cancellation or deadline errors remain discoverable with errors.Is.
func (r *Runner) Solve(ctx context.Context, id string, p linear.Problem) (result Result, err error) {
	if r == nil || r.root == "" {
		return Result{Result: sat.Result{Status: sat.Unknown}}, fmt.Errorf("%w: uninitialized runner", sat.ErrInput)
	}
	result.Result = sat.Result{Status: sat.Unknown, SolverDigest: r.config.SolverDigest}
	ctx, cancel := context.WithTimeout(ctx, r.config.Limits.Timeout)
	defer cancel()
	if err = ctx.Err(); err != nil {
		return result, err
	}
	if err = p.CheckLimits(r.config.Limits); err != nil {
		return result, err
	}
	dir, err := toolrun.Query(r.root, id)
	if err != nil {
		return result, err
	}
	defer toolrun.Finish(dir, id, &result.Result, &err)
	input, err := jsoncompat.Marshal(inputJSON(p))
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
	if err = p.WriteSMTLIB(&formula); err != nil {
		return result, err
	}
	script := append(formula.Bytes(), "(check-sat)\n"...)
	if len(script) > r.config.Limits.MaxInputBytes {
		return result, sat.ErrLimit
	}
	result.FormulaDigest, err = toolrun.Save(dir, "input.smt2", script)
	if err != nil {
		return result, err
	}
	out, err := r.execute(ctx, dir, "solver", "input.smt2")
	if err != nil {
		return result, err
	}
	switch strings.TrimSpace(string(out)) {
	case "unsat":
		result.Status = sat.UNSAT
		return result, ctx.Err()
	case "unknown":
		return result, errors.Join(sat.ErrInconclusive, ctx.Err())
	case "sat":
		result.Status = sat.SAT
	default:
		return result, sat.ErrProtocol
	}
	if err = r.checkModel(ctx, dir, p, script, &result); err != nil {
		return result, err
	}
	return result, ctx.Err()
}

// checkModel asks a second process for values and evaluates them against p.
func (r *Runner) checkModel(ctx context.Context, dir string, p linear.Problem, script []byte, result *Result) error {
	names := make([]string, 0, len(p.Booleans.Variables)+len(p.Integers))
	for _, v := range p.Booleans.Variables {
		names = append(names, fmt.Sprintf("b%d", v.ID))
	}
	for _, v := range p.Integers {
		names = append(names, fmt.Sprintf("i%d", v.ID))
	}
	model := bytes.Clone(script)
	if len(names) > 0 {
		model = fmt.Appendf(model, "(get-value (%s))\n", strings.Join(names, " "))
	}
	if len(model) > r.config.Limits.MaxInputBytes {
		return sat.ErrLimit
	}
	if _, err := toolrun.Save(dir, "model.smt2", model); err != nil {
		return err
	}
	out, err := r.execute(ctx, dir, "model", "model.smt2")
	if err != nil {
		return err
	}
	result.Model.Present = bytes.Contains(out, []byte("(")) || (len(names) == 0 && strings.TrimSpace(string(out)) == "sat")
	result.Assignment, err = parseAssignment(out, len(p.Booleans.Variables), len(p.Integers))
	if err != nil {
		return fmt.Errorf("%w: %v", sat.ErrProtocol, err)
	}
	result.Model.Decoded = true
	if err := toolrun.WriteJSON(filepath.Join(dir, "assignment.json"), assignmentJSON(result.Assignment)); err != nil {
		return err
	}
	if err := p.CheckAssignment(result.Assignment); err != nil {
		return fmt.Errorf("%w: %v", sat.ErrValidation, err)
	}
	result.Model.Checked = true
	result.Model.Method = "total-bool-and-big-integer-evaluation/v0"
	result.Model.TargetDigest = result.InputDigest
	return nil
}

func (r *Runner) execute(ctx context.Context, dir, stem, input string) ([]byte, error) {
	record, err := toolrun.Execute(ctx, dir, stem, filepath.Join(r.root, "tools", "z3"), r.config.Limits.MaxOutputBytes, "-smt2", input)
	if err != nil {
		return nil, err
	}
	if record.ExitCode != 0 {
		return nil, fmt.Errorf("%w: z3 exit %d", sat.ErrProtocol, record.ExitCode)
	}
	return os.ReadFile(filepath.Join(dir, stem+".stdout"))
}

// problemJSON and its parts fix the form of input.json, the target of the model
// check, so it does not depend on the types of sat and linear.
type problemJSON struct {
	Booleans    cnfJSON          `json:"booleans"`
	Integers    []variableJSON   `json:"integers"`
	Constraints []constraintJSON `json:"constraints"`
}

type cnfJSON struct {
	Variables []variableJSON `json:"variables"`
	Clauses   [][]int        `json:"clauses"`
	Origins   [][]string     `json:"origins,omitempty"`
}

type variableJSON struct {
	ID       int               `json:"id"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

type constraintJSON struct {
	Terms    []termJSON `json:"terms"`
	Relation string     `json:"relation"`
	RHS      string     `json:"rhs"`
	Origins  []string   `json:"origins,omitempty"`
}

type termJSON struct {
	Variable    int    `json:"variable"`
	Coefficient string `json:"coefficient"`
}

func inputJSON(p linear.Problem) problemJSON {
	input := problemJSON{
		Booleans: cnfJSON{Variables: variablesJSON(p.Booleans.Variables), Clauses: p.Booleans.Clauses, Origins: p.Booleans.Origins},
		Integers: variablesJSON(p.Integers),
	}
	if p.Constraints != nil {
		input.Constraints = make([]constraintJSON, len(p.Constraints))
	}
	for i, c := range p.Constraints {
		input.Constraints[i] = constraintJSON{Relation: string(c.Relation), RHS: c.RHS, Origins: c.Origins}
		if c.Terms != nil {
			input.Constraints[i].Terms = make([]termJSON, len(c.Terms))
		}
		for j, term := range c.Terms {
			input.Constraints[i].Terms[j] = termJSON(term)
		}
	}
	return input
}

func variablesJSON(variables []sat.Variable) []variableJSON {
	if variables == nil {
		return nil
	}
	out := make([]variableJSON, len(variables))
	for i, v := range variables {
		out[i] = variableJSON(v)
	}
	return out
}

// assignmentJSON fixes the form of assignment.json.
type assignmentJSON struct {
	Booleans map[int]bool   `json:"booleans"`
	Integers map[int]string `json:"integers"`
}
