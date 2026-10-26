// Package cli wires the standalone command to Calx's public library packages.
package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Yui-Qi-Tang/calx/digest"
	"github.com/Yui-Qi-Tang/calx/finite"
	"github.com/Yui-Qi-Tang/calx/incidence"
	"github.com/Yui-Qi-Tang/calx/internal/jsoninput"
	"github.com/Yui-Qi-Tang/calx/internal/toolrun"
	"github.com/Yui-Qi-Tang/calx/sat"
	"github.com/Yui-Qi-Tang/calx/sat/cadical"
)

const maxInput = 16 << 20

type finiteRequest struct {
	Model   finite.Model   `json:"model"`
	Query   queryInput     `json:"query"`
	Options finite.Options `json:"options,omitempty"`
}
type queryInput struct {
	Profile string       `json:"profile"`
	Start   finite.State `json:"start"`
	Goal    finite.State `json:"goal"`
	Horizon *int         `json:"horizon"`
	Blocked [][]string   `json:"blocked,omitempty"`
}

func (r finiteRequest) compile(ctx context.Context) (*finite.Encoding, error) {
	if r.Query.Horizon == nil {
		return nil, fmt.Errorf("%w: horizon required", finite.ErrInput)
	}
	q := finite.Query{Profile: r.Query.Profile, Start: r.Query.Start, Goal: r.Query.Goal, Horizon: *r.Query.Horizon, Blocked: r.Query.Blocked}
	return finite.Compile(ctx, r.Model, q, r.Options)
}

type response struct {
	Schema      string        `json:"schema_version"`
	Outcome     string        `json:"outcome"`
	InputDigest digest.Digest `json:"input_digest,omitempty"`
	Data        any           `json:"data,omitempty"`
	Error       *failure      `json:"error,omitempty"`
}
type failure struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func emit(w io.Writer, r response) error {
	r.Schema = "calx-cli/v2"
	e := json.NewEncoder(w)
	e.SetIndent("", "  ")
	return e.Encode(r)
}
func code(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "CANCELED"
	case errors.Is(err, context.DeadlineExceeded):
		return "TIMEOUT"
	case errors.Is(err, sat.ErrLimit) || errors.Is(err, incidence.ErrLimit):
		return "LIMIT"
	case errors.Is(err, finite.ErrUnsupported) || errors.Is(err, sat.ErrUnsupported):
		return "UNSUPPORTED"
	case errors.Is(err, sat.ErrInconclusive):
		return "INCONCLUSIVE"
	case errors.Is(err, sat.ErrGuarantee):
		return "GUARANTEE"
	case errors.Is(err, sat.ErrProtocol):
		return "PROTOCOL"
	case errors.Is(err, sat.ErrValidation):
		return "VALIDATION"
	default:
		return "INPUT_OR_EXECUTION"
	}
}

// Run executes one command. Zero means help was displayed or the command met its
// checks, two means invalid command-line usage, and one means any other failure.
// SAT/UNSAT labels may still appear in diagnostic data on a failed command.
func Run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	var diagnostics bytes.Buffer
	opts, err := parseOptions(args, &diagnostics)
	if err != nil {
		exit := 2
		if errors.Is(err, flag.ErrHelp) {
			exit = 0
		}
		message := strings.TrimSpace(diagnostics.String())
		if message == "" {
			message = err.Error()
		}
		return report(stderr, message, exit)
	}
	var out response
	if opts.command == "pin-tools" {
		out = response{Outcome: "ERROR"}
		var m toolManifest
		if m, err = pinTools(opts.tools.SolverPath, opts.tools.CheckerPath, opts.output); err == nil {
			out.Data = m
		}
	} else {
		var raw []byte
		raw, err = readInput(opts.input, stdin)
		out = response{Outcome: "ERROR", InputDigest: digest.Sum(raw)}
		if err == nil {
			out.Data, err = execute(ctx, opts.command, opts.format, raw, opts.tools, opts.id)
		}
	}
	exit := 0
	if err != nil {
		exit = 1
		out.Error = &failure{Code: code(err), Message: err.Error()}
	} else {
		out.Outcome = "OK"
	}
	if e := emit(stdout, out); e != nil {
		return report(stderr, e.Error(), 1)
	}
	return exit
}

func readInput(path string, stdin io.Reader) (raw []byte, err error) {
	reader := stdin
	if path != "-" {
		file, openErr := os.Open(path)
		if openErr != nil {
			return nil, openErr
		}
		defer func() { err = errors.Join(err, file.Close()) }()
		reader = file
	}
	raw, err = io.ReadAll(io.LimitReader(reader, maxInput+1))
	if len(raw) > maxInput {
		err = errors.Join(err, sat.ErrLimit)
	}
	return raw, err
}

func report(w io.Writer, message string, exitCode int) int {
	if _, err := fmt.Fprintln(w, message); err != nil {
		return 1
	}
	return exitCode
}

