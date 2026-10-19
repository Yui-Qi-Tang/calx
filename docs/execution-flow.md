# Internal execution: graph → matrix → logic resolver → result

These diagrams follow Calx's current functions and data. Start with the overview,
then follow the matrix compiler, solver/checker and result-decoding paths.
The worked computation is the README example: one action changes `state=off`
to `state=on`, with an exact horizon of one.

| Diagram | What it explains |
| --- | --- |
| [1. Complete flow](#1-complete-flow) | Explicit topology, matrix representations, caller rules and Calx entry points |
| [2. Matrix to CNF](#2-operation-matrix-to-cnf) | Validation, time-indexed variables and the actual emitted clauses |
| [3. Resolver internals](#3-logic-resolver-internals) | Pinned tools, execution, SAT/UNSAT/UNKNOWN branches and retained artifacts |
| [4. Result to trace](#4-result-to-trace) | Error handling, answer-specific guarantees and decoding with the original Encoding |

## 1. Complete flow

![Complete graph, matrix, logic resolver and result flow, with the caller's semantic step explicitly separated.](images/execution-overview.svg)

The topology route begins with caller-supplied node IDs and edges in an
`incidence.Graph`, including explicit endpoints and relation labels.
The [JSON example](../examples/graph.json) supplies this input to the CLI's
`project` command. `incidence.Project` validates and sorts the topology, then
creates S, T, the row/column IDs and relation masks. The detailed cell-by-cell
example is in [graph-to-matrix](graph-to-matrix.md).

The orange step is application code, not a Calx API. The application declares
what its fields and actions mean. There is no built-in conversion from an
incidence matrix to guards, effects or logical propositions. Applications can
supply a finite model directly, or produce their own CNF and enter at the solver.

The two matrix contracts have different axes:

| Representation | Rows | Columns | Cell meaning |
| --- | --- | --- | --- |
| `incidence.Source` / `Target` | Nodes | Identified edges | This node starts/ends this edge |
| `finite.Guard` / `Effect` | Actions | Field/value coordinates | This action requires/assigns this value |
| `finite.Frame` | Actions | Fields | This action preserves this field |

**Source:** [incidence.Project](../incidence/incidence.go),
[finite.Compile](../finite/cnf.go), [finite.Project](../finite/matrix.go),
[Runner.Solve](../sat/cadical/runner.go).

## 2. Operation matrix to CNF

![The off/on model projected to Guard, Effect and Frame, then assigned six variables and encoded into thirteen clauses.](images/finite-to-cnf.svg)

### Validation and allocation

Compile first checks the context, options, model and query. This catches invalid
field/value references, incomplete effect/frame partitions, unsupported profiles
and invalid query bounds before solving. Project establishes stable field/value
coordinates and action rows. Each allocation and clause-building step returns
errors directly. The compiler does not hide failures in a sentinel ID or delayed
error field. The encoder estimates variable/clause budgets before allocating
time-indexed arrays.

For the example, sorted coordinates are `[state/off, state/on]` and the single
action is `enable`. Guard is `[1,0]`, Effect is `[0,1]`, and Frame is `[0]`.
A zero Guard/Effect cell adds no requirement/assignment. The only field is
assigned here, so no frame-preservation clauses are emitted in this example.

The current implementation allocates the following one-based variable IDs:

| ID | `finite.Variable` meaning |
| ---: | --- |
| 1 | `state=off` at time 0 |
| 2 | `state=on` at time 0 |
| 3 | `state=off` at time 1 |
| 4 | `state=on` at time 1 |
| 5 | Action `enable` at transition 0 |
| 6 | `different` witness for field `state` between times 0 and 1 |

These IDs identify Boolean variables; they are not yet assignments.
The different helper is not an equivalence test: false alone does not establish
that the states are equal. Here the final unit clause forces the witness true.

### Emitted formula

The complete DIMACS output for the current example is:

```text
p cnf 6 13
1 2 0
-1 -2 0
3 4 0
-3 -4 0
1 0
5 0
-5 1 0
-5 4 0
-2 0
4 0
-1 -3 -6 0
-2 -4 -6 0
6 0
```

| Clause group | What it enforces |
| --- | --- |
| `1 2`; `-1 -2`; `3 4`; `-3 -4` | Exactly one category at each state time |
| `1` | Initial state is off |
| `5` | The only action is selected for the one transition |
| `-5 1` | Selecting enable requires off before the action |
| `-5 4` | Selecting enable assigns on after the action |
| `-2`; `4` | Goal is not reached at time 0 and is reached at time 1 |
| `-1 -3 -6`; `-2 -4 -6`; `6` | The two complete states must differ |

Rows in the table omit the DIMACS `0` terminator. A negative literal means
logical negation; literals within one clause are OR, and all clauses are AND.

For models with framed fields, selecting an action also implies equality of
each framed field's before/after one-hot values. Forbidden patterns, blocked
action sequences, other fields and larger horizons add their own clauses.
They are not silently omitted when present in a request. Changing any of these
inputs can change the meaning of UNSAT.

The Encoding retains the private formula, operation matrix, state/action ID
arrays and variable mapping. `CNF()` returns a copy plus error for solving;
`Variables()` returns the validated finite-variable mapping plus error. The digest returned
by `Digest()` also has an error result and identifies the serialized
model/query separately from the formula's byte digest.

**Source:** [Model and query validation](../finite/model.go),
[matrix construction](../finite/matrix.go), [explicit encoding stages, variable, one and add](../finite/compiler.go).

## 3. Logic resolver internals

![Resolver setup and execution followed by separate SAT, UNSAT and UNKNOWN verification branches.](images/resolver-checks.svg)

`cadical.New` verifies executable pins and creates private copies in a fresh
artifact root. For each Solve call, the runner:

1. Sets UNKNOWN initially, creates one deadline and validates context/CNF limits.
2. Reserves a new query directory and registers deferred artifact finalization.
3. Saves typed `input.json` and encoded `input.cnf`, retaining each exact digest.
4. Runs the pinned CaDiCaL executable with bounded stdout/stderr capture.
5. Requires a consistent exit code and exact status line, then checks the
   answer-specific evidence.

The accepted backend branches are:

| Branch | Required status/exit pair | Additional check |
| --- | --- | --- |
| SAT | Exit 10 and exact `s SATISFIABLE` | Decode a total assignment, save it, and check every original input clause |
| UNSAT | Exit 20 and exact `s UNSATISFIABLE` | Require a regular proof within the accepted-size limit; run pinned DRAT-trim on the encoded CNF and proof |
| UNKNOWN | Exit 0 and exact `s UNKNOWN` | Preserve UNKNOWN; it does not meet a conclusive consumer requirement |

The solver output must not contain a contradictory status line. The UNSAT
checker must exit zero, emit exact `s VERIFIED`, and not emit `s NOT VERIFIED`.
The proof-size limit is checked after solver execution; it is not a disk quota.

A check failure, process failure, cancellation, deadline or resource failure
returns an error. If the backend had already answered SAT/UNSAT, that label can
remain in the partial Result. The caller must inspect error separately. UNKNOWN
can be a successful protocol exchange with nil error while still being rejected
by Require.

Once the query directory exists, deferred Finish attempts to write outcome.json
and an inventory of retained files on both successful and failed paths. A file
write or inventory failure can itself join the returned error. Earlier failures
may have no per-query artifact directory.

| Retained item | Role |
| --- | --- |
| Root `tools.json`, `limits.json`, private executables | Tool pins and effective configuration |
| `input.json`, `input.cnf` | Typed input and the exact formula submitted to the solver |
| `solver.stdout`, `.stderr`, `.json` | Solver output and process/deadline record |
| `assignment.json` when decoded | The assignment checked against the typed CNF |
| `proof.drat`, `checker.*` when produced | UNSAT proof and checker execution evidence |
| `outcome.json` and returned Artifact inventory | Partial/final outcome plus file sizes and digests |

The inventory contains outcome.json, but outcome.json omits its own inventory
so it does not recursively hash itself. CLI `request.input` is a separate
root-level copy of the original transport input.

**Source:** [Runner.New/Solve](../sat/cadical/runner.go),
[tool execution](../internal/toolrun/process.go),
[query setup and Finish](../internal/toolrun/workspace.go),
[assignment checking](../sat/cnf.go).

## 4. Result to trace

![Result and error are checked before answer-specific Require; SAT uses the retained Encoding to decode a trace, while UNSAT has no trace.](images/result-decoding.svg)

This diagram starts with the Result from the CNF submitted in the same call.
The consumer first checks `err`, then invokes Require with its acceptance policy.
Setting both requirement flags to true means “check the relevant answer branch”:

- SAT needs its present, decoded, checked model bound to InputSHA256.
- UNSAT needs its present, checked proof bound to FormulaSHA256.
- UNKNOWN is never accepted by Require.

SAT does not need `Proof.Checked=true`; the SAT example correctly has it false.
Likewise, UNSAT does not provide a satisfying assignment to decode.

For the six-variable example, the checked assignment is:

```json
{"1":true,"2":false,"3":false,"4":true,"5":true,"6":true}
```

Decode first checks this complete assignment against its own retained CNF. It
then uses its time/coordinate arrays to produce:

```json
{
  "states": [{"state":"off"},{"state":"on"}],
  "actions": ["enable"]
}
```

The different helper contributes to formula validation but not to the action
sequence. Independent replay of the consumer's original rules is a later
consumer responsibility; Decode itself does not perform that independent check.

Require does not authenticate arbitrary received/cached results or establish
that a result belongs to another query. Consumers exchanging results must retain
and check the intended input identities and trust boundary. A previously checked
H0 UNSAT result cannot be reused as the result of this H1 query merely because
Require accepts that old result's own fields.

### Keep these identities separate

| Identity | What it identifies |
| --- | --- |
| `Encoding.Digest()` | Compact Go JSON of the finite model and query |
| `Result.InputSHA256` | Exact generated typed CNF input.json |
| `Result.FormulaSHA256` | Exact generated DIMACS input.cnf |
| CLI envelope `input_sha256` | Original transport input bytes before decoding |

These hashes identify different representations; equality is not expected.
The CLI packages command outcome, data and error separately. For solve-finite,
its data can contain the result, model/query digest and a successfully decoded
SAT trace. Complete fields are in [data structures](data-structures.md).

**Source:** [Result.Require](../sat/result.go), [Encoding.Decode](../finite/cnf.go),
[standalone example](../examples/solve/main.go), [CLI envelope](../internal/cli/cli.go).

## Reproduce the non-solver stages

From the repository root:

```sh
go run ./cmd/calx project --in examples/graph.json
go run ./examples/compile
go run ./cmd/calx encode --in examples/finite.json
```

The first prints the graph projection. The second prints the complete CNF above.
The third also includes the finite variable map and model/query digest. See the
[README solve example](../README.md#4-satcadical-solve-check-and-decode) for checked SAT and
UNSAT executions with explicitly pinned local tools.
