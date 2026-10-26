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
	"path/filepath"
	"strings"

	"github.com/Yui-Qi-Tang/calx/digest"
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
	raw, err := readInput(opts.input, stdin)
	out := response{Outcome: "ERROR", InputDigest: digest.Sum(raw)}
	if err == nil {
		out.Data, err = execute(ctx, opts.command, opts.format, raw, opts.tools, opts.id)
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
	var queryDigest digest.Digest
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
		queryDigest, err = e.Digest()
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
				QueryDigest digest.Digest     `json:"query_digest"`
				CNF         sat.CNF           `json:"cnf"`
				Variables   []finite.Variable `json:"variables"`
				DIMACS      string            `json:"dimacs"`
			}{QueryDigest: queryDigest, CNF: c, Variables: variables, DIMACS: b.String()}, nil
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
