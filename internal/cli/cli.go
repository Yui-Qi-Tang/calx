// Package cli wires the standalone command to Calx's public library packages.
package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/Yui-Qi-Tang/calx/finite"
	"github.com/Yui-Qi-Tang/calx/incidence"
	"github.com/Yui-Qi-Tang/calx/internal/jsoninput"
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
	Schema      string   `json:"schema_version"`
	Outcome     string   `json:"outcome"`
	InputSHA256 string   `json:"input_sha256,omitempty"`
	Data        any      `json:"data,omitempty"`
	Error       *failure `json:"error,omitempty"`
}
type failure struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func digest(raw []byte) string { s := sha256.Sum256(raw); return hex.EncodeToString(s[:]) }
func emit(w io.Writer, r response) error {
	r.Schema = "calx-cli/v1"
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

// Run executes one command. Zero means the command completed with its required
// checks, two means invalid command-line flags, and one means any other failure.
// SAT/UNSAT labels may still appear in diagnostic data on a failed command.
func Run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return report(stderr, "usage: calx project|reconstruct|encode|decode|solve|solve-finite [flags]", 2)
	}
	cmd := args[0]
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.SetOutput(stderr)
	input := fs.String("in", "-", "input file, or - for stdin")
	format := fs.String("format", "json", "solve input: json or dimacs")
	solver := fs.String("solver", "", "CaDiCaL executable")
	solverHash := fs.String("solver-sha256", "", "pinned solver SHA256")
	checker := fs.String("checker", "", "DRAT-trim executable")
	checkerHash := fs.String("checker-sha256", "", "pinned checker SHA256")
	artifact := fs.String("artifacts", "", "new artifact directory")
	id := fs.String("id", "query", "query artifact ID")
	timeout := fs.Duration("timeout", 30*time.Second, "total solve deadline")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		return report(stderr, "unexpected positional arguments", 2)
	}
	switch cmd {
	case "project", "reconstruct", "encode", "decode", "solve", "solve-finite":
	default:
		return report(stderr, "unknown command", 2)
	}
	if *format != "json" && !(cmd == "solve" && *format == "dimacs") {
		return report(stderr, "unsupported input format", 2)
	}
	var raw []byte
	var err error
	if *input == "-" {
		raw, err = io.ReadAll(io.LimitReader(stdin, maxInput+1))
	} else {
		var f *os.File
		f, err = os.Open(*input)
		if err == nil {
			raw, err = io.ReadAll(io.LimitReader(f, maxInput+1))
			err = errors.Join(err, f.Close())
		}
	}
	if len(raw) > maxInput {
		err = errors.Join(err, sat.ErrLimit)
	}
	out := response{Outcome: "ERROR", InputSHA256: digest(raw)}
	if err == nil {
		out.Data, err = execute(ctx, cmd, *format, raw, cadical.Config{SolverPath: *solver, SolverSHA256: *solverHash, CheckerPath: *checker, CheckerSHA256: *checkerHash, ArtifactDir: *artifact, Limits: sat.Limits{Timeout: *timeout}}, *id)
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

func report(w io.Writer, message string, exitCode int) int {
	if _, err := fmt.Fprintln(w, message); err != nil {
		return 1
	}
	return exitCode
}

func execute(ctx context.Context, cmd, format string, raw []byte, cfg cadical.Config, id string) (any, error) {
	switch cmd {
	case "project":
		var g incidence.Graph
		if err := jsoninput.Decode(raw, &g); err != nil {
			return nil, err
		}
		if err := jsoninput.Require(raw, "node_ids", "edges"); err != nil {
			return nil, err
		}
		return incidence.Project(g, incidence.Limits{})
	case "reconstruct":
		var p incidence.Projection
		if err := jsoninput.Decode(raw, &p); err != nil {
			return nil, err
		}
		if err := jsoninput.Require(raw, "node_ids", "edge_ids", "source", "target", "relation_masks"); err != nil {
			return nil, err
		}
		return p.Graph(incidence.Limits{})
	case "decode":
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
	var c sat.CNF
	var e *finite.Encoding
	var queryHash string
	var err error
	if cmd == "encode" || cmd == "solve-finite" {
		var in finiteRequest
		if err = jsoninput.Decode(raw, &in); err != nil {
			return nil, err
		}
		e, err = in.compile(ctx)
		if err != nil {
			return nil, err
		}
		c, err = e.CNF()
		if err != nil {
			return nil, err
		}
		queryHash, err = e.Digest()
		if err != nil {
			return nil, err
		}
		if cmd == "encode" {
			variables, err := e.Variables()
			if err != nil {
				return nil, err
			}
			var b bytes.Buffer
			if err = c.WriteDIMACS(&b); err != nil {
				return nil, err
			}
			return struct {
				QuerySHA256 string            `json:"query_sha256"`
				CNF         sat.CNF           `json:"cnf"`
				Variables   []finite.Variable `json:"variables"`
				DIMACS      string            `json:"dimacs"`
			}{QuerySHA256: queryHash, CNF: c, Variables: variables, DIMACS: b.String()}, nil
		}
	} else if format == "dimacs" {
		c, err = sat.ReadDIMACS(bytes.NewReader(raw), sat.Limits{})
	} else {
		err = jsoninput.Decode(raw, &c)
		if err == nil {
			err = jsoninput.Require(raw, "variables", "clauses")
		}
	}
	if err != nil {
		return nil, err
	}
	if err = c.CheckShape(); err != nil {
		return nil, err
	}
	r, err := cadical.New(cfg)
	if err != nil {
		return nil, err
	}
	// Preserve the exact transport input separately from the encoded CNF identity.
	f, err := os.OpenFile(filepath.Join(cfg.ArtifactDir, "request.input"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	_, err = f.Write(raw)
	err = errors.Join(err, f.Close())
	if err != nil {
		return nil, err
	}
	result, err := r.Solve(ctx, id, c)
	data := struct {
		Result      cadical.Result `json:"result"`
		QuerySHA256 string         `json:"query_sha256,omitempty"`
		Trace       *finite.Trace  `json:"trace,omitempty"`
	}{Result: result, QuerySHA256: queryHash}
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
