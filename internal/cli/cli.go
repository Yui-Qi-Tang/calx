// Package cli wires the standalone command to Calx's public library packages.
package cli

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Yui-Qi-Tang/calx/digest"
	"github.com/Yui-Qi-Tang/calx/finite"
	"github.com/Yui-Qi-Tang/calx/incidence"
	"github.com/Yui-Qi-Tang/calx/internal/jsoncompat"
	"github.com/Yui-Qi-Tang/calx/internal/jsoninput"
	"github.com/Yui-Qi-Tang/calx/internal/toolmanifest"
	"github.com/Yui-Qi-Tang/calx/internal/toolrun"
	"github.com/Yui-Qi-Tang/calx/sat"
	"github.com/Yui-Qi-Tang/calx/sat/cadical"
)

const maxInput = 16 << 20

// errUnsupported marks a finite request with an unknown query profile.
var errUnsupported = errors.New("unsupported finite profile")

// The CLI owns its JSON forms; finite types carry no JSON tags. Conversions
// pass slices and maps through, so nil and empty stay distinct.
type finiteRequest struct {
	Model   modelInput   `json:"model"`
	Query   queryInput   `json:"query"`
	Options optionsInput `json:"options"`
}
type modelInput struct {
	Fields    map[string][]string `json:"fields"`
	Forbidden []finite.State      `json:"forbidden"`
	Actions   []actionInput       `json:"actions"`
}
type actionInput struct {
	ID     string       `json:"id"`
	Guard  finite.State `json:"guard"`
	Effect finite.State `json:"effect"`
	Frame  []string     `json:"frame"`
}
type queryInput struct {
	Profile string       `json:"profile"`
	Start   finite.State `json:"start"`
	Goal    finite.State `json:"goal"`
	Horizon *int         `json:"horizon"`
	Blocked [][]string   `json:"blocked,omitempty"`
}

func (m modelInput) finite() finite.Model {
	var actions []finite.Action
	if m.Actions != nil {
		actions = make([]finite.Action, len(m.Actions))
		for i, a := range m.Actions {
			actions[i] = finite.Action(a)
		}
	}
	return finite.Model{Fields: m.Fields, Forbidden: m.Forbidden, Actions: actions}
}

// optionsInput is the request's encoding budget. A zero or omitted field keeps
// the finite default.
type optionsInput struct {
	MaxVariables int `json:"max_variables"`
	MaxClauses   int `json:"max_clauses"`
}

func (o optionsInput) finite() []finite.Option {
	var opts []finite.Option
	if o.MaxVariables != 0 {
		opts = append(opts, finite.WithMaxVariables(o.MaxVariables))
	}
	if o.MaxClauses != 0 {
		opts = append(opts, finite.WithMaxClauses(o.MaxClauses))
	}
	return opts
}