func execute(ctx context.Context, cmd, format string, raw []byte, cfg cadical.Config, id string) (any, error) {
	switch cmd {
	case "project":
		return project(raw)
	case "reconstruct":
		return reconstruct(raw)
	case "decode":
		return decode(ctx, raw)
	case "encode":
		return encode(ctx, raw)
	case "solve-finite":
		return solveFinite(ctx, raw, cfg, id)
	default: // "solve"; parseOptions rejects every other command.
		c, err := readCNF(format, raw)
		if err != nil {
			return nil, err
		}
		return solve(ctx, raw, c, cfg, id, nil, "")
	}
}

func project(raw []byte) (any, error) {
	var g incidence.Graph
	if err := jsoninput.Decode(raw, &g); err != nil {
		return nil, err
	}
	if err := jsoninput.Require(raw, "node_ids", "edges"); err != nil {
		return nil, err
	}
	return incidence.Project(g, incidence.Limits{})
}

func reconstruct(raw []byte) (any, error) {
	var p incidence.Projection
	if err := jsoninput.Decode(raw, &p); err != nil {
		return nil, err
	}
	if err := jsoninput.Require(raw, "node_ids", "edge_ids", "source", "target", "relation_masks"); err != nil {
		return nil, err
	}
	return p.Graph(incidence.Limits{})
}

func decode(ctx context.Context, raw []byte) (any, error) {
	var in struct {
		Request    finiteRequest `json:"request"`
		Assignment map[int]bool  `json:"assignment"`
	}
	if err := jsoninput.Decode(raw, &in); err != nil {
		return nil, err
	}
	e, err := in.Request.compile(ctx)
	if err != nil {
		return nil, err
	}
	return e.Decode(in.Assignment)
}

// compileRequest decodes a finite request and compiles its query, returning
// the encoding with its formula and query digest.
func compileRequest(ctx context.Context, raw []byte) (*finite.Encoding, sat.CNF, digest.Digest, error) {
	var in finiteRequest
	if err := jsoninput.Decode(raw, &in); err != nil {
		return nil, sat.CNF{}, "", err
	}
	e, err := in.compile(ctx)
	if err != nil {
		return nil, sat.CNF{}, "", err
	}
	c, err := e.CNF()
	if err != nil {
		return nil, sat.CNF{}, "", err
	}
	queryDigest, err := e.Digest()
	if err != nil {
		return nil, sat.CNF{}, "", err
	}
	return e, c, queryDigest, nil
}

func encode(ctx context.Context, raw []byte) (any, error) {
	e, c, queryDigest, err := compileRequest(ctx, raw)
	if err != nil {
		return nil, err
	}
	variables, err := e.Variables()
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	if err := c.WriteDIMACS(&b); err != nil {
		return nil, err
	}
	return struct {
		QueryDigest digest.Digest     `json:"query_digest"`
		CNF         sat.CNF           `json:"cnf"`
		Variables   []finite.Variable `json:"variables"`
		DIMACS      string            `json:"dimacs"`
	}{QueryDigest: queryDigest, CNF: c, Variables: variables, DIMACS: b.String()}, nil
}

func solveFinite(ctx context.Context, raw []byte, cfg cadical.Config, id string) (any, error) {
	e, c, queryDigest, err := compileRequest(ctx, raw)
	if err != nil {
		return nil, err
	}
	return solve(ctx, raw, c, cfg, id, e, queryDigest)
}

func readCNF(format string, raw []byte) (sat.CNF, error) {
	if format == "dimacs" {
		return sat.ReadDIMACS(bytes.NewReader(raw), sat.Limits{})
	}
	var c sat.CNF
	if err := jsoninput.Decode(raw, &c); err != nil {
		return sat.CNF{}, err
	}
	if err := jsoninput.Require(raw, "variables", "clauses"); err != nil {
		return sat.CNF{}, err
	}
	return c, nil
}

// solve runs the checked solver on c. A non-nil e decodes a SAT assignment
// into its finite-model trace.
func solve(ctx context.Context, raw []byte, c sat.CNF, cfg cadical.Config, id string, e *finite.Encoding, queryDigest digest.Digest) (any, error) {
	if err := c.CheckShape(); err != nil {
		return nil, err
	}
	r, err := cadical.New(cfg)
	if err != nil {
		return nil, err
	}
	// Preserve the exact transport input separately from the encoded CNF identity.
	if _, err := toolrun.Save(cfg.ArtifactDir, "request.input", raw); err != nil {
		return nil, err
	}
	result, err := r.Solve(ctx, id, c)
	data := struct {
		Result      cadical.Result `json:"result"`
		QueryDigest digest.Digest  `json:"query_digest,omitempty"`
		Trace       *finite.Trace  `json:"trace,omitempty"`
	}{Result: result, QueryDigest: queryDigest}
	if err == nil {
		err = result.Require(sat.Requirements{SATModelChecked: true, UNSATProofChecked: true})
	}
	if err == nil && e != nil && result.Status == sat.SAT {
		var trace finite.Trace
		trace, err = e.Decode(result.Assignment)
		if err == nil {
			data.Trace = &trace
		}
	}
	return data, err
}
