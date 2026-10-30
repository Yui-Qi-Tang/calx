# Calx

Calx is a domain-neutral Go module for graph incidence projection, bounded
categorical-model encoding, bounded analysis of caller-supplied Boolean
automata and checked Boolean SAT solving. Packages can be imported
independently.

| Package | Responsibility | Example |
| --- | --- | --- |
| `digest` | Shared content-digest value, generation, parsing and validation | [Digest API](docs/data-structures.md#content-digests) |
| `incidence` | Directed multigraph topology ↔ source/target incidence matrices, IDs and opaque relation masks | [Project and reconstruct](#1-incidence-project-and-reconstruct) |
| `finite` | Validated categorical model → operation matrix → exact-length CNF; assignment → trace | [Matrices and CNF](#2-finite-project-and-compile) |
| `automaton` | Caller-implemented synchronous Boolean automaton → exact-length CNF; assignment → replayable trajectory (v0.1) | [v0.1 design](docs/v0.1-design.md) |
| `lga` | HPP and FHP-I lattice gases with walls as `automaton.System`; bit-packed simulation, conserved quantities, density and velocity fields (v0.1) | [v0.1 design](docs/v0.1-design.md#lga) |
| `sat` | CNF, DIMACS, total-assignment checking, limits and explicit validation records | [Check a formula](#3-sat-check-a-formula) |
| `sat/cadical` | Pinned CaDiCaL and DRAT-trim processes, checked results and retained artifacts | [Checked solve and trace](#4-satcadical-solve-check-and-decode) |

## Documentation

| Document | Read it for |
| --- | --- |
| [Architecture](docs/architecture.md) | Package boundaries, imports, data flow and scope (v0 and v0.1) |
| [Contracts](docs/contracts.md) | Validation rules, limits and what each answer guarantees |
| [Data structures](docs/data-structures.md) | Go types, JSON fields, variable numbering and matrix dimensions |
| [Error handling](docs/error-handling.md) | Error-returning accessors, variable roles and inconclusive answers |
| [Execution flow](docs/execution-flow.md) | Diagrams of the finite route from model through CNF, solver checks and decoding |
| [Graph to matrix](docs/graph-to-matrix.md) | A visual walkthrough of incidence projection |
| [v0.1 design](docs/v0.1-design.md) | The automaton and lattice-gas design, phases and measured limits |
| [Adoption](docs/adoption.md) | What it takes for AHE and Pouch repositories to import Calx |
| [Consumer conformance](docs/compatibility.md) | The bounded compatibility check against consumer sources |
| [Validation log](docs/validation.md) | Dated verification records and retained evidence |

## Library examples

Run all commands below from the repository root. The first three package
sections use only Go; the fourth requires explicitly pinned solver executables.

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
		Projection    incidence.Projection `json:"projection"`
		Reconstructed incidence.Graph      `json:"reconstructed"`
	}{Projection: projection, Reconstructed: reconstructed}, json.Deterministic(true)); err != nil {
		log.Fatal(err)
	}
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
	"encoding/json"
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
	output := json.NewEncoder(os.Stdout)
	output.SetIndent("", "  ")
	if err := output.Encode(struct {
		Matrix        finite.Matrix `json:"matrix"`
		Reconstructed finite.Model  `json:"reconstructed"`
	}{Matrix: matrix, Reconstructed: reconstructed}); err != nil {
		log.Fatal(err)
	}
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
		Profile: finite.Profile,
		Start:   finite.State{"state": "off"},
		Goal:    finite.State{"state": "on"},
		Horizon: 1, // Exactly one action.
	}

	encoding, err := finite.Compile(context.Background(), model, query, finite.Options{})
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
	"encoding/json"
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
	output := json.NewEncoder(os.Stdout)
	output.SetIndent("", "  ")
	if err := output.Encode(struct {
		DIMACS            string       `json:"dimacs"`
		Assignment        map[int]bool `json:"assignment"`
		AssignmentChecked bool         `json:"assignment_checked"`
	}{DIMACS: dimacs.String(), Assignment: assignment, AssignmentChecked: true}); err != nil {
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
	"encoding/json"
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
		Profile: finite.Profile,
		Start:   finite.State{"state": "off"},
		Goal:    finite.State{"state": "on"},
		Horizon: horizon,
	}

	// 2. Compile the model/query to a formula and its decoding map.
	encoding, err := finite.Compile(ctx, model, query, finite.Options{})
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
	var trace *finite.Trace
	if result.Status == sat.SAT {
		decoded, err := encoding.Decode(result.Assignment)
		if err != nil {
			return err
		}
		trace = &decoded
	}
	return json.NewEncoder(os.Stdout).Encode(struct {
		Status  sat.Status    `json:"status"`
		Horizon int           `json:"horizon"`
		Trace   *finite.Trace `json:"trace,omitempty"`
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

`lga.HPP` is an `automaton.System`, so one rule drives both forward simulation
and SAT analysis. [Complete runnable source](examples/lga/main.go).

Without solver tools, the example runs 8 steps of a 16×8 lattice. An
east-moving band starts in columns 0–3 and a west-moving band in column 8. The
example prints the conserved quantities and 4×4 block averages:

```sh
go run ./examples/lga
```

```json
{"mass":[40,40],"momentum_x":[24,24],"density":[[0.25,0.25,0.75,0],[0.25,0.25,0.75,0]],"velocity_x":[[-1,1,1,0],[-1,1,1,0]]}
```

With `-check` and the four `CALX_*` tool variables from section 4, the example
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
{"conserved_mass_2x2":"UNSAT","leaky_mass_4x4":"SAT","counterexample_mass":[9,8]}
```

The counterexample's particle counts depend on the solver's model; the status
values do not. Whole-lattice conservation proofs are kept to 2×2 because CDCL
proofs of counting arguments stop finishing at about 32 terms; see
[contracts](docs/contracts.md#synchronous-boolean-automata-v01).

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

The CLI takes tools from a `--tools` manifest or from the four tool flags. `cadical.Config` accepts typed digests. The Go examples read
`CALX_*` environment variables only as example plumbing, convert them to that
type and let `cadical.New` validate them. Applications can use `digest.Parse`
for earlier validation.

## Verification

```sh
make verify  # formatting, strict errcheck, tests, vet and build
make race
```

Opt-in actual solver acceptance, with the four tool variables above and
`CALX_ARTIFACTS` naming an existing fresh acceptance parent:

```sh
go test -tags resolverintegration -count=1 -run TestRealTools ./sat/cadical ./lga
```

The `lga` tests prove HPP and FHP conservation laws, find counterexamples for
deliberately broken rules, and check walls and chirality.

[Consumer conformance](docs/compatibility.md) exercises selected AHE Core/MCP,
Pouch Product Go source snapshots and frozen Lab CNF fixtures in fresh ignored directories.
The compatibility command is implemented in Go; it does not require Python.
It does not switch the formal consumers. [Validation](docs/validation.md)
records the observed scope and limitations.

## Requirements and version

Experimental, unreleased: v0 is complete and v0.1 adds `automaton` and `lga`.
Development and verification use **Go 1.27.3**, which `go.mod` also declares
as the minimum; consumers need at least that version.
The library module currently uses only the Go standard library. Verification
also runs pinned errcheck v1.30.0 as an external Go development tool.
Projection and encoding
run without external executables; SAT solving requires the pinned CaDiCaL and
DRAT-trim tools described above. The CLI exposes the same library operations to
Python and other consumers.

License and source attribution: [MIT](LICENSE), [NOTICE](NOTICE).
