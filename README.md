# Calx

Calx is a domain-neutral Go module for graph incidence projection, bounded
categorical-model encoding, bounded analysis of caller-supplied Boolean
automata, checked Boolean SAT solving, conflict localization and a small
linear integer profile. Packages can be imported independently.

| Package | Responsibility | Example |
| --- | --- | --- |
| `digest` | Shared content-digest value, generation, parsing and validation | [Identify exact bytes](#8-digest-identify-exact-bytes) |
| `incidence` | Directed multigraph topology ↔ source/target incidence matrices, IDs and opaque relation masks | [Project and reconstruct](#1-incidence-project-and-reconstruct) |
| `finite` | Validated categorical model → operation matrix → exact-length CNF; assignment → trace | [Matrices and CNF](#2-finite-project-and-compile) |
| `automaton` | Caller-implemented synchronous Boolean automaton → exact-length CNF; assignment → replayable trajectory (v0.1) | [Simulate, prove and break](#5-automaton-and-lga-simulate-prove-and-break-a-lattice-gas) |
| `lga` | HPP and FHP-I lattice gases with walls as `automaton.System`; bit-packed simulation, conserved quantities, density and velocity fields (v0.1) | [Simulate, prove and break](#5-automaton-and-lga-simulate-prove-and-break-a-lattice-gas) |
| `sat` | CNF, DIMACS, total-assignment checking, limits and explicit validation records | [Check a formula](#3-sat-check-a-formula) |
| `sat/cadical` | Pinned CaDiCaL and DRAT-trim processes, checked results and retained artifacts | [Checked solve and trace](#4-satcadical-solve-check-and-decode) |
| `mus` | Caller-implemented `Oracle` over constraint groups → one irreducible conflicting subset, for any backend | [Shrink a conflict](#6-mus-shrink-a-conflict) |
| `linear` | Boolean CNF plus a conjunction of linear integer comparisons: validation, SMT-LIB encoding and exact model checking | [Linear integers](#7-linear-and-linearz3-solve-linear-integers) |
| `linear/z3` | Pinned Z3 process, checked SAT models and retained artifacts; UNSAT has no checked proof | [Linear integers](#7-linear-and-linearz3-solve-linear-integers) |

## Documentation

| Document | Read it for |
| --- | --- |
| [Architecture](docs/architecture.md) | Package boundaries, imports, data flow and scope |
| [Contracts](docs/contracts.md) | Validation rules, limits and what each answer guarantees |
| [Data structures](docs/data-structures.md) | Go types, JSON fields, variable numbering and matrix dimensions |
| [Error handling](docs/error-handling.md) | Error-returning accessors, variable roles and inconclusive answers |
| [Execution flow](docs/execution-flow.md) | Diagrams of the finite, automaton, conflict localization and linear routes |
| [Graph to matrix](docs/graph-to-matrix.md) | A visual walkthrough of incidence projection |
| [SAT primer](docs/sat-primer.md) | A picture-based introduction to CNF, SAT/UNSAT answers and how Calx checks them |
| [Finite model primer](docs/finite-primer.md) | A picture-based introduction to finite models, queries and their CNF encoding |
| [Automata primer](docs/automata-primer.md) | A picture-based introduction to automata, their CNF encoding and Garden of Eden states |
| [Lattice gas primer](docs/lga-primer.md) | A picture-based introduction to HPP and FHP lattice gases and the questions a solver can answer about them |
| [Conflict localization primer](docs/mus-primer.md) | A picture-based walk through shrinking a conflict, its guarantees and writing an Oracle |
| [Linear integer primer](docs/linear-primer.md) | A picture-based introduction to linear integer problems, what Z3's answers guarantee and how they differ from CaDiCaL's |
| [Validation log](docs/validation.md) | Dated verification records and retained evidence |

## Library examples

Run all commands below from the repository root. Sections 1–3 and 8 use only Go.
Sections 4, 6 and 7 require explicitly pinned solver executables, and section 5
needs them only for `-check`.

### 1. incidence: project and reconstruct

Declare topology directly with `incidence.Graph`. This example includes two
parallel edges, a self-loop and an isolated node. It prints the projection and
the graph reconstructed from the matrices, axes and relation masks.

[Complete runnable source](examples/incidence/main.go):

```go
package main

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"log"
	"os"

	"github.com/Yui-Qi-Tang/calx/incidence"
)

func main() {
	graph := incidence.Graph{
		NodeIDs: []string{"c", "a", "b"},
		Edges: []incidence.Edge{
			{ID: "e1", From: "a", To: "b", Relation: "connects"},
			{ID: "e2", From: "a", To: "b", Relation: "connects"},
			{ID: "e3", From: "b", To: "b", Relation: "loops"},
		},
	}
	projection, err := incidence.Project(graph, incidence.Limits{})
	if err != nil {
		log.Fatal(err)
	}
	reconstructed, err := projection.Graph(incidence.Limits{})
	if err != nil {
		log.Fatal(err)
	}
	// Deterministic sorts map keys so the output is reproducible.
	output := jsontext.NewEncoder(os.Stdout, jsontext.WithIndent("  "))
	if err := json.MarshalEncode(output, struct {
		Projection    projectionJSON `json:"projection"`
		Reconstructed graphJSON      `json:"reconstructed"`
	}{Projection: projectionJSON(projection), Reconstructed: toGraphJSON(reconstructed)}, json.Deterministic(true)); err != nil {
		log.Fatal(err)
	}
}

// incidence types carry no JSON tags, so the example defines its own output.
type projectionJSON struct {
	NodeIDs       []string         `json:"node_ids"`
	EdgeIDs       []string         `json:"edge_ids"`
	Source        [][]int          `json:"source"`
	Target        [][]int          `json:"target"`
	RelationMasks map[string][]int `json:"relation_masks"`
}

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

func toGraphJSON(g incidence.Graph) graphJSON {
	edges := make([]edgeJSON, len(g.Edges))
	for i, e := range g.Edges {
		edges[i] = edgeJSON(e)
	}
	return graphJSON{NodeIDs: g.NodeIDs, Edges: edges}
}
```

```sh
go run ./examples/incidence
```

The sorted node axis is `[a,b,c]` and the edge axis is `[e1,e2,e3]`:

```text
source   = [[1,1,0], [0,0,1], [0,0,0]]
target   = [[0,0,0], [1,1,1], [0,0,0]]
connects = [1,1,0]
loops    = [0,0,1]
```

The reconstructed graph retains both parallel edges, the loop and isolated `c`.
Relation labels remain opaque. Payloads and provenance stay with the caller.

### 2. finite: project and compile

First inspect the operation matrices. Here `enable` changes `state=off` to
`state=on` while preserving `mode`. A forbidden pattern excludes
`mode=manual,state=on`; reconstruction receives that pattern separately.

[Complete runnable source](examples/matrix/main.go):

```go
package main

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"log"
	"os"

	"github.com/Yui-Qi-Tang/calx/finite"
)

func main() {
	model := finite.Model{
		Fields: map[string][]string{
			"mode":  {"auto", "manual"},
			"state": {"off", "on"},
		},
		Actions: []finite.Action{{
			ID:     "enable",
			Guard:  finite.State{"state": "off"},
			Effect: finite.State{"state": "on"},
			Frame:  []string{"mode"},
		}},
		Forbidden: []finite.State{{"mode": "manual", "state": "on"}},
	}
	matrix, err := finite.Project(model)
	if err != nil {
		log.Fatal(err)
	}
	// Forbidden patterns belong to the model; the matrix does not encode them.
	reconstructed, err := matrix.Model(model.Forbidden)
	if err != nil {
		log.Fatal(err)
	}
	// Deterministic sorts map keys so the output is reproducible.
	output := jsontext.NewEncoder(os.Stdout, jsontext.WithIndent("  "))
	if err := json.MarshalEncode(output, struct {
		Matrix        matrixJSON `json:"matrix"`
		Reconstructed modelJSON  `json:"reconstructed"`
	}{Matrix: toMatrixJSON(matrix), Reconstructed: toModelJSON(reconstructed)}, json.Deterministic(true)); err != nil {
		log.Fatal(err)
	}
}

// The example chooses its own JSON names; finite types have no JSON tags.
type matrixJSON struct {
	Fields      []string         `json:"fields"`
	Coordinates []coordinateJSON `json:"coordinates"`
	ActionIDs   []string         `json:"action_ids"`
	Guard       [][]int          `json:"guard"`
	Effect      [][]int          `json:"effect"`
	Frame       [][]int          `json:"frame"`
}

type coordinateJSON struct {
	Field string `json:"field"`
	Value string `json:"value"`
}

type modelJSON struct {
	Fields    map[string][]string `json:"fields"`
	Forbidden []finite.State      `json:"forbidden"`
	Actions   []actionJSON        `json:"actions"`
}

type actionJSON struct {
	ID     string       `json:"id"`
	Guard  finite.State `json:"guard"`
	Effect finite.State `json:"effect"`
	Frame  []string     `json:"frame"`
}

func toMatrixJSON(m finite.Matrix) matrixJSON {
	coordinates := make([]coordinateJSON, len(m.Coordinates))
	for i, c := range m.Coordinates {
		coordinates[i] = coordinateJSON(c)
	}
	return matrixJSON{Fields: m.Fields, Coordinates: coordinates, ActionIDs: m.ActionIDs, Guard: m.Guard, Effect: m.Effect, Frame: m.Frame}
}

func toModelJSON(m finite.Model) modelJSON {
	actions := make([]actionJSON, len(m.Actions))
	for i, a := range m.Actions {
		actions[i] = actionJSON(a)
	}
	return modelJSON{Fields: m.Fields, Forbidden: m.Forbidden, Actions: actions}
}
```

```sh
go run ./examples/matrix
```

The coordinate axis is `[mode/auto, mode/manual, state/off, state/on]`, the field
axis is `[mode,state]`, and the only action row is `enable`:

```text
guard  = [[0,0,1,0]]
effect = [[0,0,0,1]]
frame  = [[1,0]]
```

Zero guard cells impose no requirement; frame `1` preserves `mode`.
`Matrix.Model` restores the declared rules with the supplied forbidden patterns.

For CNF compilation, the next example uses a smaller model with only `state`.
This example declares one action: `enable` changes `state=off` to `state=on`.
The query asks for a path of exactly one action.

[Complete runnable source](examples/compile/main.go):

```go
package main

import (
	"context"
	"log"
	"os"

	"github.com/Yui-Qi-Tang/calx/finite"
)

func main() {
	// A single action changes state from off to on.
	model := finite.Model{
		Fields: map[string][]string{"state": {"off", "on"}},
		Actions: []finite.Action{{
			ID:     "enable",
			Guard:  finite.State{"state": "off"},
			Effect: finite.State{"state": "on"},
		}},
	}
	query := finite.Query{
		Start:   finite.State{"state": "off"},
		Goal:    finite.State{"state": "on"},
		Horizon: 1, // Exactly one action.
	}

	encoding, err := finite.Compile(context.Background(), model, query)
	if err != nil {
		log.Fatal(err)
	}
	formula, err := encoding.CNF()
	if err != nil {
		log.Fatal(err)
	}
	if err := formula.WriteDIMACS(os.Stdout); err != nil {
		log.Fatal(err)
	}
}
```

```sh
go run ./examples/compile
```

This prints the DIMACS formula. `encoding` retains the mapping needed to turn a
satisfying assignment back into field values and action IDs. Compilation does
not start a solver or claim that a path exists.

### 3. sat: check a formula

This example declares `x AND (NOT x OR y)`, applies explicit variable/clause/literal
limits, writes and reads DIMACS, then checks the supplied total assignment
`x=true,y=true`. It starts no solver.

[Complete runnable source](examples/sat/main.go):

```go
package main

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"log"
	"os"

	"github.com/Yui-Qi-Tang/calx/sat"
)

func main() {
	// x AND (NOT x OR y) is satisfied by x=true, y=true.
	formula := sat.CNF{
		Variables: []sat.Variable{{ID: 1}, {ID: 2}},
		Clauses:   [][]int{{1}, {-1, 2}},
	}
	limits, err := (sat.Limits{
		MaxVariables: 2,
		MaxClauses:   2,
		MaxLiterals:  3,
	}).WithDefaults()
	if err != nil {
		log.Fatal(err)
	}
	if err := limits.CheckCNF(formula); err != nil {
		log.Fatal(err)
	}
	var dimacs bytes.Buffer
	if err := formula.WriteDIMACS(&dimacs); err != nil {
		log.Fatal(err)
	}
	parsed, err := sat.ReadDIMACS(bytes.NewReader(dimacs.Bytes()), limits)
	if err != nil {
		log.Fatal(err)
	}
	// This witness is supplied explicitly; no solver is started.
	assignment := map[int]bool{1: true, 2: true}
	if err := parsed.CheckAssignment(assignment); err != nil {
		log.Fatal(err)
	}
	// Deterministic sorts map keys so the output is reproducible.
	output := jsontext.NewEncoder(os.Stdout, jsontext.WithIndent("  "))
	if err := json.MarshalEncode(output, struct {
		DIMACS            string       `json:"dimacs"`
		Assignment        map[int]bool `json:"assignment"`
		AssignmentChecked bool         `json:"assignment_checked"`
	}{DIMACS: dimacs.String(), Assignment: assignment, AssignmentChecked: true}, json.Deterministic(true)); err != nil {
		log.Fatal(err)
	}
}
```

```sh
go run ./examples/sat
```

Expected output:

```json
{
  "dimacs": "p cnf 2 2\n1 0\n-1 2 0\n",
  "assignment": {
    "1": true,
    "2": true
  },
  "assignment_checked": true
}
```

`CheckAssignment` requires every variable and checks every clause. Missing or
violating values return an error. `WithDefaults` fills unspecified limits before
`CheckCNF`; exceeding a configured limit returns `sat.ErrLimit`.
DIMACS carries the formula, while application metadata and origins remain separate.
The next example uses `sat.Result.Require` on the runner's actual validation
records; constructing a result struct is not a substitute for those checks.

### 4. sat/cadical: solve, check and decode

The complete program below continues through the solver. Read its five numbered
steps: declare the problem, compile it, solve it, require checked evidence, then
decode a SAT assignment.

[Complete runnable source](examples/solve/main.go):

```go
package main

import (
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"flag"
	"log"
	"os"

	"github.com/Yui-Qi-Tang/calx/digest"
	"github.com/Yui-Qi-Tang/calx/finite"
	"github.com/Yui-Qi-Tang/calx/sat"
	"github.com/Yui-Qi-Tang/calx/sat/cadical"
)

func main() {
	horizon := flag.Int("horizon", 1, "exact action count (not an upper bound); 0 checks the start state")
	artifacts := flag.String("artifacts", "artifacts/go-solve", "new run directory; parent must exist and the run directory must not exist")
	flag.Parse()
	if err := run(context.Background(), *horizon, *artifacts); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, horizon int, artifacts string) error {
	// 1. Declare the model and the exact question.
	model := finite.Model{
		Fields: map[string][]string{"state": {"off", "on"}},
		Actions: []finite.Action{{
			ID:     "enable",
			Guard:  finite.State{"state": "off"},
			Effect: finite.State{"state": "on"},
		}},
	}
	query := finite.Query{
		Start:   finite.State{"state": "off"},
		Goal:    finite.State{"state": "on"},
		Horizon: horizon,
	}

	// 2. Compile the model/query to a formula and its decoding map.
	encoding, err := finite.Compile(ctx, model, query)
	if err != nil {
		return err
	}

	// 3. Configure pinned tools and solve the formula.
	runner, err := cadical.New(cadical.Config{
		SolverPath:    os.Getenv("CALX_CADICAL"),
		SolverDigest:  digest.Digest(os.Getenv("CALX_CADICAL_DIGEST")),
		CheckerPath:   os.Getenv("CALX_DRAT"),
		CheckerDigest: digest.Digest(os.Getenv("CALX_DRAT_DIGEST")),
		ArtifactDir:   artifacts,
	})
	if err != nil {
		return err
	}
	formula, err := encoding.CNF()
	if err != nil {
		return err
	}
	result, err := runner.Solve(ctx, "example", formula)
	if err != nil {
		return err // An answer label can survive a failed validation.
	}

	// 4. Require the evidence appropriate to SAT or UNSAT.
	if err := result.Require(sat.Requirements{
		SATModelChecked:   true,
		UNSATProofChecked: true,
	}); err != nil {
		return err
	}

	// 5. Only a checked SAT assignment can be decoded into a trace.
	type traceJSON struct {
		States  []finite.State `json:"states"`
		Actions []string       `json:"actions"`
	}
	var trace *traceJSON
	if result.Status == sat.SAT {
		decoded, err := encoding.Decode(result.Assignment)
		if err != nil {
			return err
		}
		trace = &traceJSON{States: decoded.States, Actions: decoded.Actions}
	}
	return json.MarshalEncode(jsontext.NewEncoder(os.Stdout), struct {
		Status  sat.Status `json:"status"`
		Horizon int        `json:"horizon"`
		Trace   *traceJSON `json:"trace,omitempty"`
	}{Status: result.Status, Horizon: horizon, Trace: trace})
}
```

The Go solve example reads these four environment variables:

| Variable | Value |
| --- | --- |
| `CALX_CADICAL` | File path to the CaDiCaL executable |
| `CALX_CADICAL_DIGEST` | Expected digest of that executable's complete contents |
| `CALX_DRAT` | File path to the DRAT-trim proof-checker executable |
| `CALX_DRAT_DIGEST` | Expected digest of that checker's complete contents |

Both tools and both digests are required during initialization, including runs
that ultimately return SAT. A digest identifies the executable file; it is not
the digest of the model, CNF or proof. The current digest format is **SHA-256 as
64 lowercase hexadecimal characters, without a prefix**. These names describe
the value's role; the implementation currently supports this one algorithm.

For local executables you have selected, configure the paths and compute their
digests on macOS:

```sh
export CALX_CADICAL=/absolute/path/to/cadical
export CALX_DRAT=/absolute/path/to/drat-trim
export CALX_CADICAL_DIGEST="$(shasum -a 256 "$CALX_CADICAL" | awk '{print $1}')"
export CALX_DRAT_DIGEST="$(shasum -a 256 "$CALX_DRAT" | awk '{print $1}')"
```

`-horizon` is the exact action count, not an upper bound. Zero tests whether the
start already meets the goal. `-artifacts` names a new run directory; its parent
must exist and the run directory must not exist. Then run:

```sh
mkdir -p artifacts
go run ./examples/solve -horizon 1 -artifacts artifacts/go-solve-sat
```

Expected output:

```json
{"status":"SAT","horizon":1,"trace":{"states":[{"state":"off"},{"state":"on"}],"actions":["enable"]}}
```

The same goal cannot be reached in zero actions:

```sh
go run ./examples/solve -horizon 0 -artifacts artifacts/go-solve-unsat
```

```json
{"status":"UNSAT","horizon":0}
```

Each run needs a new artifact directory. `Require` checks the guarantee relevant
to the returned answer; a solver or validation error is handled before that
check. `Decode` validates the assignment against this Encoding's formula.
The trace still needs the consumer's independent original-rule replay before
being treated as an accepted application result.

Inspect `<artifact-dir>/example/outcome.json` for the answer and validation
records. SAT records a checked model; UNSAT records a checked proof. The query
directory also retains `input.json`, `input.cnf`, solver output and, for UNSAT,
checker output. `Encoding.Decode` uses the same encoding that produced the
solved formula; a raw-CNF caller supplies its own interpretation map.

### 5. automaton and lga: simulate, prove and break a lattice gas

`automaton` works on any system the caller describes. Implement `automaton.System`
with four methods, and Calx can simulate the rule and encode questions about it,
seeing only cell and bit indices. This program implements elementary rule 90 on a
ring of four cells; the [automata primer](docs/automata-primer.md) explains it.

[Complete runnable source](examples/automaton/main.go):

```go
package main

import (
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"flag"
	"log"
	"os"

	"github.com/Yui-Qi-Tang/calx/automaton"
	"github.com/Yui-Qi-Tang/calx/digest"
	"github.com/Yui-Qi-Tang/calx/sat"
	"github.com/Yui-Qi-Tang/calx/sat/cadical"
)

// rule90 is the caller's system: each cell becomes its left neighbor XOR its
// right neighbor. Calx sees only cell and bit indices and this local rule.
type rule90 struct{ cells int }

func (r rule90) Cells() int { return r.cells }

func (r rule90) Bits() int { return 1 }

func (r rule90) Inputs(cell int) []automaton.Bit {
	return []automaton.Bit{{Cell: (cell + r.cells - 1) % r.cells}, {Cell: (cell + 1) % r.cells}}
}

func (r rule90) Next(_ int, in, out []bool) { out[0] = in[0] != in[1] }

func main() {
	check := flag.Bool("check", false, "run checked SAT queries; needs the CALX_* tool variables")
	artifacts := flag.String("artifacts", "artifacts/go-automaton", "new run directory for -check; parent must exist and the run directory must not exist")
	flag.Parse()
	var err error
	if *check {
		err = runCheck(context.Background(), *artifacts)
	} else {
		err = simulate()
	}
	if err != nil {
		log.Fatal(err)
	}
}

// picture draws a state as a row of cells: ● is true, ○ is false.
func picture(s automaton.State) string {
	out := ""
	for _, alive := range s {
		if alive {
			out += "●"
		} else {
			out += "○"
		}
	}
	return out
}

// simulate runs the rule forward with no solver.
func simulate() error {
	states, err := automaton.Simulate(rule90{cells: 4}, automaton.State{true, false, false, false}, 2, nil)
	if err != nil {
		return err
	}
	pictures := make([]string, len(states))
	for i, s := range states {
		pictures[i] = picture(s)
	}
	return json.MarshalEncode(jsontext.NewEncoder(os.Stdout), struct {
		States []string `json:"states"`
	}{States: pictures})
}

// runCheck asks whether two states have a predecessor. Simulation cannot answer
// this without trying every state; a solver answers it directly, and UNSAT
// comes with a checked proof.
func runCheck(ctx context.Context, artifacts string) error {
	runner, err := cadical.New(cadical.Config{
		SolverPath:    os.Getenv("CALX_CADICAL"),
		SolverDigest:  digest.Digest(os.Getenv("CALX_CADICAL_DIGEST")),
		CheckerPath:   os.Getenv("CALX_DRAT"),
		CheckerDigest: digest.Digest(os.Getenv("CALX_DRAT_DIGEST")),
		ArtifactDir:   artifacts,
	})
	if err != nil {
		return err
	}
	sys := rule90{cells: 4}

	// ●○○○ has no predecessor: it is a Garden of Eden state.
	eden, _, err := predecessor(ctx, runner, "eden", sys, automaton.State{true, false, false, false})
	if err != nil {
		return err
	}

	// ○●○● has one, and the solver names one that Replay confirms.
	reachable, before, err := predecessor(ctx, runner, "reachable", sys, automaton.State{false, true, false, true})
	if err != nil {
		return err
	}
	return json.MarshalEncode(jsontext.NewEncoder(os.Stdout), struct {
		Eden      sat.Status `json:"predecessor_of_eden"`
		Reachable sat.Status `json:"predecessor_of_reachable"`
		Before    string     `json:"one_predecessor"`
	}{Eden: eden, Reachable: reachable, Before: picture(before)})
}

// predecessor compiles "some state steps to goal", runs the checked solver, and
// decodes and replays a SAT trajectory.
func predecessor(ctx context.Context, r *cadical.Runner, id string, sys automaton.System, goal automaton.State) (sat.Status, automaton.State, error) {
	pattern := automaton.Pattern{}
	for cell, alive := range goal {
		pattern[automaton.Bit{Cell: cell}] = alive
	}
	encoding, err := automaton.Compile(ctx, sys, automaton.Query{Steps: 1, Goal: pattern})
	if err != nil {
		return "", nil, err
	}
	formula, err := encoding.CNF()
	if err != nil {
		return "", nil, err
	}
	result, err := r.Solve(ctx, id, formula)
	if err != nil {
		return "", nil, err
	}
	if err := result.Require(sat.Requirements{SATModelChecked: true, UNSATProofChecked: true}); err != nil {
		return "", nil, err
	}
	if result.Status != sat.SAT {
		return result.Status, nil, nil
	}
	run, err := encoding.Decode(result.Assignment)
	if err != nil {
		return "", nil, err
	}
	if err := automaton.Replay(sys, run); err != nil {
		return "", nil, err
	}
	return result.Status, run.States[0], nil
}
```

Without solver tools, it runs the rule forward from `●○○○`:

```sh
go run ./examples/automaton
```

```json
{"states":["●○○○","○●○●","○○○○"]}
```

Forward simulation only goes one way. With `-check` and the four `CALX_*` tool
variables from section 4, the program asks which states have a predecessor. `●○○○`
has none, which is UNSAT with a checked DRAT proof. `○●○●` has one, and the decoded
trajectory passes `Replay`:

```sh
mkdir -p artifacts
go run ./examples/automaton -check -artifacts artifacts/go-automaton
```

```json
{"predecessor_of_eden":"UNSAT","predecessor_of_reachable":"SAT","one_predecessor":"○○●○"}
```

The statuses do not depend on the solver. `one_predecessor` does, because `○●○●` has
four predecessors and the solver returns any one of them.

`lga.HPP` is also an `automaton.System`, so one rule drives both forward simulation
and SAT analysis. The next program embeds it in a deliberately broken rule; the
[lattice gas primer](docs/lga-primer.md) explains the model.

[Complete runnable source](examples/lga/main.go):

```go
package main

import (
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/Yui-Qi-Tang/calx/automaton"
	"github.com/Yui-Qi-Tang/calx/digest"
	"github.com/Yui-Qi-Tang/calx/lga"
	"github.com/Yui-Qi-Tang/calx/sat"
	"github.com/Yui-Qi-Tang/calx/sat/cadical"
)

func main() {
	check := flag.Bool("check", false, "run checked SAT queries; needs the CALX_* tool variables")
	artifacts := flag.String("artifacts", "artifacts/go-lga", "new run directory for -check; parent must exist and the run directory must not exist")
	flag.Parse()
	var err error
	if *check {
		err = runCheck(context.Background(), *artifacts)
	} else {
		err = simulate()
	}
	if err != nil {
		log.Fatal(err)
	}
}

func value(q automaton.Quantity, s automaton.State) int {
	v := 0
	for _, t := range q {
		if s[t.Bit.Cell*4+t.Bit.Index] {
			v += t.Weight
		}
	}
	return v
}

// simulate runs 8 steps of a 16×8 lattice: an east-moving band in columns 0–3
// and a west-moving band in column 8, then reports conserved quantities and
// 4×4 block averages of density and x velocity.
func simulate() error {
	h := lga.HPP{Width: 16, Height: 8}
	start := make(automaton.State, h.Cells()*4)
	for y := range h.Height {
		for x := range 4 {
			start[(y*h.Width+x)*4+lga.East] = true
		}
		start[(y*h.Width+8)*4+lga.West] = true
	}
	states, err := automaton.Simulate(h, start, 8, nil)
	if err != nil {
		return err
	}
	last := states[len(states)-1]
	density, err := h.Density(states[len(states)-1:], 4)
	if err != nil {
		return err
	}
	ux, _, err := h.Velocity(states[len(states)-1:], 4)
	if err != nil {
		return err
	}
	return json.MarshalEncode(jsontext.NewEncoder(os.Stdout), struct {
		Mass      [2]int      `json:"mass"`
		MomentumX [2]int      `json:"momentum_x"`
		Density   [][]float64 `json:"density"`
		VelocityX [][]float64 `json:"velocity_x"`
	}{
		Mass:      [2]int{value(h.Mass(), start), value(h.Mass(), last)},
		MomentumX: [2]int{value(h.MomentumX(), start), value(h.MomentumX(), last)},
		Density:   density,
		VelocityX: ux,
	})
}

// leaky breaks HPP on purpose: an east-west head-on pair becomes a single
// north-moving particle, so mass is no longer conserved.
type leaky struct{ lga.HPP }

func (l leaky) Next(cell int, in, out []bool) {
	if in[lga.East] && in[lga.West] && !in[lga.North] && !in[lga.South] {
		out[lga.North] = true
		return
	}
	l.HPP.Next(cell, in, out)
}

func runCheck(ctx context.Context, artifacts string) error {
	runner, err := cadical.New(cadical.Config{
		SolverPath:    os.Getenv("CALX_CADICAL"),
		SolverDigest:  digest.Digest(os.Getenv("CALX_CADICAL_DIGEST")),
		CheckerPath:   os.Getenv("CALX_DRAT"),
		CheckerDigest: digest.Digest(os.Getenv("CALX_DRAT_DIGEST")),
		ArtifactDir:   artifacts,
	})
	if err != nil {
		return err
	}

	// 1. HPP conserves mass: no 3-step run of a 2×2 lattice changes it.
	small := lga.HPP{Width: 2, Height: 2}
	conserved, _, err := solve(ctx, runner, "conserved-mass", small, automaton.Query{Steps: 3, Violates: small.Mass()})
	if err != nil {
		return err
	}

	// 2. The broken rule loses mass, and the solver shows how.
	h := lga.HPP{Width: 4, Height: 4}
	broken, run, err := solve(ctx, runner, "leaky-mass", leaky{h}, automaton.Query{Steps: 3, Violates: h.Mass()})
	if err != nil {
		return err
	}
	out := struct {
		ConservedMass  sat.Status `json:"conserved_mass_2x2"`
		LeakyMass      sat.Status `json:"leaky_mass_4x4"`
		Counterexample []int      `json:"counterexample_mass,omitempty"`
	}{ConservedMass: conserved, LeakyMass: broken}
	if broken == sat.SAT {
		// The mass at every time; some step changes it.
		for _, s := range run.States {
			out.Counterexample = append(out.Counterexample, value(h.Mass(), s))
		}
	}
	return json.MarshalEncode(jsontext.NewEncoder(os.Stdout), out)
}

// solve compiles a query, runs the checked solver, requires the evidence for
// the answer, and decodes and replays a SAT trajectory.
func solve(ctx context.Context, r *cadical.Runner, id string, sys automaton.System, q automaton.Query) (sat.Status, automaton.Trajectory, error) {
	encoding, err := automaton.Compile(ctx, sys, q)
	if err != nil {
		return "", automaton.Trajectory{}, err
	}
	formula, err := encoding.CNF()
	if err != nil {
		return "", automaton.Trajectory{}, err
	}
	result, err := r.Solve(ctx, id, formula)
	if err != nil {
		return "", automaton.Trajectory{}, err
	}
	if err := result.Require(sat.Requirements{SATModelChecked: true, UNSATProofChecked: true}); err != nil {
		return "", automaton.Trajectory{}, err
	}
	if result.Status != sat.SAT {
		return result.Status, automaton.Trajectory{}, nil
	}
	run, err := encoding.Decode(result.Assignment)
	if err != nil {
		return "", automaton.Trajectory{}, err
	}
	if err := automaton.Replay(sys, run); err != nil {
		return "", automaton.Trajectory{}, fmt.Errorf("replaying %s: %w", id, err)
	}
	return result.Status, run, nil
}
```

Without solver tools, the example runs 8 steps of a 16×8 lattice. An
east-moving band starts in columns 0–3 and a west-moving band in column 8. The
example prints the conserved quantities and 4×4 block averages:

```sh
go run ./examples/lga
```

```json
{"mass":[40,40],"momentum_x":[24,24],"density":[[0.25,0.25,0.75,0],[0.25,0.25,0.75,0]],"velocity_x":[[-1,1,1,0],[-1,1,1,0]]}
```

With `-check` and the four `CALX_*` tool variables, the example
runs two checked queries:

- It proves that no 3-step run of a 2×2 HPP lattice changes mass (UNSAT with a
  checked DRAT proof).
- It asks the same question of a deliberately broken rule, in which a head-on
  pair becomes one particle. The solver returns a counterexample, which is
  decoded and replayed:

```sh
go run ./examples/lga -check -artifacts artifacts/go-lga
```

```json
{"conserved_mass_2x2":"UNSAT","leaky_mass_4x4":"SAT","counterexample_mass":[37,35,34,33]}
```

`counterexample_mass` is the particle count at each time of the counterexample;
it depends on the solver's model, while the status values do not. Whole-lattice conservation proofs are kept to 2×2 because CDCL
proofs of counting arguments over several steps stop finishing at about 24
to 32 terms; see
[contracts](docs/contracts.md#synchronous-boolean-automata-v01).

### 6. mus: shrink a conflict

An UNSAT answer says that a whole formula conflicts. `mus.Shrink` deletes one
group of constraints at a time and returns a subset that still conflicts, with
every remaining group needed. The caller implements `mus.Oracle`, so a group
can be anything, and any backend can decide it. `mus.ByOrigin` is the Oracle
for a CNF whose clauses carry `Origins`.

The program below has five clauses, of which three conflict. It uses the same
four `CALX_*` tool variables as section 4.

[Complete runnable source](examples/mus/main.go):

```go
package main

import (
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/Yui-Qi-Tang/calx/digest"
	"github.com/Yui-Qi-Tang/calx/mus"
	"github.com/Yui-Qi-Tang/calx/sat"
	"github.com/Yui-Qi-Tang/calx/sat/cadical"
)

func main() {
	artifacts := flag.String("artifacts", "artifacts/go-mus", "new run directory; parent must exist and the run directory must not exist")
	flag.Parse()
	if err := run(context.Background(), *artifacts); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, artifacts string) error {
	// 1. Declare the formula. Origins name the group of each clause.
	formula := sat.CNF{
		Variables: []sat.Variable{{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}},
		Clauses:   [][]int{{3}, {1}, {-1, 2}, {-2}, {4}},
		Origins: [][]string{
			{"clock-set"}, {"power-on"}, {"power-implies-light"}, {"light-off"}, {"tv-on"},
		},
	}

	// 2. Pin the solver and the proof checker.
	runner, err := cadical.New(cadical.Config{
		SolverPath:    os.Getenv("CALX_CADICAL"),
		SolverDigest:  digest.Digest(os.Getenv("CALX_CADICAL_DIGEST")),
		CheckerPath:   os.Getenv("CALX_DRAT"),
		CheckerDigest: digest.Digest(os.Getenv("CALX_DRAT_DIGEST")),
		ArtifactDir:   artifacts,
	})
	if err != nil {
		return err
	}

	// 3. Build the Oracle. Each check gets its own query ID.
	oracle, err := mus.ByOrigin(formula, func(ctx context.Context, step int, trial sat.CNF) (sat.Result, error) {
		solved, err := runner.Solve(ctx, fmt.Sprintf("core-%d", step), trial)
		return solved.Result, err
	})
	if err != nil {
		return err
	}

	// 4. Shrink, requiring a checked model or proof for every answer.
	core, err := mus.Shrink(ctx, oracle, sat.Requirements{SATModelChecked: true, UNSATProofChecked: true})
	if err != nil {
		return err
	}

	// 5. Translate group indices back to the caller's names.
	names := oracle.Names()
	out := struct {
		Conflict []string   `json:"conflict"`
		Minimal  bool       `json:"minimal"`
		Witness  sat.Status `json:"witness"`
		Checks   []string   `json:"checks"`
	}{Minimal: core.Minimal, Witness: core.Witness.Status}
	for _, g := range core.Groups {
		out.Conflict = append(out.Conflict, names[g])
	}
	for _, step := range core.Steps {
		checked := "all groups"
		if step.Removed >= 0 {
			checked = "without " + names[step.Removed]
		}
		out.Checks = append(out.Checks, fmt.Sprintf("%s: %s", checked, step.Result.Status))
	}
	return json.MarshalEncode(jsontext.NewEncoder(os.Stdout), out)
}
```

```sh
mkdir -p artifacts
go run ./examples/mus -artifacts artifacts/go-mus
```

```json
{"conflict":["light-off","power-implies-light","power-on"],"minimal":true,"witness":"UNSAT","checks":["all groups: UNSAT","without clock-set: UNSAT","without light-off: SAT","without power-implies-light: SAT","without power-on: SAT","without tv-on: UNSAT"]}
```

Each check is one real solver run. Deleting `clock-set` or `tv-on` leaves UNSAT,
so they go. Deleting any of the other three gives SAT, so they stay. On an error,
`core` keeps any conflict already established; see the
[contracts](docs/contracts.md#conflict-localization).

The result is one conflict, not every conflict or a smallest one, and it does
not say which group is wrong. The [primer](docs/mus-primer.md) walks through this
run step by step, and [a runnable example](mus/example_test.go) needs no solver:

```sh
go test -run ExampleShrink -v ./mus
```

`ByOrigin` is one Oracle. Section 7 implements its own over linear constraints.

### 7. linear and linear/z3: solve linear integers

`linear.Problem` holds Boolean CNF and a conjunction of comparisons of integer
sums with constants. Integers are canonical decimal strings, so they do not
overflow. The program below solves one problem whose answer does not fit in an
`int64` and one that has no integer solution. It then shrinks a conflict with its
own `mus.Oracle`, which treats each constraint as a group.

| Environment variable | Meaning |
| --- | --- |
| `CALX_Z3` | File path to the Z3 executable |
| `CALX_Z3_DIGEST` | Expected digest of that executable's complete contents |

```sh
export CALX_Z3=/absolute/path/to/z3
export CALX_Z3_DIGEST="$(shasum -a 256 "$CALX_Z3" | awk '{print $1}')"
```

[Complete runnable source](examples/linear/main.go):

```go
package main

import (
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/Yui-Qi-Tang/calx/digest"
	"github.com/Yui-Qi-Tang/calx/linear"
	"github.com/Yui-Qi-Tang/calx/linear/z3"
	"github.com/Yui-Qi-Tang/calx/mus"
	"github.com/Yui-Qi-Tang/calx/sat"
)

func main() {
	artifacts := flag.String("artifacts", "artifacts/go-linear", "new run directory; parent must exist and the run directory must not exist")
	flag.Parse()
	if err := run(context.Background(), *artifacts); err != nil {
		log.Fatal(err)
	}
}

// constraintGroups is a mus.Oracle: each constraint of problem is one group, and
// only this type knows that.
type constraintGroups struct {
	runner  *z3.Runner
	problem linear.Problem
}

func (o constraintGroups) Groups() int { return len(o.problem.Constraints) }

func (o constraintGroups) Check(ctx context.Context, step int, keep []bool) (sat.Result, error) {
	trial := linear.Problem{Booleans: o.problem.Booleans, Integers: o.problem.Integers}
	for i, c := range o.problem.Constraints {
		if keep[i] {
			trial.Constraints = append(trial.Constraints, c)
		}
	}
	solved, err := o.runner.Solve(ctx, fmt.Sprintf("core-%d", step), trial)
	return solved.Result, err
}

func run(ctx context.Context, artifacts string) error {
	// 1. Pin the solver.
	runner, err := z3.New(z3.Config{
		SolverPath:   os.Getenv("CALX_Z3"),
		SolverDigest: digest.Digest(os.Getenv("CALX_Z3_DIGEST")),
		ArtifactDir:  artifacts,
	})
	if err != nil {
		return err
	}

	// 2. x + y = 2^64 and x - y = 2. x does not fit in an int64.
	pair := linear.Problem{
		Integers: []sat.Variable{{ID: 1}, {ID: 2}},
		Constraints: []linear.Constraint{{
			Terms:    []linear.Term{{Variable: 1, Coefficient: "1"}, {Variable: 2, Coefficient: "1"}},
			Relation: linear.Equal,
			RHS:      "18446744073709551616",
		}, {
			Terms:    []linear.Term{{Variable: 1, Coefficient: "1"}, {Variable: 2, Coefficient: "-1"}},
			Relation: linear.Equal,
			RHS:      "2",
		}},
	}
	solved, err := runner.Solve(ctx, "pair", pair)
	if err != nil {
		return err
	}
	// The model was evaluated against pair with exact arithmetic.
	if err := solved.Require(sat.Requirements{SATModelChecked: true}); err != nil {
		return err
	}

	// 3. 2x = 1 has a real solution but no integer one.
	half := linear.Problem{
		Integers: []sat.Variable{{ID: 1}},
		Constraints: []linear.Constraint{{
			Terms:    []linear.Term{{Variable: 1, Coefficient: "2"}},
			Relation: linear.Equal,
			RHS:      "1",
		}},
	}
	refuted, err := runner.Solve(ctx, "half", half)
	if err != nil {
		return err
	}

	// 4. The answer is Z3's report alone, so requiring a proof must fail.
	proofErr := refuted.Require(sat.Requirements{UNSATProofChecked: true})
	if !errors.Is(proofErr, sat.ErrGuarantee) {
		return errors.New("an unproved UNSAT met the proof requirement")
	}

	// 5. Shrink x >= 6, y <= 0, x <= 5. Accepting Z3's UNSAT has to be explicit,
	// so this asks only for checked SAT models.
	names := []string{"x >= 6", "y <= 0", "x <= 5"}
	oracle := constraintGroups{runner: runner, problem: linear.Problem{
		Integers: []sat.Variable{{ID: 1}, {ID: 2}},
		Constraints: []linear.Constraint{
			{Terms: []linear.Term{{Variable: 1, Coefficient: "1"}}, Relation: linear.AtLeast, RHS: "6"},
			{Terms: []linear.Term{{Variable: 2, Coefficient: "1"}}, Relation: linear.AtMost, RHS: "0"},
			{Terms: []linear.Term{{Variable: 1, Coefficient: "1"}}, Relation: linear.AtMost, RHS: "5"},
		},
	}}
	core, err := mus.Shrink(ctx, oracle, sat.Requirements{SATModelChecked: true})
	if err != nil {
		return err
	}
	var conflict []string
	for _, g := range core.Groups {
		conflict = append(conflict, names[g])
	}

	return json.MarshalEncode(jsontext.NewEncoder(os.Stdout), struct {
		Pair        sat.Status `json:"pair"`
		X           string     `json:"x"`
		Y           string     `json:"y"`
		ModelMethod string     `json:"model_method"`
		Half        sat.Status `json:"half"`
		HalfProof   bool       `json:"half_proof_present"`
		ProofError  string     `json:"proof_requirement"`
		Conflict    []string   `json:"conflict"`
		Minimal     bool       `json:"minimal"`
	}{
		Pair: solved.Status, X: solved.Assignment.Integers[1], Y: solved.Assignment.Integers[2],
		ModelMethod: solved.Model.Method,
		Half:        refuted.Status, HalfProof: refuted.Proof.Present, ProofError: proofErr.Error(),
		Conflict: conflict, Minimal: core.Minimal,
	})
}
```

```sh
mkdir -p artifacts
go run ./examples/linear -artifacts artifacts/go-linear
```

```json
{"pair":"SAT","x":"9223372036854775809","y":"9223372036854775807","model_method":"total-bool-and-big-integer-evaluation/v0","half":"UNSAT","half_proof_present":false,"proof_requirement":"required guarantee unavailable: checked UNSAT proof","conflict":["x >= 6","x <= 5"],"minimal":true}
```

`x` and `y` are the only solution, so they do not depend on the solver. A SAT
model is evaluated against the original problem with exact arithmetic. UNSAT is
Z3's report alone: nothing checks it independently, so requiring
`UNSATProofChecked` always fails for it, as `proof_requirement` shows. For the same
reason, the `mus.Shrink` call asks only for checked SAT models, and each UNSAT in
that conflict is Z3's report. Use `sat/cadical` when a conclusion must carry a
checked proof. The [primer](docs/linear-primer.md) shows what the profile can
express, with worked problems.

### 8. digest: identify exact bytes

`digest.Digest` identifies exact bytes, and every package uses it for inputs, tools
and evidence. It says which bytes are meant, not that they are trustworthy. This
program creates values, validates external text and shows what a direct cast does
not check. It starts no solver.

[Complete runnable source](examples/digest/main.go):

```go
package main

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"log"
	"os"
	"strings"

	"github.com/Yui-Qi-Tang/calx/digest"
)

func main() {
	data := []byte("exact bytes\n")

	// 1. Sum hashes bytes in memory; Read hashes a stream to the same value.
	sum := digest.Sum(data)
	read, size, err := digest.Read(strings.NewReader(string(data)))
	if err != nil {
		log.Fatal(err)
	}

	// 2. Parse validates external text before it becomes a Digest.
	parsed, err := digest.Parse(sum.String())
	if err != nil {
		log.Fatal(err)
	}
	_, uppercase := digest.Parse(strings.ToUpper(sum.String()))

	// 3. A cast checks nothing, so a boundary must call Validate.
	cast := digest.Digest("not-a-digest")

	// 4. The zero value means absent, not the digest of empty content.
	var absent digest.Digest

	if err := json.MarshalEncode(jsontext.NewEncoder(os.Stdout, jsontext.WithIndent("  ")), struct {
		Sum             digest.Digest `json:"sum"`
		Bytes           int64         `json:"bytes"`
		ReadMatches     bool          `json:"read_matches"`
		ParseMatches    bool          `json:"parse_matches"`
		UppercaseFails  bool          `json:"uppercase_fails"`
		CastFailsCheck  bool          `json:"cast_fails_validate"`
		AbsentFails     bool          `json:"absent_fails_validate"`
		EmptyContentSum digest.Digest `json:"empty_content"`
	}{
		Sum:             sum,
		Bytes:           size,
		ReadMatches:     read == sum,
		ParseMatches:    parsed == sum,
		UppercaseFails:  errors.Is(uppercase, digest.ErrInvalid),
		CastFailsCheck:  errors.Is(cast.Validate(), digest.ErrInvalid),
		AbsentFails:     errors.Is(absent.Validate(), digest.ErrInvalid),
		EmptyContentSum: digest.Sum(nil),
	}); err != nil {
		log.Fatal(err)
	}
}
```

```sh
go run ./examples/digest
```

```json
{
  "sum": "6a77ce4ad94636f6120bb985066c1d75ce65b73f264a35f9d5ac910e252f0355",
  "bytes": 12,
  "read_matches": true,
  "parse_matches": true,
  "uppercase_fails": true,
  "cast_fails_validate": true,
  "absent_fails_validate": true,
  "empty_content": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
}
```

Use `Sum` or `Read` for content and `Parse` for text from outside. A cast such as
`digest.Digest("not-a-digest")` compiles and runs, so a boundary that accepts one
calls `Validate`, as the tool runners do. The zero value means absent, while empty
content has the ordinary digest shown in `empty_content`.

## Standalone command

```sh
go build -o bin/calx ./cmd/calx
bin/calx project --in examples/graph.json
bin/calx encode --in examples/finite.json
```

Calx does not download or bundle the solver executables. Pin the ones you
have selected once; `pin-tools` hashes both files and writes a manifest with
their absolute paths and digests:

```sh
bin/calx pin-tools --solver /absolute/path/to/cadical --checker /absolute/path/to/drat-trim --out tools.json
```

```json
{
  "schema_version": "calx-tools/v1",
  "solver": {"path": "/absolute/path/to/cadical", "digest": "<64 lowercase hex>"},
  "checker": {"path": "/absolute/path/to/drat-trim", "digest": "<64 lowercase hex>"}
}
```

Review the manifest like any other pinned dependency; it can be committed next
to the inputs it serves. Then solve with a new artifact directory whose parent
already exists:

```sh
mkdir -p artifacts
bin/calx solve-finite --in examples/finite.json --tools tools.json --artifacts artifacts/example
```

The manifest records a trust decision; it is not a trust root. Every run still
copies each executable and checks its bytes against the pinned digest, so a
tool replaced after pinning fails with a digest mismatch. The four explicit
flags `--solver`, `--solver-digest`, `--checker` and `--checker-digest` remain
available instead of `--tools`, not together with it. The CLI never reads the
`CALX_*` environment variables.

Use `bin/calx --help` to list commands, or `bin/calx solve --help` for the
flags accepted by one command. Help exits successfully without reading input.

| Flag | Applies to | Meaning and default |
| --- | --- | --- |
| `--in FILE` | All commands except `pin-tools` | Input file; `-` reads stdin (default). Maximum 16 MiB. |
| `--format FORMAT` | `solve` | CNF format: `json` (default) or `dimacs`. Other commands take JSON without this flag. |
| `--tools FILE` | `solve`, `solve-finite` | Tool manifest from `pin-tools`. Replaces the next four flags and cannot be combined with them. |
| `--solver PATH` | `solve`, `solve-finite`, `pin-tools` | CaDiCaL executable file path, not a shell command. Required without `--tools`, and for `pin-tools`. |
| `--solver-digest DIGEST` | `solve`, `solve-finite` | Expected digest of solver bytes, in the format described above. Required without `--tools`. |
| `--checker PATH` | `solve`, `solve-finite`, `pin-tools` | DRAT-trim executable file path, used even for runs that return SAT. Required without `--tools`, and for `pin-tools`. |
| `--checker-digest DIGEST` | `solve`, `solve-finite` | Expected digest of checker bytes. Required without `--tools`. |
| `--out FILE` | `pin-tools` | Required new manifest file; an existing file is never overwritten. |
| `--artifacts DIR` | `solve`, `solve-finite` | Required new run directory. Parent must exist; DIR must not exist. Stores private tools, inputs, output and validation records. |
| `--id NAME` | `solve`, `solve-finite` | Query subdirectory name, default `query`. 1–80 ASCII letters, digits, `_` or `-`; starts with a letter or digit. |
| `--timeout DURATION` | `solve`, `solve-finite` | Default `30s`; solver and checker share the deadline. Excludes model compilation and tool preparation. `0` selects the default; negatives fail. |

Other commands include `reconstruct` and `decode`. Command results use a versioned
JSON envelope; help and usage diagnostics go to stderr. Exit 0 means successful
help or an operation that met its checks, exit 1 is failure, and exit 2 is a
flag/usage error. Missing required flags and flags for other commands are
rejected before reading input. An error response can retain a backend
SAT/UNSAT/UNKNOWN label for diagnosis; inspect `outcome`, `error` and validation
records together.

An explicit UNKNOWN returns `sat.ErrInconclusive` from the Go solver API. The
CLI exits 1 with outcome ERROR and code INCONCLUSIVE, preserving the UNKNOWN
status and diagnostic artifacts. Cancellation and deadline errors are retained;
when joined with ErrInconclusive, CLI codes CANCELED/TIMEOUT take precedence.

Content identifiers use the shared `digest.Digest` type (`type Digest string`).
`digest.Sum(data)` creates a value; `digest.Read(reader)` hashes a stream;
`digest.Parse(text)` validates external text. Values provide `String`, `Validate`
and `Set` methods. A direct cast or JSON decoding does not validate the contents;
the tool runner and result-acceptance paths perform those boundary checks.

This experimental v0 now emits **`calx-cli/v2`**. Go fields use `InputDigest`,
`FormulaDigest`, `SolverDigest`, `CheckerDigest`, `TargetDigest` and
`Artifact.Digest`; JSON uses `*_digest` and artifact `digest`. Query identifiers
are `query_digest`. The previous field names and v1 output schema have no aliases.
Update consumers explicitly; existing evidence files and frozen consumer fixtures
are not rewritten. The identified bytes and digest algorithm are unchanged.

The CLI takes tools from a `--tools` manifest or from the four tool flags, and has
no Z3 command. `cadical.Config` and `z3.Config` accept typed digests. The Go
examples read `CALX_*` environment variables only as example plumbing, convert
them to that type and let `cadical.New` or `z3.New` validate them. Applications can use `digest.Parse`
for earlier validation.

## Verification

```sh
make verify  # formatting, strict errcheck, tests, vet and build
make race
```

Opt-in actual solver acceptance, with the four tool variables above and
`CALX_ARTIFACTS` naming an existing fresh acceptance parent:

```sh
go test -tags resolverintegration -count=1 -run TestRealTools ./sat/cadical ./lga ./mus
```

The `lga` tests prove HPP and FHP conservation laws, find counterexamples for
deliberately broken rules, and check walls and chirality.

The Z3 acceptance reads `CALX_Z3`, `CALX_Z3_DIGEST` and `CALX_ARTIFACTS`:

```sh
go test -tags resolverintegration -count=1 -run TestRealTools ./linear/z3
```

[Validation](docs/validation.md) records the observed scope and limitations.

## Current limitations

- Conservation proofs are counting arguments, which CDCL solvers prove slowly.
  With the per-step `Violates` encoding and the proof check, a 16-term
  whole-lattice sum (HPP 2×2 mass) is proved in about 2 s over three steps, a
  24-term sum (FHP 2×2 mass, two steps) times out at 90 s, and 32 or more terms
  often time out. One-step proofs cost the same as before, and without Start,
  Forbidden or Goal patterns one step already covers every step count.
  Counterexample search and row or column sums stay fast. See the
  [measured results](docs/validation.md#per-step-comparison-2026-10-30).
- Solving relies on the external pinned CaDiCaL, DRAT-trim and Z3. Calx contains
  no solver or proof checker of its own.
- `linear/z3` UNSAT answers have no independently checked proof.
- Target platforms are macOS arm64 and Linux amd64. On Linux amd64, `make
  verify`, `make race` and the real-tool tests pass in CI, with CaDiCaL 3.0.1
  and DRAT-trim built from upstream sources and the Z3 5.3.3 release binary.
  Windows is not a target.

## Requirements and version

Experimental, unreleased: v0 is complete, v0.1 adds `automaton` and `lga`, and
`mus`, `linear` and `linear/z3` followed.
Development and verification use **Go 1.27.3**, which `go.mod` also declares
as the minimum; consumers need at least that version.
The library module currently uses only the Go standard library. Verification
also runs pinned errcheck v1.30.0 as an external Go development tool.
Projection and encoding
run without external executables; SAT solving requires the pinned CaDiCaL and
DRAT-trim tools described above, and `linear/z3` requires a pinned Z3. The CLI
exposes the incidence, finite and checked-solve operations to Python and other
consumers; `automaton`, `lga`, `mus` and `linear` are Go-only.

License: [MIT](LICENSE).