func (r finiteRequest) compile(ctx context.Context) (*finite.Encoding, error) {
	if r.Query.Horizon == nil {
		return nil, fmt.Errorf("%w: horizon required", finite.ErrInput)
	}
	// profile versions the request format; finite.Profile is the only one.
	if r.Query.Profile != finite.Profile {
		return nil, errUnsupported
	}
	q := finite.Query{Start: r.Query.Start, Goal: r.Query.Goal, Horizon: *r.Query.Horizon, Blocked: r.Query.Blocked}
	return finite.Compile(ctx, r.Model.finite(), q, r.Options.finite()...)
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
	return jsoncompat.Write(w, r, "  ")
}
func code(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "CANCELED"
	case errors.Is(err, context.DeadlineExceeded):
		return "TIMEOUT"
	case errors.Is(err, sat.ErrLimit) || errors.Is(err, incidence.ErrLimit):
		return "LIMIT"
	case errors.Is(err, errUnsupported):
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
		var m toolmanifest.Manifest
		if m, err = toolmanifest.Write(opts.tools.SolverPath, opts.tools.CheckerPath, opts.output); err == nil {
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

// graphJSON, edgeJSON and projectionJSON are the JSON forms of incidence
// types. Conversions pass slices and maps through, so nil and empty stay
// distinct.
type graphJSON struct {
	NodeIDs []string   `json:"node_ids"`
	Edges   []edgeJSON `json:"edges"`
}
type edgeJSON struct {
	ID       string `json:"id"`
	From     string `json:"from"`
	To       string `json:"to"`
	Relation string `json:"relation"`
}
type projectionJSON struct {
	NodeIDs       []string         `json:"node_ids"`
	EdgeIDs       []string         `json:"edge_ids"`
	Source        [][]int          `json:"source"`
	Target        [][]int          `json:"target"`
	RelationMasks map[string][]int `json:"relation_masks"`
}

func project(raw []byte) (any, error) {
	var g graphJSON
	if err := jsoninput.Decode(raw, &g); err != nil {
		return nil, err
	}
	if err := jsoninput.Require(raw, "node_ids", "edges"); err != nil {
		return nil, err
	}
	var edges []incidence.Edge
	if g.Edges != nil {
		edges = make([]incidence.Edge, len(g.Edges))
		for i, e := range g.Edges {
			edges[i] = incidence.Edge(e)
		}
	}
	p, err := incidence.Project(incidence.Graph{NodeIDs: g.NodeIDs, Edges: edges}, incidence.Limits{})
	if err != nil {
		// A failed projection reports empty matrices, not nulls.
		return projectionJSON{NodeIDs: []string{}, EdgeIDs: []string{}, Source: [][]int{}, Target: [][]int{}, RelationMasks: map[string][]int{}}, err
	}
	return projectionJSON(p), nil
}

func reconstruct(raw []byte) (any, error) {
	var p projectionJSON
	if err := jsoninput.Decode(raw, &p); err != nil {
		return nil, err
	}
	if err := jsoninput.Require(raw, "node_ids", "edge_ids", "source", "target", "relation_masks"); err != nil {
		return nil, err
	}
	g, err := incidence.Projection(p).Graph(incidence.Limits{})
	if err != nil {
		// A failed reconstruction reports an empty graph, not nulls.
		return graphJSON{NodeIDs: []string{}, Edges: []edgeJSON{}}, err
	}
	var edges []edgeJSON
	if g.Edges != nil {
		edges = make([]edgeJSON, len(g.Edges))
		for i, e := range g.Edges {
			edges[i] = edgeJSON(e)
		}
	}
	return graphJSON{NodeIDs: g.NodeIDs, Edges: edges}, nil
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
	trace, err := e.Decode(in.Assignment)
	if err != nil {
		// A failed decode reports an empty trace, not nulls.
		return traceOutput{States: []finite.State{}, Actions: []string{}}, err
	}
	return traceOutput(trace), nil
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

// variableOutput is the JSON form of one finite variable in encode output.
type variableOutput struct {
	ID        int                 `json:"id"`
	Kind      finite.VariableKind `json:"kind"`
	Time      int                 `json:"time"`
	OtherTime *int                `json:"other_time,omitempty"`
	Field     finite.FieldName    `json:"field,omitempty"`
	Value     finite.Category     `json:"value,omitempty"`
	Action    finite.ActionID     `json:"action,omitempty"`
}

// traceOutput is the JSON form of a decoded finite trace.
type traceOutput struct {
	States  []finite.State `json:"states"`
	Actions []string       `json:"actions"`
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
	var outputs []variableOutput
	if variables != nil {
		outputs = make([]variableOutput, len(variables))
		for i, v := range variables {
			outputs[i] = variableOutput(v)
		}
	}
	var b bytes.Buffer
	if err := c.WriteDIMACS(&b); err != nil {
		return nil, err
	}
	return struct {
		QueryDigest digest.Digest    `json:"query_digest"`
		CNF         cnfJSON          `json:"cnf"`
		Variables   []variableOutput `json:"variables"`
		DIMACS      string           `json:"dimacs"`
	}{QueryDigest: queryDigest, CNF: cnfOutput(c), Variables: outputs, DIMACS: b.String()}, nil
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
	var c cnfJSON
	if err := jsoninput.Decode(raw, &c); err != nil {
		return sat.CNF{}, err
	}
	if err := jsoninput.Require(raw, "variables", "clauses"); err != nil {
		return sat.CNF{}, err
	}
	return c.sat(), nil
}

// cnfJSON is the JSON form of a CNF in solve input and encode output.
type cnfJSON struct {
	Variables []satVariableJSON `json:"variables"`
	Clauses   [][]int           `json:"clauses"`
	Origins   [][]string        `json:"origins,omitempty"`
}
type satVariableJSON struct {
	ID       int               `json:"id"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

func cnfOutput(c sat.CNF) cnfJSON {
	var variables []satVariableJSON
	if c.Variables != nil {
		variables = make([]satVariableJSON, len(c.Variables))
		for i, v := range c.Variables {
			variables[i] = satVariableJSON(v)
		}
	}
	return cnfJSON{Variables: variables, Clauses: c.Clauses, Origins: c.Origins}
}

func (c cnfJSON) sat() sat.CNF {
	var variables []sat.Variable
	if c.Variables != nil {
		variables = make([]sat.Variable, len(c.Variables))
		for i, v := range c.Variables {
			variables[i] = sat.Variable(v)
		}
	}
	return sat.CNF{Variables: variables, Clauses: c.Clauses, Origins: c.Origins}
}

// resultOutput, validationOutput and artifactOutput are the JSON form of a
// solver result.
type resultOutput struct {
	Status        sat.Status       `json:"status"`
	InputDigest   digest.Digest    `json:"input_digest,omitempty"`
	FormulaDigest digest.Digest    `json:"formula_digest,omitempty"`
	SolverDigest  digest.Digest    `json:"solver_digest"`
	CheckerDigest digest.Digest    `json:"checker_digest,omitempty"`
	Model         validationOutput `json:"model"`
	Proof         validationOutput `json:"proof"`
	Artifacts     []artifactOutput `json:"artifacts,omitempty"`
	Assignment    map[int]bool     `json:"assignment,omitempty"`
}
type validationOutput struct {
	Present      bool          `json:"present"`
	Decoded      bool          `json:"decoded"`
	Checked      bool          `json:"checked"`
	Method       string        `json:"method,omitempty"`
	TargetDigest digest.Digest `json:"target_digest,omitempty"`
}
type artifactOutput struct {
	ID     string        `json:"id"`
	Digest digest.Digest `json:"digest"`
	Bytes  int64         `json:"bytes"`
}

func newResultOutput(r cadical.Result) resultOutput {
	artifacts := make([]artifactOutput, len(r.Artifacts))
	for i, a := range r.Artifacts {
		artifacts[i] = artifactOutput(a)
	}
	return resultOutput{
		Status: r.Status, InputDigest: r.InputDigest, FormulaDigest: r.FormulaDigest,
		SolverDigest: r.SolverDigest, CheckerDigest: r.CheckerDigest,
		Model: validationOutput(r.Model), Proof: validationOutput(r.Proof),
		Artifacts: artifacts, Assignment: r.Assignment,
	}
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
		Result      resultOutput  `json:"result"`
		QueryDigest digest.Digest `json:"query_digest,omitempty"`
		Trace       *traceOutput  `json:"trace,omitempty"`
	}{Result: newResultOutput(result), QueryDigest: queryDigest}
	if err == nil {
		err = result.Require(sat.Requirements{SATModelChecked: true, UNSATProofChecked: true})
	}
	if err == nil && e != nil && result.Status == sat.SAT {
		var trace finite.Trace
		trace, err = e.Decode(result.Assignment)
		if err == nil {
			out := traceOutput(trace)
			data.Trace = &out
		}
	}
	return data, err
}
